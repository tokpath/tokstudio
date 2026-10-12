package app

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func (a *App) registerEmailAuthRoutes(r *gin.Engine) {
	r.GET("/v1/auth/email/status", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		httpx.OK(c, gin.H{"available": a.Identity.EmailAvailable()})
	})
	r.POST("/v1/auth/password/forgot", a.requestEmailAction(identity.EmailReset, false))
	r.POST("/v1/auth/password/reset", a.completeEmailAction(identity.EmailReset))
	r.POST("/v1/auth/email/verification/request", a.requestEmailAction(identity.EmailVerify, false))
	r.POST("/v1/auth/email/verify", a.completeEmailAction(identity.EmailVerify))
	r.POST("/v1/me/email/verification", a.requireAnyUser(), a.requestEmailAction(identity.EmailVerify, true))
}

func (a *App) emailActionContext(c *gin.Context, brandID string) (identity.EmailActionContext, error) {
	var origin string
	if brandID == identity.OfficialBrandID {
		// The configured deployment origin is trusted; a supplied Host never becomes a link.
		origin = strings.TrimRight(a.Config.WebOrigin, "/")
	} else {
		brand, err := a.Identity.BrandByID(c.Request.Context(), brandID)
		if err != nil {
			return identity.EmailActionContext{}, err
		}
		host := brand.PrimaryDomain
		if authHostname(a.authenticationHost(c)) == brand.AdminDomain {
			host = brand.AdminDomain
		}
		origin = "https://" + host
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && !a.Config.IsProduction() && loopbackAuthHost(u.Host))) {
		return identity.EmailActionContext{}, identity.ErrEmailUnavailable
	}
	locale, _ := c.Cookie("NEXT_LOCALE")
	if _, err := identity.NormalizeLocale(locale); err != nil {
		locale = "zh"
		language := strings.ToLower(c.GetHeader("Accept-Language"))
		if strings.HasPrefix(language, "en") {
			locale = "en"
		}
		if strings.HasPrefix(language, "ja") {
			locale = "ja"
		}
	}
	return identity.EmailActionContext{Origin: origin, Locale: locale}, nil
}

func (a *App) writeEmailError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, identity.ErrEmailUnavailable):
		httpx.Abort(c, 503, "email_unavailable", "邮件服务暂不可用，请稍后重试", true)
	case errors.Is(err, identity.ErrEmailLink):
		httpx.Abort(c, 400, "email_link_invalid", "链接无效、已使用或已过期，请重新申请邮件", false)
	default:
		a.writeAuthError(c, err)
	}
}

func (a *App) emailRateLimit(c *gin.Context, purpose string) bool {
	if a.Redis == nil {
		httpx.Abort(c, 503, "service_unavailable", "暂时无法处理，请稍后重试", true)
		return false
	}
	key := "tokenhub:email:rate:" + crypto.HashToken(purpose+"\x00"+c.ClientIP())
	count, err := a.Redis.Eval(c.Request.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],900) end; return n`, []string{key}).Int64()
	if err != nil {
		httpx.Abort(c, 503, "service_unavailable", "暂时无法处理，请稍后重试", true)
		return false
	}
	if count > 20 {
		c.Header("Retry-After", "900")
		httpx.Abort(c, 429, "rate_limit_exceeded", "操作过于频繁，请稍后重试", true)
		return false
	}
	return true
}

func (a *App) requestEmailAction(purpose string, authenticated bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		brandID, err := a.authenticationBrand(c)
		if err != nil {
			a.writeAuthError(c, identity.ErrInvalidCredentials)
			return
		}
		if !a.Identity.EmailAvailable() {
			a.writeEmailError(c, identity.ErrEmailUnavailable)
			return
		}
		var body struct {
			Email string `json:"email"`
		}
		var userID string
		if authenticated {
			principal := a.currentPrincipal(c)
			body.Email = principal.Email
			userID = principal.UserID
		} else if err := c.ShouldBindJSON(&body); err != nil {
			httpx.Abort(c, 400, "invalid_request", "请求体无效", false)
			return
		}
		body.Email = strings.ToLower(strings.TrimSpace(body.Email))
		if !mail.ValidAddress(body.Email) {
			httpx.Abort(c, 400, "invalid_request", "请填写有效邮箱", false)
			return
		}
		if !a.emailRateLimit(c, "request:"+purpose) {
			return
		}
		// Cooldown also applies to unknown email addresses, preserving a generic response.
		key := "tokenhub:email:cooldown:" + crypto.HashToken(brandID+"\x00"+purpose+"\x00"+body.Email)
		acquired, err := a.Redis.SetNX(c.Request.Context(), key, "1", time.Minute).Result()
		if err != nil {
			httpx.Abort(c, 503, "service_unavailable", "暂时无法处理，请稍后重试", true)
			return
		}
		if acquired {
			delivery, err := a.emailActionContext(c, brandID)
			if err != nil {
				a.writeEmailError(c, err)
				return
			}
			if err := a.Identity.RequestEmailAction(c.Request.Context(), identity.EmailActionInput{BrandID: brandID, Email: body.Email, UserID: userID, Purpose: purpose, Context: delivery}); err != nil {
				a.writeEmailError(c, err)
				return
			}
		}
		c.JSON(http.StatusAccepted, gin.H{"ok": true, "retry_after": 60, "request_id": c.GetString(httpx.ContextRequestID)})
	}
}

func (a *App) completeEmailAction(purpose string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
		brandID, err := a.authenticationBrand(c)
		if err != nil {
			a.writeEmailError(c, identity.ErrEmailLink)
			return
		}
		var body struct {
			Token    string `json:"token"`
			Password string `json:"new_password"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			httpx.Abort(c, 400, "invalid_request", "请求体无效", false)
			return
		}
		if !a.emailRateLimit(c, "complete:"+purpose) {
			return
		}
		userID, err := a.Identity.CompleteEmailAction(c.Request.Context(), brandID, purpose, body.Token, body.Password)
		if err != nil {
			a.writeEmailError(c, err)
			return
		}
		if purpose == identity.EmailReset {
			a.clearSessionCookie(c)
		}
		_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: userID, Action: "auth." + purpose, ResourceType: "user", ResourceID: userID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		httpx.OK(c, gin.H{"ok": true, "request_id": c.GetString(httpx.ContextRequestID)})
	}
}
