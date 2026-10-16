package organization

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresCatalogNoticeAudienceIsCurrentScopedStableAndDeduplicated(t *testing.T) {
	w := world(t)
	child, _ := w.child(t, w.platform, w.platformOwner, Channel, "Catalog channel")
	one := w.register(t, w.platform.Brand.ID, "catalog-one@example.test", "")
	other := w.register(t, w.platform.Brand.ID, "catalog-other@example.test", child.InviteCode)
	// A person who is both a customer and staff still receives one notice.
	if e := w.db.Table("organization_staff_memberships").Create(&Membership{ID: uuid.NewString(), OperatorID: w.platform.Operator.ID, StaffID: one.Customer.ID, Role: "readonly", Status: "active", CreatedAt: time.Now().Add(-time.Hour)}).Error; e != nil {
		t.Fatal(e)
	}
	lateStaff := uuid.NewString()
	cutoff := time.Now().Add(time.Minute)
	if e := w.db.Table("organization_staff_memberships").Create(&Membership{ID: uuid.NewString(), OperatorID: w.platform.Operator.ID, StaffID: lateStaff, Role: "readonly", Status: "active", CreatedAt: cutoff.Add(time.Hour)}).Error; e != nil {
		t.Fatal(e)
	}
	got := []string{}
	cursor := ""
	for {
		page, e := w.s.CatalogNotificationAudienceAt(w.ctx, nil, w.platform.Operator.ID, cursor, cutoff, 1)
		if e != nil {
			t.Fatal(e)
		}
		if len(page) == 0 {
			break
		}
		if page[0] <= cursor {
			t.Fatal("audience cursor did not advance")
		}
		got = append(got, page...)
		cursor = page[0]
	}
	want := []string{w.platformOwner, one.Customer.ID}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("notice audience=%v want=%v; other=%s late=%s", got, want, other.Customer.ID, lateStaff)
	}
	operators, e := w.s.CatalogNotificationOperators(w.ctx, nil, w.platform.Operator.ID)
	if e != nil || len(operators) != 2 || !slices.Contains(operators, child.Operator.ID) {
		t.Fatalf("affected operators=%v %v", operators, e)
	}
	if e = w.db.Table("organization_staff_memberships").Where("operator_id=? AND staff_id=?", w.platform.Operator.ID, one.Customer.ID).Update("status", "suspended").Error; e != nil {
		t.Fatal(e)
	}
	if e = w.db.Table("organization_customers").Where("id=?", one.Customer.ID).Update("status", "suspended").Error; e != nil {
		t.Fatal(e)
	}
	remaining, e := w.s.CatalogNotificationAudienceAt(w.ctx, nil, w.platform.Operator.ID, "", cutoff, 100)
	if e != nil || !slices.Equal(remaining, []string{w.platformOwner}) {
		t.Fatalf("suspended recipient remains=%v %v", remaining, e)
	}
}
