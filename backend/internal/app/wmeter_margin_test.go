package app_test

import (
	"context"
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

func newWMeter4Env(t *testing.T) *wmeterEnv {
	t.Helper()
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "wmeter4_admin"
	cfg.BootstrapUser = "wmeter4_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	t.Cleanup(server.Close)

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "wmeter4-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "wmeter4"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	return &wmeterEnv{
		t: t, app: application, server: server, ctx: context.Background(),
		session: session, apiKey: apiKey, userID: userIDOf(reg),
	}
}

// TestWMeter4CostFromTokenHubOnly：成本只认 TokenHub/attempt 事实，禁止估算，禁止双写。
func TestWMeter4CostFromTokenHubOnly(t *testing.T) {
	fx := newWMeter4Env(t)
	charged := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter4-cost"}},
	})
	requestID := charged["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 1, "live settle must keep one customer charge")

	attempts := getAuthJSON(t, fx.server.URL+"/v1/requests/"+requestID+"/attempts", fx.apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) < 1 {
		t.Fatalf("need attempt facts: %+v", attempts)
	}
	attemptID, _ := items[0].(map[string]any)["id"].(string)
	if attemptID == "" {
		t.Fatalf("attempt id missing: %+v", attempts)
	}
	costs, err := fx.app.Billing.ListCostFactsByAttempt(fx.ctx, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if len(costs) != 1 {
		t.Fatalf("sentinel invented-cost FAIL: want 1 TokenHub cost_entry, got %d", len(costs))
	}
	tokenHubCost := costs[0].AmountMinor

	margin := adminMarginOf(t, fx, requestID)
	row := findMarginRow(margin, attemptID)
	if row == nil {
		t.Fatalf("attempt row missing: %+v", margin)
	}
	if row["cost_source"] != billing.CostSourceTokenHub {
		t.Fatalf("cost source must be TokenHub: %+v", row)
	}
	if asInt(row["cost_minor"]) != tokenHubCost {
		t.Fatalf("row cost must follow attempt fact: %+v", row)
	}

	if postStatusConfirm(t, fx.server.URL+"/admin/supplier-entries", "wmeter4_admin", map[string]any{
		"amount_minor": 999, "source_type": "estimate", "idempotency_key": "spe-est-" + t.Name(),
	}) != http.StatusBadRequest {
		t.Fatal("invented supplier/attempt cost must FAIL")
	}
	if postStatusConfirm(t, fx.server.URL+"/admin/supplier-entries", "wmeter4_admin", map[string]any{
		"amount_minor": 999, "source_type": "attempt_cost", "idempotency_key": "spe-atm-" + t.Name(),
	}) != http.StatusBadRequest {
		t.Fatal("attempt_cost supplier source must FAIL — that invents a second cost fact")
	}

	postJSONRaw(t, fx.server.URL+"/admin/supplier-entries", "wmeter4_admin", map[string]any{
		"amount_minor":    billing.MinorPerUSD,
		"source_type":     "provider_invoice",
		"idempotency_key": "spe-cash-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	})
	afterCash := adminMarginOf(t, fx, requestID)
	afterRow := findMarginRow(afterCash, attemptID)
	if afterRow == nil {
		t.Fatalf("attempt row missing after supplier cash: %+v", afterCash)
	}
	if asInt(afterRow["cost_minor"]) != tokenHubCost || asInt(afterCash["attempt_cost_minor"]) != tokenHubCost {
		t.Fatalf("supplier cash must not change attempt cost: before=%d after=%v row=%v", tokenHubCost, afterCash, afterRow)
	}

	_ = postJSONRaw(t, fx.server.URL+"/admin/usage/replay", "wmeter4_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	again, err := fx.app.Billing.ListCostFactsByRequest(fx.ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 {
		t.Fatalf("sentinel double-write FAIL: replay minted another cost fact: %+v", again)
	}
	if again[0].AmountMinor != tokenHubCost {
		t.Fatalf("replay must not invent a new cost: %+v", again)
	}
}

// TestWMeter4MissingCostPendingNoEstimate：缺 attempt 成本只进 pending，禁止手填/估算。
func TestWMeter4MissingCostPendingNoEstimate(t *testing.T) {
	fx := newWMeter4Env(t)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter4-omit")
	requestID := omit["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 0, "missing usage must never estimate-debit")

	costs, err := fx.app.Billing.ListCostFactsByRequest(fx.ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(costs) != 0 {
		t.Fatalf("missing usage must not invent a cost_entry: %+v", costs)
	}

	margin := adminMarginOf(t, fx, requestID)
	if findMarginRowByRequest(margin, requestID) != nil {
		t.Fatalf("missing attempt cost must not appear as a filled cost row: %+v", margin)
	}
	if asInt(margin["pending_count"]) < 1 {
		t.Fatalf("missing cost must go pending: %+v", margin)
	}
	if asInt(margin["attempt_cost_minor"]) != 0 {
		t.Fatalf("missing cost must not invent a window total: %+v", margin)
	}

	if postStatusConfirm(t, fx.server.URL+"/admin/margin/corrections", "wmeter4_admin", map[string]any{
		"kind": "fill_cost", "request_id": requestID, "amount_minor": 12345,
		"idempotency_key": "mcr-amt-" + t.Name(),
	}) != http.StatusBadRequest {
		t.Fatal("fill_cost with invented amount must FAIL")
	}
	ticket := postJSONRaw(t, fx.server.URL+"/admin/margin/corrections", "wmeter4_admin", map[string]any{
		"kind": "fill_cost", "request_id": requestID, "reason": "ops ticket",
		"idempotency_key": "mcr-ok-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	})
	if ticket["item"].(map[string]any)["kind"] != billing.CorrectionFillCost {
		t.Fatalf("correction ticket: %+v", ticket)
	}
	after, err := fx.app.Billing.ListCostFactsByRequest(fx.ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("correction ticket must NOT invent a cost_entry: %+v", after)
	}
	requireChargeCount(t, fx.app.Billing, requestID, 0, "correction ticket must never estimate-debit")
	pending := getAuthJSON(t, fx.server.URL+"/admin/usage/pending", "wmeter4_admin")
	if !containsRequest(pending, requestID) {
		t.Fatalf("missing cost must stay in pending: %+v", pending)
	}
}

// TestWMeter4MarginSellMinusCost：毛利 = 售 − 成本，与报表 attempt 事实一致。
func TestWMeter4MarginSellMinusCost(t *testing.T) {
	fx := newWMeter4Env(t)
	charged := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter4-margin"}},
	})
	requestID := charged["request_id"].(string)
	margin := adminMarginOf(t, fx, requestID)
	row := findMarginRowByRequest(margin, requestID)
	if row == nil {
		t.Fatalf("assembled row missing: %+v", margin)
	}
	sell := asInt(row["sell_minor"])
	cost := asInt(row["cost_minor"])
	got := asInt(row["margin_minor"])
	if got != sell-cost {
		t.Fatalf("margin must be sell − cost: sell=%d cost=%d got=%d", sell, cost, got)
	}
	if asInt(margin["margin_minor"]) != asInt(margin["sell_minor"])-asInt(margin["attempt_cost_minor"]) {
		t.Fatalf("window totals must be sell − cost: %+v", margin)
	}
	facts, err := fx.app.Billing.ListCostFactsByRequest(fx.ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].AmountMinor != cost || asInt(margin["attempt_cost_minor"]) != cost {
		t.Fatalf("window cost must follow TokenHub attempt fact: facts=%+v row=%v margin=%v", facts, row, margin)
	}
}

// TestWMeter4CorrectionRequiresSeal：补成本 / 调毛利缺盖章必须 409。
func TestWMeter4CorrectionRequiresSeal(t *testing.T) {
	fx := newWMeter4Env(t)
	charged := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter4-seal"}},
	})
	requestID := charged["request_id"].(string)
	if postStatus(t, fx.server.URL+"/admin/margin/corrections", "wmeter4_admin", map[string]any{
		"kind": "fill_cost", "request_id": requestID, "idempotency_key": "mcr-noseal-fill",
	}) != http.StatusConflict {
		t.Fatal("fill_cost without seal must be 409")
	}
	if postStatus(t, fx.server.URL+"/admin/margin/corrections", "wmeter4_admin", map[string]any{
		"kind": "adjust_margin", "request_id": requestID, "idempotency_key": "mcr-noseal-adj",
	}) != http.StatusConflict {
		t.Fatal("adjust_margin without seal must be 409")
	}
	costs, err := fx.app.Billing.ListCostFactsByRequest(fx.ctx, requestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(costs) != 1 {
		t.Fatalf("409 must not invent or double-write cost: %+v", costs)
	}
}

func adminMarginOf(t *testing.T, fx *wmeterEnv, requestID string) map[string]any {
	t.Helper()
	body := getAuthJSON(t, fx.server.URL+"/admin/margin?request_id="+requestID, "wmeter4_admin")
	item, _ := body["item"].(map[string]any)
	if item == nil {
		t.Fatalf("margin item missing: %+v", body)
	}
	return item
}

func findMarginRow(body map[string]any, attemptID string) map[string]any {
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["attempt_id"] == attemptID {
			return item
		}
	}
	return nil
}

func findMarginRowByRequest(body map[string]any, requestID string) map[string]any {
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["request_id"] == requestID {
			return item
		}
	}
	return nil
}
