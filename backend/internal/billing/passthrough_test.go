package billing

import "testing"

func TestUsageViewDoesNotInventUpstreamFacts(t *testing.T) {
	views := usageViews([]usageRow{{
		ID: "usg_1", RequestID: "req_1", UserID: "usr_1", PublicModelID: "tokenhub/echo-1",
		State: UsagePending,
	}})
	if len(views) != 1 {
		t.Fatalf("views: %+v", views)
	}
	if views[0].ProviderID != "" || views[0].UpstreamModelID != "" || views[0].FactSource != "" {
		t.Fatalf("missing pass-through must stay empty: %+v", views[0])
	}
	if views[0].PromptTokens != 0 || views[0].CustomerMinor != 0 {
		t.Fatalf("missing usage must not invent tokens or amounts: %+v", views[0])
	}
}

func TestClassifyDiffKeepsHonestUpstreamFacts(t *testing.T) {
	empty := classifyDiff(diffInput{RequestID: "req_gap", MissingUsage: true, State: UsagePending})
	if empty.ProviderID != "" || empty.UpstreamModelID != "" {
		t.Fatalf("must not invent provider/model: %+v", empty)
	}
	present := classifyDiff(diffInput{
		RequestID: "req_ok", ProviderID: "prd_echo", UpstreamModelID: "echo-up",
		FactSource: FactSourceSandbox, AttemptID: "atm_1",
		State: UsageConfirmed, UsageMinor: 1, ChargeMinor: 1, ChargeCount: 1,
	})
	if present.ProviderID != "prd_echo" || present.UpstreamModelID != "echo-up" || present.FactSource != FactSourceSandbox {
		t.Fatalf("TokenHub facts must pass through: %+v", present)
	}
}
