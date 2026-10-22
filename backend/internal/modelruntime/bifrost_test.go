package modelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/maximhq/bifrost/core/schemas"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testCredentials struct{}

func (testCredentials) ResolveCredential(context.Context, string, string) (Credential, error) {
	return Credential{ID: "credential", Secret: "platform-secret-do-not-log"}, nil
}
func testCapability(op Operation, stream bool) Capability {
	return Capability{Operation: op, Streaming: stream, BillableBounds: map[string]string{"input": "10000", "output": "20"}, UsageRules: []UsageRule{{Component: "input", Unit: "token", Path: "/prompt_tokens"}, {Component: "output", Unit: "token", Path: "/completion_tokens"}}}
}
func testAttempt(endpoint string, c Capability) Attempt {
	return Attempt{ID: "attempt", RequestID: "request", UpstreamID: "upstream", Provider: "openai", UpstreamModel: "upstream-exact-model", PublicModel: "public-model", CredentialID: "credential", BaseURL: endpoint, Deadline: time.Now().Add(10 * time.Second), Capability: c}
}
func newTestRuntime(t *testing.T) *Bifrost {
	t.Helper()
	b, e := NewBifrost(context.Background(), BifrostConfig{AllowTestLoopback: true, Providers: []string{"openai"}, Credentials: testCredentials{}, ResourceGuard: func(_ context.Context, u string) error {
		if !strings.HasPrefix(u, "https://example.com/") {
			return ErrInvalidRequest
		}
		return nil
	}, EndpointGuard: func(_ context.Context, u string) error {
		if !strings.HasPrefix(u, "http://127.0.0.1:") {
			return ErrInvalidRequest
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(b.Close)
	return b
}

func TestBifrostChatPreservesModelToolsMultimodalAndUsage(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]json.RawMessage
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
		}
		if r.Header.Get("Authorization") != "Bearer platform-secret-do-not-log" {
			t.Error("wrong selected credential")
		}
		var model string
		_ = json.Unmarshal(body["model"], &model)
		if model != "upstream-exact-model" {
			t.Errorf("model switched to %s", model)
		}
		for _, field := range []string{"tools", "tool_choice", "response_format", "max_completion_tokens", "temperature"} {
			if _, ok := body[field]; !ok {
				t.Errorf("dropped %s", field)
			}
		}
		if !strings.Contains(string(body["messages"]), "image_url") {
			t.Error("image content dropped")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chat","object":"chat.completion","model":"upstream-exact-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
	}))
	defer server.Close()
	r, e := DecodeProtocol("request", Chat, []byte(`{"model":"public-model","messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"https://example.com/image.png"}}]}],"max_tokens":20,"temperature":0.25,"tools":[{"type":"function","function":{"name":"hello","parameters":{"type":"object","properties":{}}}}],"tool_choice":"auto","response_format":{"type":"json_object"}}`))
	if e != nil {
		t.Fatal(e)
	}
	b := newTestRuntime(t)
	out, e := b.Execute(context.Background(), testAttempt(server.URL, testCapability(Chat, false)), r, nil)
	if e != nil {
		t.Fatal(e)
	}
	if out.State != "completed" || !out.Usage.Reliable || len(out.Usage.Measurements) != 2 {
		t.Fatalf("result %#v", out)
	}
	if calls.Load() != 1 {
		t.Fatalf("unexpected %d upstream calls", calls.Load())
	}
	if strings.Contains(string(out.Output), "extra_fields") {
		t.Fatal("internal routing leaked")
	}
}
func TestBifrostRejectsSilentSchemaLossAndModelDefaults(t *testing.T) {
	b := newTestRuntime(t)
	r, _ := DecodeProtocol("request", Chat, []byte(`{"model":"public-model","messages":[{"role":"user","content":"hi"}],"max_tokens":20,"unknown":"cannot-drop"}`))
	if _, e := b.Execute(context.Background(), testAttempt("", testCapability(Chat, false)), r, nil); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("silent field loss: %v", e)
	}
	r.PublicModel = ""
	if _, e := b.Execute(context.Background(), testAttempt("", testCapability(Chat, false)), r, nil); !errors.Is(e, ErrInvalidRequest) {
		t.Fatalf("default model accepted: %v", e)
	}
}
func TestBifrostNoImplicitRetryAndErrorContentPrivate(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"platform-secret-do-not-log private prompt","type":"server_error"}}`)
	}))
	defer server.Close()
	r, _ := DecodeProtocol("request", Chat, []byte(`{"model":"public-model","messages":[{"role":"user","content":"private prompt"}],"max_tokens":20}`))
	out, e := newTestRuntime(t).Execute(context.Background(), testAttempt(server.URL, testCapability(Chat, false)), r, nil)
	if e == nil || out.State != "outcome_unknown" {
		t.Fatalf("got %#v %v", out, e)
	}
	if calls.Load() != 1 {
		t.Fatalf("SDK retried %d times", calls.Load())
	}
	if strings.Contains(e.Error(), "private") || strings.Contains(e.Error(), "secret") {
		t.Fatal("upstream content leaked into error")
	}
}
func TestBifrostStreamingUsageAndDisconnect(t *testing.T) {
	for _, drop := range []bool{false, true} {
		t.Run(fmt.Sprint(drop), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"s\",\"object\":\"chat.completion.chunk\",\"model\":\"upstream-exact-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
				w.(http.Flusher).Flush()
				if drop {
					return
				}
				fmt.Fprint(w, "data: {\"id\":\"s\",\"object\":\"chat.completion.chunk\",\"model\":\"upstream-exact-model\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			r, _ := DecodeProtocol("request", Chat, []byte(`{"model":"public-model","stream":true,"messages":[{"role":"user","content":"hi"}],"max_tokens":20}`))
			emitted := 0
			out, e := newTestRuntime(t).Execute(context.Background(), testAttempt(server.URL, testCapability(Chat, true)), r, func(context.Context, Event) error { emitted++; return nil })
			if emitted == 0 || !out.OutputDelivered {
				t.Fatal("stream content not delivered")
			}
			if drop {
				if e == nil {
					t.Fatal("truncated stream accepted as complete")
				}
			} else if e != nil || !out.Usage.Reliable {
				t.Fatalf("missing stream usage %#v %v", out, e)
			}
			if calls.Load() != 1 {
				t.Fatal("stream generation retried")
			}
		})
	}
}
func TestCapabilityBoundsRejectUnmeteredUsage(t *testing.T) {
	c := testCapability(Chat, false)
	r, _ := DecodeProtocol("request", Chat, []byte(`{"model":"public-model","messages":[{"role":"user","content":"hi"}],"max_tokens":20,"n":2}`))
	if e := ValidateRequest(testAttempt("", c), r); e != nil {
		t.Fatal(e)
	}
	u := Usage{Reliable: true, Measurements: []Measurement{{Component: "input", Unit: "token", Quantity: "2"}, {Component: "output", Unit: "token", Quantity: "21"}}}
	if !errors.Is(CheckUsageBounds(c, u), ErrBoundExceeded) {
		t.Fatal("upper bound error ignored")
	}
	c.UsageRules = nil
	if c.Verified() {
		t.Fatal("capability without metering was treated as ready")
	}
}
func TestCachedAndReasoningComponentsDoNotDoubleCharge(t *testing.T) {
	c := testCapability(Chat, false)
	c.UsageRules = []UsageRule{{Component: "uncached_input", Unit: "token", Path: "/prompt_tokens"}, {Component: "cached_input", Unit: "token", Path: "/prompt_tokens_details/cached_read_tokens"}, {Component: "ordinary_output", Unit: "token", Path: "/completion_tokens"}, {Component: "reasoning", Unit: "token", Path: "/completion_tokens_details/reasoning_tokens"}}
	c.SubtractPaths = map[string][]string{"uncached_input": {"/prompt_tokens_details/cached_read_tokens"}, "ordinary_output": {"/completion_tokens_details/reasoning_tokens"}}
	r := &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{PromptTokens: 12, CompletionTokens: 7, TotalTokens: 19, PromptTokensDetails: &schemas.ChatPromptTokensDetails{CachedReadTokens: 4}, CompletionTokensDetails: &schemas.ChatCompletionTokensDetails{ReasoningTokens: 3}}}
	u := extractUsage(r, c)
	if !u.Reliable {
		t.Fatal("reliable complete usage rejected")
	}
	want := []string{"8", "4", "4", "3"}
	for i, m := range u.Measurements {
		if m.Quantity != want[i] {
			t.Fatalf("component %s=%s want %s", m.Component, m.Quantity, want[i])
		}
	}
}
