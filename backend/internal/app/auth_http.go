package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
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
	r.GET("/admin/brands", a.requireRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.listBrands)
	r.POST("/admin/brands/:id/tls/issue", a.requireRoles("platform_admin", "tech_admin"), a.issueBrandTLS)
	r.POST("/v1/auth/register", a.register)
	r.POST("/v1/auth/login", a.login)
	r.POST("/v1/auth/logout", a.logout)
	r.GET("/v1/auth/google/status", a.googleStatus)
	r.GET("/v1/auth/google/start", a.googleStart)
	r.GET("/v1/auth/google/callback", a.googleCallbackRedirect)
	r.POST("/v1/auth/google/callback", a.googleCallback)
	r.GET("/v1/me", a.requireAnyUser(), a.me)
	r.GET("/v1/me/referral", a.requireAnyUser(), a.meReferral)
	r.POST("/v1/me/referral", a.requireAnyUser(), a.createMeReferral)
	r.PATCH("/v1/me", a.requireAnyUser(), a.patchMe)
	r.POST("/v1/me/password", a.requireAnyUser(), a.changePassword)
	r.POST("/v1/me/channel/switch", a.requireAnyUser(), a.switchChannel)
	r.GET("/admin/channels/:id/admins", a.requireRoles("platform_admin", "channel_admin"), a.listChannelAdmins)
	r.POST("/admin/channels/:id/admins", a.requireRoles("platform_admin", "channel_admin"), a.setChannelAdmin)
	r.GET("/admin/channels", a.requireRoles("platform_admin", "channel_admin", "finance_admin", "ops_admin", "audit_readonly"), a.listChannels)
	r.POST("/admin/channels", a.requireRoles("platform_admin", "channel_admin"), a.createChannel)
	r.GET("/admin/channels/:id", a.requireRoles("platform_admin", "channel_admin", "finance_admin", "ops_admin", "audit_readonly"), a.getChannel)
	r.GET("/admin/channels/:id/models", a.requireRoles("platform_admin", "ops_admin", "finance_admin", "tech_admin", "audit_readonly", "channel_admin"), a.getChannelModels)
	r.PATCH("/admin/channels/:id/models", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.patchChannelModels)
	r.PATCH("/admin/channels/:id", a.requireRoles("platform_admin", "channel_admin"), a.patchChannel)
	r.GET("/channel/models", a.requireRoles("channel_admin"), a.channelModels)
	r.PATCH("/channel/models", a.requireRoles("channel_admin"), a.patchOwnChannelModel)
	r.PATCH("/channel/model-prices", a.requireRoles("channel_admin"), a.patchOwnBrandModelPrice)
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
	r.GET("/channel/subchannels/:id/users", a.requireRoles("channel_admin"), a.listSubchannelUsers)
	r.POST("/channel/users/:id/ban", a.requireRoles("channel_admin"), a.banUser)
	r.POST("/channel/users/:id/unban", a.requireRoles("channel_admin"), a.unbanUser)
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

// sessionCookieSecure：公网 HTTPS（含 token 预览 ENV=development）必须带 Secure。
// 只看 IsProduction 会在 Cloudflare→Caddy→HTTP 上游链路上发出无 Secure 的 cookie，
// 经本地代理 / 部分浏览器策略后 /v1/me 收不到 session → permission_denied。
func (a *App) sessionCookieSecure(c *gin.Context) bool {
	if a.Config != nil && a.Config.IsProduction() {
		return true
	}
	if a.Config != nil {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Config.PublicBaseURL)), "https://") {
			return true
		}
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Config.WebOrigin)), "https://") {
			return true
		}
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	if first, _, _ := strings.Cut(proto, ","); strings.TrimSpace(first) == "https" {
		return true
	}
	return c.Request != nil && c.Request.TLS != nil
}

func (a *App) setSessionCookie(c *gin.Context, token string) {
	secure := a.sessionCookieSecure(c)
	// Domain 空串 = host-only（test.tokpath.com），避免写到父域导致跨站串会话。
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, token, 86400, "/", "", secure, true)
}

