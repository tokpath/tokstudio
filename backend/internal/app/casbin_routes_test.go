package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestCasbinCoversProtectedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	application := &App{
		Config:   &config.Config{Env: "test"},
		Identity: identity.New(nil),
	}
	roles := []string{
		"platform_admin", "finance_admin", "ops_admin", "tech_admin",
		"channel_admin", "audit_readonly", "end_user",
	}
	for _, route := range application.Router().Routes() {
		if skipCasbinRoute(route.Method, route.Path) {
			continue
		}
		allowed := false
		for _, role := range roles {
			p := &identity.Principal{UserID: "usr_test", Roles: []string{role}}
			if application.Identity.Allow(p, route.Path, route.Method) {
				allowed = true
				break
			}
		}
		if !allowed {
			t.Errorf("no casbin policy for %s %s", route.Method, route.Path)
		}
	}
}

func skipCasbinRoute(method, path string) bool {
	switch {
	case path == "/healthz" || path == "/readyz" || path == "/metrics":
		return true
	case strings.HasPrefix(path, "/v1/public/"), strings.HasPrefix(path, "/.well-known/"):
		return true
	case strings.HasPrefix(path, "/v1/auth/"):
		return true
	case method == http.MethodGet && path == "/v1/plans":
		return true
	case strings.Contains(path, "/webhook"):
		return true
	case path == "/v1/media/callbacks" || path == "/v1/media/objects":
		return true
	case path == "/v1/models" || strings.HasPrefix(path, "/v1/models/"):
		return true
	case path == "/v1/chat/completions" || path == "/v1/responses" || path == "/v1/messages":
		return true
	case strings.HasPrefix(path, "/v1/requests/"):
		return true
	default:
		return false
	}
}
