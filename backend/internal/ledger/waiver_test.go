package ledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestClosureGiftsDoNotRequirePaidWaiverOrDiscardOtherAssets(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, customer := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	issue := func(key, kind string, value int64) Grant {
		t.Helper()
		paid := Zero()
		var expires *time.Time
		if kind == "paid" {
			paid = FromInt64(value)
			end := now.Add(time.Second)
			expires = &end
		}
		g, err := s.IssueGrant(ctx, nil, GrantInput{OperatorID: op, CustomerID: customer, BusinessKey: key, ActorID: "test", Reason: "closure fixture", PaidMoney: paid, Sources: []GrantSource{{Kind: kind, Amount: FromInt64(value)}}, EffectiveAt: now, ExpiresAt: expires})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	gift := issue("gift", "manual_gift", 7)
	paid := issue("paid", "paid", 3)
	closeGifts := func() error {
		return s.db.Transaction(func(tx *gorm.DB) error {
			return s.CancelClosureGifts(ctx, tx, op, customer, "close", customer, "customer confirmed formal closure", now)
		})
	}
	if err := closeGifts(); !errors.Is(err, ErrScope) {
		t.Fatal("paid rights were not protected", err)
	}
	for _, g := range []Grant{gift, paid} {
		a, err := s.AccountInTransaction(ctx, nil, g.AccountID)
		if err != nil || a.Balance.Cmp(g.Total) != 0 {
			t.Fatal("rejected closure changed a grant", a, err)
		}
	}
	// Resolve paid rights independently. No broad waiver is given to closing.
	now = now.Add(2 * time.Second)
	if _, err := s.ExpireCredits(ctx, nil, op, customer, now); err != nil {
		t.Fatal(err)
	}
	hold, err := s.ReserveCredits(ctx, nil, CreditReservation{OperatorID: op, CustomerID: customer, BusinessKey: "in-flight", Estimate: FromInt64(1), At: now, ExpiresAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if err = closeGifts(); !errors.Is(err, ErrHoldState) {
		t.Fatal("closure discarded an in-flight gift", err)
	}
	if _, err = s.Release(ctx, nil, op, hold.ID, "finished without usage"); err != nil {
		t.Fatal(err)
	}
	cash, err := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: customer, Purpose: "customer:money", Unit: "MONEY"})
	if err != nil {
		t.Fatal(err)
	}
	control, err := s.EnsureAccount(ctx, nil, Account{OperatorID: op, OwnerID: op, Purpose: "control:cash", Unit: "MONEY", AllowNegative: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Post(ctx, nil, Posting{OperatorID: op, BusinessKey: "cash", Kind: "receipt", SourceID: "verified-test-receipt", ActorID: "test", Reason: "unrelated protected USD", Lines: []Line{{AccountID: cash.ID, Amount: FromInt64(11)}, {AccountID: control.ID, Amount: FromInt64(-11)}}}); err != nil {
		t.Fatal(err)
	}
	// A failing outer closure must also roll back gift cancellation.
	rollback := errors.New("closure finalization failed")
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if e := s.CancelClosureGifts(ctx, tx, op, customer, "close", customer, "confirmed closure", now); e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	before, err := s.AccountInTransaction(ctx, nil, gift.AccountID)
	if err != nil || before.Balance.String() != "7" {
		t.Fatal("outer rollback lost gifts", before, err)
	}
	for i := 0; i < 2; i++ {
		if err = closeGifts(); err != nil {
			t.Fatal(err)
		}
	}
	after, err := s.AccountInTransaction(ctx, nil, gift.AccountID)
	if err != nil || !after.Balance.IsZero() || !after.Held.IsZero() {
		t.Fatal(after, err)
	}
	gotCash, err := s.AccountInTransaction(ctx, nil, cash.ID)
	if err != nil || gotCash.Balance.String() != "11" {
		t.Fatal("gift closure affected cash", gotCash, err)
	}
	var events []GrantEvent
	if err = s.db.Where("grant_id=? AND kind='cancellation'", gift.ID).Find(&events).Error; err != nil || len(events) != 1 || events[0].Amount.String() != "7" || !events[0].PaidMoney.IsZero() {
		t.Fatal("gift cancellation was lost or duplicated", events, err)
	}
	for _, id := range []string{gift.AccountID, paid.AccountID, cash.ID, control.ID} {
		if err = s.Reconcile(ctx, nil, id); err != nil {
			t.Fatal(err)
		}
	}
}
