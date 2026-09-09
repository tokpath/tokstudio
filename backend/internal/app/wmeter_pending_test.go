package app_test

import (
	"bytes"
	"context"
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

func TestWMeterPendingQueue(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "wmeter2_admin"
	cfg.BootstrapUser = "wmeter2_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "wmeter2-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "wmeter2"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}

	charged := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter2-charge"}},
	})
	chargedID := charged["request_id"].(string)
	firstReplay := postJSONRaw(t, server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": chargedID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	secondReplay := postJSONRaw(t, server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": chargedID, "usage": map[string]int{"prompt_tokens": 99, "completion_tokens": 99},
	})
	firstCharge := firstReplay["item"].(map[string]any)["charge_id"]
	if firstCharge == nil || firstCharge != secondReplay["item"].(map[string]any)["charge_id"] {
		t.Fatalf("sentinel double-charge FAIL: replay must be idempotent: %+v %+v", firstReplay, secondReplay)
	}

	beforeOmit := balanceMinor(t, server.URL, session)
	omit := omitChat(t, server.URL, apiKey, "wmeter2-omit")
	omitID := omit["request_id"].(string)
	afterOmit := balanceMinor(t, server.URL, session)
	if afterOmit >= beforeOmit {
		t.Fatalf("omit should keep a reservation held: before=%d after=%d", beforeOmit, afterOmit)
	}
	if _, err := application.Billing.ChargeByRequest(ctx, omitID); err != billing.ErrNotFound {
		t.Fatalf("missing usage must NEVER estimate-debit, charge=%v", err)
	}

	pending := getAuthJSON(t, server.URL+"/admin/usage/pending", "wmeter2_admin")
	if !containsRequest(pending, omitID) {
		t.Fatalf("pending queue missing omit request: %+v", pending)
	}
	detail := getAuthJSON(t, server.URL+"/admin/usage/pending/"+omitID, "wmeter2_admin")["item"].(map[string]any)
	if detail["state"] != billing.UsagePending || detail["missing_usage"] != true {
		t.Fatalf("gap detail: %+v", detail)
	}
	if minorOf(detail["customer_amount_minor"]) != 0 {
		t.Fatalf("pending gap must not carry an estimated bill: %+v", detail)
	}

	filtered := getAuthJSON(t, server.URL+"/admin/usage?state=pending_reconciliation&public_model_id="+catalog.EchoModelID, "wmeter2_admin")
	if !containsRequest(filtered, omitID) {
		t.Fatalf("statements filter missed pending usage: %+v", filtered)
	}

	if postStatus(t, server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"request_ids": []string{omitID},
	}) != http.StatusConflict {
		t.Fatal("mark resolved without seal confirm must be 409")
	}
	if _, err := application.Billing.ChargeByRequest(ctx, omitID); err != billing.ErrNotFound {
		t.Fatal("409 resolve must not create a charge")
	}

	resolved := postJSONRaw(t, server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"request_ids": []string{omitID},
	})
	item := resolved["item"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["state"] != billing.UsageVoided {
		t.Fatalf("resolve should void pending usage: %+v", item)
	}
	again := postJSONRaw(t, server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"ids": []string{item["id"].(string)},
	})
	againItem := again["item"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if againItem["id"] != item["id"] || againItem["state"] != billing.UsageVoided {
		t.Fatalf("resolve must be idempotent: %+v", again)
	}
	afterResolve := balanceMinor(t, server.URL, session)
	if afterResolve <= afterOmit {
		t.Fatalf("mark resolved must release hold, not debit: afterOmit=%d afterResolve=%d", afterOmit, afterResolve)
	}
	if _, err := application.Billing.ChargeByRequest(ctx, omitID); err != billing.ErrNotFound {
		t.Fatal("mark resolved must never create a customer charge")
	}

	replayOmit := omitChat(t, server.URL, apiKey, "wmeter2-replay")
	replayID := replayOmit["request_id"].(string)
	r1 := postJSONRaw(t, server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": replayID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	r2 := postJSONRaw(t, server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": replayID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	if r1["item"].(map[string]any)["charge_id"] != r2["item"].(map[string]any)["charge_id"] {
		t.Fatalf("same usage replay must be idempotent: %+v %+v", r1, r2)
	}
	if r1["item"].(map[string]any)["state"] != billing.UsageConfirmed {
		t.Fatalf("replay should confirm: %+v", r1)
	}

	if postStatus(t, server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"request_ids": []string{replayID},
	}) != http.StatusConflict {
		t.Fatal("already charged request cannot be mark-resolved")
	}
}

func omitChat(t *testing.T, base, key, content string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": content}},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Omit-Usage", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 || out["request_id"] == nil {
		t.Fatalf("omit chat %d %+v", resp.StatusCode, out)
	}
	return out
}

func balanceMinor(t *testing.T, base, session string) int64 {
	t.Helper()
	bal := getAuthJSON(t, base+"/v1/me/balance", session)["balance"].(map[string]any)
	return minorOf(bal["available_minor"])
}

func minorOf(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func containsRequest(body map[string]any, requestID string) bool {
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["request_id"] == requestID {
			return true
		}
	}
	return false
}
