package gateway

import (
	"context"
	"strings"
	"testing"
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
	})
	params := toBifrostParams(ctx, ChatRequest{}, "lab")
	if params == nil || params.Metadata == nil {
		t.Fatal("missing metadata")
	}
	meta := *params.Metadata
	if meta["api_key_id"] != "key_1" || meta["user_id"] != "usr_1" || meta["channel_org_id"] != "chn_official_a" {
		t.Fatalf("caller metadata: %+v", meta)
	}
}
