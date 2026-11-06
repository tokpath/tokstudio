package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func TestPostgresRefundPendingFeeCanBeConfirmedFromItsPaymentRow(t *testing.T) {
	f := setupPortal(t)
	ctx, op, owner := t.Context(), f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	now := time.Now().UTC()
	r, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: op, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(20), Currency: "USD", OriginalAmount: "20", ValuationEvidence: "verified USD receipt", FeeStatus: "pending", PaidAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.a.Remediation.RequestUnallocatedRefund(ctx, owner, op, r.ID, uuid.NewString(), "unallocated receipt return", "original payment verified", ledger.FromInt64(20))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Remediation.ReviewRefund(ctx, owner, op, c.ID, "approve", "approved full refund"); err != nil {
		t.Fatal(err)
	}
	legs, err := f.a.Remediation.Legs(ctx, nil, op, c.ID)
	if err != nil || len(legs) != 1 {
		t.Fatal(legs, err)
	}
	l := legs[0]
	_, err = f.a.Remediation.RecordManualRefund(ctx, owner, op, l.ID, remediation.ExecutionProof{BusinessKey: uuid.NewString(), Evidence: "actual full transfer, fee awaiting statement", Currency: "USD", FeeStatus: "pending", OccurredAt: now})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + op + "/refunds"
	status, page, _ := f.call(t, "GET", base+"?record="+c.ID, nil, "")
	mustOK(t, status, page)
	foreignAmountOnly := false
	for _, raw := range page["row_operations"].([]any) {
		o := raw.(map[string]any)
		if o["name"] != "verify_manual" {
			continue
		}
		for _, raw := range o["fields"].([]any) {
			field := raw.(map[string]any)
			if field["name"] == "actual_money" {
				t.Fatal("manual refund requires retyping its obligation", field)
			}
			if field["name"] == "amount" {
				foreignAmountOnly = field["visible_when"] != nil
			}
		}
	}
	if !foreignAmountOnly {
		t.Fatal("refund amount is not conditional on foreign currency")
	}
	var paymentID string
	for _, raw := range page["items"].([]any) {
		row := raw.(map[string]any)
		if row["values"].(map[string]any)["record_kind"] == "actual_refund_payment" {
			if actions := row["allowed_actions"].([]any); len(actions) != 1 || actions[0] != "confirm_fee" {
				t.Fatal("pending fee has no action", row)
			}
			paymentID = row["id"].(string)
		}
	}
	if paymentID == "" {
		t.Fatal("actual refund payment absent")
	}
	key := uuid.NewString()
	for range 2 {
		status, out, _ := f.call(t, "POST", base+"/"+paymentID+"/confirm_fee", map[string]any{"leg_id": l.ID, "fee_money": "2", "occurred_at": now}, key)
		mustOK(t, status, out)
	}
	facts, err := f.a.Remediation.FinancialRefundFacts(ctx, nil, op, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || facts.PendingFees != 0 || !facts.OutstandingMoney.IsZero() {
		t.Fatal(facts, err)
	}
	var count int64
	if err = f.a.DB.Table("remediation_refund_fee_confirmations").Where("leg_id=?", l.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("fee confirmation duplicated", count, err)
	}
}
