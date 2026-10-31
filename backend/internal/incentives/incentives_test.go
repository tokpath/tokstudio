package incentives

import (
	"context"
	"encoding/json"
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
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func d(value string) ledger.Decimal { return ledger.MustDecimal(value) }
func TestWithdrawalFeeAndRateBoundaries(t *testing.T) {
	q, err := quoteWithdrawal(WithdrawalPolicy{MinimumMoney: d("5"), FlatFeeMoney: d("1"), FeeRate: d("0.1")}, d("10"))
	if err != nil || q.FeeMoney.Cmp(d("2")) != 0 || q.NetMoney.Cmp(d("8")) != 0 {
		t.Fatalf("quote=%+v %v", q, err)
	}
	if _, err = quoteWithdrawal(WithdrawalPolicy{MinimumMoney: d("5"), FlatFeeMoney: d("10")}, d("5")); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("negative payout accepted: %v", err)
	}
	var service Service
	if _, err = service.SetCommissionWithdrawalMinimum(context.Background(), nil, "", "", d("0.999")); !errors.Is(err, ErrWithdrawalMinimum) {
		t.Fatalf("minimum below 1 accepted: %v", err)
	}
	if rateValid(d("1.01")) || rateValid(d("-0.01")) || !rateValid(d("0.000000000000000001")) {
		t.Fatal("rate validation")
	}
}
func integrationDB(t *testing.T) *gorm.DB {
	return integrationDBBefore(t, "")
}

