package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
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

	tools := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model":    catalog.EchoModelID,
		"messages": []map[string]string{{"role": "user", "content": "lookup weather"}},
		"tools":    []map[string]any{{"type": "function", "function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object"}}}},
	})
	choices, _ := tools["choices"].([]any)
	if len(choices) == 0 {
		t.Fatalf("tools: %+v", tools)
	}
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	calls, _ := msg["tool_calls"].([]any)
	if len(calls) == 0 {
		t.Fatalf("expected tool_calls: %+v", tools)
	}
	follow := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID,
		"messages": []map[string]any{
			{"role": "user", "content": "lookup weather"},
			{"role": "tool", "tool_call_id": "call_echo", "content": "sunny"},
		},
		"tools": []map[string]any{{"type": "function", "function": map[string]any{"name": "lookup"}}},
	})
	if !containsText(follow, "tool-result:sunny") {
		t.Fatalf("tool result: %+v", follow)
	}
	js := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model":           catalog.EchoModelID,
		"messages":        []map[string]string{{"role": "user", "content": "hi"}},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "echo", "schema": map[string]any{"type": "object"}}},
	})
	jsText, _ := firstContentOf(js)
	if !strings.Contains(jsText, `"ok"`) {
		t.Fatalf("json schema: %+v", js)
	}
	vision := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID,
		"messages": []map[string]any{{"role": "user", "content": []map[string]any{
			{"type": "text", "text": "describe"},
			{"type": "image_url", "image_url": map[string]string{"url": "https://example.test/a.png"}},
		}}},
	})
	if !containsText(vision, "vision:describe") {
		t.Fatalf("vision: %+v", vision)
	}
	reason := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model":            catalog.EchoModelID,
		"messages":         []map[string]string{{"role": "user", "content": "think"}},
		"reasoning_effort": "low",
	})
	usage, _ := reason["usage"].(map[string]any)
	if asInt(usage["reasoning_tokens"]) != 3 {
		t.Fatalf("reasoning usage: %+v", reason)
	}
	anth := postJSONRaw(t, server.URL+"/v1/messages", apiKey, map[string]any{
		"model":    catalog.EchoModelID,
		"messages": []map[string]string{{"role": "user", "content": "lookup"}},
		"tools":    []map[string]any{{"name": "lookup", "input_schema": map[string]any{"type": "object"}}},
	})
	blocks, _ := anth["content"].([]any)
	if len(blocks) == 0 || blocks[0].(map[string]any)["type"] != "tool_use" {
		t.Fatalf("anthropic tools: %+v", anth)
	}

	ctx := context.Background()
	if err := application.Catalog.MarkHealth(ctx, "prd_echo_primary", "degraded"); err != nil {
		t.Fatal(err)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/routes/rg_echo", "m2_admin", map[string]any{"strategy": "health"})
	defer func() {
		_ = application.Catalog.MarkHealth(ctx, "prd_echo_primary", "available")
		_ = patchJSONRaw(t, server.URL+"/admin/routes/rg_echo", "m2_admin", map[string]any{"strategy": "priority"})
	}()
	healthy := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "health"}},
	})
	if healthy["provider"] != catalog.BackupProvider {
		t.Fatalf("health strategy should prefer available backup, got %+v", healthy)
	}

	cheap := []byte(`{"input":"0.0000001","upstream_cost_input":"0.0000001","output":"0.0000002","currency":"USD"}`)
	priceID := "price_echo_backup_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := application.DB.Exec(
		`INSERT INTO catalog_price_versions(id, public_model_id, provider_id, unit_prices_json, effective_at, status)
		 VALUES (?, 'mdl_echo', 'prd_echo_backup', ?, NOW(), 'published')`,
		priceID, cheap,
	).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = application.DB.Exec(`UPDATE catalog_price_versions SET status = 'superseded' WHERE id = ?`, priceID).Error
	}()
	_ = patchJSONRaw(t, server.URL+"/admin/routes/rg_echo", "m2_admin", map[string]any{"strategy": "price"})
	priced := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "price"}},
	})
	if priced["provider"] != catalog.BackupProvider {
		t.Fatalf("price strategy should prefer cheaper backup, got %+v", priced)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/routes/rg_echo", "m2_admin", map[string]any{"strategy": "priority"})
	_ = application.Catalog.MarkHealth(ctx, "prd_echo_primary", "available")
	reset := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "priority"}},
	})
	if reset["provider"] != catalog.PrimaryProvider {
		t.Fatalf("priority strategy should return to primary, got %+v", reset)
	}

	limited := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{
		"name": "gemini-only", "allowlist": []string{catalog.GeminiModelID},
	})
	limitedItem := limited["item"].(map[string]any)
	limitedKey := limitedItem["key"].(string)
	if allow, _ := limitedItem["allowlist"].([]any); len(allow) != 1 || allow[0] != catalog.GeminiModelID {
		t.Fatalf("create should echo allowlist: %+v", limited)
	}
	listed := getAuthJSON(t, server.URL+"/v1/me/api-keys", session)
	foundLimited := false
	for _, raw := range listed["items"].([]any) {
		row := raw.(map[string]any)
		if row["id"] != limitedItem["id"] {
			continue
		}
		foundLimited = true
		allow, _ := row["allowlist"].([]any)
		if len(allow) != 1 || allow[0] != catalog.GeminiModelID {
			t.Fatalf("list missing allowlist: %+v", row)
		}
	}
	if !foundLimited {
		t.Fatalf("created limited key missing from list: %+v", listed)
	}
	denied := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", limitedKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "deny"}},
	})
	if denied.status != http.StatusForbidden {
		t.Fatalf("echo should be model_not_allowed, got %d %+v", denied.status, denied.body)
	}
	if errObj, _ := denied.body["error"].(map[string]any); errObj["code"] != "model_not_allowed" {
		t.Fatalf("expected model_not_allowed, got %+v", denied.body)
	}
	models := getAuthJSON(t, server.URL+"/v1/models", limitedKey)
	data, _ := models["data"].([]any)
	for _, raw := range data {
		row, _ := raw.(map[string]any)
		if row["id"] == catalog.EchoModelID {
			t.Fatalf("limited key should not list echo: %+v", models)
		}
	}
	allowed := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{
		"name": "echo-only", "allowlist": []string{catalog.EchoModelID}, "rpm_limit": 30,
	})
	echoKey := allowed["item"].(map[string]any)["key"].(string)
	okChat := postJSONRaw(t, server.URL+"/v1/chat/completions", echoKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "allow"}},
	})
	if okChat["provider"] != catalog.PrimaryProvider {
		t.Fatalf("echo-only key should chat: %+v", okChat)
	}

	oneSlot := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{
		"name": "one-slot", "concurrency_limit": 1,
	})
	slotItem := oneSlot["item"].(map[string]any)
	if conc, _ := slotItem["concurrency_limit"].(float64); conc != 1 {
		t.Fatalf("create should echo concurrency_limit: %+v", oneSlot)
	}
	slotKey := slotItem["key"].(string)
	slotID := slotItem["id"].(string)
	if err := application.Redis.Incr(ctx, "tokenhub:conc:"+slotID).Err(); err != nil {
		t.Fatal(err)
	}
	blocked := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", slotKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "busy"}},
	})
	if blocked.status != http.StatusTooManyRequests {
		t.Fatalf("occupied concurrency slot should 429, got %d %+v", blocked.status, blocked.body)
	}
	if errObj, _ := blocked.body["error"].(map[string]any); errObj["code"] != "rate_limited" {
		t.Fatalf("expected rate_limited, got %+v", blocked.body)
	}
	if err := application.Redis.Decr(ctx, "tokenhub:conc:"+slotID).Err(); err != nil {
		t.Fatal(err)
	}
	free := postJSONRaw(t, server.URL+"/v1/chat/completions", slotKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "free"}},
	})
	if free["provider"] != catalog.PrimaryProvider {
		t.Fatalf("released slot should chat: %+v", free)
	}
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
	ctx := context.Background()
	if err := application.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)
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

func containsText(body map[string]any, want string) bool {
	raw, _ := json.Marshal(body)
	return strings.Contains(string(raw), want)
}

type statusBody struct {
	status int
	body   map[string]any
}

func mustStatusBody(t *testing.T, method, url, token string, payload map[string]any) statusBody {
	t.Helper()
	req, _ := http.NewRequest(method, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return statusBody{status: resp.StatusCode, body: out}
}
