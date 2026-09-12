package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func TestRequireRolesDeniesMissingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &App{
		Config:   &config.Config{Env: "test"},
		Identity: identity.New(nil),
	}
	r := gin.New()
	r.GET("/admin/secret", application.requireRoles("platform_admin"), func(c *gin.Context) {
		httpx.OK(c, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/admin/secret", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequireAnyUserMissingAuthIsUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &App{
		Config:   &config.Config{Env: "test"},
		Identity: identity.New(nil),
	}
	r := gin.New()
	r.GET("/v1/me", application.requireAnyUser(), func(c *gin.Context) {
		httpx.OK(c, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous /v1/me expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSessionCookieSecureFromPublicHTTPS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &App{
		Config: &config.Config{
			Env:           "development",
			PublicBaseURL: "https://test.tokpath.com",
			WebOrigin:     "https://test.tokpath.com",
		},
	}
	r := gin.New()
	r.POST("/login", func(c *gin.Context) {
		application.setSessionCookie(c, "thses_test_token")
		httpx.OK(c, gin.H{"ok": true})
	})
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("missing Set-Cookie: %v", rec.Header().Values("Set-Cookie"))
	}
	cookie := cookies[0]
	if cookie.Name != "tokenhub_session" || !cookie.Secure || !cookie.HttpOnly {
		t.Fatalf("want Secure+HttpOnly tokenhub_session, got %+v", cookie)
	}
	if cookie.Path != "/" || cookie.Domain != "" {
		t.Fatalf("want Path=/ host-only Domain, got path=%q domain=%q", cookie.Path, cookie.Domain)
	}
}
