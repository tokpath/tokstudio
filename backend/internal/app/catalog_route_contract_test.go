package app_test

import (
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

// Sentinel 契约：模型目录 / 路由组负例 + 单层 fallback，不打真实上游。
func TestCatalogRouteContractNegatives(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "cat_admin"
	cfg.BootstrapUser = "cat_user"
	cfg.BootstrapChannel = "cat_channel"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "cat-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	keyResp := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "contract"})
	apiKey := keyResp["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})

	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/v1/models", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("no API key on /v1/models must be 401, got %d", code)
	}
	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/admin/models", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("no token on /admin/models must be 401, got %d", code)
	}
	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/admin/routes", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("no token on /admin/routes must be 401, got %d", code)
	}
	if code := mustStatusJSON(t, http.MethodPost, server.URL+"/v1/chat/completions", "", map[string]string{"model": catalog.EchoModelID}); code != http.StatusUnauthorized {
		t.Fatalf("no API key on chat must be 401, got %d", code)
	}

	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/admin/models", session, nil); code != http.StatusForbidden {
		t.Fatalf("end user GET /admin/models must be 403, got %d", code)
	}
	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/admin/routes", "cat_admin-finance", nil); code != http.StatusForbidden {
		t.Fatalf("finance GET /admin/routes must be 403, got %d", code)
	}
	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/admin/routes", "cat_channel", nil); code != http.StatusForbidden {
		t.Fatalf("channel admin GET /admin/routes must be 403, got %d", code)
	}
	denied := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.OEMModelID, "messages": []map[string]string{{"role": "user", "content": "deny"}},
	})
	if denied.status != http.StatusForbidden {
		t.Fatalf("official tenant chatting OEM-only model must be 403, got %d %+v", denied.status, denied.body)
	}

	if code := mustStatusJSON(t, http.MethodGet, server.URL+"/admin/models/does-not-exist", "cat_admin", nil); code != http.StatusNotFound {
		t.Fatalf("unknown admin model must be 404, got %d", code)
	}
	unknownModel := mustStatusBody(t, http.MethodGet, server.URL+"/v1/models/does-not-exist", apiKey, nil)
	if unknownModel.status != http.StatusNotFound {
		t.Fatalf("unknown public model must be 404, got %d %+v", unknownModel.status, unknownModel.body)
	}
	unknownChat := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": "does-not-exist", "messages": []map[string]string{{"role": "user", "content": "x"}},
	})
	if unknownChat.status != http.StatusNotFound {
		t.Fatalf("chat unknown model must be 404, got %d %+v", unknownChat.status, unknownChat.body)
	}
	if code := mustStatusJSONConfirm(t, http.MethodPatch, server.URL+"/admin/routes/rg_does_not_exist", "cat_admin", map[string]string{"strategy": "priority"}); code != http.StatusNotFound {
		t.Fatalf("unknown route group must be 404, got %d", code)
	}

	if code := mustStatusJSONConfirm(t, http.MethodPost, server.URL+"/admin/models", "cat_admin", map[string]string{}); code != http.StatusBadRequest {
		t.Fatalf("empty model create must be 400, got %d", code)
	}
	if code := mustStatusJSONConfirm(t, http.MethodPost, server.URL+"/admin/routes", "cat_admin", map[string]string{}); code != http.StatusBadRequest {
		t.Fatalf("empty route create must be 400, got %d", code)
	}
	badChat := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "logit_bias": map[string]int{"1": 1}, "messages": []map[string]string{{"role": "user", "content": "x"}},
	})
	if badChat.status != http.StatusBadRequest {
		t.Fatalf("unsupported param must be 400, got %d %+v", badChat.status, badChat.body)
	}

	before := application.Gateway.AdapterCalls()
	ok := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "one-attempt"}},
	})
	if got := application.Gateway.AdapterCalls() - before; got != 1 {
		t.Fatalf("success must call one adapter (no dual-layer race), got %d", got)
	}
	requestID, _ := ok["request_id"].(string)
	if requestID == "" {
		t.Fatalf("success missing request_id: %+v", ok)
	}
	attempts := getAuthJSON(t, server.URL+"/v1/requests/"+requestID+"/attempts", apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("success must record one attempt, got %+v", attempts)
	}

	fb := forceFailChat(t, server.URL+"/v1/chat/completions", apiKey, catalog.PrimaryProvider)
	fbID, _ := fb.body["request_id"].(string)
	fbAttempts := getAuthJSON(t, server.URL+"/v1/requests/"+fbID+"/attempts", apiKey)
	fbItems, _ := fbAttempts["items"].([]any)
	if len(fbItems) < 2 {
		t.Fatalf("429 fallback must be sequential catalog candidates, got %+v", fbAttempts)
	}
	seen := map[float64]int{}
	for _, raw := range fbItems {
		row := raw.(map[string]any)
		n, _ := row["attempt_no"].(float64)
		seen[n]++
		if seen[n] > 1 {
			t.Fatalf("attempt_no %v raced twice (dual-layer): %+v", n, fbAttempts)
		}
	}
}

func forceFailChat(t *testing.T, url, token, provider string) statusBody {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Force-Fail", provider)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("force-fail chat %d %+v", resp.StatusCode, out)
	}
	return statusBody{status: resp.StatusCode, body: out}
}
