package cooperation

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"io/fs"
	"testing"
	"time"
)

func tierFixture() []PurchaseGiftTier {
	return []PurchaseGiftTier{{FromUSD: amount("0"), GiftPercent: amount("0")}, {FromUSD: amount("1000"), GiftPercent: amount("1")}, {FromUSD: amount("2000"), GiftPercent: amount("2")}}
}
func TestPurchaseProgressiveTiers(t *testing.T) {
	for _, tc := range []struct{ usd, gift string }{{"999", "0"}, {"1000", "0"}, {"1000.01", "0.0001"}, {"2000", "10"}, {"2500", "20"}, {"3000", "30"}, {"4000", "50"}} {
		r, e := CalculatePurchase(tierFixture(), amount(tc.usd))
		if e != nil || r.PaidATG.String() != tc.usd || r.GiftATG.String() != tc.gift {
			t.Fatalf("%s: %+v %v", tc.usd, r, e)
		}
	}
	for _, tiers := range [][]PurchaseGiftTier{nil, {{FromUSD: amount("1")}}, {{}, {FromUSD: amount("0")}}, {{}, {FromUSD: amount("1000")}, {FromUSD: amount("900")}}, {{GiftPercent: amount("-1")}}} {
		if ValidatePurchaseTiers(tiers) == nil {
			t.Fatal("invalid tiers accepted", tiers)
		}
	}
	r, e := CalculatePurchase([]PurchaseGiftTier{{GiftPercent: amount("0.1")}}, amount("0.000000000000000001"))
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
		r, e := collector.RecordManualReceipt(f.ctx, nil, f.owner, commerce.VerifiedReceipt{OperatorID: f.platform, ProviderReceiptID: key, GrossUSD: amount(usd), FeeStatus: "pending", Currency: "USD", OriginalAmount: usd, RecipientReference: "synthetic supplier bank", PayerReference: "synthetic buyer bank", ValuationEvidence: "USD", VerificationEvidence: "synthetic verified transfer", PaidAt: &now, ObservedAt: now})
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

func TestPostgresTieredQuoteUpgradePreservesAcceptedOrders(t *testing.T) {
	db := dbtest.New(t)
	apply := func(_ string, files fs.FS) {
		t.Helper()
		entries, e := fs.ReadDir(files, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if entry.Name() == "0010_tiered_purchase_quotes.sql" {
				continue
			}
			sql, e := fs.ReadFile(files, entry.Name())
			if e != nil {
				t.Fatal(e)
			}
			if e = db.Exec(string(sql)).Error; e != nil {
				t.Fatal(e)
			}
		}
	}
	apply(organization.Migrations())
	apply(ledger.Migrations())
	apply(commerce.Migrations())
	apply(Migrations())
	quoteID, orderID, authID, supplier, buyer, owner := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	if e := db.Exec(`INSERT INTO cooperation_purchase_quotes(id,supplier_id,buyer_id,price_usd,paid_atg,gift_atg,exit_terms,terms_reference,enabled,created_by,created_at) VALUES(?,?,?,10,100,20,'{}','original-terms',true,?,clock_timestamp())`, quoteID, supplier, buyer, owner).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(`INSERT INTO cooperation_purchase_orders(id,supplier_id,buyer_id,quote_id,business_key,fingerprint,due_usd,paid_atg,gift_atg,exit_terms,terms_reference,status,expires_at,created_at) VALUES(?,?,?,?,'accepted-order','original-fingerprint',10,100,20,'{}','original-terms','awaiting_payment','9999-01-01',clock_timestamp())`, orderID, supplier, buyer, quoteID).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(`INSERT INTO cooperation_replenishment_authorizations(id,buyer_id,supplier_id,quote_id,trigger_available_atg,single_limit_usd,period_limit_usd,period,mode,payment_ciphertext,consent_reference,enabled,version,authorized_by,updated_at) VALUES(?,?,?,?,0,10,10,'month','offline','original-evidence','original-consent',true,1,?,clock_timestamp())`, authID, buyer, supplier, quoteID, owner).Error; e != nil {
		t.Fatal(e)
	}
	_, files := Migrations()
	migration, e := fs.ReadFile(files, "0010_tiered_purchase_quotes.sql")
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(string(migration)).Error; e != nil {
		t.Fatal(e)
	}
	var q PurchaseQuote
	var order PurchaseOrder
	var auth ReplenishmentAuthorization
	if e = db.First(&q, "id=?", quoteID).Error; e != nil {
		t.Fatal(e)
	}
	if q.Enabled || len(q.GiftTiers) > 0 || len(q.LegacyPricing) == 0 {
		t.Fatal("old quote was reinterpreted instead of archived", q)
	}
	if e = db.First(&order, "id=?", orderID).Error; e != nil {
		t.Fatal(e)
	}
	if order.DueUSD.String() != "10" || order.PaidATG.String() != "100" || order.GiftATG.String() != "20" || order.TermsReference != "original-terms" {
		t.Fatal("accepted order changed", order)
	}
	if e = db.First(&auth, "id=?", authID).Error; e != nil {
		t.Fatal(e)
	}
	if auth.Enabled || !auth.AmountUSD.IsZero() || auth.ConsentReference != "original-consent" {
		t.Fatal("old consent silently became a new purchase amount", auth)
	}
	if e = db.Model(&PurchaseQuote{}).Where("id=?", quoteID).Update("legacy_pricing", json.RawMessage(`{}`)).Error; e == nil {
		t.Fatal("archived quote facts are mutable")
	}
}
