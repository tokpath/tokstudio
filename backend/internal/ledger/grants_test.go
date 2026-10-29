package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestGrantConsumptionExpiryPaidBasisAndRetries(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, c := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	in := GrantInput{OperatorID: op, CustomerID: c, OrderID: "order", BusinessKey: "order", ActorID: "test", Reason: "paid order", PaidMoney: FromInt64(80), Sources: []GrantSource{{"paid", FromInt64(80)}, {"package_gift", FromInt64(20)}}, EffectiveAt: now, ExpiresAt: &expiry}
	g, e := s.IssueGrant(ctx, nil, in)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.IssueGrant(ctx, nil, in)
	if e != nil || again.ID != g.ID {
		t.Fatal(again, e)
	}
	h, e := s.ReserveCredits(ctx, nil, CreditReservation{op, c, "request", FromInt64(30), now, expiry.Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	events, e := s.ExpireCredits(ctx, nil, op, c, expiry)
	if e != nil || len(events) != 1 || events[0].PaidMoney.String() != "56" || events[0].Amount.String() != "70" {
		t.Fatal(events, e)
	}
	result, e := s.SettleCredits(ctx, nil, op, c, h.ID, "settle", FromInt64(20), expiry.Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if result.PaidMoney.String() != "16" || result.Debited.String() != "20" || !result.Shortfall.IsZero() {
		t.Fatal(result)
	}
	repeat, e := s.SettleCredits(ctx, nil, op, c, h.ID, "settle", FromInt64(20), expiry.Add(time.Hour))
	if e != nil || repeat.PaidMoney.String() != "16" {
		t.Fatal(repeat, e)
	}
	events, e = s.ExpireCredits(ctx, nil, op, c, expiry.Add(time.Minute))
	if e != nil || len(events) != 1 || events[0].PaidMoney.String() != "8" {
		t.Fatal(events, e)
	}
	grants, e := s.CustomerGrants(ctx, nil, op, c)
	if e != nil || len(grants) != 1 {
		t.Fatal(e)
	}
	g = grants[0]
	if g.ConsumedPaidMoney.Add(g.ExpiredPaidMoney).String() != "80" || g.Consumed.Add(g.Expired).String() != "100" {
		t.Fatal(g)
	}
	if e = s.Reconcile(ctx, nil, g.AccountID); e != nil {
		t.Fatal(e)
	}
}

func TestGrantSelectionSkipsUnneededLocksAndPagesFEFO(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, customer := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	var grants []Grant
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < 70; i++ {
			expires := now.Add(time.Duration(i+1) * time.Hour)
			g, err := s.IssueGrant(ctx, tx, GrantInput{OperatorID: op, CustomerID: customer, BusinessKey: uuid.NewString(), ActorID: "test", Reason: "many packages", PaidMoney: FromInt64(1), Sources: []GrantSource{{"paid", FromInt64(1)}}, EffectiveAt: now, ExpiresAt: &expires})
			if err != nil {
				return err
			}
			grants = append(grants, g)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// A lock on an unrelated late-expiring grant must not stall an ordinary
	// request covered completely by the first grant.
	other := s.db.Begin()
	defer other.Rollback()
	if err := other.Exec("SELECT id FROM ledger_accounts WHERE id=? FOR UPDATE", grants[69].AccountID).Error; err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	h, err := s.ReserveCredits(bounded, nil, CreditReservation{op, customer, "prefix", MustDecimal("0.5"), now, now.Add(time.Hour)})
	if err != nil {
		t.Fatal("unneeded grant blocked reservation", err)
	}
	r, err := s.SettleCredits(bounded, nil, op, customer, h.ID, "prefix-settled", MustDecimal("0.5"), now)
	if err != nil || len(r.Events) != 1 || r.Events[0].GrantID != grants[0].ID {
		t.Fatal("unneeded grant blocked settlement or changed FEFO", r, err)
	}
	if err = other.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	h, err = s.ReserveCredits(ctx, nil, CreditReservation{op, customer, "many", FromInt64(65), now, now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.SettleCredits(ctx, nil, op, customer, h.ID, "many-settled", MustDecimal("66.25"), now)
	if err != nil || r.Debited.String() != "66.25" || !r.Shortfall.IsZero() || len(r.Events) != 67 || r.Events[66].GrantID != grants[66].ID || r.Events[66].Amount.String() != "0.75" {
		t.Fatal("paged reservation or actual overage lost FEFO", r, err)
	}
	for _, g := range grants {
		if err = s.Reconcile(ctx, nil, g.AccountID); err != nil {
			t.Fatal(err)
		}
	}
}
func TestGrantFEFOAndUnfundedShortfall(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, c := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	for _, in := range []GrantInput{
		{OperatorID: op, CustomerID: c, BusinessKey: "forever", ActorID: "test", Reason: "order", PaidMoney: FromInt64(1), Sources: []GrantSource{{"paid", FromInt64(1)}}, EffectiveAt: now},
		{OperatorID: op, CustomerID: c, BusinessKey: "expiring", ActorID: "test", Reason: "order", PaidMoney: FromInt64(1), Sources: []GrantSource{{"paid", FromInt64(1)}, {"package_gift", FromInt64(1)}}, EffectiveAt: now, ExpiresAt: &expiry},
	} {
		if _, e := s.IssueGrant(ctx, nil, in); e != nil {
			t.Fatal(e)
		}
	}
	h, e := s.ReserveCredits(ctx, nil, CreditReservation{op, c, "request", MustDecimal("2.56"), now, expiry})
	if e != nil {
		t.Fatal(e)
	}
	result, e := s.SettleCredits(ctx, nil, op, c, h.ID, "usage", MustDecimal("3.1"), now.Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if result.Debited.String() != "3" || result.Shortfall.String() != "0.1" || result.PaidMoney.String() != "2" || len(result.Events) != 2 {
		t.Fatal(result)
	}
	grants, _ := s.CustomerGrants(ctx, nil, op, c)
	if result.Events[0].GrantID != grants[0].ID {
		t.Fatal("not FEFO")
	}
}
