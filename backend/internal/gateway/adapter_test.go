package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
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

func TestGeminiAdapterSandboxPrefix(t *testing.T) {
	out, err := GeminiAdapter{}.Chat(context.Background(), "gemini-flash", "ok", ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err != nil || out.HTTPStatus != 200 || len(out.Body.Choices) == 0 {
		t.Fatalf("gemini sandbox: %+v %v", out, err)
	}
	if !strings.Contains(out.Body.Choices[0].Message.Content, "gemini:") {
		t.Fatalf("expected gemini prefix: %+v", out.Body.Choices[0].Message.Content)
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

func TestBifrostParamsCarriesAttemptID(t *testing.T) {
	ctx := ContextWithMeta(context.Background(), map[string]string{"attempt_id": "atm_1"})
	params := toBifrostParams(ctx, ChatRequest{}, "lab")
	if params == nil || params.Metadata == nil || (*params.Metadata)["attempt_id"] != "atm_1" {
		t.Fatalf("attempt metadata: %+v", params)
	}
}

type stubKeys struct {
	byKind map[string][]catalog.PlainKey
}

func (s stubKeys) ListPlainKeys(_ context.Context, _, kind string) ([]catalog.PlainKey, error) {
	return s.byKind[kind], nil
}

func (s stubKeys) HasPlainKeys(_ context.Context, _, kind string) bool {
	return len(s.byKind[kind]) > 0
}

func TestLiveProvidersIncludeCatalogKeys(t *testing.T) {
	a := &envAccount{settings: Settings{
		EncryptionKey: "enc",
		Keys: stubKeys{byKind: map[string][]catalog.PlainKey{
			"gemini": {{ID: "crd_1", Value: "sk-gemini"}},
		}},
	}}
	got, err := a.GetConfiguredProviders()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range got {
		if p == schemas.Gemini {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected gemini from catalog: %+v", got)
	}
}

func TestGetKeysPrefersContextSecret(t *testing.T) {
	a := &envAccount{settings: Settings{OpenAIAPIKey: "sk-env"}}
	ctx := context.WithValue(context.Background(), ctxAccountSecretKey, "sk-pool")
	keys, err := a.GetKeysForProvider(ctx, schemas.OpenAI)
	if err != nil || len(keys) < 2 {
		t.Fatalf("keys %+v %v", keys, err)
	}
	if keys[0].Value.GetValue() != "sk-pool" {
		t.Fatalf("preferred first: %s", keys[0].Value.GetValue())
	}
}

func TestAdapterForKeepsTestInSandbox(t *testing.T) {
	rt, err := Start(context.Background(), Settings{Sandbox: true, LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	s := New(nil, nil, nil, rt, "")
	if s.adapterFor("openai").Name() != "test" {
		t.Fatalf("sandbox openai should stay test, got %s", s.adapterFor("openai").Name())
	}
	if s.adapterFor("gemini").Name() != "gemini" {
		t.Fatalf("sandbox gemini: %s", s.adapterFor("gemini").Name())
	}
	if s.adapterFor("bifrost").Name() != "bifrost" {
		t.Fatalf("sandbox bifrost: %s", s.adapterFor("bifrost").Name())
	}
}
