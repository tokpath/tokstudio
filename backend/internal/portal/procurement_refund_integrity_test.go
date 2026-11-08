package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func TestPostgresUndeliveredProcurementRefundHasOrderPickerAndBothCashSides(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, t.Context()
	owner := f.a.Installation.OwnerStaffID
	status, out := s.buy(t, 1000)
	mustOK(t, status, out)
	orderID := out["id"].(string)
	now := time.Now().UTC()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(1020), PaymentFeeMoney: ledger.FromInt64(20), FeeStatus: "pending", Currency: "USD", OriginalAmount: "1020", ValuationEvidence: "same-currency USD cash", VerificationEvidence: "verified test cash receipt", PaidAt: &now})
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
	status, out, _ = f.call(t, "POST", base, map[string]any{"action": "procurement_nondelivery", "order_id": orderID, "reason": "unable to deliver"}, uuid.NewString())
	mustOK(t, status, out)
	caseID := out["id"].(string)
	status, out, _ = f.call(t, "POST", base+"/"+caseID+"/review", map[string]any{"decision": "approve"}, uuid.NewString())
	mustOK(t, status, out)
	legs, err := f.a.Remediation.Legs(ctx, nil, s.oem, caseID)
	if err != nil || len(legs) != 1 {
		t.Fatal(legs, err)
	}
	leg := legs[0]
	if leg.AmountMoney.String() != "1020" {
		t.Fatal("payment fee omitted from refund", leg)
	}
	// Staff confirm full payment. The system supplies its already-approved amount.
	key := uuid.NewString()
	proof := map[string]any{"currency": "USD", "occurred_at": time.Now().UTC(), "fee_money": "0"}
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
				if total.Income.String() != "1020" || total.Expense.String() != "1020" {
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

func TestPostgresOfflinePurchaseExcludesSharedPaymentFeeFromInventory(t *testing.T) {
	s := stockSetup(t)
	f, ctx := s.f, t.Context()
	owner := f.a.Installation.OwnerStaffID
	m, e := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: s.oem, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "cash"}}, 0)
	if e != nil {
		t.Fatal(e)
	}
	rule, e := paymentmethods.Current(ctx, f.a.DB, s.oem, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	rule.FlatFeeMoney = ledger.FromInt64(20)
	if e = f.a.DB.Transaction(func(tx *gorm.DB) error {
		var err error
		rule, err = paymentmethods.Save(ctx, tx, rule, rule.Version)
		return err
	}); e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	r, e := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(1020), FeeStatus: "pending", Currency: "USD", OriginalAmount: "1020", ValuationEvidence: "same currency", PaidAt: &now})
	if e != nil {
		t.Fatal(e)
	}
	body := map[string]any{"receipt_id": r.ID, "amount_money": "1000", "external_method_id": m.ID, "fee_rule_id": rule.ID}
	key := uuid.NewString()
	status, out, _ := f.call(t, "POST", "/v1/operators/"+s.oem+"/procurement/"+s.quote.ID+"/offline_purchase", body, key)
	mustOK(t, status, out)
	order, e := f.a.Cooperation.GetPurchase(ctx, nil, owner, s.oem, out["id"].(string))
	if e != nil || order.DueMoney.String() != "1000" || order.Status != "completed" {
		t.Fatal(order, e)
	}
	receipt, e := f.a.Commerce.GetReceipt(ctx, nil, s.oem, r.ID)
	if e != nil || receipt.PaymentFeeMoney.String() != "20" || receipt.AllocatedMoney.String() != "1020" {
		t.Fatal(receipt, e)
	}
	status, replay, _ := f.call(t, "POST", "/v1/operators/"+s.oem+"/procurement/"+s.quote.ID+"/offline_purchase", body, key)
	mustOK(t, status, replay)
	if replay["id"] != out["id"] {
		t.Fatal(replay)
	}
}
