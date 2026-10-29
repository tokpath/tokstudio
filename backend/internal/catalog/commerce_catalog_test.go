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
	return modelruntime.Capability{Operation: modelruntime.Chat, BillableBounds: map[string]string{"input": "1000"}, UsageRules: []modelruntime.UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}}}
}
func seedCommerceModel(t *testing.T, s CommerceCatalog) {
	t.Helper()
	ctx := context.Background()
	cap := commerceTestCapability()
	m := PublicModel{ID: "model", Identity: "vendor/model@revision", Capabilities: []modelruntime.Capability{cap}}
	if e := s.SavePublicModel(ctx, "platform", m, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveCredential(ctx, "platform", "credential", "provider", "catalog test key", "secret-to-encrypt", true, 0); e != nil {
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
	if _, e := s.SaveCredential(ctx, "oem", "hack", "provider", "catalog test key", "secret", true, 0); !errors.Is(e, ErrCatalogForbidden) {
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
			v.Rule.Components = append(v.Rule.Components, ComponentRate{Key: "output", Unit: "token", Per: "1000000", Rate: "0"}, ComponentRate{Key: "cache", Unit: "token", Per: "1000000", Rate: "0"})
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
	v.Rule.Components = append(v.Rule.Components, ComponentRate{Key: "output", Unit: "token", Per: "1000000", Rate: "0"}, ComponentRate{Key: "cache", Unit: "token", Per: "1000000", Rate: "0"})
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
		v.Rule.Components = append(v.Rule.Components, ComponentRate{Key: "output", Unit: "token", Per: "1000000", Rate: "0"}, ComponentRate{Key: "cache", Unit: "token", Per: "1000000", Rate: "0"})
		if e := s.PublishPrice(ctx, scope.SellerID, v, func(context.Context, *gorm.DB, PriceVersion) error { return nil }); e != nil {
			t.Fatal(e)
		}
	}
	r, _ := modelruntime.DecodeProtocol("request", modelruntime.Chat, []byte(`{"model":"model","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	q, e := s.Quote(ctx, "oem-brand", "channel", r, at)
	if e != nil {
		t.Fatal(e)
	}
	if len(q.Supply) != 2 || q.Supply[0].Scope.SellerID != "oem" || q.Supply[1].Scope.SellerID != "platform" {
		t.Fatalf("supply chain %#v", q.Supply)
	}
	if q.Bounds["input"] != "131072" {
		t.Fatal("lost maximum")
	}
	if len(q.Supplier) != 1 || q.Supplier[0].UpstreamID != "upstream" {
		t.Fatal("supplier USD cost snapshot missing")
	}
	charge, e := q.Supply[1].MaximumCharge(q.Bounds)
	if e != nil || charge.RatString() != "8192/15625" {
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
		_, e := s.SaveCredential(ctx, "platform", "credential", "provider", "catalog test key", "rotated-secret", true, 1)
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

func TestCredentialNameCanChangeWithoutReplacingTheSecret(t *testing.T) {
	s := newCommerceTestStore(t)
	ctx := context.Background()
	first, e := s.SaveCredential(ctx, "platform", "credential", "provider", "OpenAI 生产", "secret-to-encrypt", true, 0)
	if e != nil || first.Name != "OpenAI 生产" {
		t.Fatal(first, e)
	}
	var before string
	if e = s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Select("ciphertext").Where("id=?", "credential").Scan(&before).Error; e != nil || before == "" {
		t.Fatal(before, e)
	}
	renamed, e := s.SaveCredential(ctx, "platform", "credential", "provider", "OpenAI 备用", "", true, 1)
	if e != nil || renamed.Name != "OpenAI 备用" || renamed.Fingerprint != first.Fingerprint || renamed.Version != 2 {
		t.Fatal(renamed, e)
	}
	var after string
	if e = s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").Select("ciphertext").Where("id=?", "credential").Scan(&after).Error; e != nil || after != before {
		t.Fatal("renaming replaced the stored key", e)
	}
	if _, e = s.SaveCredential(ctx, "platform", "another", "provider", "  ", "secret", true, 0); !errors.Is(e, ErrInvalidInput) {
		t.Fatal("blank name accepted", e)
	}
	if _, e = s.SaveCredential(ctx, "platform", "another", "provider", "没有密钥", "", true, 0); !errors.Is(e, ErrInvalidInput) {
		t.Fatal("new credential accepted without a key", e)
	}
}

func TestSupplierBindingDoesNotRequireManualSupplierPrices(t *testing.T) {
	s := newCommerceTestStore(t)
	ctx := context.Background()
	capability := commerceTestCapability()
	capability.UsageRules = append(capability.UsageRules, modelruntime.UsageRule{Component: "output", Unit: "token", Path: "/completion_tokens"})
	capability.BillableBounds["output"] = "1000"
	if e := s.SavePublicModel(ctx, "platform", PublicModel{ID: "model", Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}}, 0); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveCredential(ctx, "platform", "credential", "provider", "catalog test key", "secret-to-encrypt", true, 0); e != nil {
		t.Fatal(e)
	}
	supplier := SupplierModel{ID: "supplier-model", CredentialID: "credential", Provider: "openai", Model: "exact-model", Capabilities: []modelruntime.Capability{capability}, Enabled: true}
	if e := s.SaveSupplierModel(ctx, "platform", supplier, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SaveRouteConfiguration(ctx, "platform", RouteConfiguration{PublicModelID: "model", Strategy: "priority", Bindings: []RouteBinding{{SupplierModelID: "supplier-model", Enabled: true}}}, 0); e != nil {
		t.Fatal(e)
	}
	if e := s.SavePublicModel(ctx, "platform", PublicModel{ID: "model", Capabilities: []modelruntime.Capability{{Operation: modelruntime.Chat}}, Enabled: true}, 1); e != nil {
		t.Fatal(e)
	}
	if e := s.SetListing(ctx, "platform", "model", true, 0); e != nil {
		t.Fatal(e)
	}
	at := time.Now().UTC()
	retail := priceAt(uuid.NewString(), PriceScope{Kind: "retail", SellerID: "platform"}, at, "2")
	retail.NoticeID = "notice"
	retail.Rule.Components = append(retail.Rule.Components, ComponentRate{Key: "output", Unit: "token", Per: "1000000", Rate: "0"}, ComponentRate{Key: "cache", Unit: "token", Per: "1000000", Rate: "0"})
	if e := s.PublishPrice(ctx, "platform", retail, func(context.Context, *gorm.DB, PriceVersion) error { return nil }); e != nil {
		t.Fatal(e)
	}
	request, _ := modelruntime.DecodeProtocol("request", modelruntime.Chat, []byte(`{"model":"model","messages":[{"role":"user","content":"hi"}],"max_tokens":10}`))
	quote, e := s.Quote(ctx, "official", "platform", request, at)
	if e != nil || len(quote.Supplier) != 1 || quote.Supplier[0].UpstreamID == "" {
		t.Fatalf("%v %#v", e, quote.Supplier)
	}
}

func TestPublicModelKeepsOnlyTheCheckedOperation(t *testing.T) {
	got := publicCapabilityRecords([]modelruntime.Capability{{
		Operation: modelruntime.Chat, Streaming: true, BillableBounds: map[string]string{"output": "10"},
		UsageRules: []modelruntime.UsageRule{{Component: "output", Unit: "token", Path: "/completion_tokens"}},
	}})
	if len(got) != 1 || got[0].Operation != modelruntime.Chat || got[0].Streaming || len(got[0].BillableBounds) != 0 || len(got[0].UsageRules) != 0 {
		t.Fatalf("%#v", got)
	}
}

func TestPostgresFirstListingIsPerModelAndKeepsExistingListingChangesScheduled(t *testing.T) {
	s := newCommerceTestStore(t)
	seedCommerceModel(t, s)
	ctx := context.Background()
	original, err := s.PublicModel(ctx, "model")
	if err != nil {
		t.Fatal(err)
	}
	second := original
	second.ID, second.Enabled, second.Version = "second-model", false, 0
	if err = s.SavePublicModel(ctx, "platform", second, 0); err != nil {
		t.Fatal(err)
	}
	upstreams, err := s.upstreams(ctx, "model")
	if err != nil {
		t.Fatal(err)
	}
	upstream := upstreams[0]
	upstream.ID, upstream.PublicModelID, upstream.Version = "second-upstream", second.ID, 0
	if err = s.SaveUpstream(ctx, "platform", upstream, 0); err != nil {
		t.Fatal(err)
	}
	second.Enabled = true
	if err = s.SavePublicModel(ctx, "platform", second, 1); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"model", second.ID} {
		if err = s.SetAuthorization(ctx, "platform", "oem", id, true, 0); err != nil {
			t.Fatal(err)
		}
		if err = s.SetAuthorization(ctx, "oem", "channel", id, true, 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, actor := range []string{"platform", "oem", "channel"} {
		for _, id := range []string{"model", second.ID} {
			if err = s.SetListing(ctx, actor, id, true, 0); err != nil {
				t.Fatalf("first listing %s/%s: %v", actor, id, err)
			}
		}
		models, err := s.AvailableModels(ctx, actor, true)
		if err != nil || len(models) != 2 {
			t.Fatal("new model not immediately visible", actor, models, err)
		}
		if err = s.SetListing(ctx, actor, second.ID, false, 1); !errors.Is(err, ErrChangeNotice) {
			t.Fatal("existing listing change bypassed notice", actor, err)
		}
	}
}
