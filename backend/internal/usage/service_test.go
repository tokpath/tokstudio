package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"gorm.io/gorm"
)

func d(s string) ledger.Decimal { return ledger.MustDecimal(s) }

// Test observer persists unique same-transaction facts. Incentive policy and
// qualification calculations have their own module integration tests.
type observer struct{}

func (observer) LockBusiness(context.Context, *gorm.DB, string) error { return nil }

func (observer) ObserveGrantEvent(_ context.Context, t *gorm.DB, g ledger.Grant, e ledger.GrantEvent) error {
	return t.Exec("INSERT INTO test_observed_grants(id,paid_money) VALUES(?,?) ON CONFLICT DO NOTHING", e.ID, e.PaidMoney).Error
}

type ready struct{}

func (ready) Check(context.Context, *gorm.DB, organization.Operator, organization.BusinessFeature) ([]organization.Requirement, error) {
	return []organization.Requirement{{Code: "isolated_test_fixture", Ready: true}}, nil
}

type fixture struct {
	ctx                              context.Context
	s                                *Service
	db                               *gorm.DB
	org                              *organization.Service
	l                                *ledger.Service
	coop                             *cooperation.Service
	cat                              catalog.CommerceCatalog
	customer, operator, brand, owner string
	key                              KeySecret
}

