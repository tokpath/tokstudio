package incentives

import (
	"errors"
	"gorm.io/gorm"
	"testing"
)

func TestPostgresInFlightConsumptionInheritsCommissionSourceGateBeforeItCanBePaid(t *testing.T) {
	f := setup(t)
	a := f.register(t, "pending-source-a@example.test", "")
	b := f.register(t, "pending-source-b@example.test", a.InviteCode)
	f.qualify(t, a.Customer.ID, "qualify-source-a")
	o := f.buy(t, b.Customer.ID, "preexisting-order-source", "100", ActivitySelection{})
	if e := f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.FlagCommissionOrder(f.ctx, tx, f.owner, f.operator, o.ID, "verified-provider-dispute", "verified original payment under review", d("100"), d("100"))
	}); e != nil {
		t.Fatal(e)
	}
	// The accepted request finishes after the dispute was recorded. Its new
	// commission must inherit the original source's gate in the same ledger tx.
	f.consume(t, b.Customer.ID, "100")
	spendable, e := f.s.CommissionSpendable(f.ctx, nil, f.operator, a.Customer.ID)
	if e != nil || !spendable.IsZero() {
		t.Fatal("late consumption escaped original source restriction", spendable, e)
	}
	p, e := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{PayoutKind: "bank_cn", MinimumMoney: d("1"), MethodReference: "verified manual payout"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: a.Customer.ID, BusinessKey: "blocked-late-consumption", RecipientReference: "private:bank", PolicyID: p.ID, AmountMoney: d("1")}); !errors.Is(e, ErrBlocked) {
		t.Fatal("new payout used uncertain source", e)
	}
	if e = f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.ReleaseCommissionRestrictions(f.ctx, tx, f.owner, f.operator, "verified-provider-dispute", "independently confirmed customer payment valid")
	}); e != nil {
		t.Fatal(e)
	}
	spendable, e = f.s.CommissionSpendable(f.ctx, nil, f.operator, a.Customer.ID)
	if e != nil || spendable.Cmp(d("15")) != 0 {
		t.Fatal("valid source did not resume", spendable, e)
	}
}
