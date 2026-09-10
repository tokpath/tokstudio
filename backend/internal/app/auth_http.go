package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

const sessionCookie = "tokenhub_session"

func (a *App) registerAuthRoutes(r *gin.Engine) {
	r.GET("/v1/public/brand", a.publicBrand)
	r.GET("/v1/public/models", a.publicModels)
	r.GET("/v1/public/site", a.publicSite)
	r.GET("/v1/public/tls-check", a.publicTLSCheck)
	r.GET("/.well-known/acme-challenge/:token", a.acmeHTTP01)
	r.GET("/v1/public/docs-context", a.docsContext)
	r.GET("/admin/brands", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.listBrands)
	r.POST("/admin/brands/:id/tls/issue", a.requireRoles("platform_admin", "tech_admin"), a.issueBrandTLS)
	r.POST("/v1/auth/register", a.register)
	r.POST("/v1/auth/login", a.login)
	r.GET("/v1/auth/google/start", a.googleStart)
	r.GET("/v1/auth/google/callback", a.googleCallbackRedirect)
	r.POST("/v1/auth/google/callback", a.googleCallback)
	r.GET("/v1/me", a.requireAnyUser(), a.me)
	r.PATCH("/v1/me", a.requireAnyUser(), a.patchMe)
	r.POST("/v1/me/password", a.requireAnyUser(), a.changePassword)
	r.POST("/v1/me/channel/switch", a.requireAnyUser(), a.switchChannel)
	r.GET("/admin/channels", a.requireRoles("platform_admin", "channel_admin", "finance_admin", "ops_admin", "audit_readonly"), a.listChannels)
	r.POST("/admin/channels", a.requireRoles("platform_admin", "channel_admin"), a.createChannel)
	r.GET("/admin/channels/:id", a.requireRoles("platform_admin", "channel_admin", "finance_admin", "ops_admin", "audit_readonly"), a.getChannel)
	r.GET("/admin/channels/:id/models", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.getChannelModels)
	r.PATCH("/admin/channels/:id/models", a.requireRoles("platform_admin", "ops_admin"), a.patchChannelModels)
	r.PATCH("/admin/channels/:id", a.requireRoles("platform_admin"), a.patchChannel)
	r.GET("/channel/models", a.requireRoles("channel_admin"), a.channelModels)
	r.GET("/admin/me/2fa", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPStatus)
	r.POST("/admin/me/2fa/setup", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPSetup)
	r.POST("/admin/me/2fa/enable", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPEnable)
	r.POST("/admin/me/2fa/disable", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminTOTPDisable)
	r.GET("/admin/users", a.requireRoles("platform_admin"), a.listUsersAdmin)
	r.POST("/admin/users/:id/ban", a.requireRoles("platform_admin"), a.banUser)
	r.POST("/admin/users/:id/unban", a.requireRoles("platform_admin"), a.unbanUser)
	r.POST("/admin/users/:id/attribution", a.requireRoles("platform_admin"), a.reattribute)
	r.GET("/channel/me", a.requireRoles("channel_admin"), a.channelMe)
	r.GET("/channel/users", a.requireRoles("channel_admin"), a.listUsersChannel)
	r.GET("/channel/attribution", a.requireRoles("channel_admin", "platform_admin"), a.channelAttribution)
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
	return a.enforceSession(nil, true)
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
	case errors.Is(err, identity.ErrChannelImmutable):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "渠道归属不可自行切换", false)
	case errors.Is(err, identity.ErrNotFound):
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "记录不存在", false)
	case errors.Is(err, identity.ErrInvalidCredentials):
		httpx.Abort(c, http.StatusForbidden, "authentication_error", "账号或密码错误", false)
	case errors.Is(err, identity.ErrInvalidProfile):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "显示名过长", false)
	case errors.Is(err, identity.ErrInvalidLocale):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "语言只支持 zh、en、ja", false)
	case errors.Is(err, identity.ErrInvalidReason):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "必须填写原因", false)
	case errors.Is(err, identity.ErrSelfAction):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不能对自己执行该操作", false)
	case errors.Is(err, identity.ErrAdminProtected):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不能封禁平台管理员", false)
	default:
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "处理失败", true)
	}
}

