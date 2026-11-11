package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

// Uses the real quote, merchant adapter, command and PostgreSQL paths. All
// merchant credentials and HTTP responses belong to this isolated fixture.
func setupCheckoutApp(t *testing.T, scenario string) (*testApp, string, string, *atomic.Int32) {
	t.Helper()
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	f.registered(t, "checkout@example.test")
	if err := f.a.DB.Exec("UPDATE payments_fx_settings SET enabled=false").Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	calls := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/kv/merchant/v1/webhooks":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"webhook":{"id":7}}}`))
		case "/api/exchange-rates":
			if scenario == "quote-failure" {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"base":"MYR","rates":{"USD":0.245}}`))
		case "/api/kv/merchant/v1/orders":
			calls.Add(1)
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":51,"status":"pending_payment"}}}`))
		case "/api/kv/merchant/v1/orders/51/gkash-pay":
			amount := "8.40"
			if scenario == "success" {
				amount = "8.33"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"payment": map[string]any{"action": "https://pay.example.test/form", "method": "POST", "fields": map[string]string{"v_amount": amount, "v_currency": "MYR", "Signature": "synthetic-private-signature"}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	f.a.Payments.Drivers["kvoucher"] = payments.KVoucher{HTTP: &payments.Transport{Client: upstream.Client(), AllowTestLoopback: true}}
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "kvoucher", Enabled: true, APIBaseURL: upstream.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "synthetic-key", "kvoucher_api_secret": "synthetic-secret"}}
	if _, err := f.a.Payments.SaveMerchant(t.Context(), owner, m, 0); err != nil {
		t.Fatal(err)
	}
	rule, err := paymentmethods.Current(t.Context(), f.a.DB, op, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, result, _ := f.call(t, "POST", "/v1/operators/"+op+"/payment-methods/"+m.ID+"/open_method", map[string]any{"method_kind": "kvoucher", "enabled": true, "collection": true, "driver": "kvoucher", "fee_mode": "percent", "fee_percent": "2", "expected_version": rule.Version}, uuid.NewString())
	mustOK(t, status, result)
	rule, err = paymentmethods.Current(t.Context(), f.a.DB, op, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "customer", "checkout@example.test", "customer-password-test")
	return f, m.ID, rule.ID, calls
}

