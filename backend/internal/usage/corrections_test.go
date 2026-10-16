package usage

import (
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func correctionFixture(t *testing.T) (fixture, *incentives.Service, Request) {
	t.Helper()
	f := setup(t, "0.01", nil)
	_, migrations := incentives.Migrations()
	entries, e := fs.ReadDir(migrations, ".")
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		body, e := fs.ReadFile(migrations, entry.Name())
		if e != nil {
			t.Fatal(e)
		}
		if e = f.db.Exec(string(body)).Error; e != nil {
			t.Fatal(e)
		}
	}
	// Exhaust the fixture's opening grant before connecting the real observer:
	// the tested consumption must originate solely from an actual paid order.
	if e = f.db.Transaction(func(tx *gorm.DB) error {
		var at time.Time
		if e := tx.Raw("SELECT clock_timestamp()").Scan(&at).Error; e != nil {
			return e
		}
		h, e := f.l.ReserveCredits(f.ctx, tx, ledger.CreditReservation{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "drain-fixture", Estimate: d("0.01"), At: at, ExpiresAt: at.Add(time.Hour)})
		if e != nil {
			return e
		}
		_, e = f.l.SettleCredits(f.ctx, tx, f.operator, f.customer, h.ID, "drain-fixture", d("0.01"), at)
		return e
	}); e != nil {
		t.Fatal(e)
	}
	for _, feature := range []organization.BusinessFeature{organization.NewPurchase, organization.Activity} {
		st, e := f.org.GetBusinessStatus(f.ctx, nil, f.operator)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.org.SetBusinessStatus(f.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: f.owner, OperatorID: f.operator, Feature: feature, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "correction fixture"}, ready{}); e != nil {
			t.Fatal(e)
		}
	}
	inc := incentives.New(f.db)
	inc.SetRegistrationCoverage(f.coop)
	if _, e = inc.SetPolicy(f.ctx, nil, f.owner, f.operator, incentives.PolicyInput{ThresholdUSD: d("50"), RewardValiditySeconds: 3600, KOLCrossActivityLimit: 1}); e != nil {
		t.Fatal(e)
	}
	c := commerce.New(f.db)
	c.SetSalesCoverage(f.coop)
	c.SetMarketing(inc)
	q, e := c.PublishPackage(f.ctx, nil, commerce.PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, Name: "Mixed paid and gift", TermsReference: "correction-terms", Timezone: "UTC", PriceUSD: d("80"), PaidATG: d("0.5"), GiftATG: d("0.2"), Period: ledger.Month, NewPurchaseEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	o, e := c.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: q.ID, BusinessKey: "actual-funded-order", ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	gross, zero := d("80"), ledger.Zero()
	r, e := c.RecordReceipt(f.ctx, nil, commerce.VerifiedReceipt{OperatorID: f.operator, Provider: "verified-test-bank", ProviderReceiptID: "actual-paid-receipt", GrossUSD: gross, NetUSD: &gross, FeeUSD: &zero, Currency: "USD", OriginalAmount: "80", RecipientReference: "merchant", PayerReference: "external-payer", ValuationEvidence: "USD original", VerificationEvidence: "queried settled receipt", VerifiedBy: "test:bank", PaidAt: &o.CreatedAt})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "fund-order", AmountUSD: gross}); e != nil {
		t.Fatal(e)
	}
	if _, e = c.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	f.s.GrantObserver = inc
	req, _, e := f.s.Admit(f.ctx, f.key.Key, "actual-usage", request(t, false))
	if e != nil {
		t.Fatal(e)
	}
	req, e = f.s.Settle(f.ctx, nil, req.ID, measured("400", "400"))
	if e != nil {
		t.Fatal(e)
	}
	if req.ActualATG.Cmp(d("0.8")) != 0 || req.ChargedATG.Cmp(d("0.7")) != 0 {
		t.Fatalf("unexpected charge %+v", req)
	}
	return f, inc, req
}

