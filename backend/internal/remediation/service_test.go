package remediation

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"gorm.io/gorm"
)

func d(v string) ledger.Decimal { return ledger.MustDecimal(v) }

type testReadiness struct{}

func (testReadiness) Check(context.Context, *gorm.DB, organization.Operator, organization.BusinessFeature) ([]organization.Requirement, error) {
	return []organization.Requirement{{Code: "test_dependency", Ready: true}}, nil
}

type fixture struct {
	ctx                       context.Context
	db                        *gorm.DB
	s                         *Service
	owner, operator, customer string
	quote                     commerce.PackageQuote
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := dbtest.New(t)
	apply := func(_ string, sql fs.FS) {
		files, e := fs.ReadDir(sql, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, f := range files {
			b, e := fs.ReadFile(sql, f.Name())
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
	apply(cooperation.Migrations())
	apply(incentives.Migrations())
	apply(Migrations())
	apply(payments.Migrations())
	if e := db.Exec(catalog.CommerceSchema()).Error; e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	org := organization.New(db)
	l := ledger.New(db)
	owner := uuid.NewString()
	p, e := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Test Brand", OperatorName: "Test Platform", OwnerStaffID: owner})
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal, organization.Activity} {
		st, e := org.GetBusinessStatus(ctx, nil, p.Operator.ID)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = org.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: p.Operator.ID, Feature: f, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "isolated fixture dependencies"}, testReadiness{}); e != nil {
			t.Fatal(e)
		}
	}
	intent, e := org.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: p.Brand.ID, Email: "finance@example.test", Password: "only-for-integration-123"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = org.VerifyEmail(ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
		t.Fatal(e)
	}
	reg, e := org.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
	if e != nil {
		t.Fatal(e)
	}
	coop := cooperation.New(db, l, org)
	cat := catalog.CommerceCatalog{DB: db, PlatformOperatorID: p.Operator.ID, Operators: cooperation.CatalogDirectory{Organization: org}}
	coop.Rates = cooperation.CatalogRates{Catalog: cat, Organization: org}
	com := commerce.New(db)
	inc := incentives.New(db)
	com.SetSalesCoverage(coop)
	com.SetMarketing(inc)
	inc.SetRegistrationCoverage(coop)
	if _, e = inc.SetPolicy(ctx, nil, owner, p.Operator.ID, incentives.PolicyInput{ThresholdUSD: d("100"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 3600, RewardValiditySeconds: 86400, KOLCrossActivityLimit: 10}); e != nil {
		t.Fatal(e)
	}
	quote, e := com.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: p.Operator.ID, Name: "80+20 monthly", TermsReference: "fixture terms", Timezone: "UTC", PriceUSD: d("80"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if e != nil {
		t.Fatal(e)
	}
	pay := payments.NewService(db, "isolated-remediation-test-encryption-key", org, payments.NewTransport(true), payments.CommerceSink{Commerce: com})
	s := New(db, Dependencies{Ledger: l, Organization: org, Commerce: com, Cooperation: coop, Incentives: inc, Payments: pay})
	org.SetOperatorExitChecker(s)
	com.SetReceiptGuard(s)
	return fixture{ctx, db, s, owner, p.Operator.ID, reg.Customer.ID, quote}
}
func (f fixture) receipt(t *testing.T, key, amount string) commerce.Receipt {
	t.Helper()
	now := time.Now().Add(-time.Minute)
	gross := d(amount)
	zero := d("0")
	r, e := f.s.Commerce.RecordReceipt(f.ctx, nil, commerce.VerifiedReceipt{OperatorID: f.operator, Provider: "independently_verified_fixture", ProviderReceiptID: key, GrossUSD: gross, NetUSD: &gross, FeeUSD: &zero, FeeStatus: "confirmed", Currency: "USD", OriginalAmount: amount, RecipientReference: "merchant-bank", PayerReference: "customer-bank", ValuationEvidence: "actual USD", VerificationEvidence: "verified bank receipt", VerifiedBy: f.owner, PaidAt: &now, ObservedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func (f fixture) topup(t *testing.T, key, amount string) {
	t.Helper()
	o, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: key, ExpectedOwnershipVersion: 1, AmountUSD: d(amount)})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, key, amount)
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: key, AmountUSD: d(amount)}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal(e)
	}
}
func (f fixture) balance(t *testing.T) ledger.Account {
	t.Helper()
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "customer:usd")
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func (f fixture) pay(t *testing.T, leg RefundLeg) {
	t.Helper()
	if _, e := f.s.BeginManualRefund(f.ctx, f.owner, f.operator, leg.ID, "bank-payment:"+leg.ID, "finance scheduled original payer return"); e != nil {
		t.Fatal(e)
	}
	p := ExecutionProof{BusinessKey: "confirmed-bank-payment", Reference: "bank-" + leg.ID, Evidence: "verified completed transfer", ActualUSD: leg.AmountUSD, FeeUSD: d("1"), Currency: "USD", Amount: leg.AmountUSD.String(), OccurredAt: time.Now().UTC()}
	if _, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); e != nil {
		t.Fatal("replay", e)
	}
}
func TestPostgresWalletRefundPreservesOriginsUnknownAndExactlyOnce(t *testing.T) {
	f := setup(t)
	f.topup(t, "first", "40")
	f.topup(t, "second", "60")
	in := WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "return-70", AmountUSD: d("70"), Reason: "return unused balance"}
	c, e := f.s.RequestWalletRefund(f.ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.s.RequestWalletRefund(f.ctx, in)
	if e != nil || again.ID != c.ID {
		t.Fatal("idempotency", again, e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("100")) != 0 || a.Held.Cmp(d("70")) != 0 {
		t.Fatal(a)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified original payer and unused balance"); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 2 {
		t.Fatal(legs, e)
	}
	f.pay(t, legs[0])
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, legs[1].ID, "bank-pending", "actual bank execution"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.MarkRefundUnknown(f.ctx, f.owner, f.operator, legs[1].ID, "bank response lost"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CancelRefund(f.ctx, f.owner, f.operator, c.ID, "cannot cancel paid case"); !errors.Is(e, ErrState) {
		t.Fatal(e)
	}
	if a := f.balance(t); a.Available().Cmp(d("30")) != 0 {
		t.Fatal(a)
	}
	p := ExecutionProof{BusinessKey: "verified-later", Reference: "bank-final", Evidence: "bank queried original payment", ActualUSD: legs[1].AmountUSD, Currency: "USD", Amount: legs[1].AmountUSD.String(), OccurredAt: time.Now()}
	if _, e = f.s.RecordManualRefund(f.ctx, f.owner, f.operator, legs[1].ID, p); e != nil {
		t.Fatal(e)
	}
	if a := f.balance(t); a.Balance.Cmp(d("30")) != 0 || !a.Held.IsZero() {
		t.Fatal(a)
	}
	done, e := f.s.GetCase(f.ctx, nil, f.operator, c.ID)
	if e != nil || done.State != "completed" {
		t.Fatal(done, e)
	}
	for _, leg := range legs {
		adj, e := f.s.InvoiceReceiptAdjustments(f.ctx, nil, f.operator, []string{leg.ReceiptID})
		if e != nil || len(adj) != 1 || adj[0].RemovedUSD.Cmp(leg.AmountUSD) != 0 || adj[0].ReviewRequired {
			t.Fatal(adj, e)
		}
	}
}
func TestPostgresWalletRefundConcurrentReservationsAndCancel(t *testing.T) {
	f := setup(t)
	f.topup(t, "cash", "100")
	var wg sync.WaitGroup
	var successes atomic.Int64
	cases := make(chan Case, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := f.s.RequestWalletRefund(f.ctx, WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: uuid.NewString(), AmountUSD: d("30"), Reason: "unused"})
			if e == nil {
				successes.Add(1)
				cases <- c
			}
		}()
	}
	wg.Wait()
	close(cases)
	if successes.Load() != 3 {
		t.Fatalf("successes=%d", successes.Load())
	}
	for c := range cases {
		if _, e := f.s.CustomerCancelRefund(f.ctx, f.customer, f.operator, c.ID); e != nil {
			t.Fatal(e)
		}
	}
	if a := f.balance(t); a.Available().Cmp(d("100")) != 0 {
		t.Fatal(a)
	}
}
func TestPostgresPackageExceptionRefundDoesNotRepriceConsumedOrRetainGift(t *testing.T) {
	f := setup(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "discounted", ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "discounted", "80")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ActorStaffID: f.owner, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "paid", AmountUSD: d("80")}); e != nil {
		t.Fatal(e)
	}
	del, e := f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	now, e := instant(f.db)
	if e != nil {
		t.Fatal(e)
	}
	hold, e := f.s.Ledger.ReserveCredits(f.ctx, nil, ledger.CreditReservation{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "usage-before-refund", Estimate: d("25"), At: now, ExpiresAt: now.Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Ledger.SettleCredits(f.ctx, nil, f.operator, f.customer, hold.ID, "usage-before-refund", d("25"), now); e != nil {
		t.Fatal(e)
	}
	c, e := f.s.RequestPackageException(f.ctx, PackageExceptionInput{ActorStaffID: f.owner, OperatorID: f.operator, CustomerID: f.customer, OrderID: o.ID, BusinessKey: "unfulfilled", Reason: "unable_to_fulfill", Evidence: "verified service permanent closure"})
	if e != nil {
		t.Fatal(e)
	}
	if c.AmountUSD.Cmp(d("60")) != 0 || c.CreditATG.Cmp(d("75")) != 0 {
		t.Fatal(c)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified non-fulfillment"); e != nil {
		t.Fatal(e)
	}
	g, e := f.s.Ledger.GetGrant(f.ctx, nil, *del.GrantID)
	if e != nil || g.Cancelled.Cmp(d("75")) != 0 || g.CancelledPaidUSD.Cmp(d("60")) != 0 || g.ConsumedPaidUSD.Cmp(d("20")) != 0 {
		t.Fatal(g, e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 {
		t.Fatal(legs, e)
	}
	f.pay(t, legs[0])
	a, e := f.s.Ledger.AccountInTransaction(f.ctx, f.db, g.AccountID)
	if e != nil || !a.Balance.IsZero() {
		t.Fatal(a, e)
	}
}
