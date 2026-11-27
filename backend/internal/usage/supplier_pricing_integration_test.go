package usage

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type pricingStatementLog struct {
	logger.Interface
	statements []string
}

func (l *pricingStatementLog) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	if strings.Contains(sql, "model_pricing_settings") || strings.Contains(sql, "model_pricing_rows") {
		l.statements = append(l.statements, sql)
	}
}

func TestPostgresAdmissionPinsOneSupplierSheetWithSingleConnection(t *testing.T) {
	f := setup(t, "100", nil)
	_, migrations := modelpricing.Migrations()
	files, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		sql, err := fs.ReadFile(migrations, file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if err = f.db.Exec(string(sql)).Error; err != nil {
			t.Fatal(err)
		}
	}

	r := request(t, false)
	upstreams, err := f.cat.AdminUpstreams(f.ctx, f.operator, r.PublicModel)
	if err != nil || len(upstreams) != 1 {
		t.Fatal(upstreams, err)
	}
	for _, route := range []struct{ id, provider, model string }{
		{"duplicate-cost", "openai", "test-chat"},
		{"other-provider", "anthropic", "test-chat"},
		{"missing-cost", "openai", "unpriced"},
	} {
		upstream := upstreams[0]
		upstream.ID, upstream.CostProvider, upstream.CostModel = route.id, route.provider, route.model
		if err = f.cat.SaveUpstream(f.ctx, f.operator, upstream, 0); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	var schema string
	if err = f.db.Raw("SELECT current_schema()").Row().Scan(&schema); err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(os.Getenv("TOKENHUB_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	writer, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close(context.Background())

	replaceSheet := func(version string, rate int) pgx.Tx {
		t.Helper()
		tx, err := writer.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, "DELETE FROM model_pricing_rows"); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO model_pricing_rows
SELECT provider || '|' || model || '|' || mode, provider, model, mode,
	jsonb_build_object('provider', provider, 'model', model, 'mode', mode,
		'input_cost_per_token', $1::int / 1000.0, 'output_cost_per_token', $1::int / 1000.0)
FROM (VALUES ('openai','test-chat','chat'), ('openai','test-chat','embedding'),
	('anthropic','test-chat','chat'), ('anthropic','unpriced','chat')) AS prices(provider, model, mode)`, rate); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, "UPDATE model_pricing_settings SET sheet_version=$1 WHERE id='platform'", version); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	if err = replaceSheet("old-sheet", 1).Commit(ctx); err != nil {
		t.Fatal(err)
	}
	pendingRefresh := replaceSheet("new-sheet", 2)
	defer pendingRefresh.Rollback(context.Background())

	sqlDB, err := f.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	queries := &pricingStatementLog{Interface: logger.Discard}
	f.s.DB = f.db.Session(&gorm.Session{Logger: queries})
	f.s.Catalog.Costs = &modelpricing.Service{DB: f.db}
	admit := func(key string) Request {
		t.Helper()
		queries.statements = nil
		waitCount := sqlDB.Stats().WaitCount
		admitted, created, err := f.s.Admit(ctx, f.key.Key, key, r)
		if err != nil || !created || admitted.CustomerHoldID == nil {
			t.Fatalf("admit with one connection: created=%v hold=%v err=%v", created, admitted.CustomerHoldID, err)
		}
		if sqlDB.Stats().WaitCount != waitCount {
			t.Fatal("admission waited for another connection")
		}
		if len(queries.statements) != 1 ||
			!strings.Contains(queries.statements[0], "model_pricing_settings") ||
			!strings.Contains(queries.statements[0], "model_pricing_rows") {
			t.Fatalf("admission must read price rows and version in a single statement, got %d pricing statements", len(queries.statements))
		}
		return admitted
	}
	check := func(admitted Request, version string, rate float64) {
		t.Helper()
		stored, err := f.s.GetRequest(ctx, f.customer, admitted.ID)
		if err != nil {
			t.Fatal(err)
		}
		var quote catalog.ModelQuote
		if err = json.Unmarshal(stored.PriceSnapshot, &quote); err != nil {
			t.Fatal(err)
		}
		expected := map[string]int{"upstream": 2, "duplicate-cost": 2, "other-provider": 1, "missing-cost": 0}
		for _, supplier := range quote.Supplier {
			var snapshot modelpricing.Snapshot
			wantRows, exists := expected[supplier.UpstreamID]
			if err = json.Unmarshal(supplier.Snapshot, &snapshot); err != nil || !exists ||
				snapshot.Version != version || !snapshot.AcceptedAt.Equal(stored.AcceptedAt) || len(snapshot.Rows) != wantRows {
				t.Fatalf("supplier %q: snapshot=%+v want version=%s rows=%d err=%v", supplier.UpstreamID, snapshot, version, wantRows, err)
			}
			if wantRows == 0 && (snapshot.Rows != nil || supplier.EstimateMoney != nil) {
				t.Fatal("missing sheet rows must have an unavailable estimate and a null row list")
			}
			if wantRows > 0 && supplier.EstimateMoney == nil {
				t.Fatalf("priced supplier %q has no estimate", supplier.UpstreamID)
			}
			for _, row := range snapshot.Rows {
				if row.InputCostPerToken == nil || *row.InputCostPerToken != rate {
					t.Fatalf("supplier %q did not pin the rates for %s", supplier.UpstreamID, version)
				}
			}
			delete(expected, supplier.UpstreamID)
		}
		if len(expected) != 0 || !quote.AcceptedAt.Equal(stored.AcceptedAt) {
			t.Fatalf("missing supplier snapshots %v or admission timestamp mismatch", expected)
		}
	}

	before := admit("before-sheet-commit")
	if err = pendingRefresh.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	after := admit("after-sheet-commit")
	check(before, "old-sheet", .001)
	check(after, "new-sheet", .002)
}
