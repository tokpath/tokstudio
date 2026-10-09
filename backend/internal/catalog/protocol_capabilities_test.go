package catalog

import (
	"reflect"
	"testing"
)

func TestModelProtocolCapabilitiesFollowAdapters(t *testing.T) {
	textEndpoints := []string{"/v1/chat/completions", "/v1/responses", "/v1/messages"}
	for _, adapter := range []string{"bifrost", "openai", "anthropic", "openrouter", "google", "gemini", "test", " OpenAI "} {
		t.Run(adapter, func(t *testing.T) {
			if got := modelSupportedEndpoints("text", []string{adapter}); !reflect.DeepEqual(got, textEndpoints) {
				t.Fatalf("registered text adapter %q lost public protocols: %v", adapter, got)
			}
		})
	}
	for _, tc := range []struct {
		kind     string
		adapters []string
		want     []string
	}{
		{"text", []string{"vendor-openai", "unknown"}, []string{}},
		{"text", []string{"unknown", "openrouter"}, textEndpoints},
		{"image", []string{"openai"}, []string{}},
		{"image", []string{"openrouter"}, []string{"/v1/images/generations"}},
		{"video", []string{"ark"}, []string{"/v1/videos"}},
	} {
		if got := modelSupportedEndpoints(tc.kind, tc.adapters); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("kind=%s adapters=%v protocols=%v want=%v", tc.kind, tc.adapters, got, tc.want)
		}
	}
}
