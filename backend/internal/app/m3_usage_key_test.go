package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM3_UsageByAPIKey_SandboxModes(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m3_key_admin"
	cfg.BootstrapUser = "m3_key_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "keydim-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	alphaItem := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "alpha"})["item"].(map[string]any)
	betaItem := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "beta"})["item"].(map[string]any)
	alphaKey := alphaItem["key"].(string)
	betaKey := betaItem["key"].(string)
	alphaID := alphaItem["id"].(string)
	betaID := betaItem["id"].(string)

	longPrompt := strings.Repeat("tokenhub", 5) // 40 runes → prompt_tokens=40
	alphaChat := chatSandbox(t, server.URL, alphaKey, longPrompt, "content")
	betaChat := chatSandbox(t, server.URL, betaKey, "hi", "content")
	reasoningChat := chatSandbox(t, server.URL, alphaKey, "reason-me", "reasoning")
	if jsonInt(usageMap(alphaChat)["prompt_tokens"]) != 40 {
		t.Fatalf("content mode should bill 40 prompt tokens: %+v", alphaChat["usage"])
	}
	if jsonInt(usageMap(betaChat)["prompt_tokens"]) != 8 {
		t.Fatalf("short prompt should floor at 8: %+v", betaChat["usage"])
	}
	if jsonInt(usageMap(reasoningChat)["reasoning_tokens"]) != 3 {
		t.Fatalf("reasoning mode should add 3 reasoning tokens: %+v", reasoningChat["usage"])
	}

	all := getAuthJSON(t, server.URL+"/v1/me/usage?limit=50", session)
	items, _ := all["items"].([]any)
	if len(items) < 3 {
		t.Fatalf("expected at least 3 usage rows, got %+v", all)
	}
	sawAlpha, sawBeta := false, false
	for _, raw := range items {
		row := raw.(map[string]any)
		switch row["api_key_id"] {
		case alphaID:
			sawAlpha = true
			if jsonInt(row["prompt_tokens"]) < 8 {
				t.Fatalf("alpha row missing tokens: %+v", row)
			}
		case betaID:
			sawBeta = true
		}
	}
	if !sawAlpha || !sawBeta {
		t.Fatalf("usage must split by api_key_id: %+v", all)
	}

	onlyAlpha := getAuthJSON(t, server.URL+"/v1/me/usage?api_key_id="+alphaID, session)
	for _, raw := range onlyAlpha["items"].([]any) {
		if raw.(map[string]any)["api_key_id"] != alphaID {
			t.Fatalf("api_key_id filter leaked another key: %+v", raw)
		}
	}
	keys, _ := all["keys"].([]any)
	if len(keys) < 2 {
		t.Fatalf("keys summary should include both API keys: %+v", all["keys"])
	}

	adminUsage := getAuthJSON(t, server.URL+"/admin/usage?api_key_id="+betaID, "m3_key_admin")
	adminItems, _ := adminUsage["items"].([]any)
	if len(adminItems) == 0 {
		t.Fatalf("admin usage filter empty: %+v", adminUsage)
	}
	for _, raw := range adminItems {
		if raw.(map[string]any)["api_key_id"] != betaID {
			t.Fatalf("admin api_key_id filter leaked: %+v", raw)
		}
	}

	dash := getAuthJSON(t, server.URL+"/admin/ops/dashboard", "m3_key_admin")
	dims, _ := dash["dashboard"].(map[string]any)["dimensions"].(map[string]any)
	apiKeys, _ := dims["api_key"].([]any)
	found := map[string]bool{}
	for _, raw := range apiKeys {
		row, _ := raw.(map[string]any)
		if key, _ := row["key"].(string); key != "" {
			found[key] = true
		}
	}
	if !found[alphaID] || !found[betaID] {
		t.Fatalf("dashboard api_key dimension missing keys: %+v", apiKeys)
	}
	metrics := getAuthJSON(t, server.URL+"/admin/metrics?dimension=api_key", "m3_key_admin")
	if metrics["dimension"] != "api_key" {
		t.Fatalf("metrics dimension: %+v", metrics)
	}
}

func chatSandbox(t *testing.T, base, key, content, mode string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": content}},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	if mode != "" {
		req.Header.Set("X-Tokenhub-Sandbox-Mode", mode)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("sandbox chat %s %d %+v", mode, resp.StatusCode, out)
	}
	return out
}

func usageMap(chat map[string]any) map[string]any {
	m, _ := chat["usage"].(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func jsonInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int:
		return int64(n)
	case int64:
		return n
	default:
		return 0
	}
}
