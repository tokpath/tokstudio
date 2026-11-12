package portal

import (
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestWithdrawalMethodFormsAndCustomerChoice(t *testing.T) {
	f := setupPortal(t)
	op := f.a.Installation.OperatorID
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/payment-methods"
	status, page, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	fields := operationFields(t, page, "open_method")
	for _, name := range []string{"method_kind", "enabled", "collection", "fee_mode", "flat_fee_money", "fee_percent", "payout_enabled", "expected_version"} {
		if fields[name] == nil {
			t.Fatal("missing method field", name)
		}
	}
	for _, name := range []string{"fee_rate", "minimum_money", "requires_dual_approval", "configuration"} {
		if fields[name] != nil {
			t.Fatal("legacy policy fields replaced the method form", name)
		}
	}
	for _, method := range []map[string]any{
		{"action": "open_method", "method_kind": "bank_transfer", "enabled": true, "collection": false, "fee_mode": "fixed", "flat_fee_money": "1", "payout_enabled": true},
		{"action": "open_method", "method_kind": "alipay", "enabled": true, "collection": false, "fee_mode": "percent", "fee_percent": "2", "payout_enabled": true},
	} {
		key := uuid.NewString()
		status, out, _ := f.call(t, "POST", base, method, key)
		mustOK(t, status, out)
		status, replay, _ := f.call(t, "POST", base, method, key)
		mustOK(t, status, replay)
		if replay["id"] != out["id"] {
			t.Fatal("retry created another withdrawal method")
		}
	}
	status, page, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	var row map[string]any
	for _, raw := range page["items"].([]any) {
		candidate := raw.(map[string]any)
		if candidate["values"].(map[string]any)["method_kind"] == "bank_transfer" {
			row = candidate
		}
	}
	if row == nil {
		t.Fatal("bank transfer missing")
	}
	method := row["values"].(map[string]any)
	status, out, _ := f.call(t, "POST", base+"/"+row["id"].(string)+"/open_method", map[string]any{"method_kind": method["method_kind"], "enabled": true, "collection": false, "fee_mode": "fixed", "flat_fee_money": "1.5", "payout_enabled": true, "expected_version": row["version"]}, uuid.NewString())
	mustOK(t, status, out)
	f.openRegistration(t)
	f.registered(t, "method-choice@example.test")
	f.login(t, "customer", "method-choice@example.test", "customer-password-test")
	status, page, _ = f.call(t, "GET", "/v1/me/withdrawals/workspace", nil, "")
	mustOK(t, status, page)
	if len(page["operations"].([]any)) != 0 {
		t.Fatal("customer withdrawal page still publishes a separate fee quote", page["operations"])
	}
	methods := page["payout_methods"].([]any)
	if len(methods) != 2 {
		t.Fatal("customer cannot choose the configured payout method", methods)
	}
	foundAlipay := false
	for _, raw := range methods {
		method := raw.(map[string]any)
		if method["kind"] != "alipay" {
			continue
		}
		foundAlipay = true
		if len(method["fields"].([]any)) != 2 || method["summary"] == "" {
			t.Fatal("alipay payout did not publish its recipient materials", method)
		}
	}
	if !foundAlipay {
		t.Fatal("customer payout methods omitted alipay", methods)
	}
}

func TestPaymentQuoteSourceIsOnlyVisibleAndConfigurableAtPlatform(t *testing.T) {
	f := setupPortal(t)
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	oem, err := f.a.Organization.CreateOperator(t.Context(), nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.OEM, Name: "Quote visibility OEM", LegalName: "OEM", BrandName: "OEM"})
	if err != nil {
		t.Fatal(err)
	}
	invitation, err := f.a.Organization.InviteStaff(t.Context(), nil, owner, oem.Operator.ID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(t.Context(), nil, invitation.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/payment-quotes"
	status, page, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	if page["title"] != "汇率报价来源" {
		t.Fatal(page)
	}
	fields := operationFields(t, page, "configure")
	if fields["source"] == nil || fields["enabled"] == nil || fields["refresh_interval_hours"] == nil || fields["rate"] != nil {
		t.Fatal("source configuration must not expose manual exchange rates", fields)
	}
	status, out, _ := f.call(t, "POST", base, map[string]any{"source": "frankfurter", "enabled": false, "refresh_interval_hours": 6, "expected_version": 1}, uuid.NewString())
	mustOK(t, status, out)
	status, page, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	if operationFields(t, page, "configure")["refresh_interval_hours"]["default_value"] != float64(6) {
		t.Fatal("saved interval was not restored", page)
	}
	for _, invalid := range []any{0, 25, 1.5} {
		status, out, _ = f.call(t, "POST", base, map[string]any{"source": "frankfurter", "enabled": true, "refresh_interval_hours": invalid, "expected_version": 2}, uuid.NewString())
		if status != 400 {
			t.Fatal("invalid interval accepted", invalid, status, out)
		}
	}
	base = "/v1/operators/" + oem.Operator.ID + "/payment-quotes"
	status, page, _ = f.call(t, "GET", base, nil, "")
	if status != 403 {
		t.Fatal("OEM can access the platform source", status, page)
	}
	status, _, _ = f.call(t, "POST", base, map[string]any{"source": "frankfurter", "enabled": true, "refresh_interval_hours": 2, "expected_version": 2}, uuid.NewString())
	if status != 403 {
		t.Fatal("OEM changed the platform source", status)
	}
}
