package remediation

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func (f fixture) approvedRefundLeg(t *testing.T, key, amount string) RefundLeg {
	t.Helper()
	c, e := f.s.RequestWalletRefund(f.ctx, WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: key, AmountMoney: d(amount), Reason: "unused cash to original payer"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified unused and original payer"); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 {
		t.Fatal(legs, e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, legs[0].ID, "bank:"+key, "verified finance execution intent"); e != nil {
		t.Fatal(e)
	}
	return legs[0]
}
func TestPostgresRefundPendingFeeSettlesCustomerAndConfirmsCostExactlyOnce(t *testing.T) {
	f := setup(t)
	f.topup(t, "fee-source", "100")
	leg := f.approvedRefundLeg(t, "refund-with-unknown-fee", "20")
	proof := ExecutionProof{BusinessKey: "bank-return-20", Reference: "actual-bank-refund-20", Evidence: "independently queried completed USD refund", ActualMoney: d("20"), FeeStatus: "pending", Currency: "USD", Amount: "20", OccurredAt: time.Now().UTC()}
	paid, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, proof)
	if e != nil || paid.State != "completed" {
		t.Fatal(paid, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("80")) != 0 || !a.Held.IsZero() {
		t.Fatalf("unknown fee blocked customer clearance %+v", a)
	}
	accounting, e := f.s.RefundAccounting(f.ctx, nil, f.owner, f.operator, leg.ID)
	if e != nil || len(accounting) != 1 || accounting[0].FeeStatus != "pending" || accounting[0].FeeMoney != nil {
		t.Fatalf("unknown fee misrepresented as zero %+v %v", accounting, e)
	}
	fee := RefundFeeInput{OperatorID: f.operator, LegID: leg.ID, PaymentBusinessKey: proof.BusinessKey, BusinessKey: "verified-final-bank-fee", ActorID: f.owner, Reference: "final-bank-statement", Evidence: "actual refund fee in USD", FeeMoney: d("2"), OccurredAt: proof.OccurredAt}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	ids := make(chan string, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			confirmation, e := f.s.ConfirmRefundFee(f.ctx, nil, fee)
			errs <- e
			ids <- confirmation.ID
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var id string
	for next := range ids {
		if id != "" && next != id {
			t.Fatal("concurrent fee duplicated")
		}
		id = next
	}
	fees, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:merchant_fees")
	if e != nil || fees.Balance.Cmp(d("-2")) != 0 {
		t.Fatalf("late cost booked twice %+v %v", fees, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("80")) != 0 {
		t.Fatal("merchant fee reduced customer refund", a)
	}
	accounting, e = f.s.RefundAccounting(f.ctx, nil, f.owner, f.operator, leg.ID)
	if e != nil || accounting[0].FeeStatus != "confirmed" || accounting[0].FeeMoney == nil || accounting[0].FeeMoney.Cmp(d("2")) != 0 {
		t.Fatal(accounting, e)
	}
	var original struct {
		FeeStatus string
		FeeMoney  *ledger.Decimal
	}
	if e = f.db.Table("remediation_payment_observations").Where("leg_id=?", leg.ID).Take(&original).Error; e != nil || original.FeeStatus != "pending" || original.FeeMoney != nil {
		t.Fatalf("original evidence was rewritten %+v %v", original, e)
	}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, proof); e != nil {
		t.Fatal("fact replay after fee reconciliation", e)
	}
	fee.FeeMoney = d("3")
	if _, e = f.s.ConfirmRefundFee(f.ctx, nil, fee); !errors.Is(e, ErrConflict) {
		t.Fatal("changed final fee accepted", e)
	}
}

func TestPostgresRefundConfirmedZeroIsDifferentFromUnknown(t *testing.T) {
	f := setup(t)
	f.topup(t, "zero-fee-source", "40")
	leg := f.approvedRefundLeg(t, "zero-fee-refund", "10")
	proof := ExecutionProof{BusinessKey: "actual-zero-fee", Reference: "bank-confirmed-no-charge", Evidence: "final statement explicitly zero fee", ActualMoney: d("10"), FeeStatus: "confirmed", Currency: "USD", Amount: "10", OccurredAt: time.Now().UTC()}
	if _, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, proof); e != nil {
		t.Fatal(e)
	}
	rows, e := f.s.RefundAccounting(f.ctx, nil, f.owner, f.operator, leg.ID)
	if e != nil || len(rows) != 1 || rows[0].FeeStatus != "confirmed" || rows[0].FeeMoney == nil || !rows[0].FeeMoney.IsZero() || rows[0].FeeConfirmationID == nil {
		t.Fatalf("known zero lost evidence %+v %v", rows, e)
	}
	var fees []RefundFeeConfirmation
	if e = f.db.Where("leg_id=?", leg.ID).Find(&fees).Error; e != nil || len(fees) != 1 || fees[0].JournalID != nil {
		t.Fatalf("zero cost fabricated journal %+v %v", fees, e)
	}
}

func TestPostgresReceiptRefundExecutionRequiresReservedCapacityAndNoNewDispute(t *testing.T) {
	f := setup(t)
	r := f.receipt(t, "guarded-refund-source", "100")
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.ReserveReceiptRefund(f.ctx, tx, f.operator, r.ID, "external-workflow", d("20"))
	}); e != nil {
		t.Fatal(e)
	}
	if e := f.s.RequireReceiptRefundExecution(f.ctx, nil, f.operator, r.ID, "external-workflow"); !errors.Is(e, ErrInvalid) {
		t.Fatal("standalone gate allowed", e)
	}
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.RequireReceiptRefundExecution(f.ctx, tx, f.operator, r.ID, "external-workflow")
	}); e != nil {
		t.Fatal(e)
	}
	cb := f.chargeback(t, r, "late-dispute", "10")
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.RequireReceiptRefundExecution(f.ctx, tx, f.operator, r.ID, "external-workflow")
	}); !errors.Is(e, ErrReview) {
		t.Fatal("new payment escaped receipt review", e)
	}
	// The bank transfer already happened before the dispute. Its fact must still
	// clear the same reservation; execution authorization is never replayed here.
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.CompleteReceiptRefund(f.ctx, tx, f.operator, r.ID, "external-workflow")
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "merchant_collection_risk", "provider verified valid original payment"); e != nil {
		t.Fatal(e)
	}
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.RequireReceiptRefundExecution(f.ctx, tx, f.operator, r.ID, "external-workflow")
	}); !errors.Is(e, ErrState) {
		t.Fatal("completed capacity reused for a new transfer", e)
	}
}
