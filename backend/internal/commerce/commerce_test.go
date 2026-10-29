package commerce

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func d(s string) ledger.Decimal   { return ledger.MustDecimal(s) }
func dp(s string) *ledger.Decimal { v := d(s); return &v }
func TestRenewalConsentComparison(t *testing.T) {
	old := PackageQuote{PackageID: "package", OperatorID: "operator", PriceMoney: d("80"), PaidATG: d("100"), GiftATG: d("20"), Period: ledger.Month, Timezone: "Asia/Shanghai", TermsReference: "terms-v1"}
	for _, change := range []func(*PackageQuote){func(q *PackageQuote) { q.PriceMoney = d("81"); q.PaidATG = d("200") }, func(q *PackageQuote) { q.PaidATG = d("99") }, func(q *PackageQuote) { q.GiftATG = d("19") }, func(q *PackageQuote) { q.TermsReference = "changed" }, func(q *PackageQuote) { q.Timezone = "UTC" }} {
		next := old
		change(&next)
		if !RequiresRenewalConfirmation(old, next) {
			t.Fatalf("worse terms accepted: %+v", next)
		}
	}
	better := old
	better.PriceMoney = d("75")
	better.PaidATG = d("110")
	if RequiresRenewalConfirmation(old, better) {
		t.Fatal("strict improvement requires consent")
	}
	if RequiresRenewalConfirmation(old, old) {
		t.Fatal("unchanged terms require consent")
	}
}

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TOKENHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TOKENHUB_TEST_DATABASE_URL not set; real PostgreSQL integration was not run")
	}
	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	baseSQL, err := base.DB()
	if err != nil {
		t.Fatal(err)
	}
	schema := "commerce_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(16)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`).Error; err != nil {
			t.Error(err)
		}
		_ = baseSQL.Close()
	})
	_, o := organization.Migrations()
	_, l := ledger.Migrations()
	_, c := Migrations()
	for _, m := range []fs.FS{o, l, c} {
		entries, err := fs.ReadDir(m, ".")
		if err != nil {
			t.Fatal(err)
		}
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			body, err := fs.ReadFile(m, name)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(body)).Error }); err != nil {
				t.Fatalf("migration %s: %v", name, err)
			}
		}
	}
	return db
}

// capacityDouble deliberately tests only commerce's transactional adapter
// contract. Production sales coverage is implemented by cooperation, not here.
type capacityDouble struct{ failFulfill atomic.Bool }

// Commerce-only fixtures isolate the provenance adapter contract. Real
// commission entry conservation is exercised by incentives/remediation suites.
type commissionFundingDouble struct{}

func (commissionFundingDouble) ReserveCommissionFunding(context.Context, *gorm.DB, string, string, string, ledger.Decimal) error {
	return nil
}
func (commissionFundingDouble) CompleteCommissionFunding(context.Context, *gorm.DB, string, string, string) error {
	return nil
}
func (commissionFundingDouble) ReleaseCommissionFunding(context.Context, *gorm.DB, string, string, string) error {
	return nil
}
func (commissionFundingDouble) RequireCommissionFundingExecution(context.Context, *gorm.DB, string, string, string, string) error {
	return nil
}
func (commissionFundingDouble) RestoreCommissionFunding(context.Context, *gorm.DB, string, string, string, string, ledger.Decimal) error {
	return nil
}

