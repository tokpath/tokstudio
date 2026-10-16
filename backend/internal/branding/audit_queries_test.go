package branding

import (
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
)

func TestPostgresBrandAuditReturnsActualBeforeAfterAndParentScope(t *testing.T) {
	f := setup(t)
	child, childOwner := f.child(t, f.platform, f.owner, organization.Channel)
	color := "#123456"
	if _, e := f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: child.Operator.ID, Values: ThemeOverrides{PrimaryColor: &color}}); e != nil {
		t.Fatal(e)
	}
	color = "#654321"
	if _, e := f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: f.owner, TargetID: child.Operator.ID, ExpectedVersion: 1, Values: ThemeOverrides{PrimaryColor: &color}}); e != nil {
		t.Fatal(e)
	}
	rows, next, e := f.s.ListAuditEvents(f.ctx, nil, f.owner, child.Operator.ID, "", 1)
	if e != nil || len(rows) != 1 || next == "" || len(rows[0].Before) == 0 || len(rows[0].After) == 0 || rows[0].ActorID != f.owner {
		t.Fatal(rows, next, e)
	}
	older, _, e := f.s.ListAuditEvents(f.ctx, nil, f.owner, child.Operator.ID, next, 1)
	if e != nil || len(older) != 1 || older[0].ID == rows[0].ID {
		t.Fatal(older, e)
	}
	if _, _, e = f.s.ListAuditEvents(f.ctx, nil, childOwner, child.Operator.ID, "", 10); !errors.Is(e, organization.ErrForbidden) {
		t.Fatal("channel self accessed parent customization", e)
	}
	brandRows, _, e := f.s.ListAuditEvents(f.ctx, nil, f.owner, f.platform.Operator.ID, "", 10)
	if e != nil || len(brandRows) != 0 {
		t.Fatal("brand cursor included channel-specific facts", brandRows, e)
	}
	if e = f.db.Exec("UPDATE branding_changes SET after_json='{}'").Error; e == nil {
		t.Fatal("branding audit mutable")
	}
}