func integrationDBBefore(t *testing.T, before string) *gorm.DB {
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
	schema := "incentives_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	_, c := commerce.Migrations()
	_, i := Migrations()
	for moduleIndex, m := range []fs.FS{o, l, c, i} {
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
			if moduleIndex == 3 && before != "" && name >= before {
				continue
			}
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

// These adapters isolate incentive contracts; production coverage is supplied
// by cooperation. Ledger grants/payments and all incentive state below are real.
type coverageDouble struct{ rewardFailure atomic.Bool }

func (*coverageDouble) Reserve(_ context.Context, _ *gorm.DB, o commerce.Order) (commerce.Commitment, error) {
	return commerce.Commitment{Reference: o.ID, Version: 1}, nil
}
func (*coverageDouble) Restore(_ context.Context, _ *gorm.DB, o commerce.Order) (commerce.Commitment, error) {
	return commerce.Commitment{Reference: o.ID, Version: 2}, nil
}
func (*coverageDouble) Fulfill(context.Context, *gorm.DB, commerce.Order) error { return nil }
func (*coverageDouble) Release(context.Context, *gorm.DB, commerce.Order) error { return nil }
func (*coverageDouble) ReserveRegistration(_ context.Context, _ *gorm.DB, p RegistrationPromise) (commerce.Commitment, error) {
	return commerce.Commitment{Reference: p.IntentID, Version: 1}, nil
}
func (c *coverageDouble) FulfillRegistration(context.Context, *gorm.DB, RegistrationPromise, ledger.Decimal) error {
	if c.rewardFailure.Load() {
		return errors.New("test injected reward delivery outage")
	}
	return nil
}
func (*coverageDouble) ReleaseRegistration(context.Context, *gorm.DB, RegistrationPromise) error {
	return nil
}

type ready struct{}

func (ready) Check(context.Context, *gorm.DB, organization.Operator, organization.BusinessFeature) ([]organization.Requirement, error) {
	return []organization.Requirement{{Code: "test_fixture_dependencies", Ready: true}}, nil
}

type fixture struct {
	ctx                    context.Context
	db                     *gorm.DB
	s                      *Service
	org                    *organization.Service
	ledger                 *ledger.Service
	commerce               *commerce.Service
	coverage               *coverageDouble
	operator, brand, owner string
	policy                 Policy
	quote                  commerce.PackageQuote
}

func setup(t *testing.T) fixture {
	return setupOnDB(t, integrationDB(t))
}

func setupOnDB(t *testing.T, db *gorm.DB) fixture {
	t.Helper()
	ctx := context.Background()
	org := organization.New(db)
	owner := uuid.NewString()
	platform, err := org.BootstrapPlatform(ctx, nil, organization.BootstrapInput{BrandName: "Brand", OperatorName: "Platform", OwnerStaffID: owner})
	if err != nil {
		t.Fatal(err)
	}
	for _, feature := range []organization.BusinessFeature{organization.Registration, organization.NewPurchase, organization.Activity} {
		status, err := org.GetBusinessStatus(ctx, nil, platform.Operator.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = org.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: platform.Operator.ID, Feature: feature, Enabled: true, ExpectedVersion: status.Version, EvidenceReference: "test fixture opening"}, ready{}); err != nil {
			t.Fatal(err)
		}
	}
	s := New(db)
	policy, err := s.SetPolicy(ctx, nil, owner, platform.Operator.ID, PolicyInput{ThresholdMoney: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 3600, KOLCrossActivityLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	store := commerce.New(db)
	cap := &coverageDouble{}
	store.SetSalesCoverage(cap)
	store.SetMarketing(s)
	quote, err := store.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: platform.Operator.ID, Name: "Monthly", TermsReference: "terms-v1", Timezone: "UTC", PriceMoney: d("100"), PaidATG: d("80"), GiftATG: d("20"), Period: ledger.Month, NewPurchaseEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{ctx: ctx, db: db, s: s, org: org, ledger: ledger.New(db), commerce: store, coverage: cap, operator: platform.Operator.ID, brand: platform.Brand.ID, owner: owner, policy: policy, quote: quote}
}
func (f fixture) register(t *testing.T, email, code string) organization.RegistrationResult {
	t.Helper()
	v, err := f.org.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: f.brand, Email: email, InviteCode: code, Password: "test-only-strong-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.VerifyEmail(f.ctx, nil, v.Intent.ID, v.VerificationToken); err != nil {
		t.Fatal(err)
	}
	r, err := f.org.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f fixture) seed(t *testing.T, customer, purpose, amount string) {
	t.Helper()
	if purpose == "customer:money" {
		key := "wallet-fixture:" + uuid.NewString()
		o, err := f.commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{CustomerID: customer, OperatorID: f.operator, BusinessKey: key, ExpectedOwnershipVersion: 1, AmountMoney: d(amount)})
		if err != nil {
			t.Fatal(err)
		}
		zero, gross := d("0"), d(amount)
		r, err := f.commerce.RecordReceipt(f.ctx, nil, commerce.VerifiedReceipt{OperatorID: f.operator, Provider: "test-verified-provider", ProviderReceiptID: key, GrossMoney: gross, NetMoney: &gross, FeeMoney: &zero, Currency: "USD", OriginalAmount: amount, RecipientReference: "test:merchant", PayerReference: "test:external-payer", ValuationEvidence: "verified USD", VerificationEvidence: "test verified receipt", VerifiedBy: "test:provider", PaidAt: &o.CreatedAt})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, BusinessKey: key, ReceiptID: r.ID, OrderID: o.ID, AmountMoney: gross}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.commerce.Fulfill(f.ctx, nil, o.ID, "test:verified-wallet-funding"); err != nil {
			t.Fatal(err)
		}
		return
	}
	account, err := f.s.account(f.ctx, nil, f.operator, customer, purpose)
	if err != nil {
		t.Fatal(err)
	}
	control, err := f.s.account(f.ctx, nil, f.operator, f.operator, "control:test-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.ledger.Post(f.ctx, nil, ledger.Posting{OperatorID: f.operator, BusinessKey: "fixture:" + uuid.NewString(), Kind: "test_fixture", SourceID: uuid.NewString(), ActorID: "test:fixture", Reason: "explicit test opening funds", Lines: []ledger.Line{{AccountID: account.ID, Amount: d(amount)}, {AccountID: control.ID, Amount: d(amount).Neg()}}}); err != nil {
		t.Fatal(err)
	}
}
func (f fixture) buy(t *testing.T, customer, key, amount string, selection ActivitySelection) commerce.Order {
	t.Helper()
	f.seed(t, customer, "customer:money", amount)
	raw, err := json.Marshal(selection)
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{CustomerID: customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: key, ExpectedOwnershipVersion: 1, InternalMoney: d(amount), ActivitySelection: raw})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.commerce.Fulfill(f.ctx, nil, o.ID, "worker:delivery"); err != nil {
		t.Fatal(err)
	}
	return o
}
func (f fixture) consume(t *testing.T, customer, quantity string, unobserved ...bool) []ledger.GrantEvent {
	t.Helper()
	var events []ledger.GrantEvent
	err := f.db.Transaction(func(tx *gorm.DB) error {
		instant, clockErr := f.s.BusinessInstant(f.ctx, tx)
		if clockErr != nil {
			return clockErr
		}
		at := instant.At
		key := uuid.NewString()
		hold, err := f.ledger.ReserveCredits(f.ctx, tx, ledger.CreditReservation{OperatorID: f.operator, CustomerID: customer, BusinessKey: "usage-hold:" + key, Estimate: d(quantity), At: at, ExpiresAt: at.Add(time.Minute)})
		if err != nil {
			return err
		}
		result, err := f.ledger.SettleCredits(f.ctx, tx, f.operator, customer, hold.ID, "usage:"+key, d(quantity), at)
		if err != nil {
			return err
		}
		grants, err := f.ledger.CustomerGrants(f.ctx, tx, f.operator, customer)
		if err != nil {
			return err
		}
		byID := map[string]ledger.Grant{}
		for _, g := range grants {
			byID[g.ID] = g
		}
		for _, event := range result.Events {
			if len(unobserved) > 0 && unobserved[0] {
				continue
			}
			if err = f.s.ObserveGrantEvent(f.ctx, tx, byID[event.GrantID], event); err != nil {
				return err
			}
		}
		events = result.Events
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return events
}
func (f fixture) balance(t *testing.T, customer, purpose string) ledger.Account {
	t.Helper()
	a, err := f.s.account(f.ctx, nil, f.operator, customer, purpose)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func (f fixture) qualify(t *testing.T, customer, key string) {
	t.Helper()
	f.buy(t, customer, key, "100", ActivitySelection{})
	f.consume(t, customer, "100")
	q, err := f.s.GetQualification(f.ctx, nil, f.operator, customer)
	if err != nil || q.AcquiredAt == nil {
		t.Fatalf("not qualified: %+v %v", q, err)
	}
}

func TestPostgresCampaignCombinationLocksOriginalRatiosAndTwoHopScope(t *testing.T) {
	f := setup(t)
	a := f.register(t, "a@example.test", "")
	b := f.register(t, "b@example.test", a.InviteCode)
	c := f.register(t, "c@example.test", b.InviteCode)
	f.qualify(t, a.Customer.ID, "qualify-a")
	f.qualify(t, b.Customer.ID, "qualify-b")
	now := time.Now().UTC()
	merchant, err := f.s.PublishCampaign(f.ctx, nil, CampaignInput{ActorStaffID: f.owner, OperatorID: f.operator, Kind: "merchant", PackageID: f.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), FinalPriceMoney: func() *ledger.Decimal { v := d("80"); return &v }(), GiftATG: d("20"), PerCustomerLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	kol, err := f.s.PublishKOLCampaign(f.ctx, nil, b.Customer.ID, CampaignInput{OperatorID: f.operator, PackageID: f.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), ConcessionRate: d("0.10"), PerCustomerLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	selection := ActivitySelection{MerchantCampaignID: merchant.CampaignID, KOLCampaignID: kol.CampaignID}
	raw, _ := json.Marshal(selection)
	f.seed(t, c.Customer.ID, "customer:money", "80")
	input := commerce.CreateOrderInput{CustomerID: c.Customer.ID, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: "combined", ExpectedOwnershipVersion: 1, InternalMoney: d("80"), ActivitySelection: raw}
	if _, err = f.commerce.CreateOrder(f.ctx, nil, input); !errors.Is(err, ErrCombination) {
		t.Fatalf("implicit combination allowed: %v", err)
	}
	if err = f.s.AllowCombination(f.ctx, nil, f.owner, f.operator, merchant.CampaignID, kol.CampaignID, true); err != nil {
		t.Fatal(err)
	}
	activityState, err := f.org.GetBusinessStatus(f.ctx, nil, f.operator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.SetBusinessStatus(f.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: f.owner, OperatorID: f.operator, Feature: organization.Activity, Enabled: false, ExpectedVersion: activityState.Version, EvidenceReference: "pause new campaign participation"}, ready{}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.PreviewQuote(f.ctx, nil, c.Customer.ID, f.operator, f.quote.ID, raw); !errors.Is(err, organization.ErrBusinessClosed) {
		t.Fatalf("activity pause bypassed: %v", err)
	}
	activityState, err = f.org.GetBusinessStatus(f.ctx, nil, f.operator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.SetBusinessStatus(f.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: f.owner, OperatorID: f.operator, Feature: organization.Activity, Enabled: true, ExpectedVersion: activityState.Version, EvidenceReference: "reopen campaign participation"}, ready{}); err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.PreviewQuote(f.ctx, nil, c.Customer.ID, f.operator, f.quote.ID, raw)
	if err != nil || preview.Reference != "" || preview.KOLGiftATG.Cmp(d("10")) != 0 {
		t.Fatalf("preview=%+v %v", preview, err)
	}
	var campaignBefore campaignRow
	if err = f.db.First(&campaignBefore, "id=?", kol.CampaignID).Error; err != nil || campaignBefore.ReservedUses != 0 || !campaignBefore.ReservedGiftATG.IsZero() {
		t.Fatalf("preview occupied campaign responsibility: %+v %v", campaignBefore, err)
	}
	offers, err := f.s.EligibleActivities(f.ctx, nil, f.operator, c.Customer.ID, f.quote.PackageID)
	if err != nil || len(offers) != 2 {
		t.Fatalf("offers=%+v %v", offers, err)
	}
	for _, offer := range offers {
		if !offer.AllowCombination || len(offer.CompatibleCampaignIDs) != 1 {
			t.Fatalf("explicit pair omitted from offer=%+v", offer)
		}
	}
	order, err := f.commerce.CreateOrder(f.ctx, nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if order.MerchantGiftATG.Cmp(d("20")) != 0 || order.KOLGiftATG.Cmp(d("10")) != 0 {
		t.Fatalf("KOL gift multiplied merchant gift: %+v", order)
	}
	if _, err = f.commerce.Fulfill(f.ctx, nil, order.ID, "worker:delivery"); err != nil {
		t.Fatal(err)
	}
	beforeA := f.balance(t, a.Customer.ID, "commission:available").Balance
	beforeB := f.balance(t, b.Customer.ID, "commission:available").Balance
	if _, err = f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("60"), DirectRate: d("0.10"), IndirectRate: d("0.03"), RewardValiditySeconds: 3600, KOLCrossActivityLimit: 1}); err != nil {
		t.Fatal(err)
	}
	events := f.consume(t, c.Customer.ID, "13")
	if len(events) != 1 || events[0].PaidMoney.Cmp(d("8")) != 0 {
		t.Fatalf("paid base=%+v", events)
	}
	afterA := f.balance(t, a.Customer.ID, "commission:available").Balance
	afterB := f.balance(t, b.Customer.ID, "commission:available").Balance
	if afterA.Sub(beforeA).Cmp(d("0.4")) != 0 || afterB.Sub(beforeB).Cmp(d("0.4")) != 0 {
		t.Fatalf("original rates lost: indirect=%s direct=%s", afterA.Sub(beforeA), afterB.Sub(beforeB))
	}
	views, _, err := f.s.CommissionViews(f.ctx, nil, f.operator, b.Customer.ID, "", "", 20)
	if err != nil || len(views) != 1 || views[0].SourceID != events[0].ID || views[0].BasisMoney.Cmp(d("8")) != 0 || views[0].AmountMoney.Cmp(d("0.4")) != 0 {
		t.Fatalf("real commission base view=%+v %v", views, err)
	}
	matched, _, err := f.s.CommissionViews(f.ctx, nil, f.operator, b.Customer.ID, "", views[0].ID, 20)
	if err != nil || len(matched) != 1 || matched[0].ID != views[0].ID {
		t.Fatalf("commission search missed its own record: %+v %v", matched, err)
	}
	other, _, err := f.s.CommissionViews(f.ctx, nil, f.operator, b.Customer.ID, "", "missing-commission-record", 20)
	if err != nil || len(other) != 0 {
		t.Fatalf("commission search ignored the query: %+v %v", other, err)
	}
	if err = f.s.AccrueGrantEvent(f.ctx, nil, GrantEventInput{EventID: events[0].ID, OperatorID: f.operator, CustomerID: c.Customer.ID, OrderID: order.ID, Kind: "usage", PaidMoney: events[0].PaidMoney, OccurredAt: events[0].CreatedAt}); err != nil {
		t.Fatal(err)
	}
	if got := f.balance(t, b.Customer.ID, "commission:available").Balance; got.Cmp(afterB) != 0 {
		t.Fatal("event replay duplicated commission")
	}
	input.BusinessKey = "cross-limit"
	if _, err = f.commerce.CreateOrder(f.ctx, nil, input); !errors.Is(err, ErrLimit) {
		t.Fatalf("KOL cross-activity limit bypassed: %v", err)
	}
	// Trusted expiry facts contribute qualification only, even with two eligible inviters.
	if err = f.s.AccrueGrantEvent(f.ctx, nil, GrantEventInput{EventID: uuid.NewString(), OperatorID: f.operator, CustomerID: c.Customer.ID, OrderID: order.ID, Kind: "expiry", PaidMoney: d("72"), OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	q, err := f.s.GetQualification(f.ctx, nil, f.operator, c.Customer.ID)
	if err != nil || q.ConfirmedMoney.Cmp(d("80")) != 0 || q.AcquiredAt == nil {
		t.Fatalf("expiry qualification=%+v %v", q, err)
	}
	if got := f.balance(t, b.Customer.ID, "commission:available").Balance; got.Cmp(afterB) != 0 {
		t.Fatal("expiry paid a commission")
	}
}

func TestPostgresQualificationConfirmationDisputeAndProtection(t *testing.T) {
	f := setup(t)
	customer := f.register(t, "customer@example.test", "")
	order := f.buy(t, customer.Customer.ID, "source", "100", ActivitySelection{})
	if _, err := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), ConfirmationSeconds: 3600, RewardValiditySeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	instant, clockErr := f.s.BusinessInstant(f.ctx, nil)
	if clockErr != nil {
		t.Fatal(clockErr)
	}
	occurred := instant.At
	event := GrantEventInput{EventID: uuid.NewString(), OperatorID: f.operator, CustomerID: customer.Customer.ID, OrderID: order.ID, Kind: "usage", PaidMoney: d("80"), OccurredAt: occurred}
	if err := f.s.AccrueGrantEvent(f.ctx, nil, event); err != nil {
		t.Fatal(err)
	}
	q, err := f.s.GetQualification(f.ctx, nil, f.operator, customer.Customer.ID)
	if err != nil || !q.ConfirmedMoney.IsZero() || q.AcquiredAt != nil {
		t.Fatalf("unmatured amount counted: %+v %v", q, err)
	}
	overview, err := f.s.PromotionOverview(f.ctx, nil, f.operator, customer.Customer.ID)
	if err != nil || overview.PendingConsumptionMoney.Cmp(d("80")) != 0 || !overview.Qualification.PendingOffsetMoney.IsZero() {
		t.Fatalf("pending consumption confused with recovery=%+v %v", overview, err)
	}
	if err = f.s.SetEventDispute(f.ctx, nil, f.owner, f.operator, event.EventID, "test dispute evidence", true); err != nil {
		t.Fatal(err)
	}
	q, err = f.s.Advance(f.ctx, nil, f.operator, customer.Customer.ID, occurred.Add(2*time.Hour))
	if err != nil || q.AcquiredAt != nil {
		t.Fatalf("disputed time alone qualified: %+v %v", q, err)
	}
	if err = f.s.SetEventDispute(f.ctx, nil, f.owner, f.operator, event.EventID, "dispute resolved", false); err != nil {
		t.Fatal(err)
	}
	q, err = f.s.Advance(f.ctx, nil, f.operator, customer.Customer.ID, occurred.Add(2*time.Hour))
	if err != nil || q.AcquiredAt == nil || !q.AcquiredAt.Equal(occurred.Add(time.Hour)) {
		t.Fatalf("logical maturity time=%+v %v", q, err)
	}
	if _, err = f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 2, ThresholdMoney: d("500"), DirectRate: d("0.10"), IndirectRate: d("0.03"), RewardValiditySeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ReverseEvent(f.ctx, nil, CorrectionInput{ActorStaffID: f.owner, OperatorID: f.operator, OriginalEventID: event.EventID, BusinessKey: "correct-80", Evidence: "proven duplicate usage", AmountMoney: d("80")}); err != nil {
		t.Fatal(err)
	}
	q, err = f.s.Advance(f.ctx, nil, f.operator, customer.Customer.ID, occurred.Add(2*time.Hour))
	if err != nil || !q.ConfirmedMoney.IsZero() || q.AcquiredAt == nil {
		t.Fatalf("normal correction revoked acquired qualification: %+v %v", q, err)
	}
}

func TestPostgresWithdrawalUnknownRecoveryAndOffsetsNeverTouchOtherAssets(t *testing.T) {
	f := setup(t)
	a := f.register(t, "a@example.test", "")
	b := f.register(t, "b@example.test", a.InviteCode)
	f.qualify(t, a.Customer.ID, "a-source")
	f.buy(t, b.Customer.ID, "b-source", "100", ActivitySelection{})
	events := f.consume(t, b.Customer.ID, "100")
	f.seed(t, a.Customer.ID, "customer:money", "9")
	policy, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{PayoutKind: "bank_cn", MinimumMoney: d("5"), FlatFeeMoney: d("2"), MethodReference: "manual verified payout"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: a.Customer.ID, BusinessKey: "withdrawal", RecipientReference: "private:recipient-profile-1", PolicyID: policy.ID, AmountMoney: d("10")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.MarkWithdrawalUnknown(f.ctx, nil, f.owner, f.operator, w.ID, "bank result uncertain"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.CancelWithdrawal(f.ctx, nil, a.Customer.ID, w.ID); !errors.Is(err, ErrWithdrawalState) {
		t.Fatalf("unknown payout hold cancelled: %v", err)
	}
	if err = f.s.FailWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID, "no proof", false); !errors.Is(err, ErrWithdrawalState) {
		t.Fatalf("unknown payout released without proof: %v", err)
	}
	if err = f.s.ReverseEvent(f.ctx, nil, CorrectionInput{ActorStaffID: f.owner, OperatorID: f.operator, OriginalEventID: events[0].ID, BusinessKey: "reverse-original", Evidence: "proven wrong paid base", AmountMoney: d("100")}); err != nil {
		t.Fatal(err)
	}
	q, err := f.s.GetQualification(f.ctx, nil, f.operator, a.Customer.ID)
	if err != nil || q.PendingOffsetMoney.Cmp(d("10")) != 0 {
		t.Fatalf("missing pending offset: %+v %v", q, err)
	}
	if account := f.balance(t, a.Customer.ID, "customer:money"); account.Balance.Cmp(d("9")) != 0 {
		t.Fatal("commission correction touched payment wallet")
	}
	if account := f.balance(t, a.Customer.ID, "commission:available"); account.Balance.Cmp(d("10")) != 0 || account.Held.Cmp(d("10")) != 0 {
		t.Fatalf("unknown payment held amount taken: %+v", account)
	}
	proof := WithdrawalPaymentProof{Evidence: "Cash paid to the customer in person", Currency: "USD", OriginalAmount: "8", ValuationEvidence: "actual USD payout", PaidMoney: d("8"), PaidAt: time.Now().UTC()}
	if paid, err := f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, proof); err != nil || paid.PaymentReference == nil || *paid.PaymentReference == "" {
		t.Fatal(paid, err)
	}
	if _, err = f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, proof); err != nil {
		t.Fatal(err)
	}
	f.buy(t, b.Customer.ID, "b-source-next", "100", ActivitySelection{})
	f.consume(t, b.Customer.ID, "100")
	q, err = f.s.GetQualification(f.ctx, nil, f.operator, a.Customer.ID)
	if err != nil || !q.PendingOffsetMoney.IsZero() {
		t.Fatalf("new release failed offset: %+v %v", q, err)
	}
	if account := f.balance(t, a.Customer.ID, "commission:available"); account.Balance.Cmp(d("5")) != 0 || !account.Held.IsZero() {
		t.Fatalf("offset net available=%+v", account)
	}
	w2, err := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: a.Customer.ID, BusinessKey: "withdrawal-fail", RecipientReference: "private:recipient-profile-1", PolicyID: policy.ID, AmountMoney: d("5")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w2.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.FailWithdrawal(f.ctx, nil, f.owner, f.operator, w2.ID, "bank verified no payment occurred", true); err != nil {
		t.Fatal(err)
	}
	if account := f.balance(t, a.Customer.ID, "commission:available"); account.Balance.Cmp(d("5")) != 0 || !account.Held.IsZero() {
		t.Fatalf("failed payout charged fee: %+v", account)
	}
}

func TestPostgresRegistrationRewardsPromiseOutageAndKOLHistory(t *testing.T) {
	f := setup(t)
	inviter := f.register(t, "inviter@example.test", "")
	if _, err := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("60"), DirectRate: d("0.15"), IndirectRate: d("0.05"), InviteeATG: d("5"), InviterATG: d("2"), RewardValiditySeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	v, err := f.org.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: f.brand, Email: "new@example.test", InviteCode: inviter.InviteCode, Password: "test-only-strong-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.PrepareRegistrationPromise(f.ctx, nil, v.Intent); !errors.Is(err, ErrNotReady) {
		t.Fatalf("unfunded reward advertised: %v", err)
	}
	f.s.SetRegistrationCoverage(f.coverage)
	promise, err := f.s.PrepareRegistrationPromise(f.ctx, nil, v.Intent)
	if err != nil {
		t.Fatal(err)
	}
	if promise.InviteeATG.Cmp(d("5")) != 0 || promise.InviterATG.Cmp(d("2")) != 0 {
		t.Fatalf("promise=%+v", promise)
	}
	if _, err = f.org.VerifyEmail(f.ctx, nil, v.Intent.ID, v.VerificationToken); err != nil {
		t.Fatal(err)
	}
	f.coverage.rewardFailure.Store(true)
	registered, err := f.org.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: v.Intent.ID, VerificationToken: v.VerificationToken}, f.s)
	if err != nil {
		t.Fatalf("delivery outage rejected funded registration: %v", err)
	}
	var pending promiseRow
	if err = f.db.First(&pending, "intent_id=?", v.Intent.ID).Error; err != nil || pending.Status != "pending_delivery" || pending.LastError == "" {
		t.Fatalf("durable reward responsibility=%+v %v", pending, err)
	}
	grants, err := f.ledger.CustomerGrants(f.ctx, nil, f.operator, registered.Customer.ID)
	if err != nil || len(grants) != 0 {
		t.Fatalf("failed delivery left partial gifts: %+v %v", grants, err)
	}
	f.coverage.rewardFailure.Store(false)
	if err = f.s.DeliverRegistrationRewards(f.ctx, nil, v.Intent.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DeliverRegistrationRewards(f.ctx, nil, v.Intent.ID); err != nil {
		t.Fatal(err)
	}
	grants, err = f.ledger.CustomerGrants(f.ctx, nil, f.operator, registered.Customer.ID)
	if err != nil || len(grants) != 1 || grants[0].Total.Cmp(d("5")) != 0 || grants[0].ExpiresAt == nil || !grants[0].ExpiresAt.Equal(registered.Customer.RegisteredAt.Add(time.Hour)) {
		t.Fatalf("reward term/idempotency=%+v %v", grants, err)
	}
	f.qualify(t, inviter.Customer.ID, "qualify-inviter")
	next, err := f.org.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: f.brand, Email: "next@example.test", InviteCode: inviter.InviteCode, Password: "test-only-strong-password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.PrepareRegistrationPromise(f.ctx, nil, next.Intent); err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.VerifyEmail(f.ctx, nil, next.Intent.ID, next.VerificationToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.org.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: next.Intent.ID, VerificationToken: next.VerificationToken}, f.s); err != nil {
		t.Fatal(err)
	}
	if err = f.db.First(&pending, "intent_id=?", next.Intent.ID).Error; err != nil || !pending.ActualInviterATG.IsZero() || pending.Status != "delivered" {
		t.Fatalf("KOL fell back to ordinary invite reward: %+v %v", pending, err)
	}
}

func TestPostgresCampaignConcurrentCountAndGiftAccounting(t *testing.T) {
	f := setup(t)
	kol := f.register(t, "kol@example.test", "")
	customer := f.register(t, "customer@example.test", kol.InviteCode)
	f.qualify(t, kol.Customer.ID, "qualify")
	now := time.Now().UTC()
	campaign, err := f.s.PublishKOLCampaign(f.ctx, nil, kol.Customer.ID, CampaignInput{OperatorID: f.operator, PackageID: f.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), ConcessionRate: d("0.1"), PerCustomerLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(ActivitySelection{KOLCampaignID: campaign.CampaignID})
	f.seed(t, customer.Customer.ID, "customer:money", "200")
	var successes atomic.Int64
	errs := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, key := range []string{"one", "two"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			<-start
			_, err := f.commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{CustomerID: customer.Customer.ID, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: key, ExpectedOwnershipVersion: 1, InternalMoney: d("100"), ActivitySelection: raw})
			if err == nil {
				successes.Add(1)
			}
			errs <- err
		}(key)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	}
	if successes.Load() != 1 {
		t.Fatalf("activity reserved %d times", successes.Load())
	}
	var saved campaignRow
	if err = f.db.First(&saved, "id=?", campaign.CampaignID).Error; err != nil || saved.ReservedGiftATG.Cmp(d("10")) != 0 || saved.ReservedUses != 1 {
		t.Fatalf("campaign reservation=%+v %v", saved, err)
	}
}

func TestPostgresCampaignTotalPlacesReleaseWithoutBudgetCaps(t *testing.T) {
	f := setup(t)
	first := f.register(t, "first@example.test", "")
	second := f.register(t, "second@example.test", "")
	now := time.Now().UTC()
	price := d("80")
	input := CampaignInput{ActorStaffID: f.owner, OperatorID: f.operator, Kind: "merchant", Name: "  Summer bonus  ", PackageID: f.quote.PackageID, Eligibility: "any", Enabled: true, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), FinalPriceMoney: &price, GiftATG: d("1000"), PerCustomerLimit: 2, TotalUseLimit: 1}
	bad := input
	tooHigh := d("101")
	bad.FinalPriceMoney = &tooHigh
	if _, err := f.s.PublishCampaign(f.ctx, nil, bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("activity price above package price accepted: %v", err)
	}
	bad.FinalPriceMoney, bad.GiftATG = nil, ledger.Zero()
	if _, err := f.s.PublishCampaign(f.ctx, nil, bad); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("enabled activity without a benefit accepted: %v", err)
	}
	campaign, err := f.s.PublishCampaign(f.ctx, nil, input)
	if err != nil || campaign.Name != "Summer bonus" {
		t.Fatalf("named campaign: %+v %v", campaign, err)
	}
	raw, _ := json.Marshal(ActivitySelection{MerchantCampaignID: campaign.CampaignID})
	create := func(customer, key string) (commerce.Order, error) {
		return f.commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{CustomerID: customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: key, ExpectedOwnershipVersion: 1, ActivitySelection: raw})
	}
	order, err := create(first.Customer.ID, "first-reserves-place")
	if err != nil || order.MerchantGiftATG.Cmp(d("1000")) != 0 || order.DueMoney.Cmp(d("80")) != 0 {
		t.Fatalf("gift or discount blocked without budget: %+v %v", order, err)
	}
	if _, err = create(second.Customer.ID, "second-no-place"); !errors.Is(err, ErrLimit) {
		t.Fatalf("total places bypassed by a different customer: %v", err)
	}
	if _, err = f.commerce.CloseOrder(f.ctx, nil, first.Customer.ID, order.ID, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "isolated unpaid order"}); err != nil {
		t.Fatal(err)
	}
	if _, err = create(second.Customer.ID, "second-reuses-released-place"); err != nil {
		t.Fatalf("closed order did not release place: %v", err)
	}
	var saved campaignRow
	if err = f.db.First(&saved, "id=?", campaign.CampaignID).Error; err != nil || saved.ReservedUses != 1 || saved.ReservedGiftATG.Cmp(d("1000")) != 0 || saved.ReservedDiscountMoney.Cmp(d("20")) != 0 {
		t.Fatalf("reservation accounting after close: %+v %v", saved, err)
	}
}