func (c *capacityDouble) Reserve(ctx context.Context, tx *gorm.DB, o Order) (Commitment, error) {
	amount := orderTotalATG(o)
	r := tx.Exec(`UPDATE test_sales_capacity SET available=available-? WHERE id=1 AND available>=?`, amount, amount)
	if r.Error != nil {
		return Commitment{}, r.Error
	}
	if r.RowsAffected != 1 {
		return Commitment{}, errors.New("test coverage exhausted")
	}
	if err := tx.Exec(`INSERT INTO test_sales_commitments(id,amount,status) VALUES(?,?,'reserved')`, o.ID, amount).Error; err != nil {
		return Commitment{}, err
	}
	return Commitment{Reference: o.ID, Version: 1}, nil
}
func (c *capacityDouble) Restore(ctx context.Context, tx *gorm.DB, o Order) (Commitment, error) {
	amount := orderTotalATG(o)
	if err := tx.Exec(`UPDATE test_sales_capacity SET available=available-? WHERE id=1`, amount).Error; err != nil {
		return Commitment{}, err
	}
	if err := tx.Exec(`UPDATE test_sales_commitments SET status='reserved' WHERE id=? AND status='released'`, o.ID).Error; err != nil {
		return Commitment{}, err
	}
	return Commitment{Reference: o.ID, Version: 2}, nil
}
func (c *capacityDouble) Fulfill(ctx context.Context, tx *gorm.DB, o Order) error {
	if c.failFulfill.Load() {
		return errors.New("test injected delivery failure")
	}
	return tx.Exec(`UPDATE test_sales_commitments SET status='fulfilled' WHERE id=?`, o.ID).Error
}
func (c *capacityDouble) Release(ctx context.Context, tx *gorm.DB, o Order) error {
	if err := tx.Exec(`UPDATE test_sales_capacity SET available=available+? WHERE id=1`, orderTotalATG(o)).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE test_sales_commitments SET status='released' WHERE id=?`, o.ID).Error
}

type ready struct{}

func (ready) Check(context.Context, *gorm.DB, organization.Operator, organization.BusinessFeature) ([]organization.Requirement, error) {
	return []organization.Requirement{{Code: "test_fixture_dependencies", Ready: true}}, nil
}

type fixture struct {
	ctx                              context.Context
	db                               *gorm.DB
	s                                *Service
	org                              *organization.Service
	ledger                           *ledger.Service
	coverage                         *capacityDouble
	operator, brand, owner, customer string
	quote                            PackageQuote
}

func setup(t *testing.T) fixture {
	t.Helper()
	db := integrationDB(t)
	ctx := context.Background()
	org := organization.New(db)
	owner := uuid.NewString()
	p, err := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Brand", OperatorName: "Platform", OwnerStaffID: owner})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Renewal} {
		st, err := org.GetBusinessStatus(ctx, nil, p.Operator.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = org.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: p.Operator.ID, Feature: f, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "test fixture opening"}, ready{}); err != nil {
			t.Fatal(err)
		}
	}
	v, err := org.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: p.Brand.ID, Email: "customer@example.test", Password: "test-only-strong-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = org.VerifyEmail(ctx, nil, v.Intent.ID, v.VerificationToken); err != nil {
		t.Fatal(err)
	}
	customer, err := org.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE test_sales_capacity(id INT PRIMARY KEY,available NUMERIC(38,18)); INSERT INTO test_sales_capacity VALUES(1,100000); CREATE TABLE test_sales_commitments(id UUID PRIMARY KEY,amount NUMERIC(38,18),status TEXT);`).Error; err != nil {
		t.Fatal(err)
	}
	s := New(db)
	s.SetCommissionFunding(commissionFundingDouble{})
	cap := &capacityDouble{}
	s.SetSalesCoverage(cap)
	quote, err := s.PublishPackage(ctx, nil, PublishPackageInput{ActorStaffID: owner, OperatorID: p.Operator.ID, Name: "Monthly", TermsReference: "terms-v1", Timezone: "UTC", PriceMoney: d("80"), PaidATG: d("100"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{ctx: ctx, db: db, s: s, org: org, ledger: ledger.New(db), coverage: cap, operator: p.Operator.ID, brand: p.Brand.ID, owner: owner, customer: customer.Customer.ID, quote: quote}
}
func (f fixture) order(t *testing.T, key, usd, commission string) Order {
	t.Helper()
	o, err := f.s.CreateOrder(f.ctx, nil, CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: key, ExpectedOwnershipVersion: 1, InternalMoney: d(usd), CommissionMoney: d(commission)})
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func (f fixture) fact(key, amount, fee string, paid time.Time) VerifiedReceipt {
	return VerifiedReceipt{OperatorID: f.operator, Provider: "test-verified-provider", ProviderReceiptID: key, GrossMoney: d(amount), NetMoney: dp(d(amount).Sub(d(fee)).String()), FeeMoney: dp(fee), Currency: "USD", OriginalAmount: amount, RecipientReference: "merchant-account-1", PayerReference: "external-payer-1", ValuationEvidence: "actual USD payment", VerificationEvidence: "test independently verified receipt", VerifiedBy: "provider:test", PaidAt: &paid}
}
func (f fixture) receipt(t *testing.T, key, amount, fee string, paid time.Time) Receipt {
	t.Helper()
	r, err := f.s.RecordReceipt(f.ctx, nil, f.fact(key, amount, fee, paid))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f fixture) allocate(t *testing.T, key string, r Receipt, o Order, amount string) Allocation {
	t.Helper()
	a, err := f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, BusinessKey: key, ReceiptID: r.ID, OrderID: o.ID, AmountMoney: d(amount)})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func (f fixture) seed(t *testing.T, purpose, amount string) {
	t.Helper()
	if purpose == "customer:money" {
		key := "wallet-fixture:" + uuid.NewString()
		o, err := f.s.CreateTopUpOrder(f.ctx, nil, CreateTopUpInput{CustomerID: f.customer, OperatorID: f.operator, BusinessKey: key, ExpectedOwnershipVersion: 1, AmountMoney: d(amount)})
		if err != nil {
			t.Fatal(err)
		}
		r := f.receipt(t, key, amount, "0", o.CreatedAt)
		f.allocate(t, key, r, o, amount)
		if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "test:verified-wallet-funding"); err != nil {
			t.Fatal(err)
		}
		return
	}
	a, err := f.s.account(f.ctx, nil, f.operator, f.customer, purpose)
	if err != nil {
		t.Fatal(err)
	}
	control, err := f.s.account(f.ctx, nil, f.operator, f.operator, "control:test-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.ledger.Post(f.ctx, nil, ledger.Posting{OperatorID: f.operator, BusinessKey: "fixture:" + uuid.NewString(), Kind: "test_fixture", SourceID: uuid.NewString(), ActorID: "test:fixture", Reason: "explicit test opening funds", Lines: []ledger.Line{{AccountID: a.ID, Amount: d(amount)}, {AccountID: control.ID, Amount: d(amount).Neg()}}}); err != nil {
		t.Fatal(err)
	}
}
func (f fixture) balance(t *testing.T, purpose string) ledger.Account {
	t.Helper()
	a, err := f.s.account(f.ctx, nil, f.operator, f.customer, purpose)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPostgresDirectReceiptDeliveryFailureRetryAndImmutableQuote(t *testing.T) {
	f := setup(t)
	base := time.Date(2030, 1, 31, 12, 0, 0, 0, time.UTC)
	f.s.databaseFulfillmentClock = false
	f.s.now = func() time.Time { return base }
	o := f.order(t, "direct", "0", "0")
	r1 := f.receipt(t, "first", "30", "3", base)
	r2 := f.receipt(t, "second", "80", "0", base)
	if _, err := f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, BusinessKey: "first-allocation", ReceiptID: r1.ID, OrderID: o.ID, AmountMoney: d("30")}); !errors.Is(err, ErrWholePayment) {
		t.Fatalf("partial payment accepted: %v", err)
	}
	partial, err := f.s.GetOrder(f.ctx, nil, f.customer, o.ID)
	if err != nil || partial.Status != "pending" || !partial.ExternalAllocatedMoney.IsZero() {
		t.Fatalf("partial=%+v %v", partial, err)
	}
	if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "worker:delivery"); !errors.Is(err, ErrNotPaid) {
		t.Fatalf("partial order delivered: %v", err)
	}
	f.allocate(t, "second-allocation", r2, o, "80")
	f.coverage.failFulfill.Store(true)
	if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "worker:delivery"); err == nil {
		t.Fatal("injected delivery failure ignored")
	}
	paid, err := f.s.GetOrder(f.ctx, nil, f.customer, o.ID)
	if err != nil || paid.Status != "paid" {
		t.Fatalf("paid responsibility lost: %+v %v", paid, err)
	}
	grants, err := f.ledger.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if err != nil || len(grants) != 0 {
		t.Fatalf("partial grant persisted: %+v %v", grants, err)
	}
	_, err = f.s.PublishPackage(f.ctx, nil, PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, PackageID: f.quote.PackageID, ExpectedVersion: 1, Name: "Monthly", TermsReference: "terms-v2", Timezone: "UTC", PriceMoney: d("120"), PaidATG: d("90"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	f.coverage.failFulfill.Store(false)
	delivery, err := f.s.Fulfill(f.ctx, nil, o.ID, "worker:delivery")
	if err != nil {
		t.Fatal(err)
	}
	if delivery.ExpiresAt == nil || delivery.ExpiresAt.Day() != 28 || delivery.ExpiresAt.Month() != time.February {
		t.Fatalf("calendar expiry=%v", delivery.ExpiresAt)
	}
	grants, err = f.ledger.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if err != nil || len(grants) != 1 || grants[0].Total.Cmp(d("120")) != 0 || grants[0].PaidMoney.Cmp(d("80")) != 0 {
		t.Fatalf("original sold terms lost: %+v %v", grants, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			again, err := f.s.Fulfill(f.ctx, nil, o.ID, "worker:delivery")
			if err == nil && again.ID != delivery.ID {
				err = ErrConflict
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if a := f.balance(t, "customer:money"); !a.Balance.IsZero() {
		t.Fatalf("direct payment credited wallet: %s", a.Balance)
	}
	replay, err := f.s.RecordReceipt(f.ctx, nil, f.fact("first", "30", "3", base))
	if err != nil || replay.ID != r1.ID {
		t.Fatalf("receipt replay=%+v %v", replay, err)
	}
	if _, err = f.s.RecordReceipt(f.ctx, nil, f.fact("first", "31", "3", base)); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt mutation accepted: %v", err)
	}
	if err = f.db.Exec(`UPDATE commerce_package_quotes SET price_money=1 WHERE id=?`, f.quote.ID).Error; err == nil {
		t.Fatal("database rewrote sold quote")
	}
}

func TestPostgresMixedInternalAndExternalFundsCaptureOnce(t *testing.T) {
	f := setup(t)
	f.seed(t, "customer:money", "30")
	f.seed(t, "commission:available", "20")
	f.seed(t, "commission:frozen", "500")
	o := f.order(t, "mixed", "30", "20")
	if a := f.balance(t, "customer:money"); a.Balance.Cmp(d("30")) != 0 || a.Held.Cmp(d("30")) != 0 {
		t.Fatalf("wallet reservation=%+v", a)
	}
	r := f.receipt(t, "payer-receipt", "30", "2", o.CreatedAt)
	f.allocate(t, "allocate-mixed", r, o, "30")
	if _, err := f.s.Fulfill(f.ctx, nil, o.ID, "worker:delivery"); err != nil {
		t.Fatal(err)
	}
	for _, purpose := range []string{"customer:money", "commission:available"} {
		a := f.balance(t, purpose)
		if !a.Balance.IsZero() || !a.Held.IsZero() {
			t.Fatalf("funding source %s not captured: %+v", purpose, a)
		}
	}
	if frozen := f.balance(t, "commission:frozen"); frozen.Balance.Cmp(d("500")) != 0 {
		t.Fatal("purchase used frozen commission")
	}
	_, err := f.s.CreateOrder(f.ctx, nil, CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: "insufficient", ExpectedOwnershipVersion: 1, InternalMoney: d("1")})
	if !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatalf("insufficient internal funding: %v", err)
	}
	var count int64
	if err = f.db.Table("commerce_orders").Where("business_key='insufficient'").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("failed order persisted: %d %v", count, err)
	}
}

