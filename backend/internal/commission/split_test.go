package commission

import "testing"

func TestSplitRespectsCapAndHierarchy(t *testing.T) {
	policy := &PolicyView{
		DirectBPS: 1500, IndirectBPS: 800, TotalBPS: 2000, CapBPS: 2000,
	}
	parts := splitCommission(AccrueInput{
		WholesaleMinor: 10_000, RoleID: "promoter", ParentRoleID: "agent",
	}, policy)
	var total int64
	kinds := map[string]int64{}
	for _, p := range parts {
		total += p.Amount
		kinds[p.Kind] = p.Amount
	}
	if total > 2000 {
		t.Fatalf("cap exceeded: %d", total)
	}
	if kinds[KindDirect] != 1500 {
		t.Fatalf("direct: %+v", parts)
	}
	if kinds[KindIndirect] != 500 {
		t.Fatalf("indirect should shrink to cap remainder: %+v", parts)
	}

	onlyDirect := splitCommission(AccrueInput{
		WholesaleMinor: 10_000, RoleID: "promoter",
	}, policy)
	if len(onlyDirect) != 1 || onlyDirect[0].Kind != KindDirect || onlyDirect[0].Amount != 1500 {
		t.Fatalf("no parent should be direct only: %+v", onlyDirect)
	}

	noRole := splitCommission(AccrueInput{WholesaleMinor: 10_000}, policy)
	if len(noRole) != 0 {
		t.Fatalf("no referrer should not accrue: %+v", noRole)
	}
}

func TestValidatePolicyRejectsOverCap(t *testing.T) {
	if err := validatePolicy(PolicyView{DirectBPS: 1500, OverrideBPS: 500, CapBPS: 3500, FreezeDays: 7}); err != nil {
		t.Fatalf("valid policy: %v", err)
	}
	if err := validatePolicy(PolicyView{DirectBPS: 1500, OverrideBPS: 1000, CapBPS: 3500}); err != nil {
		t.Fatal("3500 cap equals the sum")
	}
	if err := validatePolicy(PolicyView{DirectBPS: 2000, OverrideBPS: 2000, CapBPS: 3500}); err != ErrInvalid {
		t.Fatalf("sum over cap should be invalid, got %v", err)
	}
	if err := validatePolicy(PolicyView{DirectBPS: -1, CapBPS: 3500}); err != ErrInvalid {
		t.Fatalf("negative bps should be invalid, got %v", err)
	}
}
