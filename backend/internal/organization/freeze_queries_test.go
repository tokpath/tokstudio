package organization

import (
	"errors"
	"testing"
	"time"
)

func TestPostgresFreezeListExactScopeAndLatestDisabledVersion(t *testing.T) {
	w := world(t)
	c, cOwner := w.child(t, w.platform, w.platformOwner, Channel, "Other scope")
	user := w.register(t, w.platform.Brand.ID, "freeze-list@example.test", "")
	review := time.Now().Add(24 * time.Hour)
	in := ScopeFreezeInput{ActorStaffID: w.platformOwner, OperatorID: w.platform.Operator.ID, ScopeKind: "customer", ScopeID: user.Customer.ID, Action: "withdrawal", Enabled: true, ReviewDueAt: &review, Evidence: "verified case", Explanation: "review withdrawal activity"}
	first, e := w.s.SetScopeFreeze(w.ctx, nil, in)
	if e != nil {
		t.Fatal(e)
	}
	in.Enabled = false
	in.ExpectedVersion = first.Version
	in.Evidence = "review cleared"
	if _, e = w.s.SetScopeFreeze(w.ctx, nil, in); e != nil {
		t.Fatal(e)
	}
	rows, e := w.s.ListScopeFreezes(w.ctx, nil, w.platformOwner, w.platform.Operator.ID)
	if e != nil || len(rows) != 1 || rows[0].Enabled || rows[0].Version != 2 || rows[0].ScopeID != user.Customer.ID {
		t.Fatal(rows, e)
	}
	rows, e = w.s.ListScopeFreezes(w.ctx, nil, cOwner, c.Operator.ID)
	if e != nil || len(rows) != 0 {
		t.Fatal("foreign customer sanctions exposed", rows, e)
	}
	if _, e = w.s.ListScopeFreezes(w.ctx, nil, w.platformOwner, c.Operator.ID); !errors.Is(e, ErrForbidden) {
		t.Fatal("parent inherited security scope", e)
	}
}
