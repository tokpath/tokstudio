package portal

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestExternalFailedRefundWebhookFinishesAndUnverifiedOneIsRecoverable(t *testing.T) {
	lab := os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if lab == "" {
		t.Skip("requires independent integration-lab")
	}
	f := setupPortal(t)
	s := seedOriginalRefund(t, f, lab)
	base := "/v1/operators/" + s.operator + "/refunds"
	var merchant string
	if err := f.a.DB.Raw("SELECT merchant_id FROM payments_intents WHERE order_id=?", s.order).Row().Scan(&merchant); err != nil {
		t.Fatal(err)
	}
	originalRefundControl(t, lab, "/__control/faults", map[string]any{"path": "/v1/refunds", "mode": "timeout_after", "count": 1})
	status, out, _ := f.call(t, "POST", base+"/"+s.leg+"/execute_api", map[string]any{}, "")
	mustOK(t, status, out)
	initial, err := f.a.Payments.QueryRefund(t.Context(), s.operator, merchant, s.leg)
	if err != nil || initial.ProviderID == "" || initial.State != "pending" {
		t.Fatalf("initial refund: %s %v", initial.State, err)
	}
	event := originalRefundControl(t, lab, "/__control/refunds/"+initial.ProviderID+"/settle", map[string]any{"status": "failed"})
	eventID := event["event_id"].(string)
	callback := f.server.URL
	if host := os.Getenv("TOKENHUB_MOCK_CALLBACK_HOST"); host != "" {
		u, err := url.Parse(callback)
		if err != nil {
			t.Fatal(err)
		}
		callback = "http://" + net.JoinHostPort(host, u.Port())
	}
	originalRefundControl(t, lab, "/__control/webhooks", map[string]any{"url": callback + "/v1/payments/webhooks/" + merchant, "event_id": eventID})
	// First record the provider failure through the normal staff query.
	status, out, _ = f.call(t, "POST", base+"/"+s.leg+"/query_api", map[string]any{}, "")
	mustOK(t, status, out)
	if attempts, err := f.a.Payments.RunPending(t.Context(), 10); err != nil || len(attempts) != 1 || attempts[0].EventID != eventID || attempts[0].State != "processed" {
		t.Fatalf("terminal callback: %+v %v", attempts, err)
	}
	if err = f.a.DB.Exec(`UPDATE payments_webhook_inbox SET state='pending',attempts=11,next_attempt_at=now(),
	hint=jsonb_set(hint,'{provider_id}',to_jsonb('different fixture reference'::text)) WHERE merchant_id=? AND event_id=?`, merchant, eventID).Error; err != nil {
		t.Fatal(err)
	}
	if attempts, err := f.a.Payments.RunPending(t.Context(), 10); err != nil || len(attempts) != 1 || attempts[0].State != "dead" {
		t.Fatalf("failed callback should exhaust: %+v %v", attempts, err)
	}
	jobsPath := "/v1/operators/" + s.operator + "/jobs"
	status, out, _ = f.call(t, "GET", jobsPath, nil, "")
	mustOK(t, status, out)
	rows := out["items"].([]any)
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	jobID := rows[0].(map[string]any)["id"].(string)
	status, out, _ = f.call(t, "POST", jobsPath+"/"+jobID+"/redrive", map[string]any{}, "")
	mustOK(t, status, out)
	if err = f.a.DB.Exec(`UPDATE payments_webhook_inbox SET hint=jsonb_set(hint,'{provider_id}',to_jsonb(?::text)) WHERE merchant_id=? AND event_id=?`, initial.ProviderID, merchant, eventID).Error; err != nil {
		t.Fatal(err)
	}
	if attempts, err := f.a.Payments.RunPending(t.Context(), 10); err != nil || len(attempts) != 1 || attempts[0].State != "processed" {
		t.Fatalf("recovered original callback: %+v %v", attempts, err)
	}
}

type originalRefundFixture struct {
	operator, order, refundCase, leg string
}

const originalRefundCustomer = "original-refund@example.test"

