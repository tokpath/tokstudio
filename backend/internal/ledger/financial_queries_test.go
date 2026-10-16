package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPostgresFinancialSnapshotReconstructsClosingAndSeparatesCurrentHolds(t *testing.T) {
	s, op, customer, control := fixture(t)
	ctx := context.Background()
	j, e := s.Post(ctx, nil, debit(op, customer.ID, control.ID, "first-service", FromInt64(3)))
	if e != nil {
		t.Fatal(e)
	}
	cut := j.CreatedAt.Add(time.Microsecond)
	if _, e = s.Post(ctx, nil, debit(op, customer.ID, control.ID, "later-service", FromInt64(2))); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Reserve(ctx, nil, Reservation{OperatorID: op, BusinessKey: "currently-held", Purpose: "usage", ExpiresAt: time.Now().Add(time.Hour), Amounts: []Line{{AccountID: customer.ID, Amount: FromInt64(4)}}}); e != nil {
		t.Fatal(e)
	}
	r, e := s.FinancialFacts(ctx, nil, op, time.Now().Add(-time.Hour), cut)
	if e != nil {
		t.Fatal(e)
	}
	var closing, held Decimal
	for _, b := range r.Closing {
		if b.Purpose == "customer:atg" {
			closing = b.Balance
		}
	}
	for _, b := range r.CurrentHeld {
		if b.Purpose == "customer:atg" {
			held = b.Balance
		}
	}
	if closing.Cmp(FromInt64(17)) != 0 || held.Cmp(FromInt64(4)) != 0 {
		t.Fatalf("closing is not the current balance: %+v", r)
	}
	other, e := s.FinancialFacts(ctx, nil, uuid.NewString(), time.Now().Add(-time.Hour), cut)
	if e != nil || len(other.Flows) != 0 || len(other.Closing) != 0 {
		t.Fatal("cross-scope journal leak", e)
	}
}

func TestPostgresFinancialGrantResponsibilityRemainsHistoricalAcrossTakeoverAndCorrection(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	from, to, customer := uuid.NewString(), uuid.NewString(), uuid.NewString()
	now := time.Now().UTC()
	_, e := s.IssueGrant(ctx, nil, GrantInput{OperatorID: from, CustomerID: customer, BusinessKey: "discounted", OrderID: "order", ActorID: "fixture", Reason: "paid order", PaidUSD: FromInt64(80), Sources: []GrantSource{{"paid", FromInt64(80)}, {"package_gift", FromInt64(20)}}, EffectiveAt: now})
	if e != nil {
		t.Fatal(e)
	}
	consume := func(op, key string, amount int64, at time.Time) CreditResult {
		t.Helper()
		h, e := s.ReserveCredits(ctx, nil, CreditReservation{op, customer, key, FromInt64(amount), now, now.Add(time.Hour)})
		if e != nil {
			t.Fatal(e)
		}
		r, e := s.SettleCredits(ctx, nil, op, customer, h.ID, key, FromInt64(amount), at)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	// The logical consumption timestamp can differ from its posting timestamp.
	// Financial period selection must agree with the actual balanced journal.
	first := consume(from, "old-service", 25, now.Add(-time.Minute))
	var firstAt time.Time
	if e = s.db.Raw("SELECT created_at FROM ledger_journals WHERE id=?", *first.JournalID).Row().Scan(&firstAt); e != nil {
		t.Fatal(e)
	}
	firstCut := firstAt.Add(time.Microsecond)
	e = s.db.Transaction(func(tx *gorm.DB) error {
		_, e := s.TransferCustomerAssets(ctx, tx, CustomerTransfer{FromOperatorID: from, ToOperatorID: to, CustomerID: customer, TakeoverID: uuid.NewString(), ActorID: "fixture", Evidence: "agreed full responsibility", At: now})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	var transferredAt time.Time
	if e = s.db.Raw("SELECT max(created_at) FROM ledger_journals WHERE operator_id=?", to).Row().Scan(&transferredAt); e != nil {
		t.Fatal(e)
	}
	transferCut := transferredAt.Add(time.Microsecond)
	consume(to, "new-service", 10, now.Add(time.Minute))
	if _, e = s.ReverseUsage(ctx, nil, ReverseUsageInput{OperatorID: to, CustomerID: customer, OriginalEventID: first.Events[0].ID, BusinessKey: "fix-old", ActorID: "fixture", Reason: "verified error", Amount: FromInt64(5), At: now}); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		op        string
		cut       time.Time
		paid, atg string
	}{
		{from, firstCut, "60", "75"}, {to, firstCut, "0", "0"},
		{from, transferCut, "0", "0"}, {to, transferCut, "60", "75"},
		{from, now.Add(time.Hour), "0", "0"}, {to, now.Add(time.Hour), "56", "70"},
	} {
		r, e := s.FinancialFacts(ctx, nil, tc.op, now.Add(-time.Hour), tc.cut)
		if e != nil || r.OutstandingGrantPaidUSD.String() != tc.paid || r.OutstandingGrantATG.String() != tc.atg {
			t.Fatalf("historical liability want %s USD/%s ATG, got %+v: %v", tc.paid, tc.atg, r, e)
		}
	}
	r, e := s.FinancialFacts(ctx, nil, from, firstAt, firstCut)
	if e != nil || len(r.GrantEvents) != 1 || r.GrantEvents[0].PaidUSD.String() != "20" {
		t.Fatalf("posting period lost original consumption: %+v %v", r, e)
	}
}
