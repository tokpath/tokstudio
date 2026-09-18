package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM5PlansPayments(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m5_admin"
	cfg.BootstrapUser = "m5_user"
	cfg.BootstrapChannel = "m5_channel"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	cfg.PaymentSignKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()
	signKey := application.Payment.SignKey()

	health := getJSON(t, server.URL+"/healthz", "")
	if ver, _ := health["version"].(string); !strings.HasPrefix(ver, "0.1.0-m") {
		t.Fatalf("health version: %+v", health)
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "plan-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	userID := userIDOf(reg)

	publicPlans := getJSON(t, server.URL+"/v1/plans", "")
	if !hasPlan(publicPlans, "pln_echo_month") {
		t.Fatalf("public plans missing seed: %+v", publicPlans)
	}

	cheap := postJSONRaw(t, server.URL+"/channel/plans", "m5_channel", map[string]any{
		"name": "Too Cheap", "price_minor": 1000, "owner_type": "platform", "owner_id": identity.OfficialChannelID,
		"items": []map[string]any{{"unit_type": "usd_credit", "included_amount": 1}},
	})
	cheapItem := cheap["item"].(map[string]any)
	if cheapItem["status"] != plans.StatusPendingReview || cheapItem["owner_type"] != plans.OwnerChannel || cheapItem["owner_id"] != identity.ResellerChannelID {
		t.Fatalf("channel low price should enter review for reseller: %+v", cheap)
	}
	pending := getAuthJSON(t, server.URL+"/admin/plans?status=pending_review", "m5_admin")
	if !hasPlan(pending, cheapItem["id"].(string)) {
		t.Fatalf("admin review queue missing cheap plan: %+v", pending)
	}
	reviewed := postJSONRaw(t, server.URL+"/admin/plans/"+cheapItem["id"].(string)+"/review", "m5_admin", map[string]any{
		"action": "approve", "reason": "promo",
	})
	if reviewed["item"].(map[string]any)["status"] != plans.StatusPublished {
		t.Fatalf("review approve: %+v", reviewed)
	}

	if mustStatusJSON(t, http.MethodPost, server.URL+"/admin/entitlements/bonus", "m5_admin", map[string]string{
		"user_id": userID, "unit_type": plans.UnitUSDCredit,
	}) != http.StatusConflict {
		t.Fatal("bonus without confirm must be 409")
	}
	_, _ = grantBonusRequest(t, server.URL+"/admin/entitlements/bonus", "m5_admin", "m5-first-"+userID, map[string]any{
		"user_id": userID, "unit_type": plans.UnitUSDCredit, "amount": 2 * billing.MinorPerUSD, "expires_in_seconds": 3600,
	})
	_, _ = grantBonusRequest(t, server.URL+"/admin/entitlements/bonus", "m5_admin", "m5-second-"+userID, map[string]any{
		"user_id": userID, "unit_type": plans.UnitUSDCredit, "amount": 3 * billing.MinorPerUSD, "expires_in_seconds": 86400,
	})

	subResp := postJSONRaw(t, server.URL+"/v1/me/subscriptions", session, map[string]any{
		"plan_id": "pln_echo_month", "adapter": payment.AdapterStripe, "payment_method_ref": "pm_ok",
	})
	sub := subResp["subscription"].(map[string]any)
	checkout := subResp["checkout"].(map[string]any)
	order := checkout["order"].(map[string]any)
	orderID := order["id"].(string)
	pays := getAuthJSON(t, server.URL+"/admin/payments", "m5_admin")
	if !hasPlan(pays, orderID) {
		t.Fatalf("admin payments missing checkout order: %+v", pays)
	}
	eventID := "evt-m5-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	paid := webhook(t, server.URL, signKey, payment.AdapterStripe, eventID, orderID, "paid")
	if paid.StatusCode != http.StatusOK {
		t.Fatalf("webhook paid %d", paid.StatusCode)
	}
	replay := webhook(t, server.URL, signKey, payment.AdapterStripe, eventID, orderID, "paid")
	if replay.StatusCode != http.StatusOK {
		t.Fatalf("replay should be 2xx, got %d", replay.StatusCode)
	}
	ents := getAuthJSON(t, server.URL+"/v1/me/entitlements", session)["items"].([]any)
	if usdRemaining(ents) != 10*billing.MinorPerUSD {
		t.Fatalf("replay must not double-grant, remaining=%d items=%+v", usdRemaining(ents), ents)
	}

	bad := webhook(t, server.URL, "wrong-key", payment.AdapterStripe, "evt-bad-"+strconv.FormatInt(time.Now().UnixNano(), 10), orderID, "paid")
	if bad.StatusCode == http.StatusOK {
		t.Fatal("invalid signature must not be accepted")
	}

	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: userID, RequestID: "fifo-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		PublicModelID: catalog.EchoModelID, ReserveMinor: 2 * billing.MinorPerUSD,
		UnitPrices: []byte(`{"input":"0.000001","output":"0.000002"}`),
	}); err != nil {
		t.Fatal(err)
	}
	ents = getAuthJSON(t, server.URL+"/v1/me/entitlements", session)["items"].([]any)
	soonBonus := firstBonus(ents)
	if soonBonus == nil || asInt(soonBonus["consumed"]) != 2*billing.MinorPerUSD {
		t.Fatalf("FIFO should debit soonest bonus first: %+v", ents)
	}

	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m5"})["item"].(map[string]any)["key"].(string)
	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "hi-from-plan"}},
	})
	if chat["request_id"] == nil {
		t.Fatalf("chat with entitlements failed: %+v", chat)
	}

	alipay := postJSONRaw(t, server.URL+"/v1/me/subscriptions", session, map[string]any{
		"plan_id": "pln_echo_month", "adapter": payment.AdapterAlipay,
	})
	aliSub := alipay["subscription"].(map[string]any)
	if aliSub["renewal_policy"] != plans.RenewManual {
		t.Fatalf("alipay must be manual renew: %+v", aliSub)
	}
	aliOrder := alipay["checkout"].(map[string]any)["order"].(map[string]any)["id"].(string)
	aliEvt := "evt-ali-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if code := webhook(t, server.URL, signKey, payment.AdapterAlipay, aliEvt, aliOrder, "paid").StatusCode; code != http.StatusOK {
		t.Fatalf("alipay paid %d", code)
	}
	if err := application.Plans.ForcePeriodEnd(ctx, aliSub["id"].(string), time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Plans.ProcessRenewals(ctx, time.Now().UTC(), application.Payment.RenewCharger()); err != nil {
		t.Fatal(err)
	}
	aliAfter := mustGetSub(t, application, aliSub["id"].(string))
	if aliAfter.Status != plans.SubPastDue {
		t.Fatalf("alipay must not auto-renew, got %s", aliAfter.Status)
	}

	failSubResp := postJSONRaw(t, server.URL+"/v1/me/subscriptions", session, map[string]any{
		"plan_id": "pln_echo_month", "adapter": payment.AdapterStripe, "payment_method_ref": "pm_fail",
	})
	failSub := failSubResp["subscription"].(map[string]any)
	failOrder := failSubResp["checkout"].(map[string]any)["order"].(map[string]any)["id"].(string)
	if code := webhook(t, server.URL, signKey, payment.AdapterStripe, "evt-fail-"+strconv.FormatInt(time.Now().UnixNano(), 10), failOrder, "paid").StatusCode; code != http.StatusOK {
		t.Fatalf("stripe first period %d", code)
	}
	if err := application.Plans.ForcePeriodEnd(ctx, failSub["id"].(string), time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Plans.ProcessRenewals(ctx, time.Now().UTC(), application.Payment.RenewCharger()); err != nil {
		t.Fatal(err)
	}
	failAfter := mustGetSub(t, application, failSub["id"].(string))
	if failAfter.Status != plans.SubPastDue {
		t.Fatalf("stripe fail should be past_due, got %s", failAfter.Status)
	}

	beforeRefund := usdRemaining(getAuthJSON(t, server.URL+"/v1/me/entitlements", session)["items"].([]any))
	_ = postJSONRaw(t, server.URL+"/admin/payments/"+orderID+"/refund", "m5_admin", map[string]any{})
	afterRefund := getAuthJSON(t, server.URL+"/v1/me/entitlements", session)["items"].([]any)
	if unusedOfSource(afterRefund, sub["id"].(string)) != 0 {
		t.Fatalf("refund should reverse unused plan entitlements: %+v", afterRefund)
	}
	_ = beforeRefund

	walletOrder := postJSONRaw(t, server.URL+"/v1/payments/orders", session, map[string]any{
		"adapter": payment.AdapterWechat, "amount_minor": billing.MinorPerUSD, "purpose": payment.PurposeWallet,
	})
	wOrder := walletOrder["checkout"].(map[string]any)["order"].(map[string]any)
	beforeBal := asInt(getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"])
	if code := webhook(t, server.URL, signKey, payment.AdapterWechat, "evt-wal-"+strconv.FormatInt(time.Now().UnixNano(), 10), wOrder["id"].(string), "paid").StatusCode; code != http.StatusOK {
		t.Fatalf("wechat wallet paid %d", code)
	}
	afterBal := asInt(getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"])
	if afterBal-beforeBal != billing.MinorPerUSD {
		t.Fatalf("wallet credit mismatch %d -> %d", beforeBal, afterBal)
	}

}

func webhook(t *testing.T, base, key, adapter, eventID, orderID, status string) *http.Response {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"event_id": eventID, "order_id": orderID, "status": status, "trade_id": "tr_" + eventID})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/payments/"+adapter+"/webhook", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Payment-Signature", payment.SignWebhook(key, eventID, orderID, status))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	// 复制状态码，调用方只关心 StatusCode。
	clone := *resp
	return &clone
}

func hasPlan(body map[string]any, id string) bool {
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["id"] == id {
			return true
		}
	}
	return false
}

func usdRemaining(items []any) int64 {
	var sum int64
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["unit_type"] == plans.UnitUSDCredit && item["status"] == plans.EntActive {
			sum += asInt(item["remaining"])
		}
	}
	return sum
}

func firstBonus(items []any) map[string]any {
	var best map[string]any
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["source_type"] != plans.SourceBonus || item["unit_type"] != plans.UnitUSDCredit {
			continue
		}
		if best == nil {
			best = item
			continue
		}
		if asString(item["expires_at"]) < asString(best["expires_at"]) {
			best = item
		}
	}
	return best
}

func unusedOfSource(items []any, sourceID string) int64 {
	var sum int64
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["source_id"] == sourceID && item["status"] == plans.EntActive {
			sum += asInt(item["remaining"])
		}
	}
	return sum
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func mustGetSub(t *testing.T, application *app.App, id string) *plans.SubscriptionView {
	t.Helper()
	view, err := application.Plans.GetSubscription(context.Background(), id, "")
	if err != nil {
		t.Fatal(err)
	}
	return view
}
