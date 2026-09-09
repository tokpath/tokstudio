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

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

type wmeterEnv struct {
	t       *testing.T
	app     *app.App
	server  *httptest.Server
	ctx     context.Context
	session string
	apiKey  string
	userID  string
}

func newWMeterEnv(t *testing.T) *wmeterEnv {
	t.Helper()
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
	t.Cleanup(server.Close)

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "wmeter2-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "wmeter2"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	return &wmeterEnv{
		t: t, app: application, server: server, ctx: context.Background(),
		session: session, apiKey: apiKey, userID: userIDOf(reg),
	}
}

// TestWMeterSentinelDoubleCharge：同一 request 再结算/回放不得写出第二条客户扣费。
func TestWMeterSentinelDoubleCharge(t *testing.T) {
	fx := newWMeterEnv(t)
	charged := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter2-charge"}},
	})
	requestID := charged["request_id"].(string)
	first := requireChargeCount(t, fx.app.Billing, requestID, 1, "sentinel double-charge FAIL: live settle")
	beforeReplay := balanceMinor(t, fx.server.URL, fx.session)
	beforeDebits := usageDebitCount(t, fx.app.Billing, fx.userID, requestID)

	firstReplay := postJSONRaw(t, fx.server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	secondReplay := postJSONRaw(t, fx.server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 99, "completion_tokens": 99},
	})
	replayCharge := firstReplay["item"].(map[string]any)["charge_id"]
	if replayCharge == nil || replayCharge != first[0].ChargeID {
		t.Fatalf("sentinel double-charge FAIL: replay must reuse the live charge: live=%s replay=%v", first[0].ChargeID, replayCharge)
	}
	if replayCharge != secondReplay["item"].(map[string]any)["charge_id"] {
		t.Fatalf("sentinel double-charge FAIL: second replay minted another charge: %+v %+v", firstReplay, secondReplay)
	}
	after := requireChargeCount(t, fx.app.Billing, requestID, 1, "sentinel double-charge FAIL")
	if after[0].ChargeID != first[0].ChargeID || after[0].AmountMinor != first[0].AmountMinor {
		t.Fatalf("sentinel double-charge FAIL: charge mutated: before=%+v after=%+v", first[0], after[0])
	}
	if balanceMinor(t, fx.server.URL, fx.session) != beforeReplay {
		t.Fatalf("sentinel double-charge FAIL: wallet moved on replay")
	}
	if usageDebitCount(t, fx.app.Billing, fx.userID, requestID) != beforeDebits {
		t.Fatalf("sentinel double-charge FAIL: extra usage_debit on replay")
	}
}

