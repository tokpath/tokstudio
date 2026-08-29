package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

const sessionCookie = "tokenhub_session"

func (a *App) registerAuthRoutes(r *gin.Engine) {
	r.GET("/v1/public/brand", a.publicBrand)
	r.GET("/v1/public/tls-check", a.publicTLSCheck)
	r.GET("/v1/public/docs-context", a.docsContext)
	r.GET("/admin/brands", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.listBrands)
	r.POST("/admin/brands/:id/tls/issue", a.requireRoles("platform_admin", "tech_admin"), a.issueBrandTLS)
	r.POST("/v1/auth/register", a.register)
	r.POST("/v1/auth/login", a.login)
	r.POST("/v1/auth/otp/request", a.requestOTP)
	r.POST("/v1/auth/otp/verify", a.verifyOTP)
	r.GET("/v1/auth/google/start", a.googleStart)
	r.POST("/v1/auth/google/callback", a.googleCallback)
	r.GET("/v1/me", a.requireAnyUser(), a.me)
	r.POST("/v1/me/channel/switch", a.requireAnyUser(), a.switchChannel)
	r.GET("/admin/channels", a.requireRoles("platform_admin", "channel_admin"), a.listChannels)
	r.POST("/admin/channels", a.requireRoles("platform_admin"), a.createChannel)
	r.PATCH("/admin/channels/:id", a.requireRoles("platform_admin"), a.patchChannel)
	r.GET("/admin/me/2fa", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPStatus)
	r.POST("/admin/me/2fa/setup", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPSetup)
	r.POST("/admin/me/2fa/enable", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPEnable)
	r.POST("/admin/me/2fa/disable", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPDisable)
	r.GET("/admin/users", a.requireRoles("platform_admin"), a.listUsersAdmin)
	r.POST("/admin/users/:id/attribution", a.requireRoles("platform_admin"), a.reattribute)
	r.GET("/channel/me", a.requireRoles("channel_admin"), a.channelMe)
	r.GET("/channel/users", a.requireRoles("channel_admin"), a.listUsersChannel)
}

func (a *App) tokenFromRequest(c *gin.Context) string {
	if auth := c.GetHeader("Authorization"); strings.TrimSpace(auth) != "" {
		return auth
	}
	if cookie, err := c.Cookie(sessionCookie); err == nil && cookie != "" {
		return "Bearer " + cookie
	}
	return ""
}

func (a *App) requireAnyUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := a.Identity.Authenticate(c.Request.Context(), a.tokenFromRequest(c))
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "身份校验失败", true)
			return
		}
		if principal == nil {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
			return
		}
		c.Set("principal", principal)
		c.Next()
	}
}

func (a *App) setSessionCookie(c *gin.Context, token string) {
	secure := a.Config.IsProduction()
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, 86400, "/", "", secure, true)
}

func (a *App) writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identity.ErrEmailTaken):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "邮箱已注册", false)
	case errors.Is(err, identity.ErrWeakPassword):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "密码至少 8 位", false)
	case errors.Is(err, identity.ErrPromotionInvalid):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "推广码无效", false)
	case errors.Is(err, identity.ErrOTPInvalid):
		httpx.Abort(c, http.StatusUnauthorized, "authentication_error", "验证码无效", false)
	case errors.Is(err, identity.ErrChannelImmutable):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "渠道归属不可自行切换", false)
	case errors.Is(err, identity.ErrInvalidCredentials):
		httpx.Abort(c, http.StatusForbidden, "authentication_error", "账号或密码错误", false)
	default:
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "处理失败", true)
	}
}

