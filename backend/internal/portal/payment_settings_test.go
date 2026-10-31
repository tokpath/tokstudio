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
	base := "/v1/operators/" + op + "/withdrawals"
	status, page, _ := f.call(t, "GET", base+"?view=methods", nil, "")
	mustOK(t, status, page)
	fields := operationFields(t, page, "method")
	for _, name := range []string{"payout_kind", "method_name", "fee_mode", "flat_fee_money", "fee_percent", "enabled", "expected_version"} {
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
		{"payout_kind": "bank_cn", "method_name": "Bank transfer", "fee_mode": "fixed", "flat_fee_money": "1", "enabled": true},
		{"payout_kind": "alipay", "method_name": "Wallet payout", "fee_mode": "percent", "fee_percent": "2", "enabled": true},
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
	status, page, _ = f.call(t, "GET", base+"?view=methods", nil, "")
	mustOK(t, status, page)
	row := page["items"].([]any)[0].(map[string]any)
	method := row["values"].(map[string]any)
	status, out, _ := f.call(t, "POST", base+"/"+row["id"].(string)+"/edit_method", map[string]any{"payout_kind": "bank_cn", "method_name": method["method_name"], "fee_mode": "fixed", "flat_fee_money": "1.5", "enabled": true, "expected_version": row["version"]}, uuid.NewString())
	mustOK(t, status, out)
	f.openRegistration(t)
	f.registered(t, "method-choice@example.test")
	f.login(t, "customer", "method-choice@example.test", "customer-password-test")
	status, page, _ = f.call(t, "GET", "/v1/me/withdrawals/workspace", nil, "")
	mustOK(t, status, page)
	options := operationFields(t, page, "quote")["method_id"]["options"].([]any)
	if len(options) != 2 {
		t.Fatal("customer cannot choose the configured payout method", options)
	}
	foundAlipay := false
	for _, raw := range page["payout_methods"].([]any) {
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
		t.Fatal("customer payout methods omitted alipay", page["payout_methods"])
	}
	status, out, _ = f.call(t, "POST", "/v1/me/withdrawals/quotes", map[string]any{"method_id": options[0].(map[string]any)["value"], "amount_money": "10"}, uuid.NewString())
	mustOK(t, status, out)
}

func TestPaymentQuoteSourceIsVisibleButOnlyPlatformCanConfigureIt(t *testing.T) {
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
	fields := operationFields(t, page, "configure")
	if fields["source"] == nil || fields["enabled"] == nil || fields["rate"] != nil {
		t.Fatal("source configuration must not expose manual exchange rates", fields)
	}
	status, out, _ := f.call(t, "POST", base, map[string]any{"source": "frankfurter", "enabled": false, "expected_version": 1}, uuid.NewString())
	mustOK(t, status, out)
	base = "/v1/operators/" + oem.Operator.ID + "/payment-quotes"
	status, page, _ = f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	if len(page["operations"].([]any)) != 0 {
		t.Fatal("OEM can configure the platform source", page)
	}
	status, _, _ = f.call(t, "POST", base, map[string]any{"source": "frankfurter", "enabled": true, "expected_version": 2}, uuid.NewString())
	if status != 403 {
		t.Fatal("OEM changed the platform source", status)
	}
}
