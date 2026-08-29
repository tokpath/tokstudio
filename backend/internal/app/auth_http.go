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
	r.GET("/v1/public/docs-context", a.docsContext)
	r.POST("/v1/auth/register", a.register)
	r.POST("/v1/auth/login", a.login)
	r.POST("/v1/auth/otp/request", a.requestOTP)
	r.POST("/v1/auth/otp/verify", a.verifyOTP)
	r.GET("/v1/auth/google/start", a.googleStart)
	r.POST("/v1/auth/google/callback", a.googleCallback)
	r.GET("/v1/me", a.requireAnyUser(), a.me)
	r.POST("/v1/me/channel/switch", a.requireAnyUser(), a.switchChannel)
	r.GET("/admin/channels", a.requireRoles("platform_admin", "channel_admin"), a.listChannels)
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
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
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
	httpx.OK(c, gin.H{
		"brand": brand,
		"examples": gin.H{
			"curl":   "curl -H 'Authorization: Bearer $TOKENHUB_API_KEY' https://" + brand.APIDomain + "/v1/models",
			"python": "from openai import OpenAI\nclient = OpenAI(base_url='https://" + brand.APIDomain + "/v1', api_key='...')",
			"node":   "const client = new OpenAI({ baseURL: 'https://" + brand.APIDomain + "/v1', apiKey: process.env.TOKENHUB_API_KEY })",
		},
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) listChannels(c *gin.Context) {
	items, err := a.Identity.ListChannels(c.Request.Context(), *a.currentPrincipal(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
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
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"status": "updated", "request_id": c.GetString(httpx.ContextRequestID)})
}