func (a *App) register(c *gin.Context) {
	var body struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		PromotionCode string `json:"promotion_code"`
		OTP           string `json:"otp"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	session, err := a.Identity.Register(c.Request.Context(), identity.RegisterInput{
		Email: body.Email, Password: body.Password, PromotionCode: body.PromotionCode, OTP: body.OTP,
	})
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	a.setSessionCookie(c, session.Token)
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: session.User.ID, Action: "auth.register", ResourceType: "user", ResourceID: session.User.ID,
		After: map[string]string{"channel_org_id": session.User.ChannelOrgID, "source_code": body.PromotionCode},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) login(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	session, err := a.Identity.Login(c.Request.Context(), identity.LoginInput{Email: body.Email, Password: body.Password})
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	a.setSessionCookie(c, session.Token)
	httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) requestOTP(c *gin.Context) {
	var body struct {
		Email   string `json:"email"`
		Purpose string `json:"purpose"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	code, err := a.Identity.RequestOTP(c.Request.Context(), body.Email, body.Purpose)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	resp := gin.H{"status": "sent", "request_id": c.GetString(httpx.ContextRequestID)}
	if !a.Config.IsProduction() && a.Config.AllowDemoProbes {
		resp["dev_code"] = code
	}
	httpx.OK(c, resp)
}

