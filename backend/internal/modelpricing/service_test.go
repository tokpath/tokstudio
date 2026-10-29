package modelpricing

import (
	"context"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
	"io/fs"
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
	fixed, _ := migrations.ReadFile("migrate/0002_fixed_official_sync.sql")
	if e = db.Exec(string(fixed)).Error; e != nil {
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
	s := Service{DB: db, AllowTestLoopback: true, TestSourceURL: feed.URL}
	ctx := context.Background()
	if e = s.RunSync(ctx); e != nil {
		t.Fatal(e)
	}
	cfg, e := s.Settings(ctx)
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

	if e = s.RunSync(ctx); e != nil || requests.Load() != 2 {
		t.Fatal("failed sync retried without backoff", requests.Load(), e)
	}
	if e = db.Model(&Settings{}).Where("id='platform'").Updates(map[string]any{"last_success_at": time.Now().Add(-25 * time.Hour), "last_attempt_at": time.Now().Add(-6 * time.Minute)}).Error; e != nil {
		t.Fatal(e)
	}
	fail.Store(false)
	if e = s.RunSync(ctx); e != nil || requests.Load() != 3 {
		t.Fatal("24h automatic sync did not run", requests.Load(), e)
	}

}
func TestPostgresActualCostRegistrationStaysSimpleAndScoped(t *testing.T) {
	db := dbtest.New(t)
	_, ledgerFiles := ledger.Migrations()
	entries, _ := fs.ReadDir(ledgerFiles, ".")
	for _, entry := range entries {
		raw, _ := fs.ReadFile(ledgerFiles, entry.Name())
		if err := db.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
	files, _ := fs.ReadDir(migrations, "migrate")
	for _, file := range files {
		raw, _ := migrations.ReadFile("migrate/" + file.Name())
		if e := db.Exec(string(raw)).Error; e != nil {
			t.Fatal(e)
		}
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
	beforeRegistration := time.Now().Add(-time.Second)
	row := ActualCost{ID: "record", ProviderID: "provider", SupplierModelID: "model", StartsAt: time.Now().Add(-time.Hour), EndsAt: time.Now(), Currency: "CNY", Amount: ledger.MustDecimal("92"), Note: "monthly bill", CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}
	if e := s.SaveActual(ctx, row, 0); e != nil {
		t.Fatal(e)
	}
	records, e := s.Actuals(ctx)
	if e != nil || len(records) != 1 || records[0].Amount.String() != "92" || records[0].Currency != "CNY" || records[0].CreatedAt.Before(beforeRegistration) {
		t.Fatal(records, e)
	}
	row.Amount = ledger.MustDecimal("91.5")
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