func seedOriginalRefund(t *testing.T, f *testApp, lab string) originalRefundFixture {
	t.Helper()
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	if e := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true,settlement=true WHERE operator_id=?", op).Error; e != nil {
		t.Fatal(e)
	}
	f.openRegistration(t)
	f.registered(t, originalRefundCustomer)
	f.login(t, "customer", originalRefundCustomer, "customer-password-test")
	status, identity, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, identity)
	customer := identity["customer"].(map[string]any)["id"].(string)
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "stripe", Enabled: true, APIBaseURL: lab, Credentials: map[string]string{"currency": "USD", "secret_key": "sk_test_" + uuid.NewString(), "publishable_key": "pk_test_synthetic", "webhook_secret": "whsec_integration_test"}}
	if _, e := f.a.Payments.SaveMerchant(ctx, owner, m, 0); e != nil {
		t.Fatal(e)
	}
	plan, e := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{OperatorID: op, ActorStaffID: owner, Name: "Original-route refund plan", PriceMoney: ledger.FromInt64(10), PaidATG: ledger.FromInt64(10), Period: ledger.Lifetime, Timezone: "UTC", NewPurchaseEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	status, quote, _ := f.call(t, "POST", "/v1/me/orders/quotes", map[string]any{"plan_id": plan.PackageID}, "")
	mustOK(t, status, quote)
	status, order, _ := f.call(t, "POST", "/v1/me/orders", map[string]any{"quote_id": quote["id"], "terms_version": quote["terms_version"], "funding": map[string]string{"money_balance": "0", "commission_money": "0", "external_method_id": m.ID}}, "")
	mustOK(t, status, order)
	orderID := order["id"].(string)
	intent, e := f.a.Payments.PaymentIntent(ctx, op, m.ID, orderID)
	if e != nil || intent.ProviderID == "" {
		t.Fatal(intent.State, e)
	}
	originalRefundControl(t, lab, "/__control/payments/"+intent.ProviderID+"/settle", map[string]any{"status": "succeeded", "amount_received": 1000, "paid_at": time.Now().Unix()})
	if _, e = f.a.Payments.Reconcile(ctx, op, m.ID, orderID); e != nil {
		t.Fatal(e)
	}
	links, e := f.a.Payments.ReceiptsForOrder(ctx, op, m.ID, orderID)
	if e != nil || len(links) != 1 {
		t.Fatal(links, e)
	}
	if _, e = f.a.Commerce.AllocateReceipt(ctx, nil, commerce.AllocateReceiptInput{OperatorID: op, ActorStaffID: owner, BusinessKey: uuid.NewString(), ReceiptID: links[0].ReceiptID, OrderID: orderID, AmountMoney: ledger.FromInt64(10)}); e != nil {
		t.Fatal(e)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/refunds"
	status, requested, _ := f.call(t, "POST", base, map[string]any{"action": "package_exception", "customer_id": customer, "order_id": orderID, "reason": "unable_to_fulfill"}, "")
	mustOK(t, status, requested)
	caseID := requested["id"].(string)
	status, reviewed, _ := f.call(t, "POST", base+"/"+caseID+"/review", map[string]string{"decision": "approve"}, "")
	mustOK(t, status, reviewed)
	legs, e := f.a.Remediation.Legs(ctx, nil, op, caseID)
	if e != nil || len(legs) != 1 || legs[0].AmountMoney.String() != "10" {
		t.Fatal(legs, e)
	}
	return originalRefundFixture{operator: op, order: orderID, refundCase: caseID, leg: legs[0].ID}
}

func originalRefundControl(t *testing.T, lab, path string, data map[string]any) map[string]any {
	t.Helper()
	b, e := json.Marshal(data)
	if e != nil {
		t.Fatal(e)
	}
	req, e := http.NewRequestWithContext(t.Context(), "POST", lab+path, bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 10 * time.Second}
	res, e := client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("simulator %s returned %d", path, res.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExternalOriginalRouteRefundAppearsOnSourceOrder(t *testing.T) {
	lab := os.Getenv("TOKENHUB_PAYMENT_MOCK_URL")
	if lab == "" {
		t.Skip("requires separately running integration-lab")
	}
	f := setupPortal(t)
	s := seedOriginalRefund(t, f, lab)
	base := "/v1/operators/" + s.operator + "/refunds"
	status, page, _ := f.call(t, "GET", base+"?record="+s.refundCase, nil, "")
	mustOK(t, status, page)
	actions := page["items"].([]any)[1].(map[string]any)["allowed_actions"].([]any)
	if !slices.Equal(actions, []any{"execute_api"}) {
		t.Fatal(actions)
	}
	status, manual, _ := f.call(t, "POST", base+"/"+s.leg+"/verify_manual", map[string]any{"currency": "USD", "occurred_at": time.Now(), "fee_money": "0"}, "")
	if status != http.StatusConflict {
		t.Fatal("online receipt accepted manual proof", status, manual)
	}
	status, executed, _ := f.call(t, "POST", base+"/"+s.leg+"/execute_api", map[string]string{}, "")
	mustOK(t, status, executed)
	status, queried, _ := f.call(t, "POST", base+"/"+s.leg+"/query_api", map[string]string{}, "")
	mustOK(t, status, queried)
	f.login(t, "customer", originalRefundCustomer, "customer-password-test")
	status, order, _ := f.call(t, "GET", "/v1/me/orders/"+s.order, nil, "")
	mustOK(t, status, order)
	refunds := order["refunds"].([]any)
	if len(refunds) != 1 || refunds[0].(map[string]any)["id"] != s.refundCase || refunds[0].(map[string]any)["status"] != "completed" || refunds[0].(map[string]any)["amount_money"] != "10" {
		t.Fatal(refunds)
	}
	resp, e := f.client.Get(f.server.URL + "/v1/me/refunds/" + s.refundCase + "/voucher")
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(resp.Body)
	if e != nil || resp.StatusCode != 200 || !strings.Contains(string(raw), s.order) || !strings.Contains(string(raw), ",10,USD,") {
		t.Fatalf("completed refund voucher: %d %v %s", resp.StatusCode, e, raw)
	}
}