func (a *App) clearSessionCookie(c *gin.Context) {
	secure := a.sessionCookieSecure(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(sessionCookie, "", -1, "/", "", secure, true)
}

func (a *App) logout(c *gin.Context) {
	_ = a.Identity.Logout(c.Request.Context(), a.tokenFromRequest(c))
	a.clearSessionCookie(c)
	httpx.OK(c, gin.H{"ok": true, "request_id": c.GetString(httpx.ContextRequestID)})
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
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不能操作后台管理员", false)
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
		Email: body.Email, Password: body.Password, PromotionCode: body.PromotionCode, BrandID: a.signupBrandID(c),
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

func (a *App) googleMode() (configured, available bool) {
	configured = a.Config.GoogleTriad()
	available = configured
	return configured, available
}

func (a *App) resolveGoogleExchange() identity.GoogleExchanger {
	if a.GoogleExchange != nil {
		return a.GoogleExchange
	}
	configured, _ := a.googleMode()
	if configured {
		return identity.NewGoogleExchange(identity.GoogleOAuthConfig{
			ClientID:     a.Config.GoogleClientID,
			ClientSecret: a.Config.GoogleClientSecret,
			RedirectURL:  a.Config.GoogleRedirect,
		})
	}
	return nil
}

func (a *App) writeGoogleAuthError(c *gin.Context, err error) {
	var exchangeErr *identity.GoogleExchangeError
	switch {
	case errors.Is(err, identity.ErrGoogleUnavailable):
		httpx.Abort(c, http.StatusServiceUnavailable, "provider_unavailable", "未配置 Google 登录", false)
	case errors.As(err, &exchangeErr):
		// 不用 502：Cloudflare 会把源站 502 换成明文「error code: 502」，前端只能看到 http_502。
		msg, retryable := googleExchangeMessage(exchangeErr.Reason)
		httpx.AbortParam(c, googleExchangeHTTPStatus(exchangeErr.Reason), "provider_unavailable", msg, exchangeErr.Reason, retryable)
	case errors.Is(err, identity.ErrGoogleExchange):
		httpx.Abort(c, http.StatusServiceUnavailable, "provider_unavailable", "Google 登录失败", true)
	case errors.Is(err, identity.ErrOAuthStateConsumed):
		// 可区分码：前端可探测已有 session 再进控制台，避免假失败页。
		httpx.AbortParam(c, http.StatusForbidden, "authentication_error", "Google 登录失败", "oauth_state_consumed", true)
	case errors.Is(err, identity.ErrInvalidCredentials):
		httpx.Abort(c, http.StatusForbidden, "authentication_error", "Google 登录失败", true)
	case errors.Is(err, identity.ErrPromotionInvalid):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "推广码无效", false)
	default:
		a.writeAuthError(c, err)
	}
}

// googleExchangeHTTPStatus 避免应用层 OAuth 失败使用 502（会被 CF 盖掉 JSON）。
func googleExchangeHTTPStatus(reason string) int {
	if reason == "network_error" {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadRequest
}

func googleExchangeMessage(reason string) (string, bool) {
	switch reason {
	case "redirect_uri_mismatch":
		return "Google Redirect URI 不匹配", false
	case "invalid_client":
		return "Google 客户端配置无效", false
	case "invalid_grant":
		return "Google 授权码无效或已使用", true
	case "access_denied":
		return "已取消 Google 授权", false
	case "email_unverified":
		return "Google 邮箱尚未验证", false
	case "state_mismatch":
		return "请在发起 Google 登录的浏览器中重试", false
	case "account_conflict":
		return "此邮箱已绑定其他 Google 账户", false
	case "network_error":
		return "无法连接 Google，请检查服务器出网", true
	case "empty_profile", "userinfo_error", "empty_token":
		return "无法读取 Google 账号资料", true
	default:
		return "Google 登录失败", true
	}
}

func (a *App) googleStatus(c *gin.Context) {
	configured, available := a.googleMode()
	available = available && a.googleBrandAvailable(c)
	httpx.OK(c, gin.H{
		"available":  available,
		"configured": configured,
		"mock":       false,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) googleStart(c *gin.Context) {
	configured, available := a.googleMode()
	if !available || !a.googleBrandAvailable(c) {
		a.writeGoogleAuthError(c, identity.ErrGoogleUnavailable)
		return
	}
	challenge, err := crypto.RandomToken("browser_")
	if err != nil {
		a.writeGoogleAuthError(c, err)
		return
	}
	state, err := a.Identity.StartGoogleWithIntent(c.Request.Context(), identity.OAuthIntent{BrowserToken: challenge, PromotionCode: c.Query("promotion_code"), BrandID: a.signupBrandID(c), NextPath: identity.SafeReturnPath(c.Query("next"))})
	if err != nil {
		a.writeGoogleAuthError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(googleBrowserCookie(state), challenge, 900, "/", "", a.sessionCookieSecure(c), true)
	authURL := ""
	if configured {
		values := url.Values{}
		values.Set("client_id", a.Config.GoogleClientID)
		values.Set("redirect_uri", a.Config.GoogleRedirect)
		values.Set("response_type", "code")
		values.Set("scope", "openid email profile")
		values.Set("state", state)
		authURL = identity.GoogleAuthorizeURL + "?" + values.Encode()
	}
	httpx.OK(c, gin.H{"state": state, "auth_url": authURL, "mock": false, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) finishGoogleSession(c *gin.Context, state, code string) (*identity.Session, error) {
	exchange := a.resolveGoogleExchange()
	if exchange == nil {
		return nil, identity.ErrGoogleUnavailable
	}
	return a.Identity.FinishGoogle(c.Request.Context(), state, code, exchange)
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
	ctx := c.Request.Context()
	if a.resolveGoogleExchange() == nil {
		a.writeGoogleAuthError(c, identity.ErrGoogleUnavailable)
		return
	}
	if !a.googleBrowserMatches(c, body.State) {
		a.writeGoogleAuthError(c, identity.NewGoogleExchangeError("state_mismatch"))
		return
	}
	intent := a.Identity.GoogleIntent(ctx, body.State)
	if intent.NextPath != "" {
		c.Header("X-Login-Next", intent.NextPath)
	}
	if intent.PromotionCode != "" {
		c.Header("X-Promotion-Code", intent.PromotionCode)
	}
	if body.State == "" || body.Code == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Google 授权未完成", false)
		return
	}
	if session := a.recallGoogleOAuth(ctx, body.State); session != nil {
		a.setSessionCookie(c, session.Token)
		httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID), "idempotent": true})
		return
	}
	if !a.beginGoogleOAuthFlight(ctx, body.State) {
		if session := a.waitGoogleOAuth(ctx, body.State, googleOAuthWaitBudget); session != nil {
			a.setSessionCookie(c, session.Token)
			httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID), "idempotent": true})
			return
		}
		a.writeGoogleAuthError(c, identity.ErrOAuthStateConsumed)
		return
	}
	defer a.endGoogleOAuthFlight(ctx, body.State)

	session, err := a.finishGoogleSession(c, body.State, body.Code)
	if err != nil {
		if errors.Is(err, identity.ErrOAuthStateConsumed) {
			if replay := a.waitGoogleOAuth(ctx, body.State, googleOAuthWaitBudget); replay != nil {
				a.setSessionCookie(c, replay.Token)
				httpx.OK(c, gin.H{"session": replay, "request_id": c.GetString(httpx.ContextRequestID), "idempotent": true})
				return
			}
		}
		a.writeGoogleAuthError(c, err)
		return
	}
	a.rememberGoogleOAuth(ctx, body.State, session)
	a.afterGoogleSession(c, session)
	httpx.OK(c, gin.H{"session": session, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) googleCallbackRedirect(c *gin.Context) {
	state, code := c.Query("state"), c.Query("code")
	origin := a.oauthCallbackOrigin(c)
	if !a.googleBrowserMatches(c, state) {
		c.Redirect(http.StatusFound, origin+"/login?oauth=state_mismatch")
		return
	}
	intent := a.Identity.GoogleIntent(c.Request.Context(), state)
	failure := func(reason string) string {
		params := url.Values{}
		params.Set("oauth_error", "Google 登录失败，请重试")
		params.Set("error_code", reason)
		if intent.NextPath != "" {
			params.Set("next", intent.NextPath)
		}
		if intent.PromotionCode != "" {
			params.Set("promotion_code", intent.PromotionCode)
		}
		return origin + "/login?" + params.Encode()
	}
	destination := func(session *identity.Session) string {
		next := identity.SafeReturnPath(session.NextPath)
		if next == "" {
			next = "/enter"
		}
		return origin + next
	}
	if strings.TrimSpace(c.Query("error")) != "" {
		c.Redirect(http.StatusFound, failure("access_denied"))
		return
	}
	ctx := c.Request.Context()
	if session := a.recallGoogleOAuth(ctx, state); session != nil {
		a.setSessionCookie(c, session.Token)
		c.Redirect(http.StatusFound, destination(session))
		return
	}
	session, err := a.finishGoogleSession(c, state, code)
	if err != nil {
		if errors.Is(err, identity.ErrOAuthStateConsumed) {
			if replay := a.waitGoogleOAuth(ctx, state, googleOAuthWaitBudget); replay != nil {
				a.setSessionCookie(c, replay.Token)
				c.Redirect(http.StatusFound, destination(replay))
				return
			}
		}
		c.Redirect(http.StatusFound, failure("authentication_error"))
		return
	}
	a.rememberGoogleOAuth(ctx, state, session)
	a.afterGoogleSession(c, session)
	c.Redirect(http.StatusFound, destination(session))
}

// Local development can use explicit test promo codes. A known public brand
// constrains the invitation and default registration to that brand.
func googleBrowserCookie(state string) string {
	return "tokenhub_google_" + crypto.HashToken(state)[:16]
}
func (a *App) googleBrowserMatches(c *gin.Context, state string) bool {
	challenge, err := c.Cookie(googleBrowserCookie(state))
	return err == nil && a.Identity.GoogleBrowserMatches(c.Request.Context(), state, challenge)
}

// Known brands return to their configured Google frontend origin.
func (a *App) oauthCallbackOrigin(c *gin.Context) string {
	if a.Identity.KnownBrandHost(c.Request.Context(), a.requestHost(c)) && a.googleBrandAvailable(c) {
		if callback, err := url.Parse(a.Config.GoogleRedirect); err == nil && (callback.Scheme == "https" || callback.Scheme == "http") && callback.Host != "" {
			return callback.Scheme + "://" + callback.Host
		}
	}
	return a.webOrigin()
}

func (a *App) signupBrandID(c *gin.Context) string {
	host := a.requestHost(c)
	if !a.Identity.KnownBrandHost(c.Request.Context(), host) {
		return ""
	}
	brand, err := a.Identity.BrandByHost(c.Request.Context(), host)
	if err != nil {
		return ""
	}
	return brand.ID
}

// A single configured Google callback cannot establish a session on an
// unrelated OEM domain. Advertise Google only for its configured brand.
func (a *App) googleBrandAvailable(c *gin.Context) bool {
	brandID := a.signupBrandID(c)
	if brandID == "" {
		return true
	}
	redirect, err := url.Parse(a.Config.GoogleRedirect)
	if err != nil || redirect.Hostname() == "" {
		return false
	}
	current, parseErr := url.Parse("//" + a.requestHost(c))
	if parseErr != nil || !strings.EqualFold(current.Hostname(), redirect.Hostname()) {
		return false
	}
	brand, err := a.Identity.BrandByHost(c.Request.Context(), redirect.Hostname())
	return err == nil && brand.ID == brandID
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
		Limit:  0,
	})
	items := page.Items
	next := ""
	if limit := catalog.ParseLimit(c.Query("limit")); limit > 0 {
		items, next = httpx.Paginate(items, limit, c.Query("cursor"), func(item catalog.ModelView) string { return item.ID })
	}
	httpx.OK(c, gin.H{
		"next_cursor": next,
		"items":       publicModelCards(items), "total": page.Total, "facets": page.Facets,
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
			"status": model.ServiceStatus, "service_status": model.ServiceStatus,
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
	apiBase := docsAPIBase(brand.APIDomain, a.Config.PublicBaseURL)
	if apiBase == "" {
		httpx.Abort(c, http.StatusServiceUnavailable, "brand_api_unavailable", "品牌 API 地址尚未配置，请联系管理员", false)
		return
	}
	channelID, err := a.Identity.ChannelIDByBrand(c.Request.Context(), brand.ID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道失败", true)
		return
	}
	models, err := a.Catalog.ListVisibleModels(c.Request.Context(), channelID, nil)
	if err != nil {
		httpx.Abort(c, http.StatusServiceUnavailable, "catalog_unavailable", "模型说明暂不可用", true)
		return
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	examples := gin.H{}
	endpoints := []string{}
	wanted := c.Query("model")
	for _, model := range models {
		if model.ID == wanted {
			for _, path := range docsEndpointList(model.Capabilities) {
				endpoints = append(endpoints, path)
			}
			examples = docsExamplesFor(apiBase, wanted, endpoints)
		}
	}
	httpx.OK(c, gin.H{
		"brand":        brand,
		"models":       ids,
		"api_base_url": apiBase,
		"examples":     examples, "supported_endpoints": endpoints, "model": wanted,
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
		if errors.Is(err, identity.ErrACMEFailed) || errors.Is(err, identity.ErrCloudflareFailed) {
			httpx.Abort(c, http.StatusBadGateway, "provider_unavailable", "证书签发失败："+err.Error(), true)
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
	if c.Query("managed") == "1" && a.currentPrincipal(c).VisibleChannelID() == "" {
		managed := make([]identity.ChannelView, 0, len(items))
		for _, item := range items {
			if item.ID == identity.OfficialChannelID || item.ParentID == identity.OfficialChannelID || (item.ParentID == "" && (item.Type == identity.ChannelTypeB || item.Type == identity.ChannelTypeC)) {
				managed = append(managed, item)
			}
		}
		items = managed
	}
	if kind := c.Query("type"); kind != "" {
		filtered := make([]identity.ChannelView, 0, len(items))
		for _, item := range items {
			if item.Type == kind {
				filtered = append(filtered, item)
			}
		}
		items = filtered
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
	target, err := a.Identity.GetChannel(c.Request.Context(), *principal, c.Param("id"))
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	includeCatalog := principal.HasRole("platform_admin", "ops_admin")
	var items []catalog.ChannelModelView
	if !includeCatalog && target.ParentID == principal.ChannelOrgID && target.ID != principal.ChannelOrgID {
		items, err = a.Catalog.ListDelegableChannelModels(c.Request.Context(), target.ID, principal.ChannelOrgID)
	} else {
		items, err = a.Catalog.ListChannelModels(c.Request.Context(), target.ID, includeCatalog)
	}
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
	principal := a.currentPrincipal(c)
	target, err := a.Identity.GetChannel(c.Request.Context(), *principal, c.Param("id"))
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	parentID := ""
	if principal.IsChannelStaff() && !principal.HasRole("platform_admin", "ops_admin") {
		own, ownErr := a.Identity.GetChannel(c.Request.Context(), *principal, principal.ChannelOrgID)
		if ownErr != nil || own.Type != identity.ChannelTypeC || target.ParentID != own.ID || target.ID == own.ID {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能授权自己的直属下属渠道", false)
			return
		}
		parentID = own.ID
	} else if target.ID != identity.OfficialChannelID && target.ParentID != identity.OfficialChannelID &&
		!(target.ParentID == "" && (target.Type == identity.ChannelTypeB || target.Type == identity.ChannelTypeC)) {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "平台只授权直属渠道和 OEM", false)
		return
	}
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Items []catalog.ChannelModelGrant `json:"items"`
	}
	_ = c.ShouldBindJSON(&body)
	for _, grant := range body.Items {
		if len(grant.CustomerOverride) != 0 {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "客户价格由品牌平台统一设置", false)
			return
		}
	}
	if parentID != "" {
		err = a.Catalog.SetDelegatedChannelModels(c.Request.Context(), target.ID, parentID, body.Items)
	} else {
		err = a.Catalog.SetChannelModels(c.Request.Context(), target.ID, body.Items)
	}
	if err != nil {
		switch {
		case errors.Is(err, catalog.ErrUnknownModel):
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "只能授权平台目录中已有的模型，租户不能自建提供商或模型", false)
		case errors.Is(err, catalog.ErrInvalidInput):
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "模型授权需要有效的渠道结算价", false)
		case errors.Is(err, catalog.ErrModelNotVisible):
			httpx.Abort(c, http.StatusConflict, "invalid_request", "模型须先发布并启用路由，才能新增渠道授权", false)
		default:
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "写入渠道模型失败", true)
		}
		return
	}
	var items []catalog.ChannelModelView
	if parentID != "" {
		items, err = a.Catalog.ListDelegableChannelModels(c.Request.Context(), target.ID, parentID)
	} else {
		items, err = a.Catalog.ListChannelModels(c.Request.Context(), target.ID, true)
	}
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

func (a *App) patchOwnChannelModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		PublicID string `json:"public_id"`
		Enabled  *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.PublicID) == "" || body.Enabled == nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "模型状态无效", false)
		return
	}
	principal := a.currentPrincipal(c)
	if err := a.Catalog.SetOwnChannelModelEnabled(c.Request.Context(), principal.ChannelOrgID, body.PublicID, *body.Enabled); err != nil {
		if errors.Is(err, catalog.ErrModelNotVisible) {
			httpx.Abort(c, http.StatusConflict, "model_not_allowed", "上级或平台已下架此模型", false)
		} else {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "本渠道没有此模型授权", false)
		}
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "channel.model.status", ResourceType: "channel", ResourceID: principal.ChannelOrgID,
		After: map[string]any{"public_id": body.PublicID, "enabled": *body.Enabled},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"public_id": body.PublicID, "enabled": *body.Enabled, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchOwnBrandModelPrice(c *gin.Context) {
	principal := a.currentPrincipal(c)
	channel, err := a.Identity.GetChannel(c.Request.Context(), *principal, principal.ChannelOrgID)
	if err != nil || channel.Type != identity.ChannelTypeC {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅品牌平台可设置客户价", false)
		return
	}
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		PublicID string            `json:"public_id"`
		Prices   map[string]string `json:"customer_price"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.PublicID) == "" || body.Prices == nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "客户价格无效", false)
		return
	}
	if err := a.Catalog.SetOwnChannelCustomerPrices(c.Request.Context(), channel.ID, body.PublicID, body.Prices); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "客户价格无效或模型未授权", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "channel.model.customer_price", ResourceType: "channel", ResourceID: channel.ID,
		After: map[string]any{"public_id": body.PublicID, "customer_price": body.Prices}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"public_id": body.PublicID, "request_id": c.GetString(httpx.ContextRequestID)})
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
	if item.Type == identity.ChannelTypeC {
		_ = a.Billing.EnsureChannelQuota(c.Request.Context(), item.ID)
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
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "users.csv", []string{"id", "email", "status", "channel_org_id", "source_code"}, items, func(item identity.UserView) []string {
			return []string{item.ID, item.Email, item.Status, item.ChannelOrgID, item.SourceCode}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.UserView) string { return item.ID })
}

func (a *App) listUsersChannel(c *gin.Context) {
	channelID, ok := a.channelForQuery(c)
	if !ok {
		return
	}
	items, err := a.Identity.ListUsersForChannel(c.Request.Context(), *a.currentPrincipal(c), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取用户失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listSubchannelUsers(c *gin.Context) {
	principal := a.currentPrincipal(c)
	child, err := a.Identity.GetChannel(c.Request.Context(), *principal, c.Param("id"))
	if err != nil || child.Type != identity.ChannelTypeB || child.ParentID != principal.ChannelOrgID {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能查看直属下属渠道用户", false)
		return
	}
	parent, err := a.Identity.GetChannel(c.Request.Context(), *principal, principal.ChannelOrgID)
	if err != nil || parent.Type != identity.ChannelTypeC {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 OEM 可查看下属渠道用户", false)
		return
	}
	items, err := a.Identity.ListUsersForChannel(c.Request.Context(), *principal, child.ID)
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
	channel, err := a.Identity.GetChannel(c.Request.Context(), *principal, principal.ChannelOrgID)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	httpx.OK(c, gin.H{
		"channel_org_id": principal.ChannelOrgID,
		"channel_type":   channel.Type,
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
	var item *identity.UserView
	var before string
	var identityErr error
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		item, before, identityErr = a.Identity.AdminSetUserStatusTx(tx, *principal, c.Param("id"), status, body.Reason)
		if identityErr != nil {
			return identityErr
		}
		return a.Commission.SetUserHoldTx(tx, item.ID, status == identity.UserStatusBanned)
	})
	if err != nil {
		if identityErr != nil {
			a.writeAuthError(c, identityErr)
		} else {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "用户状态与佣金处理未能同时完成，本次修改已回滚，请重试。", true)
		}
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: action, ResourceType: "user", ResourceID: item.ID,
		Before: map[string]string{"status": before}, After: map[string]string{"status": item.Status, "reason": body.Reason},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}
