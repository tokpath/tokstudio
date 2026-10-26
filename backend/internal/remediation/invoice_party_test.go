package remediation

import (
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

func TestPostgresSeparateReceiptsFromSamePayerKeepCustomerInvoiceRefundsIsolated(t *testing.T) {
	f := setup(t)
	op, e := f.s.Organization.GetOperator(f.ctx, nil, f.operator)
	if e != nil {
		t.Fatal(e)
	}
	intent, e := f.s.Organization.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: op.BrandID, Email: "other-invoice-party@example.test", Password: "only-test-long-password"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Organization.VerifyEmail(f.ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
		t.Fatal(e)
	}
	other, e := f.s.Organization.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
	if e != nil {
		t.Fatal(e)
	}
	var r commerce.Receipt
	receipts := map[string]commerce.Receipt{}
	for _, party := range []struct{ id, amount, key string }{{f.customer, "40", "party-a"}, {other.Customer.ID, "60", "party-b"}} {
		receipt := f.receipt(t, party.key+"-bank-payment", party.amount)
		receipts[party.id] = receipt
		if party.id == f.customer {
			r = receipt
		}
		o, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: party.id, BusinessKey: party.key, ExpectedOwnershipVersion: 1, AmountUSD: d(party.amount)})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: receipt.ID, OrderID: o.ID, BusinessKey: party.key, AmountUSD: d(party.amount)}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
			t.Fatal(e)
		}
	}
	leg := f.approvedRefundLeg(t, "party-a-only-refund", "20")
	p := ExecutionProof{BusinessKey: "actual-return-a", Reference: "bank-confirmed-a", Evidence: "original source returned only to party a", ActualUSD: d("20"), FeeStatus: "pending", Currency: "USD", Amount: "20", OccurredAt: time.Now().UTC()}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); e != nil {
		t.Fatal(e)
	}
	for _, party := range []struct{ id, removed string }{{f.customer, "20"}, {other.Customer.ID, "0"}} {
		rows, e := f.s.InvoicePartyReceiptAdjustments(f.ctx, nil, f.operator, party.id, []string{receipts[party.id].ID})
		if e != nil || len(rows) != 1 || rows[0].RemovedUSD.Cmp(d(party.removed)) != 0 || rows[0].ReviewRequired {
			t.Fatal("one payer refund reduced another customer's basis", rows, e)
		}
	}
	global, e := f.s.InvoiceReceiptAdjustments(f.ctx, nil, f.operator, []string{r.ID})
	if e != nil || len(global) != 1 || global[0].RemovedUSD.Cmp(d("20")) != 0 {
		t.Fatal(global, e)
	}
	facts, e := f.s.FinancialRefundFacts(f.ctx, nil, f.operator, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if e != nil || facts.PendingFees != 1 || facts.PendingExecutions != 0 || !facts.OutstandingUSD.IsZero() {
		t.Fatal(facts, e)
	}
	if _, e = f.s.ConfirmRefundFee(f.ctx, nil, RefundFeeInput{OperatorID: f.operator, LegID: leg.ID, PaymentBusinessKey: p.BusinessKey, BusinessKey: "late-fee", ActorID: f.owner, Reference: "final-bank-statement", Evidence: "confirmed actual merchant charge", FeeUSD: d("1"), OccurredAt: p.OccurredAt}); e != nil {
		t.Fatal(e)
	}
	facts, e = f.s.FinancialRefundFacts(f.ctx, nil, f.operator, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if e != nil || facts.PendingFees != 0 {
		t.Fatal(facts, e)
	}
	leg = f.approvedRefundLeg(t, "party-a-fx-shortfall", "10")
	p.BusinessKey, p.Reference, p.ActualUSD, p.Amount = "short-conversion", "bank-shortfall", d("9"), "9"
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); e != ErrPaymentMismatch {
		t.Fatal("actual short refund not preserved for reconciliation", e)
	}
	facts, e = f.s.FinancialRefundFacts(f.ctx, nil, f.operator, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if e != nil || facts.MismatchCount != 1 || facts.OutstandingUSD.Cmp(d("1")) != 0 {
		t.Fatal("actual FX shortfall disappeared from liability report", facts, e)
	}
}
