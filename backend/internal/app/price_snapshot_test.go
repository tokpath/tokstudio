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
	oldID, _ := first["id"].(string)
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
	if live == nil || live["status"] != "published" || !hasEffectiveAt(live["effective_at"]) {
		t.Fatalf("published version missing effective_at: %+v", books)
	}
	if fmtString(live["upstream"]) == "" || fmtString(live["wholesale"]) == "" || fmtString(live["sell"]) == "" || fmtString(live["channel"]) == "" {
		t.Fatalf("list must expose four price columns: %+v", live)
	}
	if prior == nil || prior["status"] != "superseded" || !hasEffectiveAt(prior["effective_at"]) {
		t.Fatalf("superseded chain missing effective_at: %+v", books)
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

	againItems := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)
	frozen := usageByID(againItems, oldID)
	if frozen == nil || frozen["customer_amount_minor"] != oldAmount || !jsonEqual(frozen["unit_prices"], oldPrices) {
		t.Fatalf("price change rewrote old bill/snapshot: old=%v now=%+v", oldAmount, frozen)
	}

	freshChat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter-after-publish"}},
	})
	if freshChat["request_id"] == nil {
		t.Fatalf("new usage after publish: %+v", freshChat)
	}
	afterItems := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)
	stillFrozen := usageByID(afterItems, oldID)
	if stillFrozen == nil || stillFrozen["customer_amount_minor"] != oldAmount || !jsonEqual(stillFrozen["unit_prices"], oldPrices) {
		t.Fatalf("new usage mutated old snapshot: %+v", stillFrozen)
	}
	fresh := newestUsageExcept(afterItems, oldID)
	if fresh == nil {
		t.Fatalf("expected new usage after publish, got %+v", afterItems)
	}
	rawFresh, _ := json.Marshal(fresh["unit_prices"])
	if err := catalog.RequireFourPriceSnapshot(rawFresh, true); err != nil {
		t.Fatalf("new usage must persist four-price snapshot: %v raw=%s", err, rawFresh)
	}
	up, wholesale, sell, channel := catalog.FourPriceDims(rawFresh)
	if up != "0.000003/0.000004" || wholesale != "0.000006/0.000008" || sell != "0.000009/0.000011" || channel != "0.000010/0.000012" {
		t.Fatalf("new usage snapshot dims upstream=%q wholesale=%q sell=%q channel=%q", up, wholesale, sell, channel)
	}
	if fresh["customer_amount_minor"] == oldAmount {
		t.Fatalf("new usage should bill the new sell snapshot, still %v", oldAmount)
	}
	if fresh["price_version_id"] != versionID {
		t.Fatalf("new usage should pin published version %s, got %+v", versionID, fresh)
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

func hasEffectiveAt(v any) bool {
	s := fmtString(v)
	return s != "" && !strings.HasPrefix(s, "0001-01-01")
}

func usageByID(items []any, id string) map[string]any {
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["id"] == id {
			return item
		}
	}
	return nil
}

func newestUsageExcept(items []any, oldID string) map[string]any {
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["id"] != oldID {
			return item
		}
	}
	return nil
}