func TestPostgresReceiptAllocationConservationAndScope(t *testing.T) {
	f := setup(t)
	a := f.order(t, "a", "0", "0")
	b := f.order(t, "b", "0", "0")
	r := f.receipt(t, "split", "100", "0", a.CreatedAt)
	f.allocate(t, "a-80", r, a, "80")
	if _, err := f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: "b-20", ReceiptID: r.ID, OrderID: b.ID, AmountMoney: d("20")}); !errors.Is(err, ErrWholePayment) {
		t.Fatalf("one receipt split across orders: %v", err)
	}
	replay, err := f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: "a-80", ReceiptID: r.ID, OrderID: a.ID, AmountMoney: d("80")})
	if err != nil || replay.AmountMoney.Cmp(d("80")) != 0 {
		t.Fatalf("allocation replay=%+v %v", replay, err)
	}
	r2 := f.receipt(t, "concurrent", "80", "0", a.CreatedAt)
	c := f.order(t, "c", "0", "0")
	e := f.order(t, "e", "0", "0")
	var successes atomic.Int64
	errorsCh := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, order := range []Order{c, e} {
		wg.Add(1)
		go func(order Order) {
			defer wg.Done()
			<-start
			_, err := f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: order.ID, ReceiptID: r2.ID, OrderID: order.ID, AmountMoney: d("80")})
			if err == nil {
				successes.Add(1)
			}
			errorsCh <- err
		}(order)
	}
	close(start)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil && !errors.Is(err, ErrExcessAllocation) {
			t.Fatal(err)
		}
	}
	if successes.Load() != 1 {
		t.Fatalf("concurrent receipt allocated %d times", successes.Load())
	}
	shared := f.receipt(t, "shared-procurement", "80", "0", a.CreatedAt)
	destination := uuid.NewString()
	reserve := ReceiptFundsInput{OperatorID: f.operator, ReceiptID: shared.ID, DestinationID: destination, BusinessKey: "procurement-shared", ActorID: "system:procurement", AmountMoney: d("80"), Deadline: a.ExpiresAt}
	injected := errors.New("procurement update failed")
	err = f.db.Transaction(func(tx *gorm.DB) error {
		if _, err := f.s.ReserveReceiptFunds(f.ctx, tx, reserve); err != nil {
			return err
		}
		return injected
	})
	if !errors.Is(err, injected) {
		t.Fatal(err)
	}
	unchanged, err := f.s.GetReceipt(f.ctx, nil, f.operator, shared.ID)
	if err != nil || !unchanged.AllocatedMoney.IsZero() {
		t.Fatalf("external transaction rollback lost receipt funds: %+v %v", unchanged, err)
	}
	if err = f.db.Transaction(func(tx *gorm.DB) error { _, err := f.s.ReserveReceiptFunds(f.ctx, tx, reserve); return err }); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: "shared-customer", ReceiptID: shared.ID, OrderID: b.ID, AmountMoney: d("80")}); !errors.Is(err, ErrExcessAllocation) {
		t.Fatal("procurement receipt reused by retail", err)
	}
	counted, err := f.s.GetReceipt(f.ctx, nil, f.operator, shared.ID)
	if err != nil || counted.AllocatedMoney.Cmp(d("80")) != 0 {
		t.Fatalf("external then retail shared receipt total=%+v %v", counted, err)
	}
	reverse := f.receipt(t, "retail-first-shared", "80", "0", a.CreatedAt)
	reverseOrder := f.order(t, "retail-first-destination", "0", "0")
	f.allocate(t, "retail-first-80", reverse, reverseOrder, "80")
	reverseReserve := reserve
	reverseReserve.ReceiptID = reverse.ID
	reverseReserve.BusinessKey = "retail-first-external"
	if err = f.db.Transaction(func(tx *gorm.DB) error { _, e := f.s.ReserveReceiptFunds(f.ctx, tx, reverseReserve); return e }); !errors.Is(err, ErrExcessAllocation) {
		t.Fatal("retail receipt reused by procurement", err)
	}
	counted, err = f.s.GetReceipt(f.ctx, nil, f.operator, reverse.ID)
	if err != nil || counted.AllocatedMoney.Cmp(d("80")) != 0 {
		t.Fatalf("retail then external shared receipt total=%+v %v", counted, err)
	}
	reserve.BusinessKey = "procurement-double-spend"
	reserve.AmountMoney = d("1")
	err = f.db.Transaction(func(tx *gorm.DB) error { _, err := f.s.ReserveReceiptFunds(f.ctx, tx, reserve); return err })
	if !errors.Is(err, ErrExcessAllocation) {
		t.Fatalf("customer and procurement double spent receipt: %v", err)
	}
	child, err := f.org.CreateOperator(f.ctx, nil, organization.CreateOperatorInput{ActorStaffID: f.owner, ParentOperatorID: f.operator, Kind: organization.Channel, Name: "Other operator"})
	if err != nil {
		t.Fatal(err)
	}
	foreign := f.fact("foreign", "80", "0", a.CreatedAt)
	foreign.OperatorID = child.Operator.ID
	foreignReceipt, err := f.s.RecordReceipt(f.ctx, nil, foreign)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: "cross", ReceiptID: foreignReceipt.ID, OrderID: b.ID, AmountMoney: d("1")}); !errors.Is(err, ErrScope) {
		t.Fatalf("cross-operator money mixed: %v", err)
	}
}

