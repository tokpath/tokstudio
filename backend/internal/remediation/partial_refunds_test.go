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
	proof := ExecutionProof{BusinessKey: "cash-return-paid", Evidence: "Cash returned to the customer in person", ActualMoney: d("10"), Currency: "USD", Amount: "10", OccurredAt: time.Now().UTC()}
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

func TestPostgresPartialRefundClearsOnlyActualPrincipalAndSupplementHasIndependentFact(t *testing.T) {
	f := setup(t)
	f.topup(t, "actual-partial-source", "100")
	leg := f.approvedRefundLeg(t, "actual-partial-return", "10")
	p := ExecutionProof{BusinessKey: "actual-9", Reference: "actual-first-bank-transfer", Evidence: "independently verified actual payment 9", ActualMoney: d("9"), FeeStatus: "pending", Currency: "USD", Amount: "9", OccurredAt: time.Now().UTC()}
	out, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p)
	if !errors.Is(e, ErrPaymentMismatch) || out.State != "exception" || out.SettledMoney.Cmp(d("9")) != 0 {
		t.Fatal(out, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("91")) != 0 || a.Held.Cmp(d("1")) != 0 {
		t.Fatal("full principal remained frozen after partial payment", a)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, leg.ReceiptID)
	if e != nil || cap.CompletedMoney.Cmp(d("9")) != 0 || cap.ReservedMoney.Cmp(d("1")) != 0 || cap.AvailableMoney.Cmp(d("90")) != 0 {
		t.Fatal(cap, e)
	}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal("partial replay", e)
	}
	changedKey := p
	changedKey.BusinessKey = "same-actual-transfer-under-new-key"
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, changedKey); !errors.Is(e, ErrState) {
		t.Fatal("actual transfer reused with another command", e)
	}
	var allocated []struct{ RefundedMoney ledger.Decimal }
	if e = f.db.Table("commerce_wallet_funding_allocations a").Select("a.refunded_money").Joins("JOIN remediation_wallet_links l ON l.allocation_id=a.id").Where("l.leg_id=?", leg.ID).Scan(&allocated).Error; e != nil || len(allocated) != 1 || allocated[0].RefundedMoney.Cmp(d("9")) != 0 {
		t.Fatal(allocated, e)
	}
	rows, e := f.s.InvoicePartyReceiptAdjustments(f.ctx, nil, f.operator, f.customer, []string{leg.ReceiptID})
	if e != nil || rows[0].RemovedMoney.Cmp(d("9")) != 0 || !rows[0].ReviewRequired {
		t.Fatal(rows, e)
	}
	cash, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:external_cash")
	if e != nil || cash.Balance.Cmp(d("9")) != 0 {
		t.Fatal("actual cash missing from ledger", cash, e)
	}
	second := p
	second.BusinessKey = "actual-1"
	second.Reference = "actual-second-bank-transfer"
	second.ActualMoney = d("1")
	second.Amount = "1"
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, second); !errors.Is(e, ErrState) {
		t.Fatal("new payment cleared under previous instruction", e)
	}
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, leg.ID, "explicit-supplemental-instruction", "verified first actual payment; authorize only remaining 1"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, changedKey); !errors.Is(e, ErrConflict) {
		t.Fatal("actual transfer was reusable after new instruction", e)
	}
	out, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, second)
	if e != nil || out.State != "completed" || out.SettledMoney.Cmp(d("10")) != 0 {
		t.Fatal(out, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("90")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	cap, e = f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, leg.ReceiptID)
	if e != nil || cap.CompletedMoney.Cmp(d("10")) != 0 || !cap.ReservedMoney.IsZero() {
		t.Fatal(cap, e)
	}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, second); e != nil {
		t.Fatal("supplement replay", e)
	}
	var count int64
	_ = f.db.Table("remediation_payment_observations").Where("leg_id=?", leg.ID).Count(&count).Error
	if count != 2 {
		t.Fatal("payment facts collapsed or duplicated", count)
	}
	if e = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ObserveReceiptRefund(f.ctx, tx, f.operator, leg.ReceiptID, "different-obligation", "payment:does-not-exist", d("1"))
		return e
	}); e == nil {
		t.Fatal("unknown obligation consumed capacity")
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

func TestPostgresRefundOverpaymentRecordsCashWithoutTakingOtherCustomerPrincipal(t *testing.T) {
	f := setup(t)
	f.topup(t, "actual-overpayment-source", "100")
	leg := f.approvedRefundLeg(t, "actual-overpayment-return", "10")
	p := ExecutionProof{BusinessKey: "actual-21", Reference: "bank-accidental-extra-payment", Evidence: "actual bank execution 21 against approved 10", ActualMoney: d("21"), Currency: "USD", Amount: "21", OccurredAt: time.Now().UTC()}
	out, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p)
	if !errors.Is(e, ErrPaymentMismatch) || out.State != "completed" || out.SettledMoney.Cmp(d("10")) != 0 || out.ExcessMoney.Cmp(d("11")) != 0 {
		t.Fatal(out, e)
	}
	c, e := f.s.GetCase(f.ctx, nil, f.operator, leg.CaseID)
	if e != nil || c.State != "completed" {
		t.Fatal("merchant mistake blocked cleared customer liability", c, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("90")) != 0 || !a.Held.IsZero() {
		t.Fatal("overpayment seized legitimate unapproved customer balance", a)
	}
	cash, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:external_cash")
	if e != nil || cash.Balance.Cmp(d("21")) != 0 {
		t.Fatal(cash, e)
	}
	loss, e := f.s.account(f.ctx, f.db, f.operator, f.operator, "control:refund_overpayment")
	if e != nil || loss.Balance.Cmp(d("-11")) != 0 {
		t.Fatal(loss, e)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, leg.ReceiptID)
	if e != nil || cap.CompletedMoney.Cmp(d("10")) != 0 || cap.AvailableMoney.Cmp(d("90")) != 0 {
		t.Fatal("excess consumed another source's refundable capacity", cap, e)
	}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); !errors.Is(e, ErrPaymentMismatch) {
		t.Fatal("excess review replay lost", e)
	}
	facts, e := f.s.FinancialRefundFacts(f.ctx, nil, f.operator, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if e != nil || facts.MismatchCount != 1 || !facts.OutstandingMoney.IsZero() {
		t.Fatal(facts, e)
	}
}
