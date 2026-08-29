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