func TestPostgresTimelyLateNotificationRestoresAndActualLatePaymentWaits(t *testing.T) {
	f := setup(t)
	base := time.Now().UTC()
	f.s.databaseFulfillmentClock = false
	f.s.now = func() time.Time { return base }
	a := f.order(t, "delayed-notice", "0", "0")
	b := f.order(t, "late-money", "0", "0")
	if _, err := f.s.CloseOrder(f.ctx, nil, f.customer, a.ID, PaymentClosureProof{}); !errors.Is(err, ErrPaymentUnknown) {
		t.Fatalf("closed from missing callback: %v", err)
	}
	base = a.ExpiresAt.Add(time.Hour)
	for _, o := range []Order{a, b} {
		if _, err := f.s.CloseOrder(f.ctx, nil, f.customer, o.ID, PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "provider reconciled unpaid"}); err != nil {
			t.Fatal(err)
		}
	}
	timely := f.receipt(t, "timely", "80", "0", a.ExpiresAt.Add(-time.Second))
	f.allocate(t, "timely-allocate", timely, a, "80")
	if _, err := f.s.Fulfill(f.ctx, nil, a.ID, "worker:delivery"); err != nil {
		t.Fatal(err)
	}
	late := f.receipt(t, "late", "80", "0", b.ExpiresAt.Add(time.Second))
	if _, err := f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: "late-allocate", ReceiptID: late.ID, OrderID: b.ID, AmountMoney: d("80")}); !errors.Is(err, ErrLatePayment) {
		t.Fatalf("actual late money silently sold: %v", err)
	}
	left, err := f.s.UnallocatedReceipts(f.ctx, nil, f.owner, f.operator)
	if err != nil || len(left) != 1 || left[0].ID != late.ID {
		t.Fatalf("late receipt responsibility lost: %+v %v", left, err)
	}
}

