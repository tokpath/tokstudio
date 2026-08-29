package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM7OpsHardening(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m7_admin"
	cfg.BootstrapUser = "m7_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	health := getJSON(t, server.URL+"/healthz", "")
	if ver, _ := health["version"].(string); ver != "0.1.0-m7" {
		t.Fatalf("health: %+v", health)
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "ops-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})
	key := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m7"})["item"].(map[string]any)["key"].(string)
	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "m7-dash"}},
	})
	if chat["request_id"] == nil {
		t.Fatalf("chat: %+v", chat)
	}

	metrics := getAuthJSON(t, server.URL+"/admin/metrics?dimension=model", "m7_admin")
	found := false
	for _, raw := range metrics["items"].([]any) {
		item := raw.(map[string]any)
		if item["key"] == catalog.EchoModelID && asInt(item["requests"]) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("dashboard missing echo model: %+v", metrics)
	}
	dash := getAuthJSON(t, server.URL+"/admin/ops/dashboard", "m7_admin")["dashboard"].(map[string]any)
	totals := dash["totals"].(map[string]any)
	if totals["revenue_minor"] == nil || totals["gross_profit_minor"] == nil {
		t.Fatalf("totals incomplete: %+v", totals)
	}

	limited := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "slow", "rpm_limit": 1})["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/chat/completions", limited, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "first"}},
	})
	if code := postStatus(t, server.URL+"/v1/chat/completions", limited, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "second"}},
	}); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", code)
	}

	_ = postJSONRaw(t, server.URL+"/admin/ops/circuit/prd_echo_primary", "m7_admin", map[string]any{"action": "trip"})
	afterTrip := postJSONRaw(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "after-trip"}},
	})
	if afterTrip["provider"] != catalog.BackupProvider {
		t.Fatalf("tripped primary should fallback: %+v", afterTrip)
	}
	_ = postJSONRaw(t, server.URL+"/admin/ops/circuit/prd_echo_primary", "m7_admin", map[string]any{"action": "reset"})

	_ = postJSONRaw(t, server.URL+"/admin/ops/canary", "m7_admin", map[string]any{"provider_slug": catalog.BackupProvider, "percent": 100})
	forced := doChatHeader(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "canary"}},
	}, map[string]string{"X-Tokenhub-Canary": "1"})
	if forced["provider"] != catalog.BackupProvider {
		t.Fatalf("canary should prefer backup: %+v", forced)
	}
	_ = postJSONRaw(t, server.URL+"/admin/ops/canary", "m7_admin", map[string]any{"provider_slug": catalog.BackupProvider, "percent": 0})

	_ = postJSONRaw(t, server.URL+"/admin/audit-probes", "m7_admin", map[string]any{})
	searched := getAuthJSON(t, server.URL+"/admin/audit-logs?action=audit.probe", "m7_admin")["items"].([]any)
	if len(searched) == 0 {
		t.Fatal("audit search by action should find probe")
	}

	drill := postJSONRaw(t, server.URL+"/admin/ops/backup-drill", "m7_admin", map[string]any{})
	item := drill["item"].(map[string]any)
	if item["status"] != "passed" || asInt(item["rpo_minutes"]) > 15 || asInt(item["rto_minutes"]) > 60 {
		t.Fatalf("backup drill: %+v", drill)
	}
	if pay := postJSONRaw(t, server.URL+"/admin/ops/drills/payment", "m7_admin", map[string]any{}); pay["item"].(map[string]any)["passed"] != true {
		t.Fatalf("payment drill: %+v", pay)
	}
	if media := postJSONRaw(t, server.URL+"/admin/ops/drills/media", "m7_admin", map[string]any{}); media["item"].(map[string]any)["passed"] != true {
		t.Fatalf("media drill: %+v", media)
	}
	_ = postJSONRaw(t, server.URL+"/admin/ops/alerts/evaluate", "m7_admin", map[string]any{})
	if books := getAuthJSON(t, server.URL+"/admin/ops/runbooks", "m7_admin")["items"].([]any); len(books) == 0 {
		t.Fatal("runbooks missing")
	}
}

func postStatus(t *testing.T, url, token string, payload map[string]any) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func doChatHeader(t *testing.T, url, token string, payload map[string]any, headers map[string]string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("POST %s %d %v", url, resp.StatusCode, out)
	}
	return out
}