func TestPostgresTopupCheckoutFailureReturnsOriginalUnpaidOrder(t *testing.T) {
	for _, scenario := range []string{"mismatch", "quote-failure"} {
		t.Run(scenario, func(t *testing.T) {
			f, merchantID, ruleID, calls := setupCheckoutApp(t, scenario)
			body := map[string]any{"amount_money": "2", "external_method_id": merchantID, "fee_rule_id": ruleID}
			key := uuid.NewString()
			status, result, _ := f.call(t, "POST", "/v1/me/topups", body, key)
			mustOK(t, status, result)
			id := result["id"].(string)
			order := result["order"].(map[string]any)
			if result["message"] != order["payment_notice"] || result["message"] == nil {
				t.Fatal("checkout failure missing from submission feedback", result)
			}
			state, notice, wantCalls := "outcome_unknown", "支付通道返回的币种或金额与本单报价不一致", int32(1)
			if scenario == "quote-failure" {
				state, notice, wantCalls = "failed", "订单已建立，但付款指引暂时不可用", 0
			}
			check := func(order map[string]any) {
				t.Helper()
				if order["id"] != id || order["status"] != "pending" || order["paid_money"] != "0" || order["payment_state"] != state || order["payment_action"] != nil || !strings.Contains(order["payment_notice"].(string), notice) {
					t.Fatal(order)
				}
				fee := order["fee_quote"].(map[string]any)
				if fee["amount_money"] != "2" || fee["fee_money"] != "0.04" || fee["total_money"] != "2.04" || fee["rule_id"] != ruleID {
					t.Fatal(fee)
				}
				if order["cancel_allowed"] != (scenario == "quote-failure") || order["method_change_allowed"] != (scenario == "quote-failure") {
					t.Fatal("payment uncertainty changed cancellation or replacement permission", order)
				}
				raw, _ := json.Marshal(order)
				if strings.Contains(string(raw), "synthetic-private-signature") || strings.Contains(string(raw), "synthetic-secret") {
					t.Fatal("unverified payment form exposed")
				}
			}
			check(order)
			status, replay, _ := f.call(t, "POST", "/v1/me/topups", body, key)
			mustOK(t, status, replay)
			check(replay["order"].(map[string]any))
			status, detail, _ := f.call(t, "GET", "/v1/me/orders/"+id, nil, "")
			mustOK(t, status, detail)
			check(detail)
			if calls.Load() != wantCalls {
				t.Fatal("retry created another provider order", calls.Load())
			}
			intent, err := f.a.Payments.PaymentIntent(t.Context(), f.a.Installation.OperatorID, merchantID, id)
			if err != nil || intent.State != state || intent.Checkout != nil || (scenario == "mismatch" && (intent.ProviderID != "51" || intent.Problem != payments.CheckoutAmountMismatch)) {
				t.Fatal(intent, err)
			}
			var facts int64
			if err := f.a.DB.Table("payments_verified_facts").Where("order_id=?", id).Count(&facts).Error; err != nil || facts != 0 {
				t.Fatal("unpaid order acquired a receipt", facts, err)
			}
			var balance ledger.Decimal
			if err := f.a.DB.Raw("SELECT COALESCE(SUM(balance),0) FROM ledger_accounts WHERE operator_id=? AND purpose='customer:money'", f.a.Installation.OperatorID).Scan(&balance).Error; err != nil || !balance.IsZero() {
				t.Fatal("unpaid order credited a wallet", balance, err)
			}
			if scenario == "mismatch" {
				// Old unknown intents have no recorded mismatch reason. Do not
				// invent one, expose the form, or submit another provider order.
				if err := f.a.DB.Exec("UPDATE payments_intents SET checkout_ciphertext='' WHERE order_id=?", id).Error; err != nil {
					t.Fatal(err)
				}
				status, detail, _ = f.call(t, "GET", "/v1/me/orders/"+id, nil, "")
				mustOK(t, status, detail)
				if !strings.Contains(detail["payment_notice"].(string), "订单已建立，但付款指引暂时不可用") || detail["payment_action"] != nil || calls.Load() != 1 {
					t.Fatal(detail, calls.Load())
				}
			}
		})
	}
}

func TestPostgresPackageCheckoutFailureReturnsOriginalOrder(t *testing.T) {
	f, merchantID, ruleID, calls := setupCheckoutApp(t, "mismatch")
	plan, err := f.a.Commerce.PublishPackage(t.Context(), nil, commerce.PublishPackageInput{ActorStaffID: f.a.Installation.OwnerStaffID, OperatorID: f.a.Installation.OperatorID, Name: "Checkout test", PriceMoney: ledger.FromInt64(2), PaidATG: ledger.FromInt64(2), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true, TermsReference: "test"})
	if err != nil {
		t.Fatal(err)
	}
	status, quote, _ := f.call(t, "POST", "/v1/me/orders/quotes", map[string]any{"plan_id": plan.PackageID}, uuid.NewString())
	mustOK(t, status, quote)
	status, order, _ := f.call(t, "POST", "/v1/me/orders", map[string]any{"quote_id": quote["id"], "terms_version": quote["terms_version"], "funding": map[string]any{"external_method_id": merchantID, "fee_rule_id": ruleID}}, uuid.NewString())
	mustOK(t, status, order)
	if order["id"] == nil || order["payment_state"] != "outcome_unknown" || order["payment_action"] != nil || order["payment_notice"] == nil || calls.Load() != 1 {
		t.Fatal(order, calls.Load())
	}
}

func TestCheckoutMismatchIsNotAVersionConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/me/topups", nil)
	handleError(c, payments.ErrCheckoutMismatch)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "payment_checkout_mismatch") || strings.Contains(w.Body.String(), "version_conflict") || strings.Contains(w.Body.String(), `"retryable":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
}