func setup(t *testing.T, credits string, limit *ledger.Decimal, capabilityOverride ...modelruntime.Capability) fixture {
	t.Helper()
	db := dbtest.New(t)
	apply := func(_ string, f fs.FS) {
		entries, e := fs.ReadDir(f, ".")
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			b, e := fs.ReadFile(f, entry.Name())
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
	apply(Migrations())
	if e := db.Exec(catalog.CommerceSchema()).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec("CREATE TABLE test_observed_grants(id UUID PRIMARY KEY,paid_money NUMERIC(38,18))").Error; e != nil {
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
	for _, feature := range []organization.BusinessFeature{organization.Registration, organization.ModelUsage} {
		st, e := org.GetBusinessStatus(ctx, nil, p.Operator.ID)
		if e != nil {
			t.Fatal(e)
		}
		_, e = org.SetBusinessStatus(ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: owner, OperatorID: p.Operator.ID, Feature: feature, Enabled: true, ExpectedVersion: st.Version, EvidenceReference: "isolated test dependencies"}, ready{})
		if e != nil {
			t.Fatal(e)
		}
	}
	intent, e := org.BeginRegistration(ctx, nil, organization.RegistrationIntentRequest{BrandID: p.Brand.ID, Email: "customer@example.test", Password: "test-only-long-password"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = org.VerifyEmail(ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
		t.Fatal(e)
	}
	registered, e := org.CompleteRegistration(ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
	if e != nil {
		t.Fatal(e)
	}
	coop := cooperation.New(db, l, org)
	cat := catalog.CommerceCatalog{DB: db, PlatformOperatorID: p.Operator.ID, Operators: cooperation.CatalogDirectory{Organization: org}, EncryptionKey: "integration-only-encryption-key", Costs: &testCostCatalog{rate: .001}}
	capability := modelruntime.Capability{Operation: modelruntime.Chat, BillableBounds: map[string]string{"input": "1000", "output": "1000"}, UsageRules: []modelruntime.UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}, {Component: "output", Unit: "token", Path: "/completion_tokens"}}}
	if len(capabilityOverride) > 0 {
		capability = capabilityOverride[0]
	}
	streamed := capability
	streamed.Streaming = true
	model := catalog.PublicModel{ID: "public-model", Identity: "vendor/test@1", Capabilities: []modelruntime.Capability{capability, streamed}}
	if capability.Operation == modelruntime.Video {
		model.Capabilities = []modelruntime.Capability{capability}
	}
	if e = cat.SavePublicModel(ctx, p.Operator.ID, model, 0); e != nil {
		t.Fatal(e)
	}
	if _, e = cat.SaveCredential(ctx, p.Operator.ID, "credential", "provider", "integration test key", "integration-test-secret", true, 0); e != nil {
		t.Fatal(e)
	}
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	upstream := catalog.UpstreamModel{ID: "upstream", PublicModelID: model.ID, Identity: model.Identity, ProviderID: "provider", Provider: "openai", Model: "test-chat", CredentialID: "credential", Endpoints: map[modelruntime.Operation]string{modelruntime.Chat: base + "/v1/chat/completions"}, Capabilities: model.Capabilities, Enabled: true, Healthy: true}
	if capability.Operation == modelruntime.Video {
		upstream.Model = "test-video"
		upstream.Endpoints = map[modelruntime.Operation]string{modelruntime.Video: base + "/v1/videos", modelruntime.VideoRetrieve: base + "/v1/videos/{task_id}", modelruntime.VideoDownload: base + "/v1/videos/{task_id}/content"}
	}
	if e = cat.SaveUpstream(ctx, p.Operator.ID, upstream, 0); e != nil {
		t.Fatal(e)
	}
	model.Enabled = true
	if e = cat.SavePublicModel(ctx, p.Operator.ID, model, 1); e != nil {
		t.Fatal(e)
	}
	if e = cat.SetListing(ctx, p.Operator.ID, model.ID, true, 0); e != nil {
		t.Fatal(e)
	}
	for _, scope := range []catalog.PriceScope{{Kind: "retail", SellerID: p.Operator.ID}} {
		price := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: model.ID, Layer: "regular", EffectiveAt: time.Now().Add(-time.Hour), Published: true, NoticeID: "fixture-published-terms", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, {Key: "output", Unit: "token", Per: "1", Rate: "0.001"}, catalog.ComponentRate{Key: "cache", Unit: "token", Per: "1", Rate: "0.001"}}}}
		if capability.Operation == modelruntime.Video {
			price.Rule.Components = []catalog.ComponentRate{{Key: "output_video_seconds", Unit: "second", Per: "1", Rate: "0.01"}}
		}
		// Fixture bootstrap has no sold liabilities. The production callback uses
		// CatalogRates and emits coverage risk facts for affected operators.
		if e = cat.PublishPrice(ctx, p.Operator.ID, price, func(_ context.Context, t *gorm.DB, _ catalog.PriceVersion) error {
			var count int64
			if e := t.Model(&ledger.Grant{}).Count(&count).Error; e != nil {
				return e
			}
			if count != 0 {
				return errors.New("fixture expected no sold grants")
			}
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	coop.Rates = cooperation.CatalogRates{Catalog: cat, Organization: org}
	if _, e = l.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: p.Operator.ID, CustomerID: registered.Customer.ID, OrderID: "fixture-order", BusinessKey: "fixture-grant", ActorID: owner, Reason: "test funded credits", PaidMoney: d(credits), Sources: []ledger.GrantSource{{Kind: "paid", Amount: d(credits)}}, EffectiveAt: time.Now().Add(-time.Minute)}); e != nil {
		t.Fatal(e)
	}
	s := New(db, Dependencies{Ledger: l, Organization: org, Cooperation: coop, Catalog: cat, GrantObserver: observer{}, EncryptionKey: cat.EncryptionKey, UsageTimeout: time.Minute})
	if endpoint := os.Getenv("TOKENHUB_TEST_S3_ENDPOINT"); endpoint != "" {
		store, e := objectstore.New(ctx, objectstore.Settings{Endpoint: endpoint, Region: "us-east-1", Bucket: os.Getenv("TOKENHUB_TEST_S3_BUCKET"), AccessKey: os.Getenv("TOKENHUB_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TOKENHUB_TEST_S3_SECRET_KEY"), ForcePathStyle: true})
		if e != nil {
			t.Fatal(e)
		}
		if e = store.Ready(ctx); e != nil {
			t.Fatal(e)
		}
		s.ResultStore = store
	}
	key, e := s.CreateKey(ctx, registered.Customer.ID, "key-creation", KeyInput{Name: "Test key", Period: ledger.Month, Timezone: "UTC", BudgetLimit: limit})
	if e != nil {
		t.Fatal(e)
	}
	return fixture{ctx, s, db, org, l, coop, cat, registered.Customer.ID, p.Operator.ID, p.Brand.ID, owner, key}
}
func request(t *testing.T, stream bool) modelruntime.Request {
	t.Helper()
	// 520 output tokens prices this fixture between 0.5 and 0.6 ATG. One credit
	// can hold only one such request, and the 0.6 budget still accepts it.
	body := `{"model":"public-model","messages":[{"role":"user","content":"hello"}],"max_tokens":520}`
	if stream {
		body = `{"model":"public-model","messages":[{"role":"user","content":"hello"}],"max_tokens":520,"stream":true}`
	}
	r, e := modelruntime.DecodeProtocol("incoming", modelruntime.Chat, []byte(body))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func measured(input, output string) modelruntime.Usage {
	return modelruntime.Usage{Source: "provider_usage", Reliable: true, Measurements: []modelruntime.Measurement{{Component: "input", Unit: "token", Quantity: input}, {Component: "output", Unit: "token", Quantity: output}, {Component: "cache", Unit: "token", Quantity: "0"}}}
}
func TestFixtureRequestReservationSplitsOneCreditOnce(t *testing.T) {
	q := quote(catalog.ComponentRate{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, catalog.ComponentRate{Key: "output", Unit: "token", Per: "1", Rate: "0.001"}, catalog.ComponentRate{Key: "cache", Unit: "token", Per: "1", Rate: "0.001"})
	est, err := Estimate(q, request(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if est.Cmp(d("0.5")) <= 0 || est.Cmp(d("0.6")) > 0 {
		t.Fatalf("fixture reservation %s must be above 0.5 ATG and at most 0.6 ATG", est)
	}
}

func TestPostgresActualOverageNoCustomerDebtAndKeyResetPreservesBudget(t *testing.T) {
	limit := d("0.6")
	f := setup(t, "0.7", &limit)
	r, created, e := f.s.Admit(f.ctx, f.key.Key, "request", request(t, false))
	if e != nil || !created {
		t.Fatalf("admit: %v", e)
	}
	reset, e := f.s.ResetKey(f.ctx, f.customer, f.key.Key.ID, "reset", 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.AuthenticateKey(f.ctx, f.brand, f.key.Secret); !errors.Is(e, ErrKey) {
		t.Fatalf("old secret remained valid: %v", e)
	}
	settled, e := f.s.Settle(f.ctx, nil, r.ID, measured("400", "400"))
	if e != nil {
		t.Fatal(e)
	}
	if settled.ActualATG.String() != "0.8" || settled.ChargedATG.String() != "0.7" || settled.MerchantShortfall.String() != "0.1" || settled.BudgetOverage.String() != "0.2" {
		t.Fatalf("settlement %+v", settled)
	}
	b, e := f.s.KeyBudget(f.ctx, nil, reset.Key, time.Now())
	if e != nil || b.SpentATG.String() != "0.8" || !b.HeldATG.IsZero() {
		t.Fatalf("reset erased budget: %+v %v", b, e)
	}
	again, e := f.s.Settle(f.ctx, nil, r.ID, measured("400", "400"))
	if e != nil || again.ChargedATG.Cmp(settled.ChargedATG) != 0 {
		t.Fatal("settlement not idempotent", e)
	}
	var observed ledger.Decimal
	if e = f.db.Raw("SELECT COALESCE(sum(paid_money),0)::text FROM test_observed_grants").Row().Scan(&observed); e != nil {
		t.Fatal(e)
	}
	if observed.String() != "0.7" {
		t.Fatalf("shortfall counted toward incentives %s", observed)
	}
}
func TestPostgresConcurrentSharedPoolAndUnknownUsageFinality(t *testing.T) {
	f := setup(t, "1", nil)
	second, e := f.s.CreateKey(f.ctx, f.customer, "second", KeyInput{Name: "Second", Period: ledger.Lifetime, Timezone: "UTC"})
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var admitted atomic.Int64
	ids := make(chan string, 10)
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := f.key.Key
			if i%2 == 0 {
				key = second.Key
			}
			r, _, e := f.s.Admit(f.ctx, key, uuid.NewString(), request(t, false))
			if e == nil {
				admitted.Add(1)
				ids <- r.ID
			} else if !errors.Is(e, ledger.ErrInsufficient) {
				errs <- e
			}
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if admitted.Load() != 1 {
		t.Fatalf("shared pool spent twice: %d", admitted.Load())
	}
	id := <-ids
	f.s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	expired, e := f.s.ResolveTimeout(f.ctx, id)
	if e != nil || expired.Status != "free" {
		t.Fatalf("timeout %+v %v", expired, e)
	}
	late, e := f.s.Settle(f.ctx, nil, id, measured("100", "100"))
	if e != nil || !late.ChargedATG.IsZero() {
		t.Fatalf("late usage charged: %+v %v", late, e)
	}
	grants, e := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if e != nil {
		t.Fatal(e)
	}
	a, e := f.l.AccountInTransaction(f.ctx, nil, grants[0].AccountID)
	if e != nil || a.Available().String() != "1" {
		t.Fatalf("released balance %+v %v", a, e)
	}
}
func TestExternalModelToLedgerHTTPIdempotencyAndMissingUsage(t *testing.T) {
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	if base == "" {
		t.Skip("TOKENHUB_MODEL_MOCK_URL required")
	}
	origin, e := url.Parse(base)
	if e != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("explicit test loopback required")
	}
	f := setup(t, "100", nil)
	if f.s.ResultStore == nil {
		t.Fatal("HTTP replay integration requires TOKENHUB_TEST_S3_ENDPOINT and private bucket credentials")
	}
	runtime, e := modelruntime.NewBifrost(f.ctx, modelruntime.BifrostConfig{Providers: []string{"openai"}, AllowTestLoopback: true, Credentials: f.cat, Costs: &modelpricing.Service{}, EndpointGuard: func(_ context.Context, target string) error {
		u, e := url.Parse(target)
		if e != nil || u.Host != origin.Host {
			return modelruntime.ErrInvalidRequest
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	f.s.Runtime = runtime
	first, e := f.s.Execute(f.ctx, f.key.Key, "real-http", request(t, false), nil)
	if e != nil {
		t.Fatal(e)
	}
	if first.Request.Status != "completed" || first.Request.ChargedATG.String() != "0.017" {
		t.Fatalf("HTTP cost %+v", first.Request)
	}
	repeat, e := f.s.Execute(f.ctx, f.key.Key, "real-http", request(t, false), nil)
	if e != nil || !repeat.Replay {
		t.Fatalf("repeat %+v %v", repeat, e)
	}
	var attempts int64
	if e = f.db.Model(&AttemptRecord{}).Where("request_id=?", first.Request.ID).Count(&attempts).Error; e != nil || attempts != 1 {
		t.Fatalf("duplicate provider call %d %v", attempts, e)
	}
	var row struct{ Definition []byte }
	if e = f.db.Table("tokenhub_upstreams").Select("definition").Where("id='upstream'").Take(&row).Error; e != nil {
		t.Fatal(e)
	}
	var u catalog.UpstreamModel
	if e = json.Unmarshal(row.Definition, &u); e != nil {
		t.Fatal(e)
	}
	u.Model = "test-chat-usage-missing"
	if e = f.cat.SaveUpstream(f.ctx, f.operator, u, 1); e != nil {
		t.Fatal(e)
	}
	missing, e := f.s.Execute(f.ctx, f.key.Key, "missing-usage", request(t, false), nil)
	if !errors.Is(e, modelruntime.ErrUsageUnknown) || missing.Request.Status != "awaiting_usage" {
		t.Fatalf("missing usage %+v %v", missing.Request, e)
	}
	f.s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	free, e := f.s.ResolveTimeout(f.ctx, missing.Request.ID)
	if e != nil || free.Status != "free" || !free.ChargedATG.IsZero() {
		t.Fatalf("unverified result %+v %v", free, e)
	}
}

type testCostCatalog struct{ rate float64 }

func (c *testCostCatalog) Snapshot(_ context.Context, provider, model string, at time.Time) (json.RawMessage, error) {
	rate := c.rate
	rows := []tables.TableModelPricing{{Provider: provider, Model: model, Mode: "chat", InputCostPerToken: &rate, OutputCostPerToken: &rate, CacheReadInputTokenCost: &rate}}
	if strings.Contains(model, "video") {
		videoRate := .01
		rows = []tables.TableModelPricing{{Provider: provider, Model: model, Mode: "video_generation", OutputCostPerSecond: &videoRate}}
	}
	return json.Marshal(modelpricing.Snapshot{Version: fmt.Sprint(rate), AcceptedAt: at, Rows: rows})
}
func (c *testCostCatalog) Estimate(ctx context.Context, snapshot json.RawMessage, r modelruntime.Request, cap modelruntime.Capability) *string {
	return (&modelpricing.Service{}).Estimate(ctx, snapshot, r, cap)
}

type unavailableInputRuntime struct{}

func (unavailableInputRuntime) Execute(context.Context, modelruntime.Attempt, modelruntime.Request, modelruntime.Emit) (modelruntime.Result, error) {
	return modelruntime.Result{State: "not_dispatched", ErrorCode: "input_quantity_unavailable"}, modelruntime.ErrUsageUnknown
}
func TestUnmeasurableInputReleasesAllHoldsWithoutBreakingSupplierCircuit(t *testing.T) {
	f := setup(t, "100", nil)
	f.s.Runtime = unavailableInputRuntime{}
	ex, e := f.s.Execute(f.ctx, f.key.Key, "unmeasurable-input", request(t, false), nil)
	if !errors.Is(e, modelruntime.ErrUsageUnknown) || ex.Request.Status != "failed" || !ex.Request.ChargedATG.IsZero() {
		t.Fatal(ex.Request, e)
	}
	grants, e := f.l.CustomerGrants(f.ctx, nil, f.operator, f.customer)
	if e != nil || len(grants) != 1 {
		t.Fatal(grants, e)
	}
	account, e := f.l.AccountInTransaction(f.ctx, nil, grants[0].AccountID)
	if e != nil || account.Balance.String() != "100" || !account.Held.IsZero() {
		t.Fatal(account, e)
	}
	var attempt AttemptRecord
	if e = f.db.Where("request_id=?", ex.Request.ID).Take(&attempt).Error; e != nil || attempt.State != "not_dispatched" || attempt.CostStatus != "confirmed" || attempt.ReferenceCostAmount != nil || attempt.ReportedCostAmount != nil {
		t.Fatal(attempt, e)
	}
	health, _, e := f.cat.RouteHealth(f.ctx, "public")
	if e != nil {
		t.Fatal(e)
	}
	for _, route := range health {
		if route.Circuit == "open" || route.Failures != 0 || route.UnknownUsage != 0 {
			t.Fatal("local media validation affected supplier health", route)
		}
	}
}
