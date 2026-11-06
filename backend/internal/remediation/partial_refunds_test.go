package remediation

import (
	"errors"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func TestPostgresCashRefundUsesInternalReferenceAndRemainsIdempotent(t *testing.T) {
	f := setup(t)
	f.topup(t, "cash-return-source", "100")
	leg := f.approvedRefundLeg(t, "cash-return", "10")
	proof := ExecutionProof{BusinessKey: "cash-return-paid", ActualMoney: d("10"), Currency: "USD", Amount: "10", OccurredAt: time.Now().UTC()}
	for range 2 {
		out, err := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, proof)
		if err != nil || out.State != "completed" || out.SettledMoney.Cmp(d("10")) != 0 {
			t.Fatal(out, err)
		}
	}
	var n int64
	if err := f.db.Table("remediation_payment_observations").Where("leg_id=?", leg.ID).Count(&n).Error; err != nil || n != 1 {
		t.Fatal("cash refund duplicated", n, err)
	}
}

func TestPostgresManualRefundRejectsPartialAmountAndThenPaysFullOnce(t *testing.T) {
	f := setup(t)
	f.topup(t, "actual-partial-source", "100")
	leg := f.approvedRefundLeg(t, "actual-partial-return", "10")
	p := ExecutionProof{BusinessKey: "actual-9", Reference: "test-first-bank-transfer", Evidence: "test amount input", ActualMoney: d("9"), Currency: "USD", Amount: "9", OccurredAt: time.Now().UTC()}
	if _, err := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); !errors.Is(err, ErrRefundFullPayment) {
		t.Fatal("partial amount accepted", err)
	}
	if a := f.balance(t); a.Balance.String() != "100" || !a.Held.IsZero() {
		t.Fatal("rejected input changed wallet", a)
	}
	var n int64
	if err := f.db.Table("remediation_payment_observations").Where("leg_id=?", leg.ID).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("rejected input created payment facts", n, err)
	}
	p.BusinessKey = "full-once"
	p.Reference = ""
	p.ActualMoney = ledger.Zero()
	p.Amount = ""
	for range 2 {
		out, err := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p)
		if err != nil || out.State != "completed" || out.SettledMoney.String() != "10" {
			t.Fatal(out, err)
		}
	}
	if a := f.balance(t); a.Balance.String() != "100" || !a.Held.IsZero() {
		t.Fatal(a)
	}
	if err := f.db.Table("remediation_payment_observations").Where("leg_id=?", leg.ID).Count(&n).Error; err != nil || n != 1 {
		t.Fatal("full payment duplicated", n, err)
	}
}

func TestPostgresReceiptRefundObservationUniquenessAndPartialCapacity(t *testing.T) {
	f := setup(t)
	r := f.receipt(t, "shared-capacity-source", "100")
	observe := func(reference, fact, amount string) (ReceiptRefundObservation, error) {
		var out ReceiptRefundObservation
		e := f.db.Transaction(func(tx *gorm.DB) error {
			var e error
			out, e = f.s.ObserveReceiptRefund(f.ctx, tx, f.operator, r.ID, reference, fact, d(amount))
			return e
		})
		return out, e
	}
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.ReserveReceiptRefund(f.ctx, tx, f.operator, r.ID, "approved-10", d("10"))
	}); e != nil {
		t.Fatal(e)
	}
	first, e := observe("approved-10", "actual-first-transfer", "9")
	if e != nil || first.SettledMoney.Cmp(d("9")) != 0 || first.RemainingMoney.Cmp(d("1")) != 0 {
		t.Fatal(first, e)
	}
	again, e := observe("approved-10", "actual-first-transfer", "9")
	if e != nil || !again.Replayed {
		t.Fatal(again, e)
	}
	if _, e = observe("another-refund", "actual-first-transfer", "9"); !errors.Is(e, ErrConflict) {
		t.Fatal("same actual fact consumed twice", e)
	}
	if _, e = observe("approved-10", "actual-first-transfer", "8"); !errors.Is(e, ErrConflict) {
		t.Fatal("same actual fact mutated", e)
	}
	last, e := observe("approved-10", "actual-supplemental-transfer", "2")
	if e != nil || last.SettledMoney.Cmp(d("1")) != 0 || last.ExcessMoney.Cmp(d("1")) != 0 || !last.RemainingMoney.IsZero() {
		t.Fatal(last, e)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, r.ID)
	if e != nil || cap.CompletedMoney.Cmp(d("10")) != 0 || !cap.ReservedMoney.IsZero() || cap.AvailableMoney.Cmp(d("90")) != 0 {
		t.Fatal(cap, e)
	}
}

func TestPostgresManualRefundRejectsAmountAboveApprovedObligation(t *testing.T) {
	f := setup(t)
	f.topup(t, "actual-overpayment-source", "100")
	leg := f.approvedRefundLeg(t, "actual-overpayment-return", "10")
	p := ExecutionProof{BusinessKey: "actual-21", Reference: "test-overpayment", Evidence: "test amount input", ActualMoney: d("21"), Currency: "USD", Amount: "21", OccurredAt: time.Now().UTC()}
	if _, err := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); !errors.Is(err, ErrRefundFullPayment) {
		t.Fatal("excess amount accepted", err)
	}
	if a := f.balance(t); a.Balance.String() != "100" || !a.Held.IsZero() {
		t.Fatal("rejected amount changed wallet", a)
	}
	cap, err := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, leg.ReceiptID)
	if err != nil || !cap.CompletedMoney.IsZero() || cap.ReservedMoney.String() != "10" {
		t.Fatal("rejected amount consumed refund capacity", cap, err)
	}
}