func TestPostgresChargeCorrectionConservesActualFundingAndOriginalKeyBudget(t *testing.T) {
	f, inc, req := correctionFixture(t)
	q, e := inc.GetQualification(f.ctx, nil, f.operator, f.customer)
	if e != nil || q.ConfirmedUSD.Cmp(d("80")) != 0 || q.AcquiredAt == nil {
		t.Fatalf("qualification before correction=%+v %v", q, e)
	}
	reset, e := f.s.ResetKey(f.ctx, f.customer, f.key.Key.ID, "rotate-before-correction", 1)
	if e != nil {
		t.Fatal(e)
	}
	input := ChargeCorrectionInput{OperatorID: f.operator, RequestID: req.ID, BusinessKey: "shortfall-only", ActorID: f.owner, Evidence: "verified retail mischarge", AmountATG: d("0.05")}
	first, e := f.s.CorrectCustomerCharge(f.ctx, nil, input, inc)
	if e != nil || !first.ReturnedATG.IsZero() || !first.PaidUSD.IsZero() {
		t.Fatalf("merchant shortfall refunded customer=%+v %v", first, e)
	}
	input.BusinessKey, input.AmountATG = "partial-return", d("0.15")
	partial, e := f.s.CorrectCustomerCharge(f.ctx, nil, input, inc)
	if e != nil || partial.ReturnedATG.Cmp(d("0.1")) != 0 {
		t.Fatalf("actual customer return=%+v %v", partial, e)
	}
	again, e := f.s.CorrectCustomerCharge(f.ctx, nil, input, inc)
	if e != nil || again.ID != partial.ID {
		t.Fatalf("duplicate correction=%+v %v", again, e)
	}
	input.AmountATG = d("0.14")
	if _, e = f.s.CorrectCustomerCharge(f.ctx, nil, input, inc); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed duplicate accepted: %v", e)
	}
	input.BusinessKey, input.AmountATG = "final-return", d("0.6")
	last, e := f.s.CorrectCustomerCharge(f.ctx, nil, input, inc)
	if e != nil || last.ReturnedATG.Cmp(d("0.6")) != 0 || partial.PaidUSD.Add(last.PaidUSD).Cmp(d("80")) != 0 {
		t.Fatalf("paid discount attribution lost=%+v / %+v %v", partial, last, e)
	}
	q, e = inc.GetQualification(f.ctx, nil, f.operator, f.customer)
	if e != nil || !q.ConfirmedUSD.IsZero() || q.AcquiredAt == nil {
		t.Fatalf("correction erased acquisition or retained consumption=%+v %v", q, e)
	}
	b, e := f.s.KeyBudget(f.ctx, nil, reset.Key, time.Now())
	if e != nil || !b.SpentATG.IsZero() {
		t.Fatalf("rotated key original usage budget=%+v %v", b, e)
	}
	grants, e := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if e != nil {
		t.Fatal(e)
	}
	var available ledger.Decimal
	for _, g := range grants {
		a, e := f.l.AccountInTransaction(f.ctx, nil, g.AccountID)
		if e != nil {
			t.Fatal(e)
		}
		available = available.Add(a.Available())
	}
	if available.Cmp(d("0.7")) != 0 {
		t.Fatalf("customer rights minted/lost=%s", available)
	}
}

func TestPostgresConcurrentChargeCorrectionsCannotOverReturn(t *testing.T) {
	f, inc, req := correctionFixture(t)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, key := range []string{"correction-a", "correction-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			_, e := f.s.CorrectCustomerCharge(f.ctx, nil, ChargeCorrectionInput{OperatorID: f.operator, RequestID: req.ID, BusinessKey: key, ActorID: f.owner, Evidence: "independent concurrent correction", AmountATG: d("0.6")}, inc)
			errs <- e
		}(key)
	}
	wg.Wait()
	close(errs)
	ok, conflict := 0, 0
	for e := range errs {
		if e == nil {
			ok++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("concurrent results successes=%d conflicts=%d", ok, conflict)
	}
	var final Request
	if e := f.db.First(&final, "id=?", req.ID).Error; e != nil {
		t.Fatal(e)
	}
	if final.CorrectedATG.Cmp(d("0.6")) != 0 || final.CorrectedChargedATG.Cmp(d("0.5")) != 0 {
		t.Fatalf("concurrent over-return=%+v", final)
	}
}
