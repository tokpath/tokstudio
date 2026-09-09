package app_test

import (
	"encoding/json"
	"io"
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

func TestWMeterPriceSnapshot(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "wmeter_admin"
	cfg.BootstrapUser = "wmeter_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "wmeter-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "wmeter"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter-snap"}},
	})
	if chat["request_id"] == nil {
		t.Fatalf("chat: %+v", chat)
	}
	first := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)[0].(map[string]any)
	oldAmount := first["customer_amount_minor"]
	oldPrices := first["unit_prices"]

	if code := postStatus(t, server.URL+"/admin/price-books", "wmeter_admin", map[string]any{
		"model": catalog.EchoModelID, "input": "0.03", "output": "0.04",
	}); code != http.StatusConflict {
		t.Fatalf("publish without confirm should be 409, got %d", code)
	}

	published := postJSONRaw(t, server.URL+"/admin/price-books", "wmeter_admin", map[string]any{
		"model": catalog.EchoModelID,
		"customer_sell":    map[string]any{"input": "0.000009", "output": "0.000011"},
		"wholesale":        map[string]any{"input": "0.000006", "output": "0.000008"},
		"upstream_cost":    map[string]any{"input": "0.000003", "output": "0.000004"},
		"channel_override": map[string]any{"input": "0.000010", "output": "0.000012"},
		"currency":         "USD",
	})
	price, _ := published["price"].(map[string]any)
	versionID, _ := price["version_id"].(string)
	if versionID == "" || price["public_id"] != catalog.EchoModelID {
		t.Fatalf("publish should return version: %+v", published)
	}
	raw, _ := price["unit_prices"].(map[string]any)
	if raw["upstream_cost_input"] == nil || raw["wholesale_input"] == nil || raw["customer_sell_input"] == nil || raw["channel_customer_input"] == nil {
		t.Fatalf("four prices missing on publish: %+v", raw)
	}

	books := getAuthJSON(t, server.URL+"/admin/price-books?q="+catalog.EchoModelID, "wmeter_admin")
	var live, prior map[string]any
	for _, item := range books["items"].([]any) {
		row, _ := item.(map[string]any)
		if row["id"] == versionID {
			live = row
		}
		if row["public_id"] == catalog.EchoModelID && row["status"] == "superseded" {
			prior = row
		}
	}
	if live == nil || live["status"] != "published" || live["effective_at"] == "" {
		t.Fatalf("published version missing effective_at: %+v", books)
	}
	if fmtString(live["upstream"]) == "" || fmtString(live["wholesale"]) == "" || fmtString(live["sell"]) == "" || fmtString(live["channel"]) == "" {
		t.Fatalf("list must expose four price columns: %+v", live)
	}
	if prior == nil || prior["effective_at"] == "" {
		t.Fatalf("superseded chain missing: %+v", books)
	}

	csvBody := getAuthText(t, server.URL+"/admin/price-books?format=csv&q="+catalog.EchoModelID, "wmeter_admin")
	header := strings.Split(strings.Split(csvBody, "\n")[0], ",")
	for _, col := range []string{"id", "public_id", "status", "effective_at", "upstream", "wholesale", "sell", "channel"} {
		if !containsStr(header, col) {
			t.Fatalf("csv missing %s: %s", col, csvBody)
		}
	}
	if !strings.Contains(csvBody, versionID) || !strings.Contains(csvBody, "published") || !strings.Contains(csvBody, "superseded") {
		t.Fatalf("csv missing version chain: %s", csvBody)
	}
	if !strings.Contains(csvBody, "0.000003/0.000004") || !strings.Contains(csvBody, "0.000006/0.000008") || !strings.Contains(csvBody, "0.000009/0.000011") {
		t.Fatalf("csv missing four-price breakdown: %s", csvBody)
	}

	again := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)[0].(map[string]any)
	if again["customer_amount_minor"] != oldAmount {
		t.Fatalf("price change rewrote old bill: %v -> %v", oldAmount, again["customer_amount_minor"])
	}
	if !jsonEqual(again["unit_prices"], oldPrices) {
		t.Fatalf("old usage snapshot rewritten: %v -> %v", oldPrices, again["unit_prices"])
	}
}

func getAuthText(t *testing.T, url, token string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("GET %s %d %s", url, resp.StatusCode, body)
	}
	return string(body)
}

func fmtString(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func containsStr(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func jsonEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
