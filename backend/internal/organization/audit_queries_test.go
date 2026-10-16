package organization

import (
	"errors"
	"testing"
	"time"
)

func TestPostgresAuditCursorTiesScopeAndImmutableFacts(t *testing.T) {
	w := world(t)
	child, childOwner := w.child(t, w.platform, w.platformOwner, Channel, "Independent channel")
	w.s.now = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	for range 3 {
		if e := w.s.event(w.db, w.platform.Operator.ID, w.platformOwner, "audit.test", "evidence", map[string]any{"before": map[string]int{"version": 1}, "after": map[string]int{"version": 2}}); e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for range 30 {
		rows, next, e := w.s.ListAuditEvents(w.ctx, nil, w.platformOwner, w.platform.Operator.ID, cursor, 1)
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range rows {
			if r.OperatorID != w.platform.Operator.ID || seen[r.ID] {
				t.Fatal("audit skipped/duplicated scope", r)
			}
			seen[r.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	var count int64
	if e := w.db.Table("organization_events").Where("operator_id=?", w.platform.Operator.ID).Count(&count).Error; e != nil || int64(len(seen)) != count {
		t.Fatalf("cursor lost facts seen=%d count=%d error=%v", len(seen), count, e)
	}
	if _, _, e := w.s.ListAuditEvents(w.ctx, nil, w.platformOwner, child.Operator.ID, "", 10); !errors.Is(e, ErrForbidden) {
		t.Fatal("parent gained child audit access", e)
	}
	if _, _, e := w.s.ListAuditEvents(w.ctx, nil, childOwner, child.Operator.ID, "", 10); e != nil {
		t.Fatal(e)
	}
	if e := w.db.Exec("UPDATE organization_events SET evidence_reference='rewritten' WHERE operator_id=?", w.platform.Operator.ID).Error; e == nil {
		t.Fatal("audit facts mutable")
	}
}
