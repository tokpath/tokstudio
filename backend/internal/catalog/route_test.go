package catalog

import "testing"

func TestApplyStrategy(t *testing.T) {
	primary := RouteCandidate{ProviderSlug: "echo-primary", Priority: 1, Weight: 1, Health: "degraded", CostMinor: 100}
	backup := RouteCandidate{ProviderSlug: "echo-backup", Priority: 2, Weight: 9, Health: "available", CostMinor: 10}

	health := []RouteCandidate{primary, backup}
	applyStrategy(health, StrategyHealth)
	if health[0].ProviderSlug != "echo-backup" {
		t.Fatalf("health should prefer available: %+v", health)
	}

	price := []RouteCandidate{primary, backup}
	applyStrategy(price, StrategyPrice)
	if price[0].ProviderSlug != "echo-backup" {
		t.Fatalf("price should prefer cheaper: %+v", price)
	}

	weight := []RouteCandidate{primary, backup}
	applyStrategy(weight, StrategyWeight)
	if weight[0].ProviderSlug != "echo-backup" {
		t.Fatalf("weight should prefer higher weight: %+v", weight)
	}

	priority := []RouteCandidate{backup, primary}
	applyStrategy(priority, StrategyPriority)
	if priority[0].ProviderSlug != "echo-primary" {
		t.Fatalf("priority should keep lower priority number first: %+v", priority)
	}

	cheap := costMinor([]byte(`{"upstream_cost_input":"0.0000001"}`))
	dear := costMinor([]byte(`{"upstream_cost_input":"0.0000004"}`))
	if cheap == 0 || cheap >= dear {
		t.Fatalf("tiny USD prices must stay ordered: cheap=%d dear=%d", cheap, dear)
	}
}
