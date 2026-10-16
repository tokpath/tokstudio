package ledger

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPostgresExpiredUsageCorrectionDoesNotRestoreUsableRights(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, c := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	end := now.Add(time.Hour)
	g, e := s.IssueGrant(ctx, nil, GrantInput{OperatorID: op, CustomerID: c, OrderID: "order", BusinessKey: "order", ActorID: "test", Reason: "paid sale", PaidUSD: FromInt64(80), Sources: []GrantSource{{Kind: "paid", Amount: FromInt64(80)}, {Kind: "package_gift", Amount: FromInt64(20)}}, EffectiveAt: now, ExpiresAt: &end})
	if e != nil {
		t.Fatal(e)
	}
	hold, e := s.ReserveCredits(ctx, nil, CreditReservation{OperatorID: op, CustomerID: c, BusinessKey: "request", Estimate: FromInt64(50), At: now, ExpiresAt: end})
	if e != nil {
		t.Fatal(e)
	}
	used, e := s.SettleCredits(ctx, nil, op, c, hold.ID, "use", FromInt64(50), now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ExpireCredits(ctx, nil, op, c, end); e != nil {
		t.Fatal(e)
	}
	in := ReverseUsageInput{OperatorID: op, CustomerID: c, OriginalEventID: used.Events[0].ID, BusinessKey: "fix", ActorID: "reviewer", Reason: "verified erroneous metering", Amount: FromInt64(20), At: end.Add(time.Minute)}
	correction, e := s.ReverseUsage(ctx, nil, in)
	if e != nil {
		t.Fatal(e)
	}
	if correction.Reversal.PaidUSD.String() != "16" || len(correction.ExpiryEvents) != 1 || correction.ExpiryEvents[0].PaidUSD.String() != "16" {
		t.Fatalf("reclassification %+v", correction)
	}
	current, e := s.GetGrant(ctx, nil, g.ID)
	if e != nil {
		t.Fatal(e)
	}
	if current.Consumed.String() != "30" || current.Expired.String() != "70" || current.ConsumedPaidUSD.String() != "24" || current.ExpiredPaidUSD.String() != "56" {
		t.Fatalf("incorrect attribution %+v", current)
	}
	balance, e := s.GetAccount(ctx, g.AccountID)
	if e != nil || !balance.Balance.IsZero() {
		t.Fatalf("expired credit revived %+v %v", balance, e)
	}
	repeat, e := s.ReverseUsage(ctx, nil, in)
	if e != nil || repeat.Reversal.ID != correction.Reversal.ID {
		t.Fatalf("repeat correction %+v %v", repeat, e)
	}
	in.BusinessKey = "too-much"
	in.Amount = FromInt64(31)
	if _, e = s.ReverseUsage(ctx, nil, in); !errors.Is(e, ErrAmount) {
		t.Fatalf("reversed beyond original %v", e)
	}
	if e = s.Reconcile(ctx, nil, g.AccountID); e != nil {
		t.Fatal(e)
	}
}
func TestPostgresCancellationIsSeparateFromConsumptionAndExpiry(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, c := uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	g, e := s.IssueGrant(ctx, nil, GrantInput{OperatorID: op, CustomerID: c, OrderID: "order", BusinessKey: "order", ActorID: "test", Reason: "funded sale", PaidUSD: FromInt64(80), Sources: []GrantSource{{Kind: "paid", Amount: FromInt64(80)}, {Kind: "package_gift", Amount: FromInt64(20)}}, EffectiveAt: now})
	if e != nil {
		t.Fatal(e)
	}
	h, e := s.Reserve(ctx, nil, Reservation{OperatorID: op, BusinessKey: "refund", Purpose: "refund", ExpiresAt: now.Add(time.Hour), Amounts: []Line{{AccountID: g.AccountID, Amount: FromInt64(100)}}})
	if e != nil {
		t.Fatal(e)
	}
	event, e := s.CancelReservedGrant(ctx, nil, op, c, g.ID, h.ID, "cancel", "reviewer", "verified nonfulfillment", now)
	if e != nil {
		t.Fatal(e)
	}
	if event.Kind != "cancellation" || event.PaidUSD.String() != "80" {
		t.Fatalf("cancellation %+v", event)
	}
	g, e = s.GetGrant(ctx, nil, g.ID)
	if e != nil || g.Cancelled.String() != "100" || !g.ConsumedPaidUSD.IsZero() || !g.ExpiredPaidUSD.IsZero() {
		t.Fatalf("cancel counted as spending %+v %v", g, e)
	}
}
func TestPostgresSubAtomCarryAndRollback(t *testing.T) {
	s, _, a, _ := fixture(t)
	ctx := context.Background()
	fee, _ := new(big.Rat).SetString("0.00000000000000000001")
	posted := Zero()
	for i := 0; i < 100; i++ {
		var amount Decimal
		e := s.Transaction(ctx, func(tx *gorm.DB) error {
			var e error
			amount, e = s.AccumulateCharge(ctx, tx, a.ID, "usage", uuid.NewString(), fee)
			return e
		})
		if e != nil {
			t.Fatal(e)
		}
		posted = posted.Add(amount)
	}
	if posted.String() != "0.000000000000000001" {
		t.Fatalf("micro fees lost: %s", posted)
	}
	key := uuid.NewString()
	sentinel := errors.New("rollback")
	if e := s.Transaction(ctx, func(tx *gorm.DB) error {
		if _, e := s.AccumulateCharge(ctx, tx, a.ID, "usage", key, big.NewRat(1, 1)); e != nil {
			return e
		}
		return sentinel
	}); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	var result Decimal
	if e := s.Transaction(ctx, func(tx *gorm.DB) error {
		var e error
		result, e = s.AccumulateCharge(ctx, tx, a.ID, "usage", key, big.NewRat(1, 1))
		return e
	}); e != nil || result.String() != "1" {
		t.Fatalf("rollback consumed carry: %s %v", result, e)
	}
}
