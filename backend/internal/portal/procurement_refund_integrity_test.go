package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresUndeliveredProcurementRefundHasOrderPickerAndBothCashSides(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, t.Context()
	owner := f.a.Installation.OwnerStaffID
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	orderID := out["id"].(string)
	now := time.Now().UTC()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(1000), FeeStatus: "pending", Currency: "USD", OriginalAmount: "1000", ValuationEvidence: "same-currency USD cash", VerificationEvidence: "verified test cash receipt", PaidAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	// Retain paid-but-undelivered state, rather than the normal HTTP auto-delivery.
	if _, err = f.a.Cooperation.FundPurchase(ctx, nil, f.a.Commerce, owner, orderID, receipt.ID, uuid.NewString(), ledger.FromInt64(1000)); err != nil {
		t.Fatal(err)
	}
	base := "/v1/operators/" + s.oem + "/refunds"
	status, page, _ := f.call(t, "GET", base, nil, "")
	mustOK(t, status, page)
	fields := operationFields(t, page, "procurement_nondelivery")
	if fields["order_id"]["lookup_url"] != "/api/v1/operators/"+s.oem+"/references/orders?purpose=procurement-refund" {
		t.Fatal("wrong order picker", fields)
	}
	status, choices, _ := f.call(t, "GET", "/v1/operators/"+s.oem+"/references/orders?purpose=procurement-refund", nil, "")
	mustOK(t, status, choices)
	found := false
	for _, raw := range choices["items"].([]any) {
		if raw.(map[string]any)["value"] == orderID {
			found = true
		}
	}
	if !found {
		t.Fatal("paid procurement missing from picker", choices)
	}
	status, out, _ = f.call(t, "POST", base, map[string]any{"action": "procurement_nondelivery", "order_id": orderID, "reason": "unable to deliver", "evidence": "paid order with no issued ATG"}, uuid.NewString())
	mustOK(t, status, out)
	caseID := out["id"].(string)
	status, out, _ = f.call(t, "POST", base+"/"+caseID+"/review", map[string]any{"decision": "approve", "evidence": "return full payment"}, uuid.NewString())
	mustOK(t, status, out)
	legs, err := f.a.Remediation.Legs(ctx, nil, s.oem, caseID)
	if err != nil || len(legs) != 1 {
		t.Fatal(legs, err)
	}
	leg := legs[0]
	// Staff confirm full payment. The system supplies its already-approved amount.
	key := uuid.NewString()
	proof := map[string]any{"currency": "USD", "evidence": "1000 returned in cash", "occurred_at": time.Now().UTC(), "fee_money": "0"}
	for range 2 {
		status, out, _ = f.call(t, "POST", base+"/"+leg.ID+"/verify_manual", proof, key)
		mustOK(t, status, out)
	}
	for _, operator := range []string{s.oem, s.channel} {
		report, err := f.a.Operations.FinancialReport(ctx, owner, operator, now.Add(-time.Hour), time.Now().UTC().Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, total := range report.OriginalCurrencyTotals {
			if total.Currency == "USD" {
				found = true
				if total.Income.String() != "1000" || total.Expense.String() != "1000" {
					t.Fatal("refund missing or duplicated on one side", operator, total)
				}
			}
		}
		if !found {
			t.Fatal("no original currency facts for", operator)
		}
	}
	status, page, _ = f.call(t, "GET", base+"?record="+caseID, nil, "")
	mustOK(t, status, page)
	for _, raw := range page["row_operations"].([]any) {
		name := raw.(map[string]any)["name"]
		if name == "close_execution" || name == "verify_manual_failure" {
			t.Fatal("manual error recovery branch is still exposed", name)
		}
	}
}
