package modelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

type testSubscriptionCredential struct{ endpoint string }

func (c testSubscriptionCredential) ResolveSubscriptionCredential(context.Context, Attempt, Operation) (SubscriptionCredential, error) {
	return SubscriptionCredential{AccessToken: "mock-subscription-token", AccountID: "mock-account", Endpoint: c.endpoint}, nil
}

func TestSubscriptionRoutesRejectMediaGeneration(t *testing.T) {
	for _, kind := range []string{subscriptionauth.Codex, subscriptionauth.Claude, subscriptionauth.Grok, subscriptionauth.Gemini, subscriptionauth.Antigravity} {
		for _, operation := range []Operation{Image, ImageEdit, Video, VideoRetrieve, VideoDownload} {
			if err := ValidateSubscriptionRequest(kind, 100, Request{Operation: operation}); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("%s accepted %s: %v", kind, operation, err)
			}
		}
	}
	for _, params := range []string{`{"tools":[{"type":"image_generation"}]}`, `{"tool_choice":{"type":"video_generation"}}`} {
		request := Request{Operation: Responses, Params: json.RawMessage(params)}
		if err := ValidateSubscriptionRequest(subscriptionauth.Codex, 100, request); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("Responses accepted a hosted media tool: %v", err)
		}
	}
	request := Request{Operation: Responses, InputTypes: []string{"text", "image"},
		Params: json.RawMessage(`{"tools":[{"type":"function","name":"image_generation"}],"tool_choice":"auto"}`)}
	if err := ValidateSubscriptionRequest(subscriptionauth.Codex, 100, request); err != nil {
		t.Fatal("Responses rejected image input or a client-side function", err)
	}
}

func TestSubscriptionBifrostNativeContracts(t *testing.T) {
	for _, sample := range []struct {
		kind, provider, path, raw string
		op                        Operation
		configured                bool
		modelLimit                int64
		wantCap                   string
	}{
		{subscriptionauth.Codex, "openai", "/backend-api/codex/responses", `{"model":"public-model","input":"hello"}`, Responses, false, 20, ""},
		{subscriptionauth.Codex, "openai", "/backend-api/codex/responses", `{"model":"public-model","input":"hello","stream":true}`, Responses, false, 20, ""},
		{subscriptionauth.Claude, "anthropic", "/v1/messages", `{"model":"public-model","messages":[{"role":"user","content":"hello"}]}`, Chat, false, 32_000, "32000"},
		{subscriptionauth.Grok, "openai", "/v1/responses", `{"model":"public-model","input":"hello","max_output_tokens":20}`, Responses, true, 20, "20"},
		{subscriptionauth.Gemini, "openai", "/v1/chat/completions", `{"model":"public-model","messages":[{"role":"user","content":"hello"}]}`, Chat, true, 128_000, "50000"},
		{subscriptionauth.Antigravity, "openai", "/v1/chat/completions", `{"model":"public-model","messages":[{"role":"user","content":"hello"}],"max_tokens":8000}`, Chat, true, 128_000, "8000"},
	} {
		t.Run(sample.kind+"/"+sample.raw, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != sample.path || r.Header.Get("Authorization") != "Bearer mock-subscription-token" || r.Header.Get("X-Api-Key") != "" {
					t.Errorf("wrong upstream path or authorization mechanism")
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if string(body["model"]) != `"upstream-exact-model"` {
					t.Error("vendor model was changed")
				}
				if sample.configured {
					if r.Header.Get("ChatGPT-Account-Id") != "" || r.Header.Get("Anthropic-Beta") != "" {
						t.Error("another supplier's account headers were forwarded")
					}
					w.Header().Set("Content-Type", "application/json")
					if sample.op == Responses {
						if string(body["max_output_tokens"]) != sample.wantCap {
							t.Error("Grok requested output cap was changed")
						}
						fmt.Fprint(w, `{"id":"resp_test","object":"response","model":"upstream-exact-model","status":"completed","output":[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
					} else {
						if string(body["max_completion_tokens"]) != sample.wantCap {
							t.Errorf("chat output cap = %s, want %s", body["max_completion_tokens"], sample.wantCap)
						}
						fmt.Fprint(w, `{"id":"msg_test","object":"chat.completion","model":"upstream-exact-model","choices":[{"message":{"role":"assistant","content":"ok"},"index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
					}
					return
				}
				if sample.kind == subscriptionauth.Claude {
					if !strings.Contains(r.Header.Get("Anthropic-Beta"), "oauth-2025-04-20") || string(body["max_tokens"]) != sample.wantCap {
						t.Error("missing Claude authorization or token cap")
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"upstream-exact-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
					return
				}
				if r.Header.Get("ChatGPT-Account-Id") != "mock-account" || string(body["store"]) != "false" || string(body["stream"]) != "true" {
					t.Error("missing Codex account or stateless streaming options")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: response.completed\ndata: "+`{"type":"response.completed","response":{"id":"resp_test","object":"response","model":"upstream-exact-model","status":"completed","output":[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`+"\n\n")
			}))
			t.Cleanup(server.Close)
			endpoint := server.URL + sample.path
			credentials := testSubscriptionCredential{}
			if sample.configured {
				credentials.endpoint = endpoint
			}
			ctx := context.Background()
			b, err := NewBifrost(ctx, BifrostConfig{
				Providers: []string{sample.provider}, Credentials: testCredentials{}, Subscriptions: credentials,
				AllowTestLoopback: true, subscriptionTestEndpoints: map[string]string{sample.kind: endpoint},
				EndpointGuard: func(_ context.Context, url string) error {
					if url != endpoint {
						return ErrInvalidRequest
					}
					return nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(b.Close)
			r, err := DecodeProtocol("request", sample.op, []byte(sample.raw))
			if err != nil {
				t.Fatal(err)
			}
			capability := DefaultBilling(sample.op).Capability(sample.op, r.Stream)
			a := testAttempt("", capability)
			a.Provider, a.SubscriptionKind, a.SubscriptionAccountID, a.SubscriptionOutputLimit = sample.provider, sample.kind, "account", sample.modelLimit
			events := 0
			out, err := b.Execute(ctx, a, r, func(_ context.Context, event Event) error {
				events++
				if strings.Contains(string(event.Data), "mock-subscription-token") {
					t.Error("upstream token was emitted")
				}
				return nil
			})
			if err != nil || out.State != "completed" || !out.Usage.Reliable || out.Cost != nil {
				t.Fatalf("native subscription result state=%s usage=%+v err=%v", out.State, out.Usage, err)
			}
			if r.Stream && events == 0 || !r.Stream && !strings.Contains(string(out.Output), "public-model") {
				t.Fatal("expected public model output")
			}
			if sample.kind == subscriptionauth.Codex {
				r.Params = json.RawMessage(`{"max_output_tokens":10}`)
				if _, err = b.Execute(ctx, a, r, nil); !errors.Is(err, ErrUnsupported) {
					t.Fatalf("silently changed a Codex output limit: %v", err)
				}
			}
		})
	}
}
