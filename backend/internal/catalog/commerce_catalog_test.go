package catalog

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type catalogTestOperators map[string]CatalogOperator

func (d catalogTestOperators) CatalogOperator(_ context.Context, id string) (CatalogOperator, error) {
	o, ok := d[id]
	if !ok {
		return o, ErrCatalogForbidden
	}
	return o, nil
}
func newCommerceTestStore(t *testing.T) CommerceCatalog {
	t.Helper()
	dsn := os.Getenv("TOKENHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TOKENHUB_TEST_DATABASE_URL required for PostgreSQL catalog integration tests")
	}
	admin, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	schema := "catalog_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = admin.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		sqlDB, _ := admin.DB()
		_ = sqlDB.Close()
	})
	u, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, e := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	if e = db.Exec(CommerceSchema()).Error; e != nil {
		t.Fatal(e)
	}
	return CommerceCatalog{DB: db, PlatformOperatorID: "platform", EncryptionKey: "test-encryption-material", Operators: catalogTestOperators{"platform": {ID: "platform", BrandID: "official", Kind: "platform"}, "oem": {ID: "oem", BrandID: "oem-brand", ParentID: "platform", Kind: "oem"}, "channel": {ID: "channel", BrandID: "oem-brand", ParentID: "oem", Kind: "channel"}, "other": {ID: "other", BrandID: "other-brand", ParentID: "platform", Kind: "oem"}}}
}
func commerceTestCapability() modelruntime.Capability {
	return modelruntime.Capability{Operation: modelruntime.Chat, InputTypes: []string{"text"}, Parameters: []string{"max_completion_tokens"}, RequiredParameters: []string{"max_completion_tokens"}, MaxInputBytes: 10000, MaxOutput: 100, OutputLimitParameter: "max_completion_tokens", BillableBounds: map[string]string{"input": "1000"}, UsageRules: []modelruntime.UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}}, EvidenceID: "test-evidence", VerifiedAt: time.Now(), ParametersVerified: true, UsageVerified: true, BoundsVerified: true, RecoveryVerified: true}
}
func seedCommerceModel(t *testing.T, s CommerceCatalog) {
	t.Helper()
	ctx := context.Background()
	cap := commerceTestCapability()
	m := PublicModel{ID: "model", Identity: "vendor/model@revision", DisplayName: "Model", Capabilities: []modelruntime.Capability{cap}}
	if e := s.SavePublicModel(ctx, "platform", m, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveCredential(ctx, "platform", "credential", "provider", "secret-to-encrypt", true, 0); e != nil {
		t.Fatal(e)
	}
	u := UpstreamModel{ID: "upstream", PublicModelID: m.ID, Identity: m.Identity, ProviderID: "provider", Provider: "openai", Model: "exact-model", CredentialID: "credential", Capabilities: m.Capabilities, Enabled: true, Healthy: true}
	if e := s.SaveUpstream(ctx, "platform", u, 0); e != nil {
		t.Fatal(e)
	}
	m.Enabled = true
	if e := s.SavePublicModel(ctx, "platform", m, 1); e != nil {
		t.Fatal(e)
	}
}
func TestCommerceAuthorizationListingIdentityAndCredentials(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	ctx := context.Background()
	if _, e := s.SaveCredential(ctx, "oem", "hack", "provider", "secret", true, 0); !errors.Is(e, ErrCatalogForbidden) {
		t.Fatal("OEM supplier key accepted")
	}
	if e := s.SetAuthorization(ctx, "platform", "oem", "model", true, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SetAuthorization(ctx, "oem", "channel", "model", true, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SetListing(ctx, "channel", "model", true, 0); e != nil {
		t.Fatal(e)
	}
	r, _ := modelruntime.DecodeProtocol("request", modelruntime.Chat, []byte(`{"model":"model","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	if _, e := s.Routes(ctx, "oem-brand", "channel", r); e != nil {
		t.Fatalf("parent retail listing must not control child authorization: %v", e)
	}
	if _, e := s.Routes(ctx, "other-brand", "channel", r); !errors.Is(e, ErrCatalogForbidden) {
		t.Fatal("cross-brand route accepted")
	}
	if e := s.SetAuthorization(ctx, "other", "channel", "model", true, 0); !errors.Is(e, ErrCatalogForbidden) {
		t.Fatal("unrelated seller authorized model")
	}
	credential, e := s.ResolveCredential(ctx, "upstream", "credential")
	if e != nil || credential.Secret != "secret-to-encrypt" {
		t.Fatalf("credential: %v", e)
	}
	if _, e = s.ResolveCredential(ctx, "missing-upstream", "credential"); e == nil {
		t.Fatal("credential bypassed upstream binding")
	}
	applyTestAuthorizationChange(t, s, "platform", "oem", "model", false, 1)
	if _, e = s.Routes(ctx, "oem-brand", "channel", r); !errors.Is(e, ErrModelNotVisible) {
		t.Fatal("ancestor revocation ignored")
	}
	if e = s.SetListing(ctx, "channel", "model", false, 99); !errors.Is(e, ErrCatalogVersion) {
		t.Fatal("stale version overwrote listing")
	}
	bad := UpstreamModel{ID: "bad", PublicModelID: "model", Identity: "similar-but-different-model", ProviderID: "provider", Provider: "openai", Model: "model", CredentialID: "credential"}
	if e = s.SaveUpstream(ctx, "platform", bad, 0); e == nil {
		t.Fatal("model identity alias accepted")
	}
}
func TestCommerceConcurrentPromotionPublicationAndRollback(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	ctx := context.Background()
	at := time.Now().UTC()
	end := at.Add(time.Hour)
	var success atomic.Int32
	var callbacks atomic.Int32
	var wg sync.WaitGroup
	for _, id := range []string{"promotion-a", "promotion-b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			v := priceAt(id, PriceScope{Kind: "retail", SellerID: "platform"}, at, "1")
			v.Layer = "promotion"
			v.EndsAt = &end
			v.NoticeID = "notice"
			e := s.PublishPrice(ctx, "platform", v, func(context.Context, *gorm.DB, PriceVersion) error { callbacks.Add(1); return nil })
			if e == nil {
				success.Add(1)
			}
		}(id)
	}
	wg.Wait()
	if success.Load() != 1 || callbacks.Load() != 1 {
		t.Fatalf("published=%d callbacks=%d", success.Load(), callbacks.Load())
	}
	v := priceAt("rejected", PriceScope{Kind: "retail", SellerID: "platform"}, at, "1")
	v.NoticeID = "notice"
	if e := s.PublishPrice(ctx, "platform", v, func(context.Context, *gorm.DB, PriceVersion) error { return ErrUnboundedCoverage }); e == nil {
		t.Fatal("coverage failure committed")
	}
	var count int64
	_ = s.DB.Table("tokenhub_model_price_versions").Where("id='rejected'").Count(&count).Error
	if count != 0 {
		t.Fatal("failed price left published version")
	}
}
func TestCommerceQuoteLocksEverySupplyPriceAndMaximum(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	ctx := context.Background()
	at := time.Now().UTC()
	for _, edge := range [][2]string{{"platform", "oem"}, {"oem", "channel"}} {
		if e := s.SetAuthorization(ctx, edge[0], edge[1], "model", true, 0); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.SetListing(ctx, "channel", "model", true, 0); e != nil {
		t.Fatal(e)
	}
	for i, scope := range []PriceScope{{Kind: "retail", SellerID: "channel"}, {Kind: "supply", SellerID: "oem", BuyerID: "channel"}, {Kind: "supply", SellerID: "platform", BuyerID: "oem"}} {
		v := priceAt(uuid.NewString(), scope, at, []string{"10", "6", "4"}[i])
		v.NoticeID = "notice"
		if e := s.PublishPrice(ctx, scope.SellerID, v, func(context.Context, *gorm.DB, PriceVersion) error { return nil }); e != nil {
			t.Fatal(e)
		}
	}
	supplier := priceAt("supplier-price", PriceScope{Kind: "supplier", SellerID: "provider", BuyerID: "upstream"}, at, "1")
	supplier.NoticeID = "notice"
	if e := s.PublishPrice(ctx, "platform", supplier, func(context.Context, *gorm.DB, PriceVersion) error { return nil }); e != nil {
		t.Fatal(e)
	}
	r, _ := modelruntime.DecodeProtocol("request", modelruntime.Chat, []byte(`{"model":"model","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	q, e := s.Quote(ctx, "oem-brand", "channel", r, at)
	if e != nil {
		t.Fatal(e)
	}
	if len(q.Supply) != 2 || q.Supply[0].Scope.SellerID != "oem" || q.Supply[1].Scope.SellerID != "platform" {
		t.Fatalf("supply chain %#v", q.Supply)
	}
	if q.Bounds["input"] != "1000" {
		t.Fatal("lost maximum")
	}
	if len(q.Supplier) != 1 || q.Supplier[0].Price.AssetUnit != "USD" {
		t.Fatal("supplier USD cost snapshot missing")
	}
	charge, e := q.Supply[1].MaximumCharge(q.Bounds)
	if e != nil || charge.RatString() != "1/250" {
		t.Fatalf("maximum %v %v", charge, e)
	}
}
func TestCommerceRevisionLockSerializesAdmissionAndChanges(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	ctx := context.Background()
	tx := s.DB.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	before, e := s.WithDB(tx).LockRevision(ctx)
	if e != nil {
		t.Fatal(e)
	}
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, e := s.SaveCredential(ctx, "platform", "credential", "provider", "rotated-secret", true, 1)
		done <- e
	}()
	<-started
	select {
	case e := <-done:
		t.Fatalf("catalog mutation bypassed accepted snapshot lock: %v", e)
	case <-time.After(50 * time.Millisecond):
	}
	if e = tx.Commit().Error; e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("catalog mutation did not resume after admission")
	}
	after, e := s.LockRevision(ctx)
	if e != nil || after != before+1 {
		t.Fatalf("revision before=%d after=%d err=%v", before, after, e)
	}
}