// TestWMeterSentinelReplayIdempotent：同一 usage 回放必须幂等（同 charge、同金额、余额不动）。
func TestWMeterSentinelReplayIdempotent(t *testing.T) {
	fx := newWMeterEnv(t)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter2-replay")
	requestID := omit["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 0, "omit must not estimate-debit before replay")

	r1 := postJSONRaw(t, fx.server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	afterFirst := balanceMinor(t, fx.server.URL, fx.session)
	charge1 := r1["item"].(map[string]any)
	if charge1["state"] != billing.UsageConfirmed || charge1["charge_id"] == nil {
		t.Fatalf("replay should confirm with a charge: %+v", r1)
	}
	firstCharges := requireChargeCount(t, fx.app.Billing, requestID, 1, "same usage replay must write exactly one charge")

	r2 := postJSONRaw(t, fx.server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	if charge1["charge_id"] != r2["item"].(map[string]any)["charge_id"] {
		t.Fatalf("same usage replay must be idempotent: %+v %+v", r1, r2)
	}
	secondCharges := requireChargeCount(t, fx.app.Billing, requestID, 1, "same usage replay must stay one charge")
	if secondCharges[0].ChargeID != firstCharges[0].ChargeID || secondCharges[0].AmountMinor != firstCharges[0].AmountMinor {
		t.Fatalf("same usage replay mutated the bill: first=%+v second=%+v", firstCharges[0], secondCharges[0])
	}
	if balanceMinor(t, fx.server.URL, fx.session) != afterFirst {
		t.Fatalf("same usage replay must not move the wallet again")
	}
	if n := usageDebitCount(t, fx.app.Billing, fx.userID, requestID); n > 1 {
		t.Fatalf("same usage replay wrote %d usage_debit rows", n)
	}
}

// TestWMeterSentinelMissingUsageNoEstimate：缺 usage 只进 pending_reconciliation，永不估算扣款。
func TestWMeterSentinelMissingUsageNoEstimate(t *testing.T) {
	fx := newWMeterEnv(t)
	beforeOmit := balanceMinor(t, fx.server.URL, fx.session)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter2-omit")
	requestID := omit["request_id"].(string)
	afterOmit := balanceMinor(t, fx.server.URL, fx.session)
	if afterOmit >= beforeOmit {
		t.Fatalf("omit should keep a reservation held: before=%d after=%d", beforeOmit, afterOmit)
	}
	requireChargeCount(t, fx.app.Billing, requestID, 0, "missing usage must NEVER estimate-debit")
	if _, err := fx.app.Billing.ChargeByRequest(fx.ctx, requestID); err != billing.ErrNotFound {
		t.Fatalf("missing usage must NEVER estimate-debit, charge=%v", err)
	}
	if usageDebitCount(t, fx.app.Billing, fx.userID, requestID) != 0 {
		t.Fatal("missing usage must not write usage_debit")
	}

	gaps, err := fx.app.Billing.ListPendingReconciliation(fx.ctx, billing.QueryUsageInput{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, gap := range gaps {
		if gap.RequestID != requestID {
			continue
		}
		found = true
		if gap.State != billing.UsagePending || !gap.MissingUsage {
			t.Fatalf("missing usage must be pending_reconciliation only: %+v", gap)
		}
		if gap.CustomerMinor != 0 || gap.SettledMinor != 0 {
			t.Fatalf("pending gap must not carry an estimated bill: %+v", gap)
		}
	}
	if !found {
		t.Fatalf("missing usage must enter pending_reconciliation: %+v", gaps)
	}

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{
		UserID: fx.userID, State: billing.UsagePending, PublicModelID: catalog.EchoModelID, Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for _, row := range usages {
		if row.RequestID == requestID {
			saw = true
			if row.CustomerMinor != 0 {
				t.Fatalf("statements must not show an estimated debit: %+v", row)
			}
		}
	}
	if !saw {
		t.Fatalf("TokenHub QueryUsage missed pending usage: %+v", usages)
	}
}

func TestWMeterPendingQueue(t *testing.T) {
	fx := newWMeterEnv(t)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter2-queue")
	omitID := omit["request_id"].(string)
	afterOmit := balanceMinor(t, fx.server.URL, fx.session)

	pending := getAuthJSON(t, fx.server.URL+"/admin/usage/pending", "wmeter2_admin")
	if !containsRequest(pending, omitID) {
		t.Fatalf("pending queue missing omit request: %+v", pending)
	}
	detail := getAuthJSON(t, fx.server.URL+"/admin/usage/pending/"+omitID, "wmeter2_admin")["item"].(map[string]any)
	if detail["state"] != billing.UsagePending || detail["missing_usage"] != true {
		t.Fatalf("gap detail: %+v", detail)
	}
	if minorOf(detail["customer_amount_minor"]) != 0 {
		t.Fatalf("pending gap must not carry an estimated bill: %+v", detail)
	}

	filtered := getAuthJSON(t, fx.server.URL+"/admin/usage?state=pending_reconciliation&public_model_id="+catalog.EchoModelID, "wmeter2_admin")
	if !containsRequest(filtered, omitID) {
		t.Fatalf("statements filter missed pending usage: %+v", filtered)
	}

	if postStatus(t, fx.server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"request_ids": []string{omitID},
	}) != http.StatusConflict {
		t.Fatal("mark resolved without seal confirm must be 409")
	}
	requireChargeCount(t, fx.app.Billing, omitID, 0, "409 resolve must not create a charge")

	resolved := postJSONRaw(t, fx.server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"request_ids": []string{omitID},
	})
	item := resolved["item"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["state"] != billing.UsageVoided {
		t.Fatalf("resolve should void pending usage: %+v", item)
	}
	again := postJSONRaw(t, fx.server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
		"ids": []string{item["id"].(string)},
	})
	againItem := again["item"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if againItem["id"] != item["id"] || againItem["state"] != billing.UsageVoided {
		t.Fatalf("resolve must be idempotent: %+v", again)
	}
	afterResolve := balanceMinor(t, fx.server.URL, fx.session)
	if afterResolve <= afterOmit {
		t.Fatalf("mark resolved must release hold, not debit: afterOmit=%d afterResolve=%d", afterOmit, afterResolve)
	}
	requireChargeCount(t, fx.app.Billing, omitID, 0, "mark resolved must never create a customer charge")

	replayOmit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter2-replay-queue")
	replayID := replayOmit["request_id"].(string)
	r1 := postJSONRaw(t, fx.server.URL+"/admin/usage/replay", "wmeter2_admin", map[string]any{
		"request_id": replayID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	if r1["item"].(map[string]any)["state"] != billing.UsageConfirmed {
		t.Fatalf("replay should confirm: %+v", r1)
	}
	requireChargeCount(t, fx.app.Billing, replayID, 1, "replay of missing usage writes exactly one charge")
	if postStatus(t, fx.server.URL+"/admin/usage/pending/resolve", "wmeter2_admin", map[string]any{
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

func requireChargeCount(t *testing.T, svc *billing.Service, requestID string, want int, msg string) []billing.Settlement {
	t.Helper()
	rows, err := svc.ListChargesByRequest(context.Background(), requestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != want {
		t.Fatalf("%s: want %d customer charges, got %d %+v", msg, want, len(rows), rows)
	}
	committed := 0
	for _, row := range rows {
		if row.State == billing.ChargeCommitted {
			committed++
		}
	}
	if committed > 1 {
		t.Fatalf("sentinel double-charge FAIL: %d committed charges for %s: %+v", committed, requestID, rows)
	}
	return rows
}

func usageDebitCount(t *testing.T, svc *billing.Service, userID, requestID string) int {
	t.Helper()
	entries, err := svc.ListLedger(context.Background(), userID, 100)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, entry := range entries {
		if entry.EventType == billing.EventUsageDebit && entry.ReferenceID == requestID {
			n++
		}
	}
	return n
}
