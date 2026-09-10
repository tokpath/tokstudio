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

func TestFromBifrostChatLiveLabelsRealRoutingFacts(t *testing.T) {
	out := fromBifrostChat(&schemas.BifrostChatResponse{
		ID:    "chat_live",
		Model: "gemini-2.0-flash",
		ExtraFields: schemas.BifrostResponseExtraFields{
			RoutingInfo: schemas.RoutingInfo{Provider: schemas.Gemini, Model: "gemini-2.0-flash"},
		},
	}, false)
	if out.FactSource != FactSourceLive {
		t.Fatalf("real Gemini routing must be live: %+v", out)
	}
	if out.EchoedMeta["bifrost_provider"] != string(schemas.Gemini) || out.EchoedMeta["upstream_model"] != "gemini-2.0-flash" {
		t.Fatalf("live must carry provider/model: %+v", out.EchoedMeta)
	}
}

func TestGeminiLiveEnabledKeyGate(t *testing.T) {
	if GeminiLiveEnabled(nil) {
		t.Fatal("nil runtime is not live")
	}
	if GeminiLiveEnabled(&Runtime{Sandbox: true, GeminiAPIKey: "AIza-secret"}) {
		t.Fatal("sandbox must stay echo even when a Key is present")
	}
	if GeminiLiveEnabled(&Runtime{Sandbox: false, GeminiAPIKey: ""}) {
		t.Fatal("empty Key is not live")
	}
	if GeminiLiveEnabled(&Runtime{Sandbox: false, GeminiAPIKey: "   "}) {
		t.Fatal("whitespace Key is not live")
	}
	if !GeminiLiveEnabled(&Runtime{Sandbox: false, GeminiAPIKey: "AIza-secret"}) {
		t.Fatal("sandbox=false and non-empty Key must enable live Gemini")
	}
}

func TestGeminiAdapterSandboxEchoesWithoutFakingLive(t *testing.T) {
	rt, err := Start(context.Background(), Settings{Sandbox: true, GeminiAPIKey: "AIza-present", LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	out, err := GeminiAdapter{Runtime: rt}.Chat(context.Background(), "gemini-flash", "ok", ChatRequest{
		Model:    "google/gemini-flash",
		Messages: []ChatMessage{{Role: "user", Content: "hi-gate"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HTTPStatus != 200 || len(out.Body.Choices) == 0 || !strings.Contains(out.Body.Choices[0].Message.Content, "gemini:echo:hi-gate") {
		t.Fatalf("sandbox gemini must echo: %+v", out)
	}
	if out.FactSource != FactSourceSandbox || out.FactSource == FactSourceLive {
		t.Fatalf("sandbox gemini must label echo, not live: %+v", out)
	}
}

func TestGeminiAdapterLiveWithoutClientDoesNotEcho(t *testing.T) {
	out, err := GeminiAdapter{Runtime: &Runtime{Sandbox: false, GeminiAPIKey: "AIza-secret"}}.Chat(
		context.Background(), "gemini-flash", "ok", ChatRequest{
			Model:    "gemini-2.0-flash",
			Messages: []ChatMessage{{Role: "user", Content: "must-not-echo"}},
		},
	)
	if err == nil || out.HTTPStatus != 503 || out.ErrorClass != "provider_unavailable" {
		t.Fatalf("live+Key without Bifrost must 503, not echo: %+v %v", out, err)
	}
	if out.FactSource == FactSourceLive {
		t.Fatal("unavailable live Gemini must not impersonate upstream")
	}
	if len(out.Body.Choices) > 0 && strings.Contains(out.Body.Choices[0].Message.Content, "echo") {
		t.Fatalf("live miss must not fall back to echo: %+v", out)
	}
}

func TestEnvAccountLiveListsGeminiWhenKeyPresent(t *testing.T) {
	a := &envAccount{settings: Settings{Sandbox: false, GeminiAPIKey: "AIza-test"}}
	got, err := a.GetConfiguredProviders()
	if err != nil || len(got) != 1 || got[0] != schemas.Gemini {
		t.Fatalf("live Gemini key should configure Gemini only: %v %+v", err, got)
	}
	keys, err := a.GetKeysForProvider(context.Background(), schemas.Gemini)
	if err != nil || len(keys) != 1 {
		t.Fatalf("gemini keys: %v %+v", err, keys)
	}
}

func TestAttemptFactSourceGeminiGate(t *testing.T) {
	sandboxRT := &Runtime{Sandbox: true, GeminiAPIKey: "AIza-secret"}
	if attemptFactSource("gemini", AdapterResult{FactSource: FactSourceLive}, sandboxRT) != FactSourceSandbox {
		t.Fatal("sandbox must force sandbox even if the adapter claimed live")
	}
	liveRT := &Runtime{Sandbox: false, GeminiAPIKey: "AIza-secret"}
	if attemptFactSource("gemini", AdapterResult{FactSource: FactSourceLive}, liveRT) != FactSourceLive {
		t.Fatal("live+Key must keep real live facts")
	}
	if attemptFactSource("gemini", AdapterResult{}, &Runtime{Sandbox: false}) != FactSourceSandbox {
		t.Fatal("no Key gemini stays sandbox")
	}
	if attemptFactSource("gemini", AdapterResult{}, liveRT) != "" {
		t.Fatal("live+Key with empty facts must stay empty, not invent live")
	}
}

func TestUsageAdapterNameLiveGeminiDoesNotInvent(t *testing.T) {
	if usageAdapterName("gemini", &Runtime{Sandbox: true, GeminiAPIKey: "k"}) != "gemini" {
		t.Fatal("sandbox gemini keeps test-style usage fill")
	}
	if usageAdapterName("gemini", &Runtime{Sandbox: false, GeminiAPIKey: "k"}) != "bifrost" {
		t.Fatal("live gemini must reuse bifrost honest usage (no 8/4/12 invent)")
	}
	empty := resolveAttemptUsage(usageAdapterName("gemini", &Runtime{Sandbox: false, GeminiAPIKey: "k"}), SandboxFixed, ChatRequest{}, "", nil)
	if len(empty) != 0 {
		t.Fatalf("live gemini missing usage must stay empty: %+v", empty)
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
