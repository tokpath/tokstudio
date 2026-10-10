package app_test

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

var diagnosticTestStaff sync.Map
var diagnosticTestApps sync.Map

// Tests explicitly opt into the staff-only diagnostic endpoint. Security tests
// use ordinary HTTP requests and never call this helper.
func diagnosticTestRequest(t *testing.T, req *http.Request) {
	t.Helper()
	staff := ""
	var fixture *app.App
	longest := 0
	diagnosticTestStaff.Range(func(k, v any) bool {
		name := k.(string)
		if (t.Name() == name || strings.HasPrefix(t.Name(), name+"/")) && len(name) > longest {
			staff = v.(string)
			longest = len(name)
			if value, ok := diagnosticTestApps.Load(name); ok {
				fixture = value.(*app.App)
			}
		}
		return true
	})
	if staff == "" {
		t.Fatal("diagnostic test requires mustApp staff fixture")
	}
	key := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	req.URL.Path = strings.Replace(req.URL.Path, "/v1/", "/admin/diagnostics/", 1)
	if key != "" && fixture != nil {
		principal, err := fixture.Identity.AuthenticateAPIKey(context.Background(), key)
		if err != nil || principal == nil {
			t.Fatalf("diagnostic fixture Key: %v", err)
		}
		// This explicit test helper equips the Key owner with a temporary technical
		// role/session. Production diagnostics must never spend another user's Key.
		role := fixture.DB.Exec("INSERT INTO identity_user_roles (user_id,role_id,scope_type,scope_id) SELECT ?,id,'platform','*' FROM identity_roles WHERE code='tech_admin' ON CONFLICT DO NOTHING", principal.UserID)
		if role.Error != nil {
			t.Fatal(role.Error)
		}
		if role.RowsAffected > 0 {
			uid := principal.UserID
			t.Cleanup(func() {
				fixture.DB.Exec("DELETE FROM identity_user_roles WHERE user_id=? AND role_id IN (SELECT id FROM identity_roles WHERE code='tech_admin') AND scope_type='platform' AND scope_id='*'", uid)
			})
		}
		raw, err := crypto.RandomToken("thses_")
		if err != nil {
			t.Fatal(err)
		}
		tokenID := id.New("tok")
		if err := fixture.DB.Table("identity_access_tokens").Create(map[string]any{"id": tokenID, "user_id": principal.UserID, "token_hash": crypto.HashToken(raw), "prefix": "thses_", "status": "active", "created_at": time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { fixture.DB.Exec("DELETE FROM identity_access_tokens WHERE id=?", tokenID) })
		staff = raw
	}
	req.Header.Set("Authorization", "Bearer "+staff)
	req.Header.Set("X-Tokenhub-Test-Key", key)
}
func postDiagnosticJSONRaw(t *testing.T, url, key string, body any) map[string]any {
	t.Helper()
	code, out := doDiagnosticJSON(t, "POST", url, key, body)
	if code >= 300 {
		t.Fatalf("diagnostic POST %d %+v", code, out)
	}
	return out
}
func doDiagnosticJSON(t *testing.T, method, url, key string, body any) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(string(mustJSON(body))))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	diagnosticTestRequest(t, req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, out
}
func postDiagnosticStatus(t *testing.T, url, key string, body any, headers map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(mustJSON(body))))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	diagnosticTestRequest(t, req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func getDiagnosticJSON(t *testing.T, url, key string) map[string]any {
	t.Helper()
	code, out := doDiagnosticJSON(t, "GET", url, key, nil)
	if code >= 300 {
		t.Fatalf("diagnostic GET %d %+v", code, out)
	}
	return out
}

// Publishing a global fixture price must not change unrelated test budgets.
func isolateEchoPrice(t *testing.T, a *app.App) {
	t.Helper()
	var previous string
	if err := a.DB.Raw("SELECT id FROM catalog_price_versions WHERE public_model_id='mdl_echo' AND status='published' ORDER BY effective_at DESC LIMIT 1").Scan(&previous).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.DB.Exec("UPDATE catalog_price_versions SET status='superseded' WHERE public_model_id='mdl_echo'").Error; err != nil {
			t.Error(err)
		}
		if err := a.DB.Exec("UPDATE catalog_price_versions SET status='published' WHERE id=?", previous).Error; err != nil {
			t.Error(err)
		}
	})
}
