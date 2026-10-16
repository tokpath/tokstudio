package commerce

import (
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
)

func TestPostgresCommerceAuditExactScopeCursorAndImmutablePayload(t *testing.T) {
	f := setup(t)
	for _, key := range []string{"audit-a", "audit-b"} {
		f.order(t, key, "0", "0")
	}
	seen := map[string]bool{}
	cursor := ""
	for range 30 {
		rows, next, e := f.s.ListAuditEvents(f.ctx, nil, f.owner, f.operator, cursor, 1)
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range rows {
			if r.OperatorID != f.operator || seen[r.ID] || len(r.Data) == 0 {
				t.Fatal("invalid audit row", r)
			}
			seen[r.ID] = true
		}
		if next == "" {
			break
		}
		cursor = next
	}
	var count int64
	if e := f.db.Table("commerce_events").Where("operator_id=?", f.operator).Count(&count).Error; e != nil || count != int64(len(seen)) {
		t.Fatalf("pagination lost audit %d/%d %v", len(seen), count, e)
	}
	if _, _, e := f.s.ListAuditEvents(f.ctx, nil, uuid.NewString(), f.operator, "", 10); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("unauthorized audit access", e)
	}
	if e := f.db.Exec("UPDATE commerce_events SET payload='{}'").Error; e == nil {
		t.Fatal("financial evidence editable")
	}
	if e := f.db.Exec("UPDATE commerce_events SET published_at=clock_timestamp()").Error; e != nil {
		t.Fatal("relay acknowledgement blocked", e)
	}
}
