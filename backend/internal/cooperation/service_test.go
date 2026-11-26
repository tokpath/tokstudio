package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"gorm.io/gorm"
)

func amount(s string) ledger.Decimal { return ledger.MustDecimal(s) }

// Coverage arithmetic is tested independently of catalog pricing in this file;
// production installs CatalogRates, never this test-only fixed-rate source.
type fixedRates struct{ rate *big.Rat }

func (f fixedRates) CoverageRates(context.Context, *gorm.DB, string, time.Time) (CoverageRates, error) {
	return CoverageRates{Version: 1, Direct: f.rate, Children: map[string]*big.Rat{}}, nil
}

type fixture struct {
	ctx                      context.Context
	db                       *gorm.DB
	s                        *Service
	org                      *organization.Service
	l                        *ledger.Service
	platform, channel, owner string
}

func setup(t *testing.T) fixture { return setupMonetary(t, "USD", "1") }
func setupMonetary(t *testing.T, currency, ratio string) fixture {
	return setupThroughProcurementMigration(t, currency, ratio, "")
}

func setupThroughProcurementMigration(t *testing.T, currency, ratio, last string) fixture {
	t.Helper()
	db := dbtest.New(t)
	apply := func(module string, files fs.FS) {
		entries, e := fs.ReadDir(files, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			if module == "cooperation" && last != "" && entry.Name() > last {
				break
			}
			b, e := fs.ReadFile(files, entry.Name())
			if e != nil {
				t.Fatal(e)
			}
			if e = db.Exec(string(b)).Error; e != nil {
				t.Fatal(e)
			}
		}
	}
	apply(organization.Migrations())
	apply(ledger.Migrations())
	apply(commerce.Migrations())
	apply(Migrations())
	if e := db.Exec("UPDATE ledger_monetary_settings SET currency=?,atg_per_unit=?,effective_at=clock_timestamp()-interval '1 minute',configured_by='synthetic fixture' WHERE id='platform'", currency, ratio).Error; e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	org := organization.New(db)
	l := ledger.New(db)
	owner := uuid.NewString()
	p, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Official", OperatorName: "Platform", OwnerStaffID: owner})
	if e != nil {
		t.Fatal(e)
	}
	c, e := org.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: p.Operator.ID, Kind: organization.Channel, Name: "Channel"})
	if e != nil {
		t.Fatal(e)
	}
	s := New(db, l, org)
	s.Rates = fixedRates{big.NewRat(1, 1)}
	return fixture{ctx, db, s, org, l, p.Operator.ID, c.Operator.ID, owner}
}
func (f fixture) purchase(t *testing.T, key, usd, paid, gift string) ProcurementLot {
	t.Helper()
	lot, e := f.s.DeliverPurchase(f.ctx, nil, PurchaseInput{BuyerID: f.channel, SupplierID: f.platform, OrderID: key, ReceiptReference: "verified:" + key, ActorID: f.owner, PaidMoney: amount(usd), PaidATG: amount(paid), GiftATG: amount(gift), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now().UTC()})
	if e != nil {
		t.Fatal(e)
	}
	return lot
}
func TestPostgresProcurementActualSettlementAndSourceConservation(t *testing.T) {
	f := setup(t)
	f.purchase(t, "p1", "80", "80", "20")
	h, e := f.s.ReserveProcurement(f.ctx, nil, f.channel, "r1", amount("30"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	r, e := f.s.SettleProcurement(f.ctx, nil, f.channel, "r1", h.ID, amount("40"), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if r.Deducted.String() != "40" || !r.Shortfall.IsZero() || r.ShortfallOperatorID != nil || r.CostMoney.String() != "32" {
		t.Fatalf("unexpected debit or cost: %+v", r)
	}
	again, e := f.s.SettleProcurement(f.ctx, nil, f.channel, "r1", h.ID, amount("40"), time.Now())
	if e != nil || again.CostMoney.Cmp(r.CostMoney) != 0 {
		t.Fatalf("idempotent settlement: %+v %v", again, e)
	}
	f.purchase(t, "p2", "60", "100", "0")
	p, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil {
		t.Fatal(e)
	}
	if p.Quantity.String() != "160" || p.Cost.String() != "108" {
		t.Fatalf("moving average position: %+v", p)
	}
	h, e = f.s.ReserveProcurement(f.ctx, nil, f.channel, "r2", amount("160"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	r, e = f.s.SettleProcurement(f.ctx, nil, f.channel, "r2", h.ID, amount("160"), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if r.CostMoney.String() != "108" {
		t.Fatalf("final cost %s", r.CostMoney)
	}
	var lots []ProcurementLot
	if e = f.db.Find(&lots).Error; e != nil {
		t.Fatal(e)
	}
	for _, lot := range lots {
		if !lot.Remaining.IsZero() || lot.PaidConsumed.Cmp(lot.PaidATG) != 0 || lot.GiftConsumed.Cmp(lot.GiftATG) != 0 {
			t.Fatalf("source conservation: %+v", lot)
		}
	}
}

func createOEMChain(t *testing.T, f fixture) (string, string) {
	t.Helper()
	oem, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.platform, Kind: organization.OEM, Name: "OEM", BrandName: "OEM"})
	if err != nil {
		t.Fatal(err)
	}
	oemID := oem.Operator.ID
	invitation, err := f.org.InviteStaff(f.ctx, nil, f.owner, oemID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, invitation.Token, organization.VerifiedStaff{ID: f.owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("UPDATE organization_operators SET lifecycle='active' WHERE id=?", oemID).Error; err != nil {
		t.Fatal(err)
	}
	channel, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: oemID, Kind: organization.Channel, Name: "OEM Channel"})
	if err != nil {
		t.Fatal(err)
	}
	return oemID, channel.Operator.ID
}

func TestPostgresProcurementShortfallBelongsToDirectSupplier(t *testing.T) {
	f := setup(t)
	oemID, channelID := createOEMChain(t, f)
	now := time.Now().UTC()
	for i, tc := range []struct{ buyer, supplier, stock, held, actual, deducted, shortfall string }{
		{channelID, oemID, "100", "60", "48", "40", "8"},
		{oemID, f.platform, "60", "24", "41", "36", "5"},
	} {
		orderID := uuid.NewString()
		if _, err := f.s.DeliverPurchase(f.ctx, nil, PurchaseInput{BuyerID: tc.buyer, SupplierID: tc.supplier, OrderID: orderID, ReceiptReference: "verified:" + orderID, ActorID: f.owner, PaidMoney: amount(tc.stock), PaidATG: amount(tc.stock), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: now}); err != nil {
			t.Fatal(err)
		}
		expires := now.Add(time.Minute)
		if _, err := f.s.ReserveProcurement(f.ctx, nil, tc.buyer, "awaiting", amount(tc.held), expires); err != nil {
			t.Fatal(err)
		}
		// The OEM's estimate was zero. Both layers still use their spendable ATG.
		var holdID string
		if i == 0 {
			hold, err := f.s.ReserveProcurement(f.ctx, nil, tc.buyer, "actual", amount("30"), expires)
			if err != nil {
				t.Fatal(err)
			}
			holdID = hold.ID
		}
		r, err := f.s.SettleProcurement(f.ctx, nil, tc.buyer, "actual", holdID, amount(tc.actual), now)
		if err != nil || r.Deducted.String() != tc.deducted || r.Shortfall.String() != tc.shortfall || r.SupplierID != tc.supplier || r.ShortfallOperatorID == nil || *r.ShortfallOperatorID != tc.supplier {
			t.Fatalf("unexpected settlement for buyer %s: %+v %v", tc.buyer, r, err)
		}
		p, err := f.s.Position(f.ctx, nil, tc.buyer)
		if err != nil {
			t.Fatal(err)
		}
		a, err := f.l.GetAccount(f.ctx, p.AccountID)
		if err != nil || a.Balance.String() != tc.held || a.Held.String() != tc.held {
			t.Fatalf("another request's hold was used: %+v %v", a, err)
		}
	}
	for _, tc := range []struct{ operator, expected string }{{channelID, "0"}, {oemID, "8"}, {f.platform, "5"}} {
		r, err := f.s.FinancialFacts(f.ctx, nil, tc.operator, now.Add(-time.Minute), now.Add(time.Minute))
		if err != nil || r.ProcurementShortfallATG.String() != tc.expected {
			t.Fatalf("wrong shortfall responsibility for %s: %+v %v", tc.operator, r, err)
		}
	}
}

func TestPostgresProcurementShortfallMigrationKeepsOriginalResponsibility(t *testing.T) {
	f := setupThroughProcurementMigration(t, "USD", "1", "0014_monetary_purchase_snapshot.sql")
	oemID, channelID := createOEMChain(t, f)
	now := time.Now().UTC()
	if _, err := f.s.DeliverPurchase(f.ctx, nil, PurchaseInput{BuyerID: channelID, SupplierID: oemID, OrderID: "old-stock", ReceiptReference: "verified:old-stock", ActorID: f.owner, PaidMoney: amount("100"), PaidATG: amount("100"), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: now}); err != nil {
		t.Fatal(err)
	}
	h, err := f.s.ReserveProcurement(f.ctx, nil, channelID, "old-request", amount("30"), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(`INSERT INTO cooperation_procurement_settlements
 (request_id,buyer_id,hold_id,actual,deducted,platform_shortfall,cost_money,source_allocations,created_at)
 VALUES ('old-request',?,?,40,30,10,30,'[]',?)`, channelID, h.ID, now).Error; err != nil {
		t.Fatal(err)
	}
	_, files := Migrations()
	sql, err := fs.ReadFile(files, "0015_procurement_actual_settlement.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec(string(sql)).Error; err != nil {
		t.Fatal(err)
	}
	r, err := f.s.SettleProcurement(f.ctx, nil, channelID, "old-request", h.ID, amount("40"), now.Add(time.Second))
	if err != nil || r.Deducted.String() != "30" || r.Shortfall.String() != "10" || r.SupplierID != oemID || r.ShortfallOperatorID == nil || *r.ShortfallOperatorID != f.platform {
		t.Fatalf("migration rewrote or lost prior responsibility: %+v %v", r, err)
	}
}

func TestPostgresProcurementNoStockKeepsShortfallFinal(t *testing.T) {
	f := setup(t)
	now := time.Now().UTC()
	r, err := f.s.SettleProcurement(f.ctx, nil, f.channel, "no-estimate", "", amount("1"), now)
	if err != nil || !r.Deducted.IsZero() || r.Shortfall.String() != "1" || r.ShortfallOperatorID == nil || *r.ShortfallOperatorID != f.platform {
		t.Fatalf("unexpected settlement: %+v %v", r, err)
	}
	f.purchase(t, "new-stock", "10", "10", "0")
	if r, err = f.s.SettleProcurement(f.ctx, nil, f.channel, "no-estimate", "", amount("1"), now.Add(time.Second)); err != nil || !r.Deducted.IsZero() || r.Shortfall.String() != "1" {
		t.Fatalf("retry changed settlement: %+v %v", r, err)
	}
	p, err := f.s.Position(f.ctx, nil, f.channel)
	if err != nil || p.Quantity.String() != "10" {
		t.Fatalf("newly purchased ATG was debited on replay: %+v %v", p, err)
	}
}

func TestPostgresConcurrentProcurementSettlementsShareAvailableATG(t *testing.T) {
	f := setup(t)
	f.purchase(t, "stock", "100", "100", "0")
	now := time.Now().UTC()
	type outcome struct {
		settlement ProcurementSettlement
		err        error
	}
	settled := make(chan outcome, 2)
	holds := map[string]string{}
	for _, request := range []string{"one", "two"} {
		h, err := f.s.ReserveProcurement(f.ctx, nil, f.channel, request, amount("30"), now.Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		holds[request] = h.ID
	}
	for request, holdID := range holds {
		go func() {
			s, err := f.s.SettleProcurement(f.ctx, nil, f.channel, request, holdID, amount("80"), now)
			settled <- outcome{s, err}
		}()
	}
	deducted, shortfall := ledger.Zero(), ledger.Zero()
	for range 2 {
		result := <-settled
		if result.err != nil {
			t.Fatal(result.err)
		}
		deducted = deducted.Add(result.settlement.Deducted)
		shortfall = shortfall.Add(result.settlement.Shortfall)
	}
	if deducted.String() != "100" || shortfall.String() != "60" {
		t.Fatalf("wrong totals after concurrent settlement: deducted %s, shortfall %s", deducted, shortfall)
	}
	p, err := f.s.Position(f.ctx, nil, f.channel)
	if err != nil || !p.Quantity.IsZero() || !p.Cost.IsZero() {
		t.Fatalf("unexpected procurement position: %+v %v", p, err)
	}
}

func TestPostgresConcurrentSalesAndOversellNeverCreatesProcurementCredit(t *testing.T) {
	f := setup(t)
	f.purchase(t, "p", "100", "100", "0")
	var successes atomic.Int64
	var wg sync.WaitGroup
	unexpected := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := f.s.Reserve(f.ctx, nil, commerce.Order{ID: uuid.NewString(), OperatorID: f.channel, PaidATG: amount("10")})
			if e == nil {
				successes.Add(1)
			} else if !errors.Is(e, ErrCoverage) {
				unexpected <- e
			}
		}()
	}
	wg.Wait()
	close(unexpected)
	for e := range unexpected {
		t.Fatal(e)
	}
	if successes.Load() != 10 {
		t.Fatalf("oversold: %d", successes.Load())
	}
	c, e := f.s.Coverage(f.ctx, nil, f.channel, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if c.Space.String() != "0" || c.Pending.String() != "100" {
		t.Fatalf("coverage %+v", c)
	}
	if _, e = f.s.SetOversell(f.ctx, nil, f.owner, f.channel, amount("20"), amount("30"), 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Reserve(f.ctx, nil, commerce.Order{ID: uuid.NewString(), OperatorID: f.channel, PaidATG: amount("20")}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReserveProcurement(f.ctx, nil, f.channel, "overdraw", amount("101"), time.Now().Add(time.Minute)); !errors.Is(e, ledger.ErrInsufficient) {
		t.Fatalf("oversell leaked into procurement: %v", e)
	}
}
func TestPostgresCoverageCountsValidUnheldRightsAndPendingOnlyOnce(t *testing.T) {
	f := setup(t)
	f.s.Rates = fixedRates{big.NewRat(3, 5)}
	f.purchase(t, "p", "100", "100", "0")
	customer := uuid.NewString()
	g, e := f.l.IssueGrant(f.ctx, nil, ledger.GrantInput{OperatorID: f.channel, CustomerID: customer, OrderID: "delivered", BusinessKey: "rights", ActorID: f.owner, Reason: "test liability", PaidMoney: amount("80"), Sources: []ledger.GrantSource{{Kind: "paid", Amount: amount("100")}, {Kind: "package_gift", Amount: amount("20")}}, EffectiveAt: time.Now().Add(-time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	_ = g
	if _, e = f.l.ReserveCredits(f.ctx, nil, ledger.CreditReservation{OperatorID: f.channel, CustomerID: customer, BusinessKey: "request", Estimate: amount("20"), At: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReserveProcurement(f.ctx, nil, f.channel, "request", amount("18"), time.Now().Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	c, e := f.s.Coverage(f.ctx, nil, f.channel, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if c.Required.String() != "60" || c.ProcurementAvailable.String() != "82" || c.Space.String() != "22" {
		t.Fatalf("inflight double counted: %+v", c)
	}
	o := commerce.Order{ID: uuid.NewString(), OperatorID: f.channel, PaidATG: amount("30")}
	if _, e = f.s.Reserve(f.ctx, nil, o); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Reserve(f.ctx, nil, o); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Release(f.ctx, nil, o); e != nil {
		t.Fatal(e)
	}
	c, e = f.s.Coverage(f.ctx, nil, f.channel, time.Now())
	if e != nil || c.Space.String() != "22" {
		t.Fatalf("release conservation: %+v %v", c, e)
	}
}

func TestPostgresUnpaidPurchaseHoldsInventoryUntilCancelled(t *testing.T) {
	f := setup(t)
	invite, err := f.org.InviteStaff(f.ctx, nil, f.owner, f.channel, "unpaid-hold@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.AcceptStaffInvitation(f.ctx, nil, invite.Token, organization.VerifiedStaff{ID: f.owner, Email: "unpaid-hold@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Minute)
	quote, err := f.s.PublishPurchaseQuote(f.ctx, nil, f.owner, PurchaseQuote{SupplierID: f.platform, BuyerID: f.channel, Name: "不限时占用", GiftTiers: []PurchaseGiftTier{{FromMoney: ledger.Zero(), GiftPercent: amount("1100")}}, ExitPolicy: "non_refundable", Enabled: true, ValidUntil: &until})
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, quote.ID, "open-ended-hold", amount("10"))
	if err != nil {
		t.Fatal(err)
	}
	if order.ExpiresAt.Year() != 9999 || !order.ExpiresAt.Equal(openPurchaseDeadline) {
		t.Fatal("unpaid purchase still has a payment deadline", order.ExpiresAt)
	}
	var commitment SalesCommitment
	if err = f.db.Where("order_id=?", order.ID).First(&commitment).Error; err != nil || commitment.Status != "active" || commitment.ATG.Cmp(amount("120")) != 0 {
		t.Fatal("inventory was not held", commitment, err)
	}
	if err = f.s.ClosePurchase(f.ctx, nil, f.owner, order.ID, commerce.PaymentClosureProof{}); !errors.Is(err, commerce.ErrPaymentUnknown) {
		t.Fatal("cancel without a reason was accepted", err)
	}
	if err = f.s.ClosePurchase(f.ctx, nil, f.owner, order.ID, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "operator released the unpaid hold"}); err != nil {
		t.Fatal(err)
	}
	var closed PurchaseOrder
	if err = f.db.First(&closed, "id=?", order.ID).Error; err != nil || closed.Status != "closed" {
		t.Fatal(closed, err)
	}
	if err = f.db.First(&commitment, "id=?", commitment.ID).Error; err != nil || commitment.Status != "released" {
		t.Fatal("cancel did not release inventory", commitment, err)
	}
	held, err := f.s.CreatePurchaseOrder(f.ctx, nil, f.owner, f.channel, quote.ID, "paid-still-held", amount("10"))
	if err != nil {
		t.Fatal(err)
	}
	if err = f.db.Model(&PurchaseOrder{}).Where("id=?", held.ID).Update("status", "paid").Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ClosePurchase(f.ctx, nil, f.owner, held.ID, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "cannot cancel after payment"}); !errors.Is(err, commerce.ErrOrderState) {
		t.Fatal("paid order was cancelled", err)
	}
	var still SalesCommitment
	if err = f.db.Where("order_id=?", held.ID).First(&still).Error; err != nil || still.Status != "active" {
		t.Fatal("paid order released inventory early", still, err)
	}
}
