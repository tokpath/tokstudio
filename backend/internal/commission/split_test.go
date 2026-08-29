package commission

import (
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/billing"
)

func TestSplitRespectsCapAndHierarchy(t *testing.T) {
	policy := &PolicyView{
		DirectBPS: 2000, OverrideBPS: 1000, ChannelBPS: 1000, TeamBPS: 500, CapBPS: 3500,
	}
	parts := splitCommission(AccrueInput{
		WholesaleMinor: 10_000, RoleID: "kol2", RoleType: "kol_l2", ParentRoleID: "kol1",
	}, policy)
	var total int64
	for _, p := range parts {
		total += p.Amount
	}
	if total > 3500 {
		t.Fatalf("cap exceeded: %d", total)
	}
	if parts[0].Kind != KindTeam || parts[0].Amount != 0 {
		t.Fatalf("team should shrink first: %+v", parts[0])
	}

	noRole := splitCommission(AccrueInput{WholesaleMinor: 10_000}, policy)
	if noRole[1].Amount != 10_000*int64(billing.CommissionRateBPS)/10000 {
		t.Fatalf("no-role should keep M3 10 percent channel: %+v", noRole)
	}
}

func TestValidatePolicyRejectsOverCap(t *testing.T) {
	if err := validatePolicy(PolicyView{DirectBPS: 2000, OverrideBPS: 1000, ChannelBPS: 1000, CapBPS: 3500, FreezeDays: 7}); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
	if err := validatePolicy(PolicyView{DirectBPS: 2000, OverrideBPS: 1000, ChannelBPS: 1000, TeamBPS: 500, CapBPS: 3500}); err != nil {
		t.Fatal("3500 cap equals the sum")
	}
	if err := validatePolicy(PolicyView{DirectBPS: 2000, OverrideBPS: 2000, ChannelBPS: 2000, CapBPS: 3500}); err != ErrInvalid {
		t.Fatalf("sum over cap should be invalid, got %v", err)
	}
	if err := validatePolicy(PolicyView{DirectBPS: -1, CapBPS: 3500}); err != ErrInvalid {
		t.Fatalf("negative bps should be invalid, got %v", err)
	}
}