func (a *App) register(c *gin.Context) {
	var body struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		PromotionCode string `json:"promotion_code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	session, err := a.Identity.Register(c.Request.Context(), identity.RegisterInput{
		Email: body.Email, Password: body.Password, PromotionCode: body.PromotionCode,
	})
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	a.grantSignupGift(c, session.User.ID)
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

func (a *App) googleStart(c *gin.Context) {
	state, err := a.Identity.StartGoogle(c.Request.Context(), c.Query("promotion_code"))
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	live := a.Config != nil && a.Config.GoogleOAuthReady()
	authURL := a.Config.PublicBaseURL + "/v1/auth/google/mock?state=" + url.QueryEscape(state)
	if live {
		values := url.Values{}
		values.Set("client_id", a.Config.GoogleClientID)
		values.Set("redirect_uri", a.Config.GoogleRedirect)
		values.Set("response_type", "code")
		values.Set("scope", "openid email profile")
		values.Set("state", state)
		authURL = "https://accounts.google.com/o/oauth2/v2/auth?" + values.Encode()
	}
	httpx.OK(c, gin.H{"state": state, "auth_url": authURL, "mock": !live, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) googleExchanger() identity.GoogleExchanger {
	if a.Config == nil || !a.Config.GoogleOAuthReady() {
		return identity.MockGoogleExchange
	}
	return identity.GoogleOAuth{
		ClientID:     a.Config.GoogleClientID,
		ClientSecret: a.Config.GoogleClientSecret,
		RedirectURI:  a.Config.GoogleRedirect,
	}.Exchange
}

func (a *App) finishGoogleSession(c *gin.Context, state, code string) (*identity.Session, error) {
	return a.Identity.FinishGoogle(c.Request.Context(), state, code, a.googleExchanger())
}

func (a *App) afterGoogleSession(c *gin.Context, session *identity.Session) {
	if time.Since(session.User.CreatedAt) < 5*time.Minute {
		a.grantSignupGift(c, session.User.ID)
	}
	a.setSessionCookie(c, session.Token)
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: session.User.ID, Action: "auth.google", ResourceType: "user", ResourceID: session.User.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
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
	session, err := a.finishGoogleSession(c, body.State, body.Code)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	a.afterGoogleSession(c, session)
	httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) googleCallbackRedirect(c *gin.Context) {
	if qErr := strings.TrimSpace(c.Query("error")); qErr != "" {
		c.Redirect(http.StatusFound, a.oauthReturnURL("denied"))
		return
	}
	session, err := a.finishGoogleSession(c, c.Query("state"), c.Query("code"))
	if err != nil {
		c.Redirect(http.StatusFound, a.oauthReturnURL("fail"))
		return
	}
	a.afterGoogleSession(c, session)
	c.Redirect(http.StatusFound, a.webOrigin()+"/enter")
}

func (a *App) webOrigin() string {
	base := strings.TrimSpace(a.Config.WebOrigin)
	if base == "" {
		base = a.Config.PublicBaseURL
	}
	return strings.TrimRight(base, "/")
}

func (a *App) oauthReturnURL(reason string) string {
	return a.webOrigin() + "/login?oauth=" + url.QueryEscape(reason)
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

func (a *App) patchMe(c *gin.Context) {
	principal := a.currentPrincipal(c)
	var body struct {
		DisplayName *string `json:"display_name"`
		Locale      *string `json:"locale"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	me, err := a.Identity.UpdateProfile(c.Request.Context(), *principal, identity.UpdateProfileInput{
		DisplayName: body.DisplayName, Locale: body.Locale,
	})
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "user.profile.update", ResourceType: "user", ResourceID: principal.UserID,
		After: me, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"user": me, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) changePassword(c *gin.Context) {
	principal := a.currentPrincipal(c)
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	if err := a.Identity.ChangePassword(c.Request.Context(), *principal, body.CurrentPassword, body.NewPassword); err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "user.password.change", ResourceType: "user", ResourceID: principal.UserID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"ok": true, "request_id": c.GetString(httpx.ContextRequestID)})
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

