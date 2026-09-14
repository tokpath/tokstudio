package gateway

import (
	"context"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
)

func TestChatCompletionsURL(t *testing.T) {
	if got := chatCompletionsURL(""); got != "" {
		t.Fatalf("empty base: %q", got)
	}
	for in, want := range map[string]string{
		"https://coding.dashscope.aliyuncs.com/v1":                  "https://coding.dashscope.aliyuncs.com/v1/chat/completions",
		"https://coding.dashscope.aliyuncs.com/v1/":                 "https://coding.dashscope.aliyuncs.com/v1/chat/completions",
		"https://coding.dashscope.aliyuncs.com":                     "https://coding.dashscope.aliyuncs.com/v1/chat/completions",
		"https://coding.dashscope.aliyuncs.com/v1/chat/completions": "https://coding.dashscope.aliyuncs.com/v1/chat/completions",
	} {
		if got := chatCompletionsURL(in); got != want {
			t.Fatalf("chatCompletionsURL(%q)=%q want %q", in, got, want)
		}
	}
}

func TestWithProviderChatURLSetsAbsolutePath(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxProviderBaseURLKey, "https://coding.dashscope.aliyuncs.com/v1")
	ctx = withProviderChatURL(ctx)
	got, _ := ctx.Value(schemas.BifrostContextKeyURLPath).(string)
	want := "https://coding.dashscope.aliyuncs.com/v1/chat/completions"
	if got != want {
		t.Fatalf("url path %q want %q", got, want)
	}
}

func TestBifrostAdapterWithoutClient(t *testing.T) {
	out, err := BifrostAdapter{}.Chat(context.Background(), "x", "ok", ChatRequest{})
	if err == nil || out.HTTPStatus != 503 {
		t.Fatalf("empty runtime should 503, got %+v %v", out, err)
	}
}

func TestStartLiveModeRequiresProviderKey(t *testing.T) {
	_, err := Start(context.Background(), Settings{Sandbox: true, LogLevel: "error"})
	if err == nil {
		t.Fatal("expected live mode without provider keys to fail even if Sandbox=true is passed")
	}
}

func TestStartIgnoresSandboxFlag(t *testing.T) {
	rt, err := Start(context.Background(), Settings{Sandbox: true, GeminiAPIKey: "AIza-test", LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	if rt.Sandbox {
		t.Fatal("Start must force live (Sandbox=false)")
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
		"channel_org_id": "chn_official_a", "public_model_id": "google/gemini-flash",
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
	if meta["attempt_id"] != "atm_1" || meta["request_id"] != "req_abc" || meta["public_model_id"] != "google/gemini-flash" {
		t.Fatalf("pass-through ids: %+v", meta)
	}
}

func TestFromBifrostChatMissingUsageStaysEmpty(t *testing.T) {
	out := fromBifrostChat(&schemas.BifrostChatResponse{ID: "chat_empty", Object: "chat.completion"})
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
	})
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
		t.Fatal("Sandbox=true must not enable live")
	}
	if GeminiLiveEnabled(&Runtime{Sandbox: false, GeminiAPIKey: ""}) {
		t.Fatal("empty Key is not live")
	}
	if !GeminiLiveEnabled(&Runtime{Sandbox: false, GeminiAPIKey: "AIza-secret"}) {
		t.Fatal("non-empty Key must enable live Gemini")
	}
}

func TestGeminiAdapterWithoutLiveDoesNotEcho(t *testing.T) {
	out, err := GeminiAdapter{}.Chat(context.Background(), "gemini-flash", "ok", ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	})
	if err == nil || out.HTTPStatus != 503 {
		t.Fatalf("missing live must 503, not echo: %+v %v", out, err)
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
	if len(out.Body.Choices) > 0 && strings.Contains(out.Body.Choices[0].Message.Content, "echo") {
		t.Fatalf("live miss must not fall back to echo: %+v", out)
	}
}

func TestEnvAccountLiveListsGeminiWhenKeyPresent(t *testing.T) {
	a := &envAccount{settings: Settings{GeminiAPIKey: "AIza-test"}}
	got, err := a.GetConfiguredProviders()
	if err != nil || len(got) != 1 || got[0] != schemas.Gemini {
		t.Fatalf("live Gemini key should configure Gemini only: %v %+v", err, got)
	}
	keys, err := a.GetKeysForProvider(context.Background(), schemas.Gemini)
	if err != nil || len(keys) != 1 {
		t.Fatalf("gemini keys: %v %+v", err, keys)
	}
}

func TestAttemptFactSourceNoSandboxForce(t *testing.T) {
	liveRT := &Runtime{Sandbox: false, GeminiAPIKey: "AIza-secret"}
	if attemptFactSource("gemini", AdapterResult{FactSource: FactSourceLive}, liveRT) != FactSourceLive {
		t.Fatal("live+Key must keep real live facts")
	}
	if attemptFactSource("gemini", AdapterResult{}, &Runtime{Sandbox: false}) != "" {
		t.Fatal("no Key gemini must not invent sandbox facts")
	}
	if attemptFactSource("test", AdapterResult{}, nil) != "" {
		t.Fatal("unavailable test adapter must not invent sandbox facts")
	}
}

func TestUsageAdapterNameLiveGemini(t *testing.T) {
	if usageAdapterName("gemini", &Runtime{Sandbox: false, GeminiAPIKey: "k"}) != "bifrost" {
		t.Fatal("live gemini must reuse bifrost honest usage")
	}
	empty := resolveAttemptUsage(usageAdapterName("gemini", &Runtime{Sandbox: false, GeminiAPIKey: "k"}), "", ChatRequest{}, "", nil)
	if len(empty) != 0 {
		t.Fatalf("live gemini missing usage must stay empty: %+v", empty)
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

func TestAdapterForUnavailableWithoutRuntime(t *testing.T) {
	s := New(nil, nil, nil, nil, "")
	if s.adapterFor("test").Name() != "test" {
		t.Fatalf("test adapter name: %s", s.adapterFor("test").Name())
	}
	out, err := s.adapterFor("test").Chat(context.Background(), "echo-primary", "ok", ChatRequest{})
	if err == nil || out.HTTPStatus != 503 {
		t.Fatalf("test adapter must be unavailable: %+v %v", out, err)
	}
	if s.adapterFor("openai").Name() != "openai" {
		t.Fatalf("missing runtime openai: %s", s.adapterFor("openai").Name())
	}
}
