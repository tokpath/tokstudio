package portal

import (
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"testing"
)

func TestPostgresPaymentMethodOnboardingSharesIdentityAndKeepsPendingRules(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op := f.a.Installation.OperatorID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/payment-methods"
	body := map[string]any{"action": "open_method", "payout_currency": "CNY", "payout_rate": "7", "method_kind": "alipay", "enabled": true, "collection": true, "payout_enabled": true, "driver": "", "fee_mode": "percent", "fee_percent": "2"}
	key := uuid.NewString()
	status, out, _ := f.call(t, "POST", base, body, key)
	mustOK(t, status, out)
	id := out["id"].(string)
	status, replay, _ := f.call(t, "POST", base, body, key)
	mustOK(t, status, replay)
	if replay["id"] != id {
		t.Fatal(replay)
	}
	rule, e := paymentmethods.Current(ctx, f.a.DB, op, id)
	if e != nil || rule.Kind != "alipay" || rule.Name != "支付宝" || rule.PayoutKind != "alipay" {
		t.Fatal(rule, e)
	}
	methods, e := f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 0 {
		t.Fatal("pending connection offered to payer", methods, e)
	}
	body["driver"] = "manual"
	body["credentials"] = map[string]string{"currency": "USD", "instructions": "Use the agreed offline payment details"}
	body["expected_version"] = rule.Version
	status, out, _ = f.call(t, "POST", base+"/"+id+"/open_method", body, uuid.NewString())
	mustOK(t, status, out)
	methods, e = f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 1 || methods[0].Name != "支付宝" || methods[0].FeeRate.String() != "0.02" {
		t.Fatal(methods, e)
	}
	rule, e = paymentmethods.Current(ctx, f.a.DB, op, id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = payments.CollectionFee(ctx, f.a.DB, op, methods[0].ID, rule.ID, ledger.FromInt64(100), "USD"); e != nil {
		t.Fatal(e)
	}
	for _, wallet := range []string{"balance", "commission"} {
		payout, e := f.a.Incentives.WithdrawalMethods(ctx, nil, op, true, wallet)
		if e != nil || len(payout) != 1 || payout[0].MethodID != id {
			t.Fatal(payout, e)
		}
	}
	// Turning off a use must not discard the saved connection or secrets.
	body["collection"] = false
	body["expected_version"] = rule.Version
	status, out, _ = f.call(t, "POST", base+"/"+id+"/open_method", body, uuid.NewString())
	mustOK(t, status, out)
	if current, e := f.a.Payments.PublicMethods(ctx, op); e != nil || len(current) != 0 {
		t.Fatal(current, e)
	}
	rule, e = paymentmethods.Current(ctx, f.a.DB, op, id)
	if e != nil {
		t.Fatal(e)
	}
	body["collection"] = true
	body["credentials"] = map[string]string{}
	body["expected_version"] = rule.Version
	status, out, _ = f.call(t, "POST", base+"/"+id+"/open_method", body, uuid.NewString())
	mustOK(t, status, out)
	methods, e = f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 1 || methods[0].Instructions != "Use the agreed offline payment details" {
		t.Fatal(methods, e)
	}
	rule, e = paymentmethods.Current(ctx, f.a.DB, op, id)
	if e != nil {
		t.Fatal(e)
	}
	body["enabled"] = false
	body["expected_version"] = rule.Version
	status, out, _ = f.call(t, "POST", base+"/"+id+"/open_method", body, uuid.NewString())
	mustOK(t, status, out)
	methods, e = f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 0 {
		t.Fatal(methods, e)
	}
	payout, e := f.a.Incentives.WithdrawalMethods(ctx, nil, op, true, "balance")
	if e != nil || len(payout) != 0 {
		t.Fatal(payout, e)
	}
}

func TestPostgresPaymentConnectionChangeKeepsAcceptedOrderAndFeeIdentity(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op := f.a.Installation.OperatorID
	f.openRegistration(t)
	f.registered(t, "method-history@example.test")
	f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/payment-methods"
	body := map[string]any{"action": "open_method", "method_kind": "bank_transfer", "enabled": true, "collection": true, "payout_enabled": true, "driver": "manual", "credentials": map[string]string{"currency": "USD", "instructions": "Pay account A", "recipient_reference": "Account A"}, "fee_mode": "fixed", "flat_fee_money": "2"}
	status, out, _ := f.call(t, "POST", base, body, uuid.NewString())
	mustOK(t, status, out)
	id := out["id"].(string)
	methods, e := f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 1 {
		t.Fatal(methods, e)
	}
	original := methods[0]
	f.login(t, "customer", "method-history@example.test", "customer-password-test")
	status, topup, _ := f.call(t, "POST", "/v1/me/topups", map[string]any{"amount_money": "100", "external_method_id": original.ID, "fee_rule_id": original.FeeRuleID}, uuid.NewString())
	mustOK(t, status, topup)
	orderID := topup["id"].(string)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	rule, e := paymentmethods.Current(ctx, f.a.DB, op, id)
	if e != nil {
		t.Fatal(e)
	}
	body["expected_version"] = rule.Version
	body["flat_fee_money"] = "3"
	body["credentials"] = map[string]string{"recipient_reference": "Account B", "instructions": "Pay account B"}
	status, out, _ = f.call(t, "POST", base+"/"+id+"/open_method", body, uuid.NewString())
	mustOK(t, status, out)
	methods, e = f.a.Payments.PublicMethods(ctx, op)
	if e != nil || len(methods) != 1 || methods[0].ID == original.ID || methods[0].FlatFeeMoney.String() != "3" {
		t.Fatal(methods, e)
	}
	connection, e := f.a.Payments.Merchant(ctx, op, methods[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	connection.PaymentMethodID = uuid.NewString()
	if _, e := f.a.Payments.SaveMerchant(ctx, f.a.Installation.OwnerStaffID, connection, connection.Version); !errors.Is(e, payments.ErrForbidden) {
		t.Fatalf("connection fee identity changed: %v", e)
	}
	f.login(t, "customer", "method-history@example.test", "customer-password-test")
	status, old, _ := f.call(t, "GET", "/v1/me/orders/"+orderID, nil, "")
	mustOK(t, status, old)
	quote := old["fee_quote"].(map[string]any)
	action := old["payment_action"].(map[string]any)
	if quote["fee_money"] != "2" || action["amount"] != "102" || action["instructions"] != "Pay account A" {
		t.Fatal(old)
	}
	status, out, _ = f.call(t, "POST", "/v1/me/topups", map[string]any{"amount_money": "100", "external_method_id": original.ID, "fee_rule_id": methods[0].FeeRuleID}, uuid.NewString())
	if status == 200 {
		t.Fatal("retired connection accepted new payment", out)
	}
}

func TestPostgresManualPaymentCapabilitiesAcrossOperatorRoles(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	roles := f.seedPaymentOperatorRoles(t)
	roles = append(roles, map[string]string{"operator": f.a.Installation.OperatorID, "email": "owner@example.test", "password": "owner-password-test"})
	for _, role := range roles {
		f.login(t, "staff", role["email"], role["password"])
		op := role["operator"]
		base := "/v1/operators/" + op + "/payment-methods"
		status, page, _ := f.call(t, "GET", base, nil, "")
		mustOK(t, status, page)
		status, out, _ := f.call(t, "POST", base+"/offline/open_method", map[string]any{"action": "open_method", "method_kind": "offline", "enabled": true, "collection": true, "payout_enabled": true, "fee_mode": "none"}, uuid.NewString())
		mustOK(t, status, out)
		methods, e := f.a.Payments.PublicMethods(ctx, op)
		if e != nil || len(methods) != 1 || methods[0].Name != "线下支付" {
			t.Fatal(methods, e)
		}
		payouts, e := f.a.Incentives.WithdrawalMethods(ctx, nil, op, true, "balance")
		if e != nil || len(payouts) != 1 || payouts[0].PayoutKind != "offline" {
			t.Fatal(payouts, e)
		}
		status, out, _ = f.call(t, "POST", base, map[string]any{"action": "open_method", "method_kind": "stripe", "enabled": true, "collection": true, "payout_enabled": true, "fee_mode": "none"}, uuid.NewString())
		if status == 200 {
			t.Fatal("Stripe became manual bank payouts", out)
		}
		status, out, _ = f.call(t, "POST", base, map[string]any{"action": "open_method", "method_kind": "usdt", "enabled": true, "collection": false, "payout_enabled": true, "payout_currency": "USDT", "payout_rate": "1.01", "fee_mode": "none"}, uuid.NewString())
		if status == 200 {
			t.Fatal("unconfigured payout network accepted", out)
		}
		status, out, _ = f.call(t, "POST", base, map[string]any{"action": "open_method", "method_kind": "usdt", "enabled": true, "collection": false, "payout_enabled": true, "payout_currency": "USDT", "payout_rate": "1.01", "payout_networks": []string{"TRC20"}, "fee_mode": "none"}, uuid.NewString())
		mustOK(t, status, out)
		payouts, e = f.a.Incentives.WithdrawalMethods(ctx, nil, op, true, "commission")
		if e != nil || len(payouts) != 2 {
			t.Fatal(payouts, e)
		}
		for _, p := range customerPayoutMethods(payouts) {
			if p["kind"] == "usdt" {
				fields := p["fields"].([]incentives.PayoutField)
				if len(fields[0].Options) != 1 || fields[0].Options[0].Value != "TRC20" {
					t.Fatal(fields)
				}
			}
		}
		if op != f.a.Installation.OperatorID {
			status, out, _ = f.call(t, "GET", "/v1/operators/"+f.a.Installation.OperatorID+"/payment-methods", nil, "")
			if status == 200 {
				t.Fatal("lower operator sees platform payment settings", out)
			}
		}
	}
}
