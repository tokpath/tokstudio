package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresLiveAssetSummaryUsesFulfillmentClock(t *testing.T) {
	s := New(testDB(t))
	ctx := context.Background()
	op, customer := uuid.NewString(), uuid.NewString()
	var now time.Time
	if err := s.db.Raw("SELECT clock_timestamp()").Row().Scan(&now); err != nil {
		t.Fatal(err)
	}
	for i, at := range []time.Time{now, now.Add(time.Hour)} {
		_, err := s.IssueGrant(ctx, nil, GrantInput{OperatorID: op, CustomerID: customer, OrderID: uuid.NewString(), BusinessKey: uuid.NewString(), ActorID: "test", Reason: "database-clock delivery", PaidUSD: FromInt64(10), Sources: []GrantSource{{Kind: "paid", Amount: FromInt64(10)}, {Kind: "package_gift", Amount: FromInt64(2)}}, EffectiveAt: at})
		if err != nil {
			t.Fatal(i, err)
		}
	}
	live, err := s.CustomerAssetSummary(ctx, nil, op, customer, time.Time{})
	if err != nil || live.AvailableATG.String() != "12" || live.PurchasedRemainingATG.String() != "10" || live.GiftRemainingATG.String() != "2" {
		t.Fatal("live view missed delivered rights or included future rights", live, err)
	}
	historical, err := s.CustomerAssetSummary(ctx, nil, op, customer, now.Add(-time.Minute))
	if err != nil || !historical.AvailableATG.IsZero() {
		t.Fatal("explicit historical time was replaced", historical, err)
	}
}
