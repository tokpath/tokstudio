package catalog

import "testing"

func TestPriceEstimateCoverage(t *testing.T) {
	for _, tc := range []struct {
		name, prices string
		media, want  bool
	}{
		{"default reasoning", `{"input":"0.000001","output":"0.000002","reasoning":"0.000003"}`, false, true},
		{"unknown charged cache", `{"input":"0.000001","cache_read":"0.000001"}`, false, false},
		{"zero unknown dimension", `{"input":"0","cache_read":"0"}`, false, true},
		{"media dimensions", `{"video_second":"0.1","audio_second":"0.01","input":"0.000001"}`, true, true},
		{"media on text path", `{"input":"0.000001","video_second":"0.1"}`, false, false},
		{"negative", `{"input":"-1"}`, false, false},
		{"non USD", `{"currency":"EUR","input":"1"}`, false, false},
		{"absent customer price", `{"wholesale_input":"1","upstream_cost_input":"1"}`, false, false},
		{"bad rate", `{"input":"unknown"}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := estimablePrices([]byte(tc.prices), tc.media); got != tc.want {
				t.Fatalf("estimate coverage=%v want=%v", got, tc.want)
			}
		})
	}
	for _, adapter := range []string{"bifrost", "openai", "anthropic", "openrouter", "google", "gemini", "test"} {
		if !TextBudgetCandidate(RouteCandidate{Adapter: adapter, BaseURL: "https://compatible.example.test/v1", UpstreamModelID: "any-model"}) {
			t.Fatalf("adapter %s still has a hard-ceiling whitelist", adapter)
		}
	}
	if TextBudgetCandidate(RouteCandidate{Adapter: "not-implemented"}) {
		t.Fatal("unknown adapter advertised estimate support")
	}
}
