package incentives

import (
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
)

func TestPostgresIncentiveAuditRequiresExactFinanceScopeAndPreservesEvidence(t *testing.T) {
	f := setup(t)
	for range 3 {
		if e := f.s.event(f.db, f.operator, uuid.NewString(), "audit.test", map[string]any{"actor_id": f.owner, "evidence": "independent verified fact"}); e != nil {
			t.Fatal(e)
		}
	}
	rows, next, e := f.s.ListAuditEvents(f.ctx, nil, f.owner, f.operator, "", 2)
	if e != nil || len(rows) != 2 || next == "" {
		t.Fatal(rows, next, e)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.ID] = true
		if r.OperatorID != f.operator || len(r.Data) == 0 {
			t.Fatal(r)
		}
	}
	older, _, e := f.s.ListAuditEvents(f.ctx, nil, f.owner, f.operator, next, 200)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range older {
		if seen[r.ID] {
			t.Fatal("cursor repeated event", r)
		}
	}
	if _, _, e = f.s.ListAuditEvents(f.ctx, nil, uuid.NewString(), f.operator, "", 10); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("foreign staff read incentives", e)
	}
	if e = f.db.Exec("UPDATE incentives_events SET payload='{}'").Error; e == nil {
		t.Fatal("financial audit mutable")
	}
	if e = f.db.Exec("UPDATE incentives_events SET published_at=clock_timestamp()").Error; e != nil {
		t.Fatal("relay acknowledgement blocked", e)
	}
}
