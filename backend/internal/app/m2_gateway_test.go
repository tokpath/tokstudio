package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
	"net/http/httptest"
)

func TestM2GatewayFallbackAndParams(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m2_admin"
	cfg.BootstrapUser = "m2_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "gw-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	keyResp := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "e2e"})
	item := keyResp["item"].(map[string]any)
	apiKey := item["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})

	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	if chat["provider"] != catalog.PrimaryProvider {
		t.Fatalf("expected primary provider, got %+v", chat)
	}

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader(mustJSON(map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Force-Fail", catalog.PrimaryProvider)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var fallback map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&fallback)
	if fallback["provider"] != catalog.BackupProvider {
		t.Fatalf("expected backup fallback, got %+v", fallback)
	}
	requestID := fallback["request_id"].(string)
	attempts := getAuthJSON(t, server.URL+"/v1/requests/"+requestID+"/attempts", apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) < 2 {
		t.Fatalf("expected two attempts, got %+v", attempts)
	}

	bad := mustStatusJSON(t, http.MethodPost, server.URL+"/v1/chat/completions", apiKey, nil)
	// rebuild with logit_bias
	req2, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader(mustJSON(map[string]any{
		"model": catalog.EchoModelID, "logit_bias": map[string]int{"1": 1}, "messages": []map[string]string{{"role": "user", "content": "x"}},
	})))
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := http.DefaultClient.Do(req2)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsupported param expected 400, got %d", resp2.StatusCode)
	}
	_ = bad
}

func mustApp(t *testing.T, cfg *config.Config) *app.App {
	t.Helper()
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb, err := redisx.Open(cfg.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	application := app.New(cfg, gdb, rdb, logx.New("error", os.Stdout))
	if err := application.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := application.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	return application
}

func postJSONRaw(t *testing.T, url, token string, payload map[string]any) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
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

func patchJSONRaw(t *testing.T, url, token string, payload map[string]any) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("PATCH %s %d %v", url, resp.StatusCode, out)
	}
	return out
}

func mustJSON(v any) []byte {
	body, _ := json.Marshal(v)
	return body
}