func TestPostgresTopupAndMissingSalesCoverage(t *testing.T) {
	f := setup(t)
	topup, err := f.s.CreateTopUpOrder(f.ctx, nil, CreateTopUpInput{CustomerID: f.customer, OperatorID: f.operator, BusinessKey: "topup", ExpectedOwnershipVersion: 1, AmountMoney: d("50")})
	if err != nil {
		t.Fatal(err)
	}
	r := f.receipt(t, "topup", "50", "3", topup.CreatedAt)
	f.allocate(t, "topup", r, topup, "50")
	delivery, err := f.s.Fulfill(f.ctx, nil, topup.ID, "worker:delivery")
	if err != nil || delivery.GrantID != nil {
		t.Fatalf("topup delivery=%+v %v", delivery, err)
	}
	if a := f.balance(t, "customer:money"); a.Balance.Cmp(d("50")) != 0 {
		t.Fatalf("net fee reduced wallet credit: %s", a.Balance)
	}
	f.s.SetSalesCoverage(nil)
	if _, err = f.s.CreateOrder(f.ctx, nil, CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: "without-coverage", ExpectedOwnershipVersion: 1}); !errors.Is(err, ErrNotReady) {
		t.Fatalf("coverage unavailable sale accepted: %v", err)
	}
}

func TestPostgresPendingFeeAndObservedPaymentUpperBound(t *testing.T) {
	f := setup(t)
	base := time.Now().UTC().Truncate(time.Microsecond)
	f.s.databaseFulfillmentClock = false
	f.s.now = func() time.Time { return base }
	a := f.order(t, "observed-on-time", "0", "0")
	b := f.order(t, "observed-too-late", "0", "0")
	fact := f.fact("fee-pending", "80", "0", base)
	fact.FeeStatus = "pending"
	fact.NetMoney = nil
	fact.FeeMoney = nil
	fact.PaidAt = nil
	fact.ObservedAt = base
	r, err := f.s.RecordReceipt(f.ctx, nil, fact)
	if err != nil || r.NetMoney != nil || r.FeeMoney != nil || r.PaidAt != nil || r.FeeStatus != "pending" {
		t.Fatalf("unknown values falsified: %+v %v", r, err)
	}
	allocation := f.allocate(t, "observed-bound", r, a, "80")
	if allocation.TimingProof != "observed_before_expiry" {
		t.Fatalf("timing basis hidden: %+v", allocation)
	}
	if _, err = f.s.Fulfill(f.ctx, nil, a.ID, "worker:delivery"); err != nil {
		t.Fatalf("pending merchant fees blocked customer delivery: %v", err)
	}
	settled := base.Add(time.Second)
	base = settled
	accounting := ReceiptAccounting{SettlementFeeConfirmed: true, ReceiptID: r.ID, NetMoney: d("77"), FeeMoney: d("3"), SettledAt: &settled, Evidence: "verified settlement statement", VerifiedBy: "provider:test"}
	confirmed, err := f.s.RecordReceiptAccounting(f.ctx, nil, accounting)
	if err != nil || confirmed.FeeStatus != "confirmed" || confirmed.FeeMoney == nil || confirmed.FeeMoney.Cmp(d("3")) != 0 {
		t.Fatalf("late fees=%+v %v", confirmed, err)
	}
	if _, err = f.s.RecordReceiptAccounting(f.ctx, nil, accounting); err != nil {
		t.Fatal(err)
	}
	var feeJournals int64
	if err = f.db.Table("ledger_journals").Where("business_key=?", "receipt-fees:"+r.ID).Count(&feeJournals).Error; err != nil || feeJournals != 1 {
		t.Fatalf("late fee charged repeatedly: %d %v", feeJournals, err)
	}
	receivable, err := f.s.account(f.ctx, nil, f.operator, f.operator, "control:provider_receivable")
	if err != nil || !receivable.Balance.IsZero() {
		t.Fatalf("settlement receivable=%+v %v", receivable, err)
	}
	base = b.ExpiresAt.Add(time.Minute)
	fact = f.fact("time-unknown", "80", "0", base)
	fact.PaidAt = nil
	fact.ObservedAt = base
	late, err := f.s.RecordReceipt(f.ctx, nil, fact)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AllocateReceipt(f.ctx, nil, AllocateReceiptInput{OperatorID: f.operator, BusinessKey: "unknown-timing", ReceiptID: late.ID, OrderID: b.ID, AmountMoney: d("80")}); !errors.Is(err, ErrTimingUnverified) {
		t.Fatalf("late observation invented paid time: %v", err)
	}
	if _, err = f.s.ConfirmReceiptPaymentTime(f.ctx, nil, late.ID, b.ExpiresAt.Add(-time.Second), "provider actual paid event timestamp", "provider:test"); err != nil {
		t.Fatal(err)
	}
	if proof := f.allocate(t, "actual-time-recovered", late, b, "80"); proof.TimingProof != "actual_provider_time" {
		t.Fatalf("actual timing proof=%+v", proof)
	}
}

