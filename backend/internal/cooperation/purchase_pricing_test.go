package cooperation

import (
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
	"time"
)

func tierFixture() []PurchaseGiftTier {
	return []PurchaseGiftTier{{FromMoney: amount("0"), GiftPercent: amount("0")}, {FromMoney: amount("1000"), GiftPercent: amount("1")}, {FromMoney: amount("2000"), GiftPercent: amount("2")}}
}
func TestPurchaseProgressiveTiers(t *testing.T) {
	for _, tc := range []struct{ usd, gift string }{{"999", "0"}, {"1000", "0"}, {"1000.01", "0.0001"}, {"2000", "10"}, {"2500", "20"}, {"3000", "30"}, {"4000", "50"}} {
		r, e := CalculatePurchase(tierFixture(), amount(tc.usd), ledger.MonetarySnapshot{Currency: "USD", AtgPerUnit: amount("1")})
		if e != nil || r.PaidATG.String() != tc.usd || r.GiftATG.String() != tc.gift {
			t.Fatalf("%s: %+v %v", tc.usd, r, e)
		}
	}
	for _, tiers := range [][]PurchaseGiftTier{nil, {{FromMoney: amount("1")}}, {{}, {FromMoney: amount("0")}}, {{}, {FromMoney: amount("1000")}, {FromMoney: amount("900")}}, {{GiftPercent: amount("-1")}}} {
		if ValidatePurchaseTiers(tiers) == nil {
			t.Fatal("invalid tiers accepted", tiers)
		}
	}
	r, e := CalculatePurchase([]PurchaseGiftTier{{GiftPercent: amount("0.1")}}, amount("0.000000000000000001"), ledger.MonetarySnapshot{Currency: "USD", AtgPerUnit: amount("1")})
	if e != nil || !r.GiftATG.IsZero() || r.GiftRemainder == "0" {
		t.Fatal("fractional calculation evidence lost", r, e)
	}
}
func TestPostgresPurchaseOneWholePaymentAndOfflineDelivery(t *testing.T) {
	f, _, _, _ := replenishmentFixture(t, "")
	q, e := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, PurchaseQuote{SupplierID: f.platform, BuyerID: f.channel, GiftTiers: tierFixture(), Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	order, e := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, q.ID, "single-payment", amount("2000"))
	if e != nil {
		t.Fatal(e)
	}
	if order.PaidATG.String() != "2000" || order.GiftATG.String() != "10" {
		t.Fatal(order)
	}
	if _, e = f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, q.ID, "single-payment", amount("1000")); e == nil {
		t.Fatal("amount changed under same idempotency key")
	}
	collector := commerce.New(f.db)
	receipt := func(key, usd string) commerce.Receipt {
		t.Helper()
		now := time.Now().UTC()
		r, e := collector.RecordManualReceipt(f.ctx, nil, f.owner, commerce.VerifiedReceipt{OperatorID: f.platform, ProviderReceiptID: key, GrossMoney: amount(usd), FeeStatus: "pending", Currency: "USD", OriginalAmount: usd, RecipientReference: "synthetic supplier bank", PayerReference: "synthetic buyer bank", ValuationEvidence: "USD", VerificationEvidence: "synthetic verified transfer", PaidAt: &now, ObservedAt: now})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	small, large, exact := receipt("small", "1000"), receipt("large", "2100"), receipt("exact", "2000")
	for _, r := range []commerce.Receipt{small, large} {
		if _, e = f.s.FundPurchase(f.ctx, nil, collector, f.owner, order.ID, r.ID, r.ID, amount("2000")); e == nil {
			t.Fatal("non-exact receipt accepted")
		}
	}
	if _, e = f.s.FundPurchase(f.ctx, nil, collector, f.owner, order.ID, small.ID, "partial", amount("1000")); e == nil {
		t.Fatal("partial funding accepted")
	}
	if e = f.s.SetQuoteEnabled(f.ctx, nil, f.owner, q.ID, false); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.FundPurchase(f.ctx, nil, collector, f.owner, order.ID, exact.ID, "full", amount("2000")); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.FundPurchase(f.ctx, nil, collector, f.owner, order.ID, exact.ID, "different-callback-key", amount("2000")); e != nil {
		t.Fatal("same payment not idempotent", e)
	}
	if _, e = f.s.FulfillPurchase(f.ctx, nil, order.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.FundPurchase(f.ctx, nil, collector, f.owner, order.ID, large.ID, "second", amount("2000")); e == nil {
		t.Fatal("second payment accepted")
	}
	var count int64
	f.db.Model(&PurchaseAllocation{}).Where("order_id=?", order.ID).Count(&count)
	if count != 1 {
		t.Fatal("multiple allocations", count)
	}
	// A separate bank transfer is a separate order, calculated from its full USD value.
	if e = f.s.SetQuoteEnabled(f.ctx, nil, f.owner, q.ID, true); e != nil {
		t.Fatal(e)
	}
	offline, e := f.s.PurchaseFromReceipt(f.ctx, nil, collector, f.owner, f.platform, q.ID, large.ID, "offline")
	if e != nil || offline.Status != "completed" || offline.PaidATG.String() != "2100" || offline.GiftATG.String() != "12" {
		t.Fatal(offline, e)
	}
	again, e := f.s.PurchaseFromReceipt(f.ctx, nil, collector, f.owner, f.platform, q.ID, large.ID, "offline-retry")
	if e != nil || again.ID != offline.ID {
		t.Fatal("offline duplicate", again, e)
	}
	if _, e = f.s.PurchaseFromReceipt(f.ctx, nil, collector, f.owner, f.channel, q.ID, small.ID, "foreign"); e == nil {
		t.Fatal("buyer used supplier-only collection")
	}
	position, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil || position.Quantity.String() != "4122" || position.Cost.String() != "4100" {
		t.Fatal(position, e)
	}
	var snapshot PurchasePricing
	if json.Unmarshal(order.PricingSnapshot, &snapshot) != nil || len(snapshot.GiftTiers) != 3 {
		t.Fatal("quote snapshot missing")
	}
	if e = f.db.Model(&PurchaseOrder{}).Where("id=?", order.ID).Update("pricing_snapshot", json.RawMessage(`{}`)).Error; e == nil {
		t.Fatal("snapshot is mutable")
	}
	// Permission must be checked even on the receipt replay path.
	if _, e = f.s.PurchaseFromReceipt(f.ctx, nil, collector, "00000000-0000-4000-8000-000000000001", f.platform, q.ID, large.ID, "unauthorized"); e != organization.ErrForbidden && e == nil {
		t.Fatal(e)
	}
}

func TestPurchaseUsesGlobalRatioForPaidAndProgressiveGift(t *testing.T) {
	for _, tc := range []struct{ ratio, paid, gift string }{{"10", "20000", "100"}, {"0.125", "250", "1.25"}} {
		out, e := CalculatePurchase(tierFixture(), amount("2000"), ledger.MonetarySnapshot{Currency: "CNY", AtgPerUnit: amount(tc.ratio), Version: 7})
		if e != nil || out.PaidATG.String() != tc.paid || out.GiftATG.String() != tc.gift || out.MonetarySnapshot.Currency != "CNY" || out.MonetarySnapshot.Version != 7 {
			t.Fatal(out, e)
		}
	}
	if _, e := CalculatePurchase(tierFixture(), amount("1"), ledger.MonetarySnapshot{}); e == nil {
		t.Fatal("missing ratio silently defaulted to one")
	}
}

func TestPostgresNonUnitProcurementRatioPreservesFundingAndMovingCost(t *testing.T) {
	f := setupMonetary(t, "CNY", "10")
	inv, e := f.org.InviteStaff(f.ctx, nil, f.owner, f.channel, "non-unit-owner@example.test", "owner")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.org.AcceptStaffInvitation(f.ctx, nil, inv.Token, organization.VerifiedStaff{ID: f.owner, Email: "non-unit-owner@example.test", EmailVerified: true}); e != nil {
		t.Fatal(e)
	}
	q, e := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, PurchaseQuote{SupplierID: f.platform, BuyerID: f.channel, GiftTiers: tierFixture(), Enabled: true})
	if e != nil {
		t.Fatal(e)
	}
	order, e := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, q.ID, "non-unit-ratio", amount("2000"))
	if e != nil || order.PaidATG.String() != "20000" || order.GiftATG.String() != "100" || order.MonetarySnapshot.Currency != "CNY" || order.MonetarySnapshot.AtgPerUnit.String() != "10" {
		t.Fatal(order, e)
	}
	collector := commerce.New(f.db)
	fee, net := amount("20"), amount("1980")
	receipt, e := collector.RecordManualReceipt(f.ctx, nil, f.owner, commerce.VerifiedReceipt{OperatorID: f.platform, ProviderReceiptID: "actual-cny-2000", GrossMoney: amount("2000"), FeeMoney: &fee, NetMoney: &net, FeeStatus: "confirmed", Currency: "CNY", OriginalAmount: "2000", ValuationEvidence: "same-currency actual payment", VerificationEvidence: "verified cashier receipt"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.FundPurchase(f.ctx, nil, collector, f.owner, order.ID, receipt.ID, "whole-receipt", amount("2000")); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.FulfillPurchase(f.ctx, nil, order.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	position, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil || position.Quantity.String() != "20100" || position.Cost.String() != "2000" {
		t.Fatal(position, e)
	}
	hold, e := f.s.ReserveProcurement(f.ctx, nil, f.channel, "actual-usage", amount("100.5"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	settled, e := f.s.SettleProcurement(f.ctx, nil, f.channel, "actual-usage", hold.ID, amount("100.5"), time.Now())
	if e != nil || settled.CostMoney.String() != "10" {
		t.Fatal(settled, e)
	}
}
