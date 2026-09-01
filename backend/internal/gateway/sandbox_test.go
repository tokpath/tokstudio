package gateway

import "testing"

func TestApplySandboxUsageModes(t *testing.T) {
	chat := ChatRequest{Messages: []ChatMessage{{Role: "user", Content: "hello-world-long"}}}
	fixed := ApplySandboxUsage("fixed", chat, "echo", map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12})
	if fixed["prompt_tokens"] != 8 || fixed["completion_tokens"] != 4 {
		t.Fatalf("fixed should keep adapter usage: %+v", fixed)
	}
	content := ApplySandboxUsage("content", chat, "echo:hello-world-long via lab", nil)
	if content["prompt_tokens"] != len([]rune("hello-world-long")) {
		t.Fatalf("content prompt should match rune length: %+v", content)
	}
	if content["completion_tokens"] != len([]rune("echo:hello-world-long via lab")) {
		t.Fatalf("content completion should match reply length: %+v", content)
	}
	short := ApplySandboxUsage("content", ChatRequest{Messages: []ChatMessage{{Content: "hi"}}}, "ok", nil)
	if short["prompt_tokens"] != 8 || short["completion_tokens"] != 4 {
		t.Fatalf("short text should floor at 8/4: %+v", short)
	}
	reasoning := ApplySandboxUsage("reasoning", chat, "reply", nil)
	if reasoning["reasoning_tokens"] != 3 || reasoning["total_tokens"] != reasoning["prompt_tokens"]+reasoning["completion_tokens"]+3 {
		t.Fatalf("reasoning should add 3 tokens: %+v", reasoning)
	}
	omit := ApplySandboxUsage("omit", chat, "x", map[string]int{"prompt_tokens": 8})
	if len(omit) != 0 {
		t.Fatalf("omit should drop usage: %+v", omit)
	}
}

func TestNormalizeSandboxMode(t *testing.T) {
	if got := NormalizeSandboxMode("", false); got != SandboxFixed {
		t.Fatalf("default %s", got)
	}
	if got := NormalizeSandboxMode("content", true); got != SandboxOmit {
		t.Fatalf("omit header wins: %s", got)
	}
	if got := NormalizeSandboxMode("REASONING", false); got != SandboxReasoning {
		t.Fatalf("case fold: %s", got)
	}
}
