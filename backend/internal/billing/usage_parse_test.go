package billing

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseUnitUsage(t *testing.T) {
	prompt, completion, reasoning := ParseUnitUsage(json.RawMessage(`{"prompt_tokens":40,"completion_tokens":9,"reasoning_tokens":3}`))
	if prompt != 40 || completion != 9 || reasoning != 3 {
		t.Fatalf("got %d %d %d", prompt, completion, reasoning)
	}
	prompt, completion, reasoning = ParseUnitUsage(json.RawMessage(`{"missing":true}`))
	if prompt != 0 || completion != 0 || reasoning != 0 {
		t.Fatalf("missing usage should be zero: %d %d %d", prompt, completion, reasoning)
	}
}

func TestUsageViewsIncludeAPIKey(t *testing.T) {
	key := "key_demo"
	channel := "chn_official_a"
	provider := "prov_echo"
	rows := []usageRow{{
		ID: "usg_1", RequestID: "req_1", UserID: "usr_1",
		APIKeyID: &key, ChannelOrgID: &channel, PublicModelID: "tokenhub/echo-1", ProviderID: &provider,
		UnitUsage:           json.RawMessage(`{"prompt_tokens":16,"completion_tokens":5}`),
		CustomerAmountMinor: 26, WholesaleAmountMinor: 18, State: UsageConfirmed, OccurredAt: time.Unix(0, 0).UTC(),
	}}
	views := usageViews(rows)
	if len(views) != 1 {
		t.Fatalf("len=%d", len(views))
	}
	if views[0].APIKeyID != key || views[0].PromptTokens != 16 || views[0].CompletionTokens != 5 {
		t.Fatalf("view: %+v", views[0])
	}
}
