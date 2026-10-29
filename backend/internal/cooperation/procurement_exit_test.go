package cooperation

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"testing"
	"time"
)

type allowRemoval struct{}

func (allowRemoval) RequireProcurementRemoval(context.Context, *gorm.DB, string, ledger.Decimal, string, string) error {
	return nil
}
func TestPostgresRemovalSeparatesConsumptionAndBookCost(t *testing.T) {
	f := setup(t)
	f.purchase(t, "first", "80", "80", "20")
	h, e := f.s.ReserveProcurement(f.ctx, nil, f.channel, "usage-first", amount("30"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	original, e := f.s.SettleProcurement(f.ctx, nil, f.channel, "usage-first", h.ID, amount("30"), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	f.purchase(t, "second", "60", "100", "0")
	var removal ProcurementRemoval
	e = f.db.Transaction(func(tx *gorm.DB) error {
		var e error
		removal, e = f.s.RemoveProcurement(f.ctx, tx, RemovalInput{BuyerID: f.channel, BusinessKey: "takeover-part", ActorID: f.owner, Mode: "takeover", Reason: "approved actual responsibility transfer", Evidence: "transfer:accepted", Quantity: amount("85"), At: time.Now(), Guard: allowRemoval{}})
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	if removal.BookCostMoney.String() != "58" {
		t.Fatalf("current moving-average book cost %+v", removal)
	}
	p, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil || p.Quantity.String() != "85" || p.Cost.String() != "58" {
		t.Fatal("position removal mismatch", p, e)
	}
	var historical ProcurementSettlement
	if e = f.db.First(&historical, "buyer_id=? AND request_id=?", f.channel, original.RequestID).Error; e != nil || historical.CostMoney.String() != "24" || historical.Deducted.String() != "30" {
		t.Fatal("removal rewrote original usage")
	}
	var lots []ProcurementLot
	if e = f.db.Where("buyer_id=?", f.channel).Find(&lots).Error; e != nil {
		t.Fatal(e)
	}
	removed := ledger.Zero()
	consumed := ledger.Zero()
	for _, lot := range lots {
		removed = removed.Add(lot.PaidRemoved).Add(lot.GiftRemoved)
		consumed = consumed.Add(lot.PaidConsumed).Add(lot.GiftConsumed)
	}
	if removed.String() != "85" || consumed.String() != "30" {
		t.Fatal("removed procurement conflated with normal consumption")
	}
	if e = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.RemoveProcurement(f.ctx, tx, RemovalInput{BuyerID: f.channel, BusinessKey: "no-proof", ActorID: f.owner, Mode: "takeover", Reason: "reason", Evidence: "text-only", Quantity: amount("1"), At: time.Now()})
		return e
	}); !errors.Is(e, ErrConflict) {
		t.Fatal("text evidence bypassed responsibility evaluation")
	}
}
func TestPostgresReversalRestoresOriginalCostAfterDifferentPricePurchase(t *testing.T) {
	f := setup(t)
	f.purchase(t, "original", "80", "80", "20")
	h, e := f.s.ReserveProcurement(f.ctx, nil, f.channel, "corrected-request", amount("30"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.SettleProcurement(f.ctx, nil, f.channel, "corrected-request", h.ID, amount("30"), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	f.purchase(t, "later", "60", "100", "0")
	reverse := func(key, q string) (ProcurementReversal, error) {
		var result ProcurementReversal
		e := f.db.Transaction(func(tx *gorm.DB) error {
			var e error
			result, e = f.s.ReverseProcurementUsage(f.ctx, tx, f.channel, "corrected-request", key, f.owner, "verified incorrect metering", amount(q), time.Now())
			return e
		})
		return result, e
	}
	first, e := reverse("one", "10")
	if e != nil || first.CostMoney.String() != "8" {
		t.Fatal("correction used current average cost", first, e)
	}
	again, e := reverse("one", "10")
	if e != nil || again.ID != first.ID {
		t.Fatal("reversal replay duplicated assets")
	}
	second, e := reverse("two", "20")
	if e != nil || second.CostMoney.String() != "16" {
		t.Fatal("historical remainder lost", second, e)
	}
	if _, e = reverse("three", "0.000000000000000001"); !errors.Is(e, ErrConflict) {
		t.Fatal("reversal exceeded original deducted quantity")
	}
	p, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil || p.Quantity.String() != "200" || p.Cost.String() != "140" {
		t.Fatalf("restored position %+v %v", p, e)
	}
	var lots []ProcurementLot
	_ = f.db.Where("buyer_id=?", f.channel).Find(&lots).Error
	for _, lot := range lots {
		if !lot.PaidConsumed.IsZero() || !lot.GiftConsumed.IsZero() {
			t.Fatal("original paid/gift source consumption not restored")
		}
	}
}