func (a *App) requestHost(c *gin.Context) string {
	host := c.Query("host")
	if host == "" {
		host = c.GetHeader("X-Forwarded-Host")
	}
	if host == "" {
		host = c.Request.Host
	}
	return host
}

func (a *App) publicBrand(c *gin.Context) {
	brand, err := a.Identity.BrandByHost(c.Request.Context(), a.requestHost(c))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到品牌", false)
		return
	}
	httpx.OK(c, gin.H{"brand": brand, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) publicModels(c *gin.Context) {
	brand, err := a.Identity.BrandByHost(c.Request.Context(), a.requestHost(c))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到品牌", false)
		return
	}
	channelID, err := a.Identity.ChannelIDByBrand(c.Request.Context(), brand.ID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道失败", true)
		return
	}
	models, err := a.Catalog.ListVisibleModels(c.Request.Context(), channelID, nil)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取模型失败", true)
		return
	}
	page := catalog.ApplyPublicFilters(models, catalog.ModelListQuery{
		Vendor: c.Query("vendor"),
		Kind:   c.Query("kind"),
		Q:      c.Query("q"),
		ID:     c.Query("id"),
		Limit:  catalog.ParseLimit(c.Query("limit")),
	})
	httpx.OK(c, gin.H{
		"items": publicModelCards(page.Items), "total": page.Total, "facets": page.Facets,
		"brand_id": brand.ID, "request_id": c.GetString(httpx.ContextRequestID),
	})
}

func publicModelCards(models []catalog.ModelView) []gin.H {
	items := make([]gin.H, 0, len(models))
	for _, model := range models {
		// 公开价目只给 sell_price（客户侧），不含上游成本。
		items = append(items, gin.H{
			"id": model.ID, "vendor": model.Vendor, "display_name": model.DisplayName,
			"capabilities": model.Capabilities, "sell_price": model.SellPrice,
			"status":      catalog.PublicModelStatus(model.Status),
			"description": model.Description, "kind": model.Kind,
			"context_length": model.ContextLength, "max_completion_tokens": model.MaxCompletionTokens,
		})
	}
	return items
}

