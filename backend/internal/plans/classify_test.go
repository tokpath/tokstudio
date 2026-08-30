package plans

import "testing"

func TestClassifyChannelPlanReview(t *testing.T) {
	cheap := CreatePlanInput{
		OwnerType: OwnerChannel, PriceMinor: PriceFloor - 1,
		Items: []PlanItemInput{{UnitType: UnitUSDCredit, Included: 1}},
	}
	status, reason := classifyPlan(cheap)
	if status != StatusPendingReview || reason != "below_price_floor" {
		t.Fatalf("cheap channel plan: %s %s", status, reason)
	}

	risky := CreatePlanInput{
		OwnerType: OwnerChannel, PriceMinor: PriceFloor,
		Items: []PlanItemInput{{UnitType: UnitVideoSecond, Included: 3601}},
	}
	status, reason = classifyPlan(risky)
	if status != StatusPendingReview || reason != "high_risk_media_quota" {
		t.Fatalf("risky media: %s %s", status, reason)
	}

	ok := CreatePlanInput{
		OwnerType: OwnerChannel, PriceMinor: PriceFloor,
		Items: []PlanItemInput{{UnitType: UnitUSDCredit, Included: 1_000_000}},
	}
	status, reason = classifyPlan(ok)
	if status != StatusPublished || reason != "" {
		t.Fatalf("normal channel plan should publish: %s %s", status, reason)
	}

	platform := CreatePlanInput{
		OwnerType: OwnerPlatform, PriceMinor: 1,
		Items: []PlanItemInput{{UnitType: UnitUSDCredit, Included: 1}},
	}
	status, _ = classifyPlan(platform)
	if status != StatusPublished {
		t.Fatalf("platform plan should skip review, got %s", status)
	}
}
