package organization

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestPostgresTakeoverUsesDatabaseOwnershipClock(t *testing.T) {
	w := world(t)
	c, _ := w.child(t, w.platform, w.platformOwner, Channel, "Channel")
	a := w.register(t, w.platform.Brand.ID, "clock@example.test", c.InviteCode)
	old, err := w.s.CurrentOwnership(w.ctx, nil, a.Customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Registration is recorded by the database. A slower application clock must
	// never close that ownership before it became effective.
	w.s.now = func() time.Time { return old.EffectiveAt.Add(-time.Hour) }
	var next []Ownership
	err = w.db.Transaction(func(tx *gorm.DB) error {
		var e error
		next, e = w.s.ApplyChannelTakeoverOwnership(w.ctx, tx, TakeoverOwnershipInput{ChannelID: c.Operator.ID, BrandOwnerOperatorID: w.platform.Operator.ID, EvidenceReference: "approved-clock-takeover", ExpectedVersions: map[string]int64{a.Customer.ID: old.Version}})
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next) != 1 || next[0].EffectiveAt.Before(old.EffectiveAt) {
		t.Fatalf("invalid ownership chronology: old=%+v new=%+v", old, next)
	}
	var ended ownershipRow
	if err = w.db.First(&ended, "id=?", old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ended.EndedAt == nil || !ended.EndedAt.Equal(next[0].EffectiveAt) {
		t.Fatalf("ownership boundary is not continuous: old=%+v new=%+v", ended, next)
	}
}
