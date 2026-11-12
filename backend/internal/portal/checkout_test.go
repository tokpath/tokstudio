package portal

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

// Uses the real quote, merchant adapter, command and PostgreSQL paths. All
// merchant credentials and HTTP responses belong to this isolated fixture.
func setupCheckoutApp(t *testing.T, scenario string) (*testApp, string, string, *atomic.Int32) {
	paid := &atomic.Bool{}
	paid.Store(scenario == "paid")
	return setupCheckoutWithPaymentState(t, scenario, paid)
}

func setupCheckoutWithPaymentState(t *testing.T, scenario string, paid *atomic.Bool) (*testApp, string, string, *atomic.Int32) {
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
	var upstream *httptest.Server
	upstream = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			amount, currency := "8.71", "MYR"
			if scenario == "mismatch" {
				currency = "USD"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "data": map[string]any{"payment": map[string]any{"action": upstream.URL + "/test-payment", "method": "POST", "fields": map[string]string{"v_amount": amount, "v_currency": currency, "Signature": "synthetic-private-signature"}}}})
		case "/api/kv/merchant/v1/orders/51":
			status := "pending_payment"
			if scenario == "submitted" {
				status = "payment_submitted"
			}
			if paid.Load() {
				status = "pins_loaded"
			}
			_, _ = fmt.Fprintf(w, `{"ok":true,"data":{"order":{"id":51,"status":%q,"items":[{"denomination":8.33,"qty":1}]}}}`, status)
		case "/api/kv/merchant/v1/stock/redeemed":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"items":[]}}`))
		case "/api/kv/merchant/v1/stock/retrieve":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"retrieved":[{"pin_code":"SYNTHETIC-ONLY","denomination":8.33}]}}`))
		case "/api/kv/merchant/v1/account":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"is_test_mode":false}}`))
		case "/test-payment":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = r.ParseForm()
			_, _ = fmt.Fprintf(w, "<!doctype html><title>模拟付款页面</title><h1>模拟付款页面</h1><p>应付 %s MYR</p><p>未付款。仅供隔离测试，不发生真实转账。</p>", html.EscapeString(r.Form.Get("v_amount")))
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

func TestPostgresUnpaidKVoucherCheckIsANormalPendingResult(t *testing.T) {
	f, merchantID, ruleID, calls := setupCheckoutApp(t, "success")
	status, result, _ := f.call(t, "POST", "/v1/me/topups", map[string]any{"amount_money": "2", "external_method_id": merchantID, "fee_rule_id": ruleID}, uuid.NewString())
	mustOK(t, status, result)
	id := result["id"].(string)
	if order := result["order"].(map[string]any); order["payment_state"] != "created" || order["payment_action"] == nil {
		t.Fatal(order)
	}
	status, pending, _ := f.call(t, "POST", "/v1/me/orders/"+id+"/reconcile", map[string]any{}, uuid.NewString())
	mustOK(t, status, pending)
	if pending["status"] != "pending_payment" || !strings.Contains(pending["message"].(string), "尚未确认收款") || calls.Load() != 1 {
		t.Fatal(pending, calls.Load())
	}
	status, detail, _ := f.call(t, "GET", "/v1/me/orders/"+id, nil, "")
	mustOK(t, status, detail)
	if detail["paid_money"] != "0" || detail["status"] != "pending" || detail["payment_action"] == nil {
		t.Fatal("unpaid check removed the original payment or credited money", detail)
	}
}

func TestPostgresKVoucherProviderFeesAndVoucherReceiptDoNotChangeWalletCredit(t *testing.T) {
	f, merchant, rule, _ := setupCheckoutApp(t, "paid")
	status, result, _ := f.call(t, "POST", "/v1/me/topups", map[string]any{"amount_money": "2", "external_method_id": merchant, "fee_rule_id": rule}, uuid.NewString())
	mustOK(t, status, result)
	id := result["id"].(string)
	intent, err := f.a.Payments.PaymentIntent(t.Context(), f.a.Installation.OperatorID, merchant, id)
	if err != nil || intent.Checkout == nil || intent.Checkout.PayMoney.Amount.String() != "8.71" || intent.FeeQuote.TotalMoney.String() != "2.04" {
		t.Fatal("provider surcharge changed submitted amount", intent, err)
	}
	for i := 0; i < 2; i++ {
		if _, err = f.a.Payments.Reconcile(t.Context(), f.a.Installation.OperatorID, merchant, id); err != nil {
			t.Fatal(err)
		}
		if err = f.a.RunDeliveryJobs(t.Context(), "checkout-regression", 10); err != nil {
			t.Fatal(err)
		}
	}
	var order commerce.Order
	if err = f.a.DB.Table("commerce_orders").First(&order, "id=?", id).Error; err != nil || order.Status != "completed" {
		t.Fatal(order, err)
	}
	var wallet ledger.Account
	if err = f.a.DB.Where("owner_id=? AND purpose='customer:money'", order.CustomerID).First(&wallet).Error; err != nil || wallet.Balance.String() != "2" {
		t.Fatal(wallet, err)
	}
	var receipt commerce.Receipt
	if err = f.a.DB.Table("commerce_receipts").Where("operator_id=?", order.OperatorID).First(&receipt).Error; err != nil || receipt.SettlementCurrency != "MYR" || receipt.SettlementNetAmount == nil || receipt.SettlementNetAmount.String() != "8.33" || receipt.GrossMoney.String() != "2.04" {
		t.Fatal(receipt, err)
	}
	var count int64
	f.a.DB.Table("ledger_journals").Where("business_key=?", "receipt-native:"+receipt.ID).Count(&count)
	if count != 1 {
		t.Fatal("duplicate voucher receipt", count)
	}
}

func TestPostgresUnpaidCheckoutCanChangeMethodButPaidCheckoutCannot(t *testing.T) {
	for _, scenario := range []string{"success", "paid", "submitted"} {
		t.Run(scenario, func(t *testing.T) {
			paid := &atomic.Bool{}
			paid.Store(scenario == "paid")
			f, merchant, rule, _ := setupCheckoutWithPaymentState(t, scenario, paid)
			op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
			offline, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "offline fixture"}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			nextRule, err := paymentmethods.Current(t.Context(), f.a.DB, op, offline.ID)
			if err != nil {
				t.Fatal(err)
			}
			status, result, _ := f.call(t, "POST", "/v1/me/topups", map[string]any{"amount_money": "2", "external_method_id": merchant, "fee_rule_id": rule}, uuid.NewString())
			mustOK(t, status, result)
			id := result["id"].(string)
			status, result, _ = f.call(t, "POST", "/v1/me/orders/"+id+"/payment-method", map[string]any{"external_method_id": offline.ID, "fee_rule_id": nextRule.ID}, uuid.NewString())
			if scenario != "success" {
				if status < 400 {
					t.Fatal("paid order switched", result)
				}
				return
			}
			mustOK(t, status, result)
			intent, err := f.a.Payments.PaymentIntent(t.Context(), op, merchant, id)
			if err != nil || intent.State != "created" {
				t.Fatal("original attempt was falsified", intent, err)
			}
			for _, selected := range []struct{ method, fee string }{{merchant, rule}, {offline.ID, nextRule.ID}} {
				status, result, _ = f.call(t, "POST", "/v1/me/orders/"+id+"/payment-method", map[string]any{"external_method_id": selected.method, "fee_rule_id": selected.fee}, uuid.NewString())
				mustOK(t, status, result)
				if result["payment_action"] == nil {
					t.Fatal("returning to previous method lost original checkout", result)
				}
			}
			paid.Store(true)
			if _, err = f.a.Payments.Reconcile(t.Context(), op, merchant, id); err != nil {
				t.Fatal(err)
			}
			if err = f.a.RunDeliveryJobs(t.Context(), "late-payment", 10); err != nil {
				t.Fatal(err)
			}
			var order commerce.Order
			if err = f.a.DB.Table("commerce_orders").First(&order, "id=?", id).Error; err != nil || order.Status != "completed" {
				t.Fatal("old fee snapshot no longer funds order", order, err)
			}
		})
	}
}

func TestPostgresProcurementPaymentSwitchRequiresConfirmedUnpaid(t *testing.T) {
	for _, scenario := range []string{"success", "paid", "submitted"} {
		t.Run(scenario, func(t *testing.T) {
			f, merchant, rule, _ := setupCheckoutApp(t, scenario)
			ctx := t.Context()
			op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
			child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "Payment buyer"})
			if err != nil {
				t.Fatal(err)
			}
			invite, err := f.a.Organization.InviteStaff(ctx, nil, owner, child.Operator.ID, "owner@example.test", "owner")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
				t.Fatal(err)
			}
			quote, err := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: op, BuyerID: child.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{}}, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			order, err := f.a.Cooperation.CreatePurchaseOrder(ctx, nil, owner, child.Operator.ID, quote.ID, uuid.NewString(), ledger.FromInt64(2))
			if err != nil {
				t.Fatal(err)
			}
			offline, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "offline fixture"}}, 0)
			if err != nil {
				t.Fatal(err)
			}
			nextRule, err := paymentmethods.Current(ctx, f.a.DB, op, offline.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.login(t, "staff", "owner@example.test", "owner-password-test")
			base := "/v1/operators/" + child.Operator.ID + "/procurement"
			status, result, _ := f.call(t, "POST", base+"/"+order.ID+"/checkout", map[string]any{"merchant_id": merchant, "fee_rule_id": rule}, uuid.NewString())
			mustOK(t, status, result)
			status, page, _ := f.call(t, "GET", base+"?view=buyer&section=orders&record="+order.ID, nil, "")
			mustOK(t, status, page)
			found := false
			for _, raw := range page["items"].([]any) {
				row := raw.(map[string]any)
				if row["id"] == order.ID {
					for _, action := range row["allowed_actions"].([]any) {
						if action == "checkout" {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("existing payment has no usable method selector", page)
			}
			status, result, _ = f.call(t, "POST", base+"/"+order.ID+"/checkout", map[string]any{"merchant_id": offline.ID, "fee_rule_id": nextRule.ID}, uuid.NewString())
			if scenario != "success" {
				if status < 400 {
					t.Fatal("unconfirmed payment switched", result)
				}
				return
			}
			mustOK(t, status, result)
			var saved procurementCheckout
			if err = f.a.DB.First(&saved, "order_id=?", order.ID).Error; err != nil || saved.MerchantID != offline.ID {
				t.Fatal(saved, err)
			}
			original, err := f.a.Payments.PaymentIntent(ctx, op, merchant, order.ID)
			if err != nil || original.State != "created" {
				t.Fatal("old payment was not retained", original, err)
			}
			status, result, _ = f.call(t, "POST", base+"/"+order.ID+"/checkout", map[string]any{"merchant_id": merchant, "fee_rule_id": rule}, uuid.NewString())
			mustOK(t, status, result)
			if result["order"].(map[string]any)["payment_action"] == nil {
				t.Fatal("returning to procurement method lost original checkout", result)
			}
		})
	}
}
