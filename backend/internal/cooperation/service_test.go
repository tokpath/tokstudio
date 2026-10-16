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

func setup(t *testing.T) fixture {
	t.Helper()
	db := dbtest.New(t)
	apply := func(_ string, files fs.FS) {
		entries, e := fs.ReadDir(files, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
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
	ctx := context.Background()
	org := organization.New(db)
	l := ledger.New(db)
	owner := uuid.NewString()
	p, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Official", OperatorName: "Platform", OwnerStaffID: owner})
	if e != nil {
		t.Fatal(e)
	}
	c, e := org.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: p.Operator.ID, Kind: organization.Channel, Name: "Channel", QuoteReference: "contract-v1", ExitTermsReference: "exit-v1"})
	if e != nil {
		t.Fatal(e)
	}
	if e = org.SetSupplyStatus(ctx, nil, owner, c.Operator.ID, "active", "signed test agreement"); e != nil {
		t.Fatal(e)
	}
	s := New(db, l, org)
	s.Rates = fixedRates{big.NewRat(1, 1)}
	return fixture{ctx, db, s, org, l, p.Operator.ID, c.Operator.ID, owner}
}
func (f fixture) purchase(t *testing.T, key, usd, paid, gift string) ProcurementLot {
	t.Helper()
	lot, e := f.s.DeliverPurchase(f.ctx, nil, PurchaseInput{BuyerID: f.channel, SupplierID: f.platform, OrderID: key, ReceiptReference: "verified:" + key, ActorID: f.owner, PaidUSD: amount(usd), PaidATG: amount(paid), GiftATG: amount(gift), ExitTerms: json.RawMessage(`{"voluntary_refund":false}`), At: time.Now().UTC()})
	if e != nil {
		t.Fatal(e)
	}
	return lot
}
func TestPostgresProcurementMovingAverageBoundAndSourceConservation(t *testing.T) {
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
	if r.Deducted.String() != "30" || r.PlatformShortfall.String() != "10" || r.CostUSD.String() != "24" {
		t.Fatalf("unexpected bound or cost: %+v", r)
	}
	again, e := f.s.SettleProcurement(f.ctx, nil, f.channel, "r1", h.ID, amount("40"), time.Now())
	if e != nil || again.CostUSD.Cmp(r.CostUSD) != 0 {
		t.Fatalf("idempotent settlement: %+v %v", again, e)
	}
	f.purchase(t, "p2", "60", "100", "0")
	p, e := f.s.Position(f.ctx, nil, f.channel)
	if e != nil {
		t.Fatal(e)
	}
	if p.Quantity.String() != "170" || p.Cost.String() != "116" {
		t.Fatalf("moving average position: %+v", p)
	}
	h, e = f.s.ReserveProcurement(f.ctx, nil, f.channel, "r2", amount("170"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	r, e = f.s.SettleProcurement(f.ctx, nil, f.channel, "r2", h.ID, amount("170"), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if r.CostUSD.String() != "116" {
		t.Fatalf("final cost %s", r.CostUSD)
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
	g, e := f.l.IssueGrant(f.ctx, nil, ledger.GrantInput{OperatorID: f.channel, CustomerID: customer, OrderID: "delivered", BusinessKey: "rights", ActorID: f.owner, Reason: "test liability", PaidUSD: amount("80"), Sources: []ledger.GrantSource{{Kind: "paid", Amount: amount("100")}, {Kind: "package_gift", Amount: amount("20")}}, EffectiveAt: time.Now().Add(-time.Minute)})
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
