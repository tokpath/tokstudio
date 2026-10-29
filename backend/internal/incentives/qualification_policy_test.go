package incentives

import "testing"

func TestPostgresLowerThresholdQualifiesAtPolicyInstantWithoutReplayingEarlierCommissions(t *testing.T) {
	f := setup(t)
	if _, e := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("200"), DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 3600}); e != nil {
		t.Fatal(e)
	}
	inviter := f.register(t, "lower-threshold-inviter@example.test", "")
	f.buy(t, inviter.Customer.ID, "real-own-paid100", "100", ActivitySelection{})
	f.consume(t, inviter.Customer.ID, "100")
	before, e := f.s.GetQualification(f.ctx, nil, f.operator, inviter.Customer.ID)
	if e != nil || before.AcquiredAt != nil || before.ConfirmedMoney.Cmp(d("100")) != 0 {
		t.Fatal(before, e)
	}
	child := f.register(t, "lower-threshold-child@example.test", inviter.InviteCode)
	f.buy(t, child.Customer.ID, "real-child-paid100", "100", ActivitySelection{})
	f.consume(t, child.Customer.ID, "40")
	lowered, e := f.s.SetPolicy(f.ctx, nil, f.owner, f.operator, PolicyInput{ExpectedVersion: 2, ThresholdMoney: d("100"), DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 3600})
	if e != nil {
		t.Fatal(e)
	}
	// No worker or manual qualification operation occurs between the policy
	// change and the next real child consumption decision.
	f.consume(t, child.Customer.ID, "60")
	after, e := f.s.GetQualification(f.ctx, nil, f.operator, inviter.Customer.ID)
	if e != nil || after.AcquiredAt == nil || !after.AcquiredAt.Equal(lowered.EffectiveAt) {
		t.Fatal("lower threshold delayed until worker/read time", after, lowered, e)
	}
	entries, e := f.s.CommissionEntries(f.ctx, nil, f.operator, inviter.Customer.ID)
	if e != nil || len(entries) != 1 || entries[0].AmountMoney.Cmp(d("9")) != 0 {
		t.Fatal("pre-upgrade consumption was replayed or later consumption lost", entries, e)
	}
}