func TestPostgresTwoHopsUseIndependentCurrentQualification(t *testing.T) {
	f := setup(t)
	a := f.register(t, "a@example.test", "")
	b := f.register(t, "b@example.test", a.InviteCode)
	c := f.register(t, "c@example.test", b.InviteCode)
	f.qualify(t, a.Customer.ID, "a-qualify")
	f.buy(t, c.Customer.ID, "c-order", "100", ActivitySelection{})
	f.consume(t, c.Customer.ID, "20")
	if balance := f.balance(t, b.Customer.ID, "commission:available").Balance; !balance.IsZero() {
		t.Fatalf("unqualified direct hop earned %s", balance)
	}
	if balance := f.balance(t, a.Customer.ID, "commission:available").Balance; balance.Cmp(d("1")) != 0 {
		t.Fatalf("indirect hop lost its own share or absorbed direct share: %s", balance)
	}
	historical := f.consume(t, c.Customer.ID, "5", true)
	f.qualify(t, b.Customer.ID, "b-qualify")
	grants, err := f.ledger.CustomerGrants(f.ctx, nil, f.operator, c.Customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ledger.Grant{}
	for _, grant := range grants {
		byID[grant.ID] = grant
	}
	if err = f.db.Transaction(func(tx *gorm.DB) error {
		for _, event := range historical {
			if err := f.s.ObserveGrantEvent(f.ctx, tx, byID[event.GrantID], event); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if balance := f.balance(t, b.Customer.ID, "commission:available").Balance; !balance.IsZero() {
		t.Fatalf("historical replay retroactively paid a newly qualified direct inviter: %s", balance)
	}
	beforeA := f.balance(t, a.Customer.ID, "commission:available").Balance
	f.consume(t, c.Customer.ID, "20")
	if balance := f.balance(t, b.Customer.ID, "commission:available").Balance; balance.Cmp(d("3")) != 0 {
		t.Fatalf("qualification was retroactive or missing: %s", balance)
	}
	if balance := f.balance(t, a.Customer.ID, "commission:available").Balance; balance.Sub(beforeA).Cmp(d("1")) != 0 {
		t.Fatalf("indirect share=%s", balance.Sub(beforeA))
	}
	if _, err := f.s.SetFunctionState(f.ctx, nil, f.owner, f.operator, b.Customer.ID, "suspended", "confirmed promotion violation", false); err != nil {
		t.Fatal(err)
	}
	beforeA = f.balance(t, a.Customer.ID, "commission:available").Balance
	f.consume(t, c.Customer.ID, "20")
	if balance := f.balance(t, b.Customer.ID, "commission:available").Balance; balance.Cmp(d("3")) != 0 {
		t.Fatalf("suspended future commission generated: %s", balance)
	}
	if balance := f.balance(t, a.Customer.ID, "commission:available").Balance; balance.Sub(beforeA).Cmp(d("1")) != 0 {
		t.Fatal("direct suspension removed independent indirect share")
	}
	q, err := f.s.GetQualification(f.ctx, nil, f.operator, b.Customer.ID)
	if err != nil || q.AcquiredAt == nil {
		t.Fatalf("suspension removed qualification history: %+v %v", q, err)
	}
}

func TestPostgresWithdrawalIndependentReviewerAndActualPayment(t *testing.T) {
	f := setup(t)
	customer := f.register(t, "customer@example.test", "")
	f.qualify(t, customer.Customer.ID, "independent-review-qualified")
	if _, e := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("60"), DirectRate: d("0.1"), RewardValiditySeconds: 3600}); e != nil {
		t.Fatal(e)
	}
	peer := f.register(t, "commission-origin@example.test", customer.InviteCode)
	f.buy(t, peer.Customer.ID, "real-commission-origin", "100", ActivitySelection{})
	f.consume(t, peer.Customer.ID, "100")
	policy, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{PayoutKind: "bank_cn", MinimumMoney: d("5"), FlatFeeMoney: d("2"), MethodReference: "manual transfer", RequiresDualApproval: true}, 0)
	if err != nil || policy.RequiresDualApproval {
		t.Fatal("withdrawal approval is single-person and cannot store a second reviewer", policy, err)
	}
	w, err := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: customer.Customer.ID, BusinessKey: "single-approval", RecipientReference: "private:payout-profile", PolicyID: policy.ID, AmountMoney: d("10")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); err != nil {
		t.Fatal(err)
	}
	if account := f.balance(t, customer.Customer.ID, "commission:available"); account.Balance.Cmp(d("10")) != 0 || account.Held.Cmp(d("10")) != 0 {
		t.Fatalf("approval treated as paid: %+v", account)
	}
	if _, err = f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, WithdrawalPaymentProof{PaymentReference: "bank-actual-payout", Evidence: "verified bank execution", Currency: "USD", PaidAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if account := f.balance(t, customer.Customer.ID, "commission:available"); !account.Balance.IsZero() || !account.Held.IsZero() {
		t.Fatalf("verified payout not captured: %+v", account)
	}
}

func TestPostgresSubAtomCommissionCarryAndCorrectionAreConserved(t *testing.T) {
	f := setup(t)
	if _, err := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: ledger.Zero(), DirectRate: d("0.1"), IndirectRate: ledger.Zero(), RewardValiditySeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	a := f.register(t, "a@example.test", "")
	b := f.register(t, "b@example.test", a.InviteCode)
	quote, err := f.commerce.PublishPackage(f.ctx, nil, commerce.PublishPackageInput{ActorStaffID: f.owner, OperatorID: f.operator, PackageID: f.quote.PackageID, ExpectedVersion: 1, Name: "Micro", TermsReference: "terms-v1", Timezone: "UTC", PriceMoney: d("0.00000000000000001"), PaidATG: d("1"), Period: ledger.Month, NewPurchaseEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	f.quote = quote
	f.buy(t, b.Customer.ID, "micro-order", "0.00000000000000001", ActivitySelection{})
	var original ledger.GrantEvent
	for i := 0; i < 10; i++ {
		events := f.consume(t, b.Customer.ID, "0.1")
		if i == 0 {
			original = events[0]
		}
	}
	if balance := f.balance(t, a.Customer.ID, "commission:available").Balance; balance.Cmp(d("0.000000000000000001")) != 0 {
		t.Fatalf("sub-atom commissions vanished or inflated: %s", balance)
	}
	if err = f.s.ReverseEvent(f.ctx, nil, CorrectionInput{ActorStaffID: f.owner, OperatorID: f.operator, OriginalEventID: original.ID, BusinessKey: "reverse-one-micro-base", Evidence: "source quantity proven wrong", AmountMoney: d("0.000000000000000001")}); err != nil {
		t.Fatal(err)
	}
	if balance := f.balance(t, a.Customer.ID, "commission:available").Balance; !balance.IsZero() {
		t.Fatalf("correction failed to reverse atom carried into a later event: %s", balance)
	}
	var carry commissionCarry
	if err = f.db.Table("incentives_commission_carries").Where("beneficiary_id=?", a.Customer.ID).First(&carry).Error; err != nil || carry.BaseMoney.Cmp(d("0.000000000000000009")) != 0 || carry.CreditedMoney.Sub(carry.ReversedMoney).Sign() != 0 {
		t.Fatalf("cumulative carry=%+v %v", carry, err)
	}
}
