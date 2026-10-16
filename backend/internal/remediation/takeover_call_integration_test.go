package remediation

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"gorm.io/gorm"
)

func TestExternalTakenOverCustomerKeepsKeyAndFutureCostBelongsToBrand(t *testing.T) {
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	if base == "" || os.Getenv("TOKENHUB_TEST_S3_ENDPOINT") == "" {
		t.Skip("independent model HTTP and private S3 required")
	}
	origin, err := url.Parse(base)
	if err != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("explicit local protocol fixture required")
	}
	brand, channel, _, key := takeoverFixture(t)
	p := beginTakeover(t, brand, channel)
	p, err = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); err != nil {
		t.Fatal(err)
	}
	var oldJournals int64
	if err = brand.db.Table("ledger_journals").Where("operator_id=?", channel.operator).Count(&oldJournals).Error; err != nil {
		t.Fatal(err)
	}
	cat := catalog.CommerceCatalog{DB: brand.db, PlatformOperatorID: brand.operator, Operators: cooperation.CatalogDirectory{Organization: brand.s.Organization}, EncryptionKey: "takeover-test-only-encryption-secret"}
	cap := modelruntime.Capability{Operation: modelruntime.Chat, InputTypes: []string{"text"}, Parameters: []string{"max_completion_tokens"}, RequiredParameters: []string{"max_completion_tokens"}, OutputLimitParameter: "max_completion_tokens", MaxInputBytes: 10000, MaxOutput: 1000, BillableBounds: map[string]string{"input": "1000", "output": "1000"}, UsageRules: []modelruntime.UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}, {Component: "output", Unit: "token", Path: "/completion_tokens"}}, EvidenceID: "local-real-http-takeover", VerifiedAt: time.Now(), ParametersVerified: true, UsageVerified: true, BoundsVerified: true, RecoveryVerified: true}
	m := catalog.PublicModel{ID: "after-takeover", Identity: "provider/test@1", DisplayName: "Test model", Capabilities: []modelruntime.Capability{cap}}
	if err = cat.SavePublicModel(brand.ctx, brand.operator, m, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = cat.SaveCredential(brand.ctx, brand.operator, "credential", "provider", "test-only-upstream-key", true, 0); err != nil {
		t.Fatal(err)
	}
	u := catalog.UpstreamModel{ID: "upstream", PublicModelID: m.ID, Identity: m.Identity, ProviderID: "provider", Provider: "openai", Model: "test-chat", CredentialID: "credential", Endpoints: map[modelruntime.Operation]string{modelruntime.Chat: base + "/v1/chat/completions"}, Capabilities: m.Capabilities, Enabled: true, Healthy: true}
	if err = cat.SaveUpstream(brand.ctx, brand.operator, u, 0); err != nil {
		t.Fatal(err)
	}
	m.Enabled = true
	if err = cat.SavePublicModel(brand.ctx, brand.operator, m, 1); err != nil {
		t.Fatal(err)
	}
	if err = cat.SetListing(brand.ctx, brand.operator, m.ID, true, 0); err != nil {
		t.Fatal(err)
	}
	brand.s.Cooperation.Rates = cooperation.CatalogRates{Catalog: cat, Organization: brand.s.Organization}
	for _, scope := range []catalog.PriceScope{{Kind: "retail", SellerID: brand.operator}, {Kind: "supplier", SellerID: "provider", BuyerID: u.ID}} {
		price := catalog.PriceVersion{ID: uuid.NewString(), Scope: scope, ModelID: m.ID, Layer: "regular", EffectiveAt: time.Now().Add(-time.Hour), Published: true, NoticeID: "opening-current-brand-price", Rule: catalog.PriceRule{Mode: "manual", Components: []catalog.ComponentRate{{Key: "input", Unit: "token", Per: "1", Rate: "0.001"}, {Key: "output", Unit: "token", Per: "1", Rate: "0.001"}}}}
		if err = cat.PublishPrice(brand.ctx, brand.operator, price, func(ctx context.Context, tx *gorm.DB, _ catalog.PriceVersion) error {
			_, e := brand.s.Cooperation.Coverage(ctx, tx, brand.operator, time.Now())
			return e
		}); err != nil {
			t.Fatal(err)
		}
	}
	status, err := brand.s.Organization.GetBusinessStatus(brand.ctx, nil, brand.operator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = brand.s.Organization.SetBusinessStatus(brand.ctx, nil, organization.SetBusinessStatusInput{ActorStaffID: brand.owner, OperatorID: brand.operator, Feature: organization.ModelUsage, Enabled: true, ExpectedVersion: status.Version, EvidenceReference: "verified fixture catalog and storage"}, testReadiness{}); err != nil {
		t.Fatal(err)
	}
	store, err := objectstore.New(brand.ctx, objectstore.Settings{Endpoint: os.Getenv("TOKENHUB_TEST_S3_ENDPOINT"), Region: "us-east-1", Bucket: os.Getenv("TOKENHUB_TEST_S3_BUCKET"), AccessKey: os.Getenv("TOKENHUB_TEST_S3_ACCESS_KEY"), SecretKey: os.Getenv("TOKENHUB_TEST_S3_SECRET_KEY"), ForcePathStyle: true})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := modelruntime.NewBifrost(brand.ctx, modelruntime.BifrostConfig{Providers: []string{"openai"}, Credentials: cat, AllowTestLoopback: true, EndpointGuard: func(_ context.Context, target string) error {
		u, e := url.Parse(target)
		if e != nil || u.Host != origin.Host {
			return modelruntime.ErrInvalidRequest
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	s := usage.New(brand.db, usage.Dependencies{Ledger: brand.s.Ledger, Organization: brand.s.Organization, Cooperation: brand.s.Cooperation, Catalog: cat, Runtime: runtime, GrantObserver: brand.s.Incentives, EncryptionKey: cat.EncryptionKey, ResultStore: store})
	authenticated, err := s.AuthenticateKey(brand.ctx, key.Key.BrandID, key.Secret)
	if err != nil {
		t.Fatal("old key stopped after takeover", err)
	}
	input, err := modelruntime.DecodeProtocol("incoming", modelruntime.Chat, []byte(`{"model":"after-takeover","messages":[{"role":"user","content":"hello"}],"max_tokens":1000}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.Execute(brand.ctx, authenticated, "after-takeover-call", input, nil)
	if err != nil || out.Request.OperatorID != brand.operator || out.Request.ChargedATG.String() != "0.017" {
		t.Fatal("transferred key/rights did not execute", out.Request.Status, err)
	}
	var oldAfter int64
	if err = brand.db.Table("ledger_journals").Where("operator_id=?", channel.operator).Count(&oldAfter).Error; err != nil || oldAfter != oldJournals {
		t.Fatal("new use posted back to original channel", oldAfter, oldJournals, err)
	}
	grants, err := brand.s.Ledger.CustomerGrants(brand.ctx, nil, brand.operator, channel.customer)
	if err != nil || len(grants) != 1 || grants[0].Consumed.String() != "0.017" || grants[0].ConsumedPaidUSD.String() != "0.0136" {
		t.Fatal("original paid basis changed", grants, err)
	}
	if err = brand.s.Ledger.Reconcile(brand.ctx, nil, grants[0].AccountID); err != nil {
		t.Fatal(err)
	}
	var after Takeover
	if err = brand.db.First(&after, "id=?", p.ID).Error; err != nil || after.SettlementDueUSD.Cmp(p.SettlementDueUSD) != 0 {
		t.Fatal("future use changed closed channel valuation", err)
	}
	accounts, err := brand.s.Ledger.ListAccounts(brand.ctx, nil, brand.operator, brand.operator)
	if err != nil {
		t.Fatal(err)
	}
	cost := ledger.Zero()
	for _, a := range accounts {
		if a.Purpose == "cost:supplier:provider" {
			cost = a.Balance
		}
	}
	if cost.String() != "0.017" {
		t.Fatal("brand did not carry future supplier cost", cost.String())
	}
}
