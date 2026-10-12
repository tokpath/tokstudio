package app

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
	"strings"
)

func (a *App) registerGoogleConfigRoutes(r *gin.Engine) {
	roles := []string{"platform_admin", "tech_admin"}
	r.GET("/admin/oem-deliveries/:id/google", a.requireRoles(roles...), a.getOEMGoogleConfig)
	r.PUT("/admin/oem-deliveries/:id/google", a.requireRoles(roles...), a.putOEMGoogleConfig)
	r.POST("/admin/oem-deliveries/:id/google/test", a.requireRoles(roles...), a.testOEMGoogleConfig)
	r.GET("/v1/auth/google/test/start", a.googleTestStart)
	r.GET("/v1/me/google/start", a.requireAnyUser(), a.googleStart)
}
func (a *App) abortGoogleConfigErr(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, identity.ErrGoogleEncryptionKey):
		httpx.Abort(c, 503, "configuration_error", "服务器未配置凭据加密密钥，请联系平台技术岗。", false)
	case errors.Is(err, identity.ErrDeliveryConflict):
		httpx.Abort(c, 409, "version_conflict", "Google 配置已变化，请刷新后核对再保存。", false)
	case errors.Is(err, identity.ErrGoogleConfigInput):
		httpx.Abort(c, 400, "invalid_request", "请填写有效的 Google Client ID 和 Secret；更换 Client ID 时需重新填写 Secret。", false)
	case errors.Is(err, identity.ErrGoogleClientTaken):
		httpx.Abort(c, 409, "client_id_taken", "该 Client ID 已用于其他品牌，请为此 OEM 创建独立客户端。", false)
	case errors.Is(err, identity.ErrGoogleNotVerified):
		httpx.Abort(c, 409, "google_not_verified", "请先保存停用的配置，并在主站完成测试登录后再启用。", false)
	case errors.Is(err, identity.ErrGoogleUnavailable):
		a.writeGoogleAuthError(c, err)
	default:
		return a.abortDeliveryErr(c, err)
	}
	return true
}
func (a *App) getOEMGoogleConfig(c *gin.Context) {
	brand, err := a.Identity.OEMGoogleBrand(c.Request.Context(), c.Param("id"))
	if a.abortGoogleConfigErr(c, err) {
		return
	}
	item, err := a.Identity.GoogleConfig(c.Request.Context(), brand.ID)
	if a.abortGoogleConfigErr(c, err) {
		return
	}
	c.Header("Cache-Control", "no-store")
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) putOEMGoogleConfig(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	brand, err := a.Identity.OEMGoogleBrand(c.Request.Context(), c.Param("id"))
	if a.abortGoogleConfigErr(c, err) {
		return
	}
	var in identity.GoogleConfigInput
	if c.ShouldBindJSON(&in) != nil {
		httpx.Abort(c, 400, "invalid_request", "配置字段无效", false)
		return
	}
	if strings.TrimSpace(in.ClientID) == a.Config.GoogleClientID && in.ClientID != "" {
		a.abortGoogleConfigErr(c, identity.ErrGoogleClientTaken)
		return
	}
	var item *identity.GoogleConfigView
	err = a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		item, err = a.Identity.SaveGoogleConfigTx(tx, *a.currentPrincipal(c), brand.ID, a.Config.EncryptionKey, in)
		if err != nil {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "oem.google.configure", ResourceType: "brand", ResourceID: brand.ID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if a.abortGoogleConfigErr(c, err) {
		return
	}
	c.Header("Cache-Control", "no-store")
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) testOEMGoogleConfig(c *gin.Context) {
	brand, err := a.Identity.OEMGoogleBrand(c.Request.Context(), c.Param("id"))
	if a.abortGoogleConfigErr(c, err) {
		return
	}
	var body struct {
		RedirectURI string `json:"redirect_uri"`
	}
	if c.ShouldBindJSON(&body) != nil {
		httpx.Abort(c, 400, "invalid_request", "请选择待测试的回调地址", false)
		return
	}
	href, err := a.Identity.CreateGoogleTestTicket(c.Request.Context(), *a.currentPrincipal(c), brand.ID, body.RedirectURI, a.Config.EncryptionKey)
	if a.abortGoogleConfigErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "oem.google.test", ResourceType: "brand", ResourceID: brand.ID, After: map[string]string{"redirect_uri": body.RedirectURI}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	c.Header("Cache-Control", "no-store")
	httpx.OK(c, gin.H{"url": href})
}
func (a *App) googleTestStart(c *gin.Context) {
	brandID, err := a.authenticationBrand(c)
	if err != nil {
		a.writeGoogleAuthError(c, identity.ErrInvalidCredentials)
		return
	}
	redirectURI := "https://" + a.authenticationHost(c) + "/login/oauth/google"
	intent, err := a.Identity.ConsumeGoogleTestTicket(c.Request.Context(), c.Query("ticket"), redirectURI)
	if err != nil || intent.BrandID != brandID {
		a.writeGoogleAuthError(c, identity.ErrInvalidCredentials)
		return
	}
	cfg, _, version, err := a.googleConfigForRequest(c, true)
	if err != nil {
		a.writeGoogleAuthError(c, err)
		return
	}
	if version != intent.ConfigVersion {
		a.writeGoogleAuthError(c, identity.NewGoogleExchangeError("configuration_changed"))
		return
	}
	a.startGoogleIntent(c, cfg, intent)
}

func (a *App) writeGoogleSession(c *gin.Context, session *identity.Session, replay bool) {
	if !session.TestOnly {
		a.setSessionCookie(c, session.Token)
	}
	if session.TestOnly {
		httpx.OK(c, gin.H{"test_only": true, "verified": true})
		return
	}
	httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID), "idempotent": replay})
}