func (a *App) verifyOTP(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	session, err := a.Identity.LoginWithOTP(c.Request.Context(), body.Email, body.Code)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	a.setSessionCookie(c, session.Token)
	httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) googleStart(c *gin.Context) {
	state, err := a.Identity.StartGoogle(c.Request.Context(), c.Query("promotion_code"))
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	authURL := a.Config.PublicBaseURL + "/v1/auth/google/mock?state=" + url.QueryEscape(state)
	if a.Config.GoogleClientID != "" && a.Config.GoogleRedirect != "" {
		values := url.Values{}
		values.Set("client_id", a.Config.GoogleClientID)
		values.Set("redirect_uri", a.Config.GoogleRedirect)
		values.Set("response_type", "code")
		values.Set("scope", "openid email profile")
		values.Set("state", state)
		authURL = "https://accounts.google.com/o/oauth2/v2/auth?" + values.Encode()
	}
	httpx.OK(c, gin.H{"state": state, "auth_url": authURL, "mock": a.Config.GoogleClientID == "", "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) googleCallback(c *gin.Context) {
	var body struct {
		State string `json:"state"`
		Code  string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	session, err := a.Identity.FinishGoogle(c.Request.Context(), body.State, body.Code, identity.MockGoogleExchange)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	a.setSessionCookie(c, session.Token)
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: session.User.ID, Action: "auth.google", ResourceType: "user", ResourceID: session.User.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) me(c *gin.Context) {
	principal := a.currentPrincipal(c)
	me, err := a.Identity.Me(c.Request.Context(), *principal)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	httpx.OK(c, gin.H{"user": me, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) switchChannel(c *gin.Context) {
	principal := a.currentPrincipal(c)
	var body struct {
		ChannelOrgID string `json:"channel_org_id"`
	}
	_ = c.ShouldBindJSON(&body)
	err := a.Identity.SwitchChannel(c.Request.Context(), *principal, body.ChannelOrgID)
	a.writeAuthError(c, err)
}

func (a *App) publicBrand(c *gin.Context) {
	host := c.Query("host")
	if host == "" {
		host = c.GetHeader("X-Forwarded-Host")
	}
	if host == "" {
		host = c.Request.Host
	}
	brand, err := a.Identity.BrandByHost(c.Request.Context(), host)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到品牌", false)
		return
	}
	httpx.OK(c, gin.H{"brand": brand, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) docsContext(c *gin.Context) {
	host := c.Query("host")
	if host == "" {
		host = c.GetHeader("X-Forwarded-Host")
	}
	if host == "" {
		host = c.Request.Host
	}
	brand, err := a.Identity.BrandByHost(c.Request.Context(), host)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到品牌", false)
		return
	}
	channelID := identity.OfficialChannelID
	if brand.ID == identity.OEMBrandID {
		channelID = identity.OEMChannelID
	}
	models, _ := a.Catalog.ListVisibleModels(c.Request.Context(), channelID, nil)
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	httpx.OK(c, gin.H{
		"brand":  brand,
		"models": ids,
		"examples": gin.H{
			"curl":   "curl -H 'Authorization: Bearer $TOKENHUB_API_KEY' https://" + brand.APIDomain + "/v1/chat/completions -d '{\"model\":\"tokenhub/echo-1\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}'",
			"python": "from openai import OpenAI\nclient = OpenAI(base_url='https://" + brand.APIDomain + "/v1', api_key='...')\nprint(client.chat.completions.create(model='tokenhub/echo-1', messages=[{'role':'user','content':'hi'}]))",
			"node":   "const client = new OpenAI({ baseURL: 'https://" + brand.APIDomain + "/v1', apiKey: process.env.TOKENHUB_API_KEY })",
		},
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) publicTLSCheck(c *gin.Context) {
	domain := c.Query("domain")
	if domain == "" {
		domain = c.Query("host")
	}
	if !a.Identity.KnownBrandHost(c.Request.Context(), domain) {
		c.Status(http.StatusNotFound)
		return
	}
	c.Status(http.StatusOK)
}

func (a *App) listBrands(c *gin.Context) {
	items, err := a.Identity.ListBrands(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取品牌失败", true)
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.BrandView) string { return item.ID })
}

func (a *App) issueBrandTLS(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	item, err := a.Identity.IssueBrandTLS(c.Request.Context(), c.Param("id"), a.Config.EdgeCNAME)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "品牌不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "brand.tls.issue", ResourceType: "brand", ResourceID: item.ID,
		After: map[string]string{"tls_status": item.TLSStatus, "cname_target": item.CNAMETarget},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listChannels(c *gin.Context) {
	items, err := a.Identity.ListChannels(c.Request.Context(), *a.currentPrincipal(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道失败", true)
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.ChannelView) string { return item.ID })
}

func (a *App) createChannel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body identity.ChannelInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Identity.CreateChannel(c.Request.Context(), *a.currentPrincipal(c), body)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "channel.create", ResourceType: "channel", ResourceID: item.ID,
		After: map[string]string{"code": item.Code, "type": item.Type},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchChannel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body identity.ChannelInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Identity.PatchChannel(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), body)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "channel.patch", ResourceType: "channel", ResourceID: item.ID,
		After: map[string]string{"status": item.Status},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminTOTPStatus(c *gin.Context) {
	item, err := a.Identity.TOTPStatus(c.Request.Context(), a.currentPrincipal(c).UserID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 2FA 失败", true)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminTOTPSetup(c *gin.Context) {
	item, err := a.Identity.SetupTOTP(c.Request.Context(), *a.currentPrincipal(c), a.Config.EncryptionKey, "TokenHub")
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "admin.2fa.setup", ResourceType: "user",
		ResourceID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminTOTPEnable(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := a.Identity.EnableTOTP(c.Request.Context(), a.currentPrincipal(c).UserID, body.Code, a.Config.EncryptionKey); err != nil {
		httpx.Abort(c, http.StatusUnauthorized, "authentication_error", "TOTP 无效", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "admin.2fa.enable", ResourceType: "user",
		ResourceID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"status": "enabled", "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminTOTPDisable(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	if err := a.Identity.DisableTOTP(c.Request.Context(), a.currentPrincipal(c).UserID); err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "关闭 2FA 失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "admin.2fa.disable", ResourceType: "user",
		ResourceID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"status": "disabled", "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listUsersAdmin(c *gin.Context) {
	items, err := a.Identity.ListUsers(c.Request.Context(), *a.currentPrincipal(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取用户失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listUsersChannel(c *gin.Context) {
	items, err := a.Identity.ListUsers(c.Request.Context(), *a.currentPrincipal(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取用户失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelMe(c *gin.Context) {
	principal := a.currentPrincipal(c)
	httpx.OK(c, gin.H{
		"channel_org_id": principal.ChannelOrgID,
		"email":          principal.Email,
		"roles":          principal.Roles,
		"request_id":     c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) reattribute(c *gin.Context) {
	var body struct {
		PromotionCode string `json:"promotion_code"`
		Reason        string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	principal := a.currentPrincipal(c)
	if err := a.Identity.AdminReattribute(c.Request.Context(), *principal, c.Param("id"), body.PromotionCode, body.Reason); err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "attribution.change", ResourceType: "user", ResourceID: c.Param("id"),
		After: map[string]string{"promotion_code": body.PromotionCode, "reason": body.Reason},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"status": "updated", "request_id": c.GetString(httpx.ContextRequestID)})
}
