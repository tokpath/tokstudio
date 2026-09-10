package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestBifrostAdapterCallsSandbox(t *testing.T) {
	rt, err := Start(context.Background(), Settings{Sandbox: true, LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	out, err := BifrostAdapter{Runtime: rt}.Chat(context.Background(), "bifrost-lab", "ok", ChatRequest{
		Model:    "tokenhub/echo-1",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HTTPStatus != 200 || len(out.Body.Choices) == 0 || !strings.Contains(out.Body.Choices[0].Message.Content, "bifrost:hello") {
		t.Fatalf("sandbox reply: %+v", out)
	}
}

func TestBifrostAdapterWithoutClient(t *testing.T) {
	out, err := BifrostAdapter{}.Chat(context.Background(), "x", "ok", ChatRequest{})
	if err == nil || out.HTTPStatus != 503 {
		t.Fatalf("empty runtime should 503, got %+v %v", out, err)
	}
}

func TestStartLiveModeRequiresProviderKey(t *testing.T) {
	_, err := Start(context.Background(), Settings{Sandbox: false, LogLevel: "error"})
	if err == nil {
		t.Fatal("expected live mode without provider keys to fail")
	}
}

func TestBifrostParamsCarriesRequestID(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req_123")
	params := toBifrostParams(ctx, ChatRequest{}, "lab")
	if params == nil || params.Metadata == nil || (*params.Metadata)["request_id"] != "req_123" {
		t.Fatalf("metadata: %+v", params)
	}
}

func TestBifrostParamsCarriesCallerMeta(t *testing.T) {
	ctx := ContextWithMeta(context.Background(), map[string]string{
		"request_id": "req_abc", "api_key_id": "key_1", "user_id": "usr_1",
		"channel_org_id": "chn_official_a", "public_model_id": "tokenhub/echo-1",
		"attempt_id": "atm_1",
	})
	params := toBifrostParams(ctx, ChatRequest{}, "lab")
	if params == nil || params.Metadata == nil {
		t.Fatal("missing metadata")
	}
	meta := *params.Metadata
	if meta["api_key_id"] != "key_1" || meta["user_id"] != "usr_1" || meta["channel_org_id"] != "chn_official_a" {
		t.Fatalf("caller metadata: %+v", meta)
	}
	if meta["attempt_id"] != "atm_1" || meta["request_id"] != "req_abc" || meta["public_model_id"] != "tokenhub/echo-1" {
		t.Fatalf("pass-through ids: %+v", meta)
	}
}

func TestBifrostSandboxEchoesMetadataAndLabelsSandbox(t *testing.T) {
	rt, err := Start(context.Background(), Settings{Sandbox: true, LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	ctx := ContextWithMeta(context.Background(), map[string]string{
		"request_id": "req_echo", "attempt_id": "atm_echo",
		"user_id": "usr_1", "channel_org_id": "chn_official_a", "public_model_id": "tokenhub/echo-1",
	})
	out, err := BifrostAdapter{Runtime: rt}.Chat(ctx, "bifrost-lab", "ok", ChatRequest{
		Model: "echo-upstream", Messages: []ChatMessage{{Role: "user", Content: "meta"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.FactSource != FactSourceSandbox {
		t.Fatalf("sandbox must label echo facts: %+v", out)
	}
	if out.FactSource == FactSourceLive {
		t.Fatal("sandbox must never pretend to be live upstream")
	}
	if out.EchoedMeta["request_id"] != "req_echo" || out.EchoedMeta["attempt_id"] != "atm_echo" {
		t.Fatalf("sandbox must echo pass-through ids: %+v", out.EchoedMeta)
	}
	if len(out.Body.Usage) == 0 {
		t.Fatal("sandbox plugin may return echo usage, but must keep it labeled sandbox")
	}
}

func TestFromBifrostChatMissingUsageStaysEmpty(t *testing.T) {
	out := fromBifrostChat(&schemas.BifrostChatResponse{ID: "chat_empty", Object: "chat.completion"}, false)
	if len(out.Body.Usage) != 0 {
		t.Fatalf("missing usage must stay empty: %+v", out.Body.Usage)
	}
	if out.FactSource == FactSourceLive {
		t.Fatal("empty extra fields must not impersonate live upstream")
	}
}

func TestResolveAttemptUsageDoesNotInventForBifrost(t *testing.T) {
	got := resolveAttemptUsage("bifrost", SandboxFixed, ChatRequest{}, "", nil)
	if len(got) != 0 {
		t.Fatalf("bifrost must not fill 8/4/12: %+v", got)
	}
	kept := resolveAttemptUsage("bifrost", SandboxFixed, ChatRequest{}, "", map[string]int{"prompt_tokens": 3})
	if kept["prompt_tokens"] != 3 {
		t.Fatalf("present bifrost usage must be kept: %+v", kept)
	}
	testFilled := resolveAttemptUsage("test", SandboxFixed, ChatRequest{}, "", nil)
	if testFilled["prompt_tokens"] != 8 {
		t.Fatalf("test adapter CI fill unchanged: %+v", testFilled)
	}
}
