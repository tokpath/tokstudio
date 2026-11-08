package portal

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"testing"
	"time"
)

func TestPostgresUnifiedFeeRuleFreezesTopupAndBothWithdrawalWallets(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	f.registered(t, "unified-fees@example.test")
	if e := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; e != nil {
		t.Fatal(e)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "Cash or bank transfer"}}
	if _, e := f.a.Payments.SaveMerchant(ctx, owner, m, 0); e != nil {
		t.Fatal(e)
	}
	rule, e := paymentmethods.Current(ctx, f.a.DB, op, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	input := map[string]any{"name": "Bank and cash", "fee_mode": "percent", "fee_percent": "2", "payout_enabled": true, "payout_kind": "bank_cn", "expected_version": rule.Version}
	status, result, _ := f.call(t, "POST", "/v1/operators/"+op+"/payment-methods/"+m.ID+"/fee_rule", input, uuid.NewString())
	mustOK(t, status, result)
	rule, e = paymentmethods.Current(ctx, f.a.DB, op, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	status, audit, _ := f.call(t, "GET", "/v1/operators/"+op+"/audit?view=payment-methods", nil, "")
	mustOK(t, status, audit)
	if rule.CreatedBy != owner || len(audit["items"].([]any)) < 2 {
		t.Fatal("fee rule audit missing", rule, audit)
	}
	for _, wallet := range []string{"balance", "commission"} {
		methods, e := f.a.Incentives.WithdrawalMethods(ctx, nil, op, true, wallet)
		if e != nil || len(methods) != 1 || methods[0].ID != rule.ID {
			t.Fatal(wallet, methods, e)
		}
		q, e := f.a.Incentives.QuoteWithdrawal(ctx, nil, op, ledger.FromInt64(100), m.ID, wallet)
		if e != nil || q.FeeMoney.String() != "2" || q.NetMoney.String() != "98" {
			t.Fatal(wallet, q, e)
		}
	}
	f.login(t, "customer", "unified-fees@example.test", "customer-password-test")
	body := map[string]any{"amount_money": "100", "external_method_id": m.ID, "fee_rule_id": rule.ID}
	key := uuid.NewString()
	status, order, _ := f.call(t, "POST", "/v1/me/topups", body, key)
	mustOK(t, status, order)
	orderID := order["id"].(string)
	action := order["order"].(map[string]any)["payment_action"].(map[string]any)
	q := action["fee_quote"].(map[string]any)
	if q["fee_money"] != "2" || q["total_money"] != "102" || action["amount"] != "102" {
		t.Fatal(order)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	input["expected_version"], input["fee_percent"] = rule.Version, "3"
	status, result, _ = f.call(t, "POST", "/v1/operators/"+op+"/payment-methods/"+m.ID+"/fee_rule", input, uuid.NewString())
	mustOK(t, status, result)
	f.login(t, "customer", "unified-fees@example.test", "customer-password-test")
	status, replayed, _ := f.call(t, "POST", "/v1/me/topups", body, key)
	mustOK(t, status, replayed)
	if replayed["id"] != orderID {
		t.Fatal("retry created another order", replayed)
	}
	status, result, _ = f.call(t, "POST", "/v1/me/topups", body, uuid.NewString())
	if status != 409 {
		t.Fatal("stale fees accepted", status, result)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	// A short payment is retained but must not deliver the principal without its fee.
	shortBody := map[string]any{"order_id": orderID, "currency": "USD", "settlement_amount": "100", "original_amount": "100", "paid_at": time.Now().UTC()}
	status, short, _ := f.call(t, "POST", "/v1/operators/"+op+"/payments", shortBody, uuid.NewString())
	mustOK(t, status, short)
	shortReceipt, e := f.a.Commerce.GetReceipt(ctx, nil, op, short["id"].(string))
	if e != nil || !shortReceipt.AllocatedMoney.IsZero() {
		t.Fatal(shortReceipt, e)
	}
	if e = f.a.deliverReceipt(ctx, op, shortReceipt.ID, orderID); e == nil {
		t.Fatal("short payment delivered")
	}
	receiptKey := uuid.NewString()
	receiptBody := map[string]any{"order_id": orderID, "currency": "USD", "settlement_amount": "102", "original_amount": "102", "paid_at": time.Now().UTC()}
	status, result, _ = f.call(t, "POST", "/v1/operators/"+op+"/payments", receiptBody, receiptKey)
	mustOK(t, status, result)
	receipt, e := f.a.Commerce.GetReceipt(ctx, nil, op, result["id"].(string))
	if e != nil || receipt.GrossMoney.String() != "102" || receipt.PaymentFeeMoney.String() != "2" || receipt.AllocatedMoney.String() != "102" || receipt.FeeMoney != nil {
		t.Fatal(receipt, e)
	}
	if _, e = f.a.Commerce.Fulfill(ctx, nil, orderID, owner); e != nil {
		t.Fatal(e)
	}
	var row checkoutRow
	if e = f.a.DB.Where("order_id=?", orderID).First(&row).Error; e != nil {
		t.Fatal(e)
	}
	var saved paymentmethods.Quote
	if e = json.Unmarshal(row.FeeQuote, &saved); e != nil || saved.RuleID != rule.ID {
		t.Fatal(saved, e)
	}
	var balance ledger.Decimal
	if e = f.a.DB.Table("ledger_accounts").Where("operator_id=? AND owner_id=? AND purpose='customer:money'", op, row.CustomerID).Select("balance").Row().Scan(&balance); e != nil || balance.String() != "100" {
		t.Fatal(balance, e)
	}
	// Mixed funding charges only the external part, and an exception refunds the
	// internal principal to the wallet plus external principal and fee together.
	plan, e := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: "Mixed fee plan", PriceMoney: ledger.FromInt64(80), PaidATG: ledger.FromInt64(80), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true, TermsReference: "test"})
	if e != nil {
		t.Fatal(e)
	}
	current, e := paymentmethods.Current(ctx, f.a.DB, op, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	f.login(t, "customer", "unified-fees@example.test", "customer-password-test")
	status, quote, _ := f.call(t, "POST", "/v1/me/orders/quotes", map[string]any{"plan_id": plan.PackageID}, uuid.NewString())
	mustOK(t, status, quote)
	status, mixed, _ := f.call(t, "POST", "/v1/me/orders", map[string]any{"quote_id": quote["id"], "terms_version": quote["terms_version"], "funding": map[string]any{"money_balance": "60", "commission_money": "0", "external_method_id": m.ID, "fee_rule_id": current.ID}}, uuid.NewString())
	mustOK(t, status, mixed)
	mixedQuote := mixed["fee_quote"].(map[string]any)
	if mixedQuote["amount_money"] != "20" || mixedQuote["fee_money"] != "0.6" || mixedQuote["total_money"] != "20.6" {
		t.Fatal(mixed)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, mixedReceipt, _ := f.call(t, "POST", "/v1/operators/"+op+"/payments", map[string]any{"order_id": mixed["id"], "currency": "USD", "settlement_amount": "20.6", "original_amount": "20.6", "paid_at": time.Now().UTC()}, uuid.NewString())
	mustOK(t, status, mixedReceipt)
	refund, e := f.a.Remediation.RequestPackageException(ctx, remediation.PackageExceptionInput{ActorStaffID: owner, OperatorID: op, CustomerID: row.CustomerID, OrderID: mixed["id"].(string), BusinessKey: uuid.NewString(), Reason: "unable_to_fulfill"})
	if e != nil || refund.AmountMoney.String() != "80.6" || refund.InternalMoney.String() != "60" {
		t.Fatal(refund, e)
	}
	legs, e := f.a.Remediation.Legs(ctx, nil, op, refund.ID)
	if e != nil || len(legs) != 1 || legs[0].AmountMoney.String() != "20.6" {
		t.Fatal(legs, e)
	}
	if _, e = f.a.Remediation.ReviewRefund(ctx, owner, op, refund.ID, "approve", ""); e != nil {
		t.Fatal(e)
	}
	status, page, _ := f.call(t, "GET", "/v1/operators/"+op+"/payments", nil, "")
	mustOK(t, status, page)
	for _, v := range page["row_operations"].([]any) {
		if v.(map[string]any)["name"] == "accounting" {
			t.Fatal("manual fee task remains")
		}
	}
}
