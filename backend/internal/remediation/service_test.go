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
	if e := db.Exec("UPDATE ledger_monetary_settings SET effective_at=clock_timestamp()-interval '1 minute' WHERE id='platform'").Error; e != nil {
		t.Fatal(e)
	}
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
	if _, e = inc.SetPolicy(ctx, nil, owner, p.Operator.ID, incentives.PolicyInput{ThresholdMoney: d("100"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 3600, RewardValiditySeconds: 86400, KOLCrossActivityLimit: 10}); e != nil {
		t.Fatal(e)
	}
	quote, e := com.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: p.Operator.ID, Name: "80+20 monthly", TermsReference: "fixture terms", Timezone: "UTC", PriceMoney: d("80"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
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
	r, e := f.s.Commerce.RecordReceipt(f.ctx, nil, commerce.VerifiedReceipt{OperatorID: f.operator, Provider: "independently_verified_fixture", ProviderReceiptID: key, GrossMoney: gross, NetMoney: &gross, FeeMoney: &zero, FeeStatus: "confirmed", Currency: "USD", OriginalAmount: amount, RecipientReference: "merchant-bank", PayerReference: "customer-bank", ValuationEvidence: "actual USD", VerificationEvidence: "verified bank receipt", VerifiedBy: f.owner, PaidAt: &now, ObservedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func (f fixture) topup(t *testing.T, key, amount string) {
	t.Helper()
	o, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: key, ExpectedOwnershipVersion: 1, AmountMoney: d(amount)})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, key, amount)
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: key, AmountMoney: d(amount)}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal(e)
	}
}
func (f fixture) balance(t *testing.T) ledger.Account {
	t.Helper()
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "customer:money")
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func (f fixture) pay(t *testing.T, leg RefundLeg) {
	t.Helper()
	p := ExecutionProof{BusinessKey: "confirmed-bank-payment", Reference: "bank-" + leg.ID, Evidence: "verified completed transfer", ActualMoney: leg.AmountMoney, FeeMoney: d("1"), FeeStatus: "confirmed", Currency: "USD", Amount: leg.AmountMoney.String(), OccurredAt: time.Now().UTC()}
	if _, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.RecordManualRefund(f.ctx, f.owner, f.operator, leg.ID, p); e != nil {
		t.Fatal("replay", e)
	}
}

// A paid purchase is independently refundable before delivery. Its payment is
// unrelated to any cash the customer has previously deposited into the wallet.
func (f fixture) undeliveredOrder(t *testing.T, key, amount string) commerce.Order {
	t.Helper()
	q, err := f.s.Commerce.PublishPackage(f.ctx, nil, commerce.PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, Name: key, TermsReference: "test purchase", Timezone: "UTC", PriceMoney: d(amount), PaidATG: d(amount), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: q.ID, BusinessKey: key, ExpectedOwnershipVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func (f fixture) requestPurchaseRefund(t *testing.T, key, amount string) (Case, error) {
	t.Helper()
	o := f.undeliveredOrder(t, key, amount)
	r := f.receipt(t, key, amount)
	if _, err := f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: key, AmountMoney: d(amount)}); err != nil {
		t.Fatal(err)
	}
	return f.s.RequestPackageException(f.ctx, PackageExceptionInput{ActorStaffID: f.owner, OperatorID: f.operator, CustomerID: f.customer, OrderID: o.ID, BusinessKey: "refund:" + key, Reason: "unable_to_fulfill", Evidence: "verified non-delivery"})
}
func TestPostgresUnallocatedRefundConcurrentReservationsAndCancel(t *testing.T) {
	f := setup(t)
	f.topup(t, "independent-wallet", "100")
	r := f.receipt(t, "unallocated", "100")
	var wg sync.WaitGroup
	var successes atomic.Int64
	cases := make(chan Case, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := f.s.RequestUnallocatedRefund(f.ctx, f.owner, f.operator, r.ID, uuid.NewString(), "return unallocated", "verified payer", d("30"))
			if err == nil {
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
		if _, err := f.s.CancelRefund(f.ctx, f.owner, f.operator, c.ID, "not paid"); err != nil {
			t.Fatal(err)
		}
	}
	if a := f.balance(t); a.Balance.Cmp(d("100")) != 0 || !a.Held.IsZero() {
		t.Fatal("receipt refunds touched pooled wallet", a)
	}
	cap, err := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, r.ID)
	if err != nil || cap.AvailableMoney.Cmp(d("100")) != 0 {
		t.Fatal(cap, err)
	}
}
func TestPostgresUndeliveredPackageCanBeRefundedAndDeliveredPackageCannot(t *testing.T) {
	f := setup(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "paid-not-delivered", ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "paid-not-delivered", "80")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ActorStaffID: f.owner, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "paid", AmountMoney: d("80")}); e != nil {
		t.Fatal(e)
	}
	c, e := f.s.RequestPackageException(f.ctx, PackageExceptionInput{ActorStaffID: f.owner, OperatorID: f.operator, CustomerID: f.customer, OrderID: o.ID, BusinessKey: "cannot-deliver", Reason: "unable_to_fulfill", Evidence: "verified service permanent closure"})
	if e != nil {
		t.Fatal(e)
	}
	if c.AmountMoney.Cmp(d("80")) != 0 || c.GrantID != nil {
		t.Fatal(c)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); !errors.Is(e, commerce.ErrOrderState) {
		t.Fatal("delivery continued after the refund request", e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified non-delivery"); e != nil {
		t.Fatal(e)
	}
	closed, e := f.s.Commerce.GetOrder(f.ctx, nil, f.customer, o.ID)
	if e != nil || closed.Status != "closed" {
		t.Fatal(closed, e)
	}
	var deliveries int64
	if e = f.db.Table("commerce_deliveries").Where("order_id=?", o.ID).Count(&deliveries).Error; e != nil || deliveries != 0 {
		t.Fatal(deliveries, e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 || legs[0].AmountMoney.Cmp(d("80")) != 0 {
		t.Fatal(legs, e)
	}
	delivered, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "already-delivered", ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	paid := f.receipt(t, "already-delivered", "80")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ActorStaffID: f.owner, ReceiptID: paid.ID, OrderID: delivered.ID, BusinessKey: "delivered-paid", AmountMoney: d("80")}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, delivered.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.RequestPackageException(f.ctx, PackageExceptionInput{ActorStaffID: f.owner, OperatorID: f.operator, CustomerID: f.customer, OrderID: delivered.ID, BusinessKey: "too-late", Reason: "unable_to_fulfill", Evidence: "customer changed their mind"}); !errors.Is(e, commerce.ErrOrderState) {
		t.Fatal("delivered ATG was refunded", e)
	}
}
