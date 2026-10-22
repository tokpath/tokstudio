package modelpricing

import (
	"context"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresSheetSyncPinsAcceptedPricesAndRetainsGoodSheet(t *testing.T) {
	db := dbtest.New(t)
	raw, e := migrations.ReadFile("migrate/0001_init.sql")
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(string(raw)).Error; e != nil {
		t.Fatal(e)
	}
	var requests atomic.Int64
	var fail atomic.Bool
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		fmt.Fprint(w, `{"gpt-test":{"provider":"openai","mode":"chat","input_cost_per_token":0.001,"output_cost_per_token":0.002},"metadata":{"provider":"openai","mode":"chat"}}`)
	}))
	defer feed.Close()
	s := Service{DB: db, AllowTestLoopback: true}
	ctx := context.Background()
	cfg, _ := s.Settings(ctx)
	cfg.SourceURL = feed.URL
	if e = s.Update(ctx, cfg, cfg.Version); e != nil {
		t.Fatal(e)
	}
	if e = s.RunSync(ctx); e != nil {
		t.Fatal(e)
	}
	cfg, e = s.Settings(ctx)
	if e != nil || cfg.LastSuccessAt == nil || cfg.LastError != "" || cfg.SheetVersion == "" {
		t.Fatal(cfg, e)
	}
	rows, e := s.Rows(ctx, "")
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	pinned, e := s.Snapshot(ctx, "openai", "gpt-test", time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RunSync(ctx); e != nil || requests.Load() != 1 {
		t.Fatal("24h scheduler refetched immediately", requests.Load(), e)
	}
	fail.Store(true)
	_ = s.RequestSync(ctx)
	if e = s.RunSync(ctx); e != nil {
		t.Fatal(e)
	}
	broken, _ := s.Settings(ctx)
	if broken.LastError == "" || broken.SheetVersion != cfg.SheetVersion || broken.LastSuccessAt == nil {
		t.Fatal("failed feed erased last good sheet", broken)
	}
	rows, _ = s.Rows(ctx, "")
	if len(rows) != 1 {
		t.Fatal("failed sync erased cache")
	}
	if len(pinned) == 0 {
		t.Fatal("accepted quote did not pin cost basis")
	}

	broken.Enabled = false
	if e = s.Update(ctx, broken, broken.Version); e != nil {
		t.Fatal(e)
	}
	if e = s.RunSync(ctx); e != nil || requests.Load() != 2 {
		t.Fatal("disabled automatic sync still fetched", requests.Load(), e)
	}
	broken, _ = s.Settings(ctx)
	s.AllowTestLoopback = false
	cfg.SourceURL = feed.URL
	if e = s.Update(ctx, cfg, broken.Version); e != ErrInput {
		t.Fatal("production accepted insecure feed", e)
	}
}
func TestPostgresActualCostRegistrationStaysSimpleAndScoped(t *testing.T) {
	db := dbtest.New(t)
	raw, _ := migrations.ReadFile("migrate/0001_init.sql")
	if e := db.Exec(string(raw)).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(catalog.CommerceSchema()).Error; e != nil {
		t.Fatal(e)
	}
	cat := catalog.CommerceCatalog{DB: db, PlatformOperatorID: "platform", EncryptionKey: "test-encryption-key"}
	if _, e := cat.SaveCredential(context.Background(), "platform", "key", "provider", "Test supplier", "secret", true, 0); e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(`INSERT INTO tokenhub_supplier_models(id,provider_id,definition,version) VALUES('model','provider','{}',1)`).Error; e != nil {
		t.Fatal(e)
	}
	s := Service{DB: db}
	ctx := context.Background()
	row := ActualCost{ID: "record", ProviderID: "provider", SupplierModelID: "model", StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now(), AmountUSD: ledger.MustDecimal("92"), Note: "monthly bill"}
	if e := s.SaveActual(ctx, row, 0); e != nil {
		t.Fatal(e)
	}
	records, e := s.Actuals(ctx)
	if e != nil || len(records) != 1 || records[0].AmountUSD.String() != "92" {
		t.Fatal(records, e)
	}
	row.AmountUSD = ledger.MustDecimal("91.5")
	if e = s.SaveActual(ctx, row, 1); e != nil {
		t.Fatal(e)
	}
	if e = s.SaveActual(ctx, row, 1); e != ErrVersion {
		t.Fatal("stale edit accepted", e)
	}
	row.ProviderID = "other"
	if e = s.SaveActual(ctx, row, 2); e != ErrInput {
		t.Fatal("cross-supplier model accepted", e)
	}
}
