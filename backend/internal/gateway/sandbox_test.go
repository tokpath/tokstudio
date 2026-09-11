package gateway

import "testing"

func TestNormalizeUsageMode(t *testing.T) {
	if got := NormalizeUsageMode("", false); got != "" {
		t.Fatalf("default passthrough: %q", got)
	}
	if got := NormalizeUsageMode("content", true); got != UsageOmit {
		t.Fatalf("omitUsage flag: %q", got)
	}
	if got := NormalizeUsageMode("omit", false); got != UsageOmit {
		t.Fatalf("omit mode: %q", got)
	}
	if got := NormalizeSandboxMode("REASONING", false); got != "" {
		t.Fatalf("legacy content modes must not invent usage: %q", got)
	}
}

func TestResolveAttemptUsagePassthroughOnly(t *testing.T) {
	empty := resolveAttemptUsage("test", "", ChatRequest{}, "", nil)
	if len(empty) != 0 {
		t.Fatalf("nil usage must stay empty: %+v", empty)
	}
	kept := resolveAttemptUsage("bifrost", "", ChatRequest{}, "", map[string]int{"prompt_tokens": 3})
	if kept["prompt_tokens"] != 3 {
		t.Fatalf("passthrough: %+v", kept)
	}
	omit := resolveAttemptUsage("bifrost", "omit", ChatRequest{}, "", map[string]int{"prompt_tokens": 8})
	if len(omit) != 0 {
		t.Fatalf("omit must clear: %+v", omit)
	}
}
