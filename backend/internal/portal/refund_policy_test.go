package portal

import (
	"context"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func TestPostgresRefundPolicyDefaultsUsePolicyVersionInsteadOfRefundRow(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	op, actor := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	path := "/v1/operators/" + op + "/refunds"
	read := func(suffix string, version float64, amount any) map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "GET", path+suffix, nil, "")
		mustOK(t, status, out)
		for _, value := range out["operations"].([]any) {
			operation := value.(map[string]any)
			if operation["name"] != "policy" {
				continue
			}
			fields := map[string]map[string]any{}
			for _, value := range operation["fields"].([]any) {
				field := value.(map[string]any)
				fields[field["name"].(string)] = field
			}
			if fields["expected_version"]["default_value"] != version || fields["expected_version"]["kind"] != "hidden" || fields["second_review_above"]["default_value"] != amount || fields["action"]["default_value"] != "policy" {
				t.Fatal("refund policy is not initialized from its own stored configuration", fields)
			}
			for _, item := range out["summary"].([]any) {
				if _, ok := item.(map[string]any)["value"].(string); !ok {
					t.Fatal("summary violates the resource-page string contract", item)
				}
			}
			return out
		}
		t.Fatal("policy operation missing")
		return nil
	}
	read("", 0, nil)
	for version, amount := range []string{"250.000000000000000001", "100.000000000000000001", "50.000000000000000001"} {
		status, out, _ := f.call(t, "POST", path, map[string]any{"action": "policy", "second_review_above": amount, "expected_version": version}, "")
		mustOK(t, status, out)
		read("", float64(version+1), amount)
	}
	now, gross, zero := time.Now().Add(-time.Minute), ledger.MustDecimal("100"), ledger.Zero()
	receipt, err := f.a.Commerce.RecordManualReceipt(context.Background(), nil, actor, commerce.VerifiedReceipt{OperatorID: op, ProviderReceiptID: "refund-policy-bank-receipt", GrossUSD: gross, NetUSD: &gross, FeeUSD: &zero, FeeStatus: "confirmed", Currency: "USD", OriginalAmount: "100", RecipientReference: "merchant-bank", PayerReference: "payer-bank", ValuationEvidence: "actual USD", VerificationEvidence: "verified bank receipt", PaidAt: &now, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	refund, err := f.a.Remediation.RequestUnallocatedRefund(context.Background(), actor, op, receipt.ID, "refund-policy-case", "duplicate payment", "bank evidence", ledger.MustDecimal("1"))
	if err != nil {
		t.Fatal(err)
	}
	out := read("?record="+refund.ID, 3, "50.000000000000000001")
	if out["items"].([]any)[0].(map[string]any)["version"] != float64(1) {
		t.Fatal("fixture must distinguish refund row and policy versions", out)
	}
	status, out, _ := f.call(t, "POST", path, map[string]any{"action": "policy", "second_review_above": nil, "expected_version": 3}, "")
	mustOK(t, status, out)
	read("?record="+refund.ID, 4, nil)
	status, out, _ = f.call(t, "POST", path, map[string]any{"action": "policy", "second_review_above": "999", "expected_version": 3}, "")
	if status != 409 || out["error"].(map[string]any)["retryable"] != false {
		t.Fatal("stale policy did not produce a reviewable CAS conflict", status, out)
	}
	var stored remediation.Policy
	if err = f.a.DB.Where("operator_id=?", op).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Version != 4 || stored.SecondReviewAbove != nil {
		t.Fatal("cleared threshold or exact policy version was not retained", stored)
	}
}