func TestPostgresEventRelayRollsBackAndConcurrentWorkersDeduplicate(t *testing.T) {
	f := setup(t)
	_ = f.order(t, "event-order", "0", "0")
	if err := f.db.Exec(`CREATE TABLE test_event_destination(event_id UUID PRIMARY KEY,kind TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("destination unavailable")
	_, err := f.s.RelayEvents(f.ctx, nil, 100, EventEnqueuerFunc(func(ctx context.Context, tx *gorm.DB, e Event) error {
		if err := tx.Exec(`INSERT INTO test_event_destination(event_id,kind) VALUES(?,?)`, e.ID, e.Kind).Error; err != nil {
			return err
		}
		return failure
	}))
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var published, destination int64
	if err = f.db.Table("commerce_events").Where("published_at IS NOT NULL").Count(&published).Error; err != nil || published != 0 {
		t.Fatalf("event marked after failed destination: %d %v", published, err)
	}
	if err = f.db.Table("test_event_destination").Count(&destination).Error; err != nil || destination != 0 {
		t.Fatalf("destination side effect survived rollback: %d %v", destination, err)
	}
	sink := EventEnqueuerFunc(func(ctx context.Context, tx *gorm.DB, e Event) error {
		return tx.Exec(`INSERT INTO test_event_destination(event_id,kind) VALUES(?,?)`, e.ID, e.Kind).Error
	})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.RelayEvents(f.ctx, nil, 100, sink); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	if err = f.db.Table("commerce_events").Count(&total).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.db.Table("test_event_destination").Count(&destination).Error; err != nil || destination != total {
		t.Fatalf("events lost or duplicated: total=%d destination=%d %v", total, destination, err)
	}
}

func TestPostgresRenewalNoticeConsentFundingAndUniqueCycle(t *testing.T) {
	f := setup(t)
	base := time.Date(2030, 1, 31, 12, 0, 0, 0, time.UTC)
	f.s.databaseFulfillmentClock = false
	f.s.now = func() time.Time { return base }
	f.seed(t, "customer:money", "80")
	o := f.order(t, "initial", "80", "0")
	if _, err := f.s.Fulfill(f.ctx, nil, o.ID, "worker:delivery"); err != nil {
		t.Fatal(err)
	}
	sub, err := f.s.EnableAutoRenewal(f.ctx, nil, f.customer, o.ID, "customer accepted quote v1", false)
	if err != nil {
		t.Fatal(err)
	}
	base = sub.NextDueAt.Add(time.Minute)
	result, err := f.s.RenewSubscription(f.ctx, nil, sub.ID)
	if err != nil || result.Status != "notification_required" {
		t.Fatalf("renewed without notice: %+v %v", result, err)
	}
	if err = f.s.RecordRenewalNotice(f.ctx, nil, sub.ID, f.quote.ID, sub.NextDueAt, "test notification delivered"); err != nil {
		t.Fatal(err)
	}
	f.seed(t, "commission:available", "100")
	result, err = f.s.RenewSubscription(f.ctx, nil, sub.ID)
	if err != nil || result.Status != "needs_funds" {
		t.Fatalf("unauthorized commission use: %+v %v", result, err)
	}
	f.seed(t, "customer:money", "80")
	result, err = f.s.RenewSubscription(f.ctx, nil, sub.ID)
	if err != nil || result.Status != "completed" || result.Order == nil || result.Order.Kind != "renewal" {
		t.Fatalf("renewal=%+v %v", result, err)
	}
	packages, err := f.s.ListCustomerDeliveries(f.ctx, nil, f.customer, 100)
	if err != nil {
		t.Fatal(err)
	}
	controls := 0
	for _, item := range packages {
		if item.Subscription != nil {
			controls++
			if item.Order.ID != result.Order.ID || item.Subscription.ID != sub.ID {
				t.Fatal("renewal controls stayed on an old delivery", item)
			}
		}
	}
	if controls != 1 {
		t.Fatal("one renewal chain has multiple or missing controls", packages)
	}
	if _, err = f.s.EnableAutoRenewal(f.ctx, nil, f.customer, result.Order.ID, "duplicate renewal attempt", false); !errors.Is(err, ErrOrderState) {
		t.Fatal("renewal order created a second charging chain", err)
	}
	if a := f.balance(t, "commission:available"); a.Balance.Cmp(d("100")) != 0 {
		t.Fatal("renewal spent commission without consent")
	}
	if _, err = f.s.RenewSubscription(f.ctx, nil, sub.ID); !errors.Is(err, ErrRenewalNotDue) {
		t.Fatalf("duplicate cycle renewed: %v", err)
	}
	worse, err := f.s.PublishPackage(f.ctx, nil, PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, PackageID: f.quote.PackageID, ExpectedVersion: 1, Name: "Monthly", TermsReference: "terms-v1", Timezone: "UTC", PriceMoney: d("100"), PaidATG: d("200"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	sub, err = f.s.GetSubscription(f.ctx, nil, f.customer, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	base = sub.NextDueAt.Add(time.Minute)
	result, err = f.s.RenewSubscription(f.ctx, nil, sub.ID)
	if err != nil || result.Status != "needs_confirmation" {
		t.Fatalf("mixed worse terms auto accepted: %+v %v", result, err)
	}
	sub, err = f.s.GetSubscription(f.ctx, nil, f.customer, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ConfirmRenewal(f.ctx, nil, f.customer, sub.ID, worse.ID, "customer accepted higher price and commission fallback", sub.Version, true); err != nil {
		t.Fatal(err)
	}
	if err = f.s.RecordRenewalNotice(f.ctx, nil, sub.ID, worse.ID, sub.NextDueAt, "new terms notice delivered"); err != nil {
		t.Fatal(err)
	}
	result, err = f.s.RenewSubscription(f.ctx, nil, sub.ID)
	if err != nil || result.Status != "completed" || result.Order.CommissionMoney.Cmp(d("100")) != 0 {
		t.Fatalf("confirmed commission fallback=%+v %v", result, err)
	}
	if err = f.s.CancelAutoRenewal(f.ctx, nil, f.customer, sub.ID); err != nil {
		t.Fatal(err)
	}
	grants, err := f.ledger.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if err != nil || len(grants) != 3 {
		t.Fatalf("cancel removed existing grants: %d %v", len(grants), err)
	}
}