func (a *App) publicSite(c *gin.Context) {
	httpx.OK(c, gin.H{"site": catalog.PublicSiteContent(), "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) docsContext(c *gin.Context) {
	brand, err := a.Identity.BrandByHost(c.Request.Context(), a.requestHost(c))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到品牌", false)
		return
	}
	channelID, err := a.Identity.ChannelIDByBrand(c.Request.Context(), brand.ID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道失败", true)
		return
	}
	models, _ := a.Catalog.ListVisibleModels(c.Request.Context(), channelID, nil)
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	httpx.OK(c, gin.H{
		"brand":      brand,
		"models":     ids,
		"examples":   docsExamples(brand.APIDomain, firstModel(ids)),
		"notes":      docsNotes(),
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) acmeHTTP01(c *gin.Context) {
	body := a.Identity.LookupACMEChallenge(c.Param("token"))
	if body == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.String(http.StatusOK, body)
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
		if errors.Is(err, identity.ErrACMEFailed) {
			httpx.Abort(c, http.StatusBadGateway, "provider_unavailable", "ACME 签发失败："+err.Error(), true)
			return
		}
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
	if q := strings.ToLower(c.Query("q")); q != "" {
		filtered := make([]identity.ChannelView, 0, len(items))
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Code+item.ID+item.Type+item.Status), q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "channels.csv", []string{"id", "code", "type", "status", "brand_id"}, items, func(item identity.ChannelView) []string {
			return []string{item.ID, item.Code, item.Type, item.Status, item.BrandID}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.ChannelView) string { return item.ID })
}

func (a *App) getChannel(c *gin.Context) {
	item, err := a.Identity.GetChannel(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getChannelModels(c *gin.Context) {
	principal := a.currentPrincipal(c)
	if _, err := a.Identity.GetChannel(c.Request.Context(), *principal, c.Param("id")); err != nil {
		a.writeAuthError(c, err)
		return
	}
	includeCatalog := principal.HasRole("platform_admin", "ops_admin")
	items, err := a.Catalog.ListChannelModels(c.Request.Context(), c.Param("id"), includeCatalog)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道模型失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelModels(c *gin.Context) {
	principal := a.currentPrincipal(c)
	items, err := a.Catalog.ListChannelModels(c.Request.Context(), principal.ChannelOrgID, false)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道模型失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchChannelModels(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	if _, err := a.Identity.GetChannel(c.Request.Context(), *a.currentPrincipal(c), c.Param("id")); err != nil {
		a.writeAuthError(c, err)
		return
	}
	var body struct {
		Items []catalog.ChannelModelGrant `json:"items"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := a.Catalog.SetChannelModels(c.Request.Context(), c.Param("id"), body.Items); err != nil {
		switch {
		case errors.Is(err, catalog.ErrUnknownModel):
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "只能授权平台目录中已有的模型，租户不能自建提供商或模型", false)
		case errors.Is(err, catalog.ErrInvalidInput):
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "模型授权无效", false)
		default:
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "写入渠道模型失败", true)
		}
		return
	}
	items, err := a.Catalog.ListChannelModels(c.Request.Context(), c.Param("id"), true)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道模型失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "channel.models.patch", ResourceType: "channel", ResourceID: c.Param("id"),
		After: map[string]any{"count": len(body.Items)},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
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
	if err := a.Catalog.GrantDefaultModels(c.Request.Context(), item.ID); err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "写入渠道默认模型失败", true)
		return
	}
	_ = a.Billing.EnsureChannelQuota(c.Request.Context(), item.ID)
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
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "users.csv", []string{"id", "email", "status", "channel_org_id", "source_code"}, items, func(item identity.UserView) []string {
			return []string{item.ID, item.Email, item.Status, item.ChannelOrgID, item.SourceCode}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.UserView) string { return item.ID })
}

func (a *App) listUsersChannel(c *gin.Context) {
	items, err := a.Identity.ListUsers(c.Request.Context(), *a.currentPrincipal(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取用户失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelAttribution(c *gin.Context) {
	items, err := a.Identity.ListChannelAttribution(c.Request.Context(), *a.currentPrincipal(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取归因失败", true)
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
	if !a.requireConfirm(c) {
		return
	}
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

func (a *App) banUser(c *gin.Context) {
	a.setUserStatus(c, identity.UserStatusBanned, "identity.user.ban")
}

func (a *App) unbanUser(c *gin.Context) {
	a.setUserStatus(c, identity.UserStatusActive, "identity.user.unban")
}

func (a *App) setUserStatus(c *gin.Context, status, action string) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	principal := a.currentPrincipal(c)
	item, before, err := a.Identity.AdminSetUserStatus(c.Request.Context(), *principal, c.Param("id"), status, body.Reason)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	if status == identity.UserStatusBanned {
		if _, err := a.Commission.HoldUnsettledForUser(c.Request.Context(), item.ID); err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "冻结未结算佣金失败", true)
			return
		}
	} else {
		if _, err := a.Commission.ReleaseHeldForUser(c.Request.Context(), item.ID); err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "恢复未结算佣金失败", true)
			return
		}
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: action, ResourceType: "user", ResourceID: item.ID,
		Before: map[string]string{"status": before}, After: map[string]string{"status": item.Status, "reason": body.Reason},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}
