package branding

import (
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"testing"
)

func TestPostgresAppearanceKeepsCurrentEditorWithoutGrowingAudit(t *testing.T) {
	f := setup(t)
	child, childOwner := f.child(t, f.platform, f.owner, organization.Channel)
	for i, color := range []string{"#123456", "#654321"} {
		saved, err := f.s.SetChannelTheme(f.ctx, nil, SetThemeInput{ActorStaffID: childOwner, TargetID: child.Operator.ID, ExpectedVersion: int64(i), Values: ThemeOverrides{PrimaryColor: &color}})
		if err != nil || saved.UpdatedBy != childOwner || saved.UpdatedAt.IsZero() {
			t.Fatal(saved, err)
		}
	}
	saved, err := f.s.ThemeConfiguration(f.ctx, nil, childOwner, "channel", child.Operator.ID)
	if err != nil || saved.Values.PrimaryColor == nil || *saved.Values.PrimaryColor != "#654321" {
		t.Fatal(saved, err)
	}
	if _, err = f.s.ThemeConfiguration(f.ctx, nil, f.owner, "channel", child.Operator.ID); !errors.Is(err, organization.ErrForbidden) {
		t.Fatal("parent accessed channel customization", err)
	}
	var count int64
	if err = f.db.Table("branding_changes").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("appearance saves still create history", count, err)
	}
	if _, err = f.s.RequestDomain(f.ctx, nil, f.owner, f.platform.Brand.ID, "audit.example.test"); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Exec("UPDATE branding_changes SET after_json='{}'").Error; err == nil {
		t.Fatal("important domain history became mutable")
	}
}
