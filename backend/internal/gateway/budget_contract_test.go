package gateway

import (
	"context"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAITextBudgetActualOutbound(t *testing.T) {
	captured := make(chan map[string]any, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer sk-local-budget-test" {
			t.Errorf("unexpected outbound %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		captured <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat_local","object":"chat.completion","model":"gpt-4.1-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":9,"total_tokens":16,"completion_tokens_details":{"reasoning_tokens":5}}}`))
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	runtime, err := Start(ctx, Settings{OpenAIAPIKey: "sk-local-budget-test", LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	cap := 32
	callCtx := context.WithValue(ctx, ctxProviderBaseURLKey, upstream.URL+"/v1")
	out, err := (BifrostAdapter{Runtime: runtime}).Chat(callCtx, "openai", "", ChatRequest{Model: "gpt-4.1-mini", MaxTokens: &cap, Messages: []ChatMessage{{Role: "user", Content: "ping"}}})
	if err != nil || out.HTTPStatus != 200 {
		t.Fatalf("adapter %v %+v", err, out)
	}
	body := <-captured
	if body["max_completion_tokens"] != float64(cap) || body["max_tokens"] != nil {
		t.Fatalf("actual output parameter absent/wrong: %+v", body)
	}
	quote, err := billing.ParseQuote("local", []byte(`{"input":"0.000001","output":"0.000002","reasoning":"0.000003"}`))
	if err != nil {
		t.Fatal(err)
	}
	reserve := billing.EstimateBoundedTextReserveMinor(quote, 512, cap)
	actual := quote.Charge(out.Body.Usage, "")
	if actual != 30 || out.Body.Usage["completion_includes_reasoning"] != 1 {
		t.Fatalf("completion charged reasoning twice: amount=%d usage=%+v", actual, out.Body.Usage)
	}
	if actual <= 0 || actual > reserve {
		t.Fatalf("measured=%d bound=%d usage=%+v", actual, reserve, out.Body.Usage)
	}
	if !catalog.TextBudgetCandidate(catalog.RouteCandidate{Adapter: "bifrost", ProviderSlug: "openai", BaseURL: "https://api.openai.com/v1"}) {
		t.Fatal("supported adapter estimate unavailable")
	}
	for _, candidate := range []catalog.RouteCandidate{{Adapter: "bifrost", ProviderSlug: "openai", BaseURL: "https://compatible.example/v1"}, {Adapter: "bifrost", ProviderSlug: "anthropic", BaseURL: ""}, {Adapter: "gemini", ProviderSlug: "google"}} {
		if !catalog.TextBudgetCandidate(candidate) {
			t.Fatalf("estimable supported path blocked %+v", candidate)
		}
	}
}

func TestOtherVerifiedTextBudgetActualOutbound(t *testing.T) {
	for _, fixture := range []struct {
		slug, model, path, capField, response string
		settings                              Settings
	}{
		{"anthropic", "claude-sonnet-4-6", "/v1/messages", "max_tokens", `{"id":"msg_local","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":7,"output_tokens":9,"output_tokens_details":{"thinking_tokens":5}}}`, Settings{AnthropicAPIKey: "sk-local-budget-test"}},
		{"openrouter", "openai/gpt-4.1-mini", "/v1/chat/completions", "max_completion_tokens", `{"id":"chat_local","object":"chat.completion","model":"openai/gpt-4.1-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":9,"total_tokens":16,"completion_tokens_details":{"reasoning_tokens":5}}}`, Settings{OpenRouterAPIKey: "sk-local-budget-test", AllowTestLoopback: true}},
	} {
		t.Run(fixture.slug, func(t *testing.T) {
			captured := make(chan map[string]any, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != fixture.path {
					t.Errorf("outbound endpoint %s", r.URL.Path)
				}
				if fixture.slug == "anthropic" {
					if r.Header.Get("x-api-key") != "sk-local-budget-test" {
						t.Error("missing native Anthropic authentication")
					}
				} else if r.Header.Get("Authorization") != "Bearer sk-local-budget-test" {
					t.Error("missing OpenRouter authentication")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				captured <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(fixture.response))
			}))
			defer upstream.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			fixture.settings.LogLevel = "error"
			runtime, err := Start(ctx, fixture.settings)
			if err != nil {
				t.Fatal(err)
			}
			defer runtime.Close()
			cap := 32
			out, err := (BifrostAdapter{Runtime: runtime}).Chat(context.WithValue(ctx, ctxProviderBaseURLKey, upstream.URL+"/v1"), fixture.slug, "", ChatRequest{Model: fixture.model, MaxTokens: &cap, Messages: []ChatMessage{{Role: "user", Content: "ping"}}})
			if err != nil || out.HTTPStatus != 200 {
				t.Fatalf("adapter %+v %v", out, err)
			}
			body := <-captured
			if body[fixture.capField] != float64(cap) {
				t.Fatalf("actual output parameter missing %+v", body)
			}
			if out.Body.Usage["prompt_tokens"] != 7 || out.Body.Usage["completion_tokens"] != 9 {
				t.Fatalf("usage including thinking lost %+v", out.Body.Usage)
			}
			q, _ := billing.ParseQuote("local", []byte(`{"input":"0.000001","output":"0.000002","reasoning":"0.000003"}`))
			if q.Charge(out.Body.Usage, "") > billing.EstimateBoundedTextReserveMinor(q, 512, cap) {
				t.Fatal("fixture actual charge unexpectedly exceeds estimate")
			}
			if !catalog.TextBudgetCandidate(catalog.RouteCandidate{Adapter: "bifrost", ProviderSlug: fixture.slug, UpstreamModelID: fixture.model}) {
				t.Fatal("supported adapter missing")
			}
		})
	}
	for _, c := range []catalog.RouteCandidate{{Adapter: "bifrost", ProviderSlug: "openrouter", UpstreamModelID: "google/gemini-3-flash-preview"}, {Adapter: "bifrost", ProviderSlug: "openrouter", UpstreamModelID: "openrouter/auto"}, {Adapter: "bifrost", ProviderSlug: "anthropic", BaseURL: "https://custom.example/v1"}} {
		if !catalog.TextBudgetCandidate(c) {
			t.Fatalf("estimable compatible contract %+v", c)
		}
	}
}

func TestOpenAIChatToolRoundTripActualOutbound(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		calls++
		messages := body["messages"].([]any)
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			if body["tools"] == nil {
				t.Error("tool definitions dropped")
			}
			_, _ = w.Write([]byte(`{"id":"chat_tool","object":"chat.completion","model":"gpt-4.1-mini","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call_local","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":7,"completion_tokens":9,"total_tokens":16}}`))
		} else {
			assistant := messages[len(messages)-2].(map[string]any)
			tool := messages[len(messages)-1].(map[string]any)
			if assistant["tool_calls"] == nil || tool["tool_call_id"] != "call_local" || tool["content"] != "file contents" {
				t.Errorf("tool roundtrip lost %+v", messages)
			}
			_, _ = w.Write([]byte(`{"id":"chat_done","object":"chat.completion","model":"gpt-4.1-mini","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":17,"completion_tokens":4,"total_tokens":21}}`))
		}
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	runtime, err := Start(ctx, Settings{OpenAIAPIKey: "sk-local-tool", LogLevel: "error"})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	adapter := BifrostAdapter{Runtime: runtime}
	ctx = context.WithValue(ctx, ctxProviderBaseURLKey, upstream.URL+"/v1")
	cap := 32
	request := ChatRequest{Stream: true, Model: "gpt-4.1-mini", MaxTokens: &cap, Messages: []ChatMessage{{Role: "user", Content: "read a.txt"}}, Tools: json.RawMessage(`[{"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}]`)}
	first, err := adapter.Chat(ctx, "openai", "", request)
	if err != nil || len(first.Body.Choices) != 1 || len(first.Body.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("tool output lost %+v %v", first, err)
	}
	if len(first.Stream) != 3 {
		t.Fatalf("missing tool SSE chunks %+v", first.Stream)
	}
	var chunk map[string]any
	if err := json.Unmarshal([]byte(first.Stream[0]), &chunk); err != nil {
		t.Fatal(err)
	}
	delta := chunk["choices"].([]any)[0].(map[string]any)["delta"].(map[string]any)
	call := delta["tool_calls"].([]any)[0].(map[string]any)
	if call["index"] != float64(0) || call["id"] != "call_local" || call["function"].(map[string]any)["arguments"] != `{"path":"a.txt"}` {
		t.Fatalf("tool SSE identity lost %+v", chunk)
	}
	if err := json.Unmarshal([]byte(first.Stream[1]), &chunk); err != nil {
		t.Fatal(err)
	}
	if chunk["choices"].([]any)[0].(map[string]any)["finish_reason"] != "tool_calls" {
		t.Fatalf("tool SSE finish lost %+v", chunk)
	}
	request.Messages = append(request.Messages, first.Body.Choices[0].Message, ChatMessage{Role: "tool", ToolCallID: "call_local", Content: "file contents"})
	second, err := adapter.Chat(ctx, "openai", "", request)
	if err != nil || second.Body.Choices[0].Message.Content != "done" || calls != 2 {
		t.Fatalf("tool result %+v %v", second, err)
	}
}

func TestRealAdapterUnknownAndMissingUsage(t *testing.T) {
	tiny := 1
	if _, err := (BifrostAdapter{}).openRouterChat(context.Background(), ChatRequest{MaxTokens: &tiny}); err == nil {
		t.Fatal("small ceiling raised to provider minimum instead of rejecting")
	}
	for _, code := range []int{0, 500, 502, 503} {
		var status *int
		if code != 0 {
			value := code
			status = &value
		}
		out := mapBifrostError(&schemas.BifrostError{StatusCode: status})
		if out.ErrorClass != "outcome_unknown" {
			t.Fatalf("SDK error silently treated as known failure: %+v", out)
		}
	}
	for _, fixture := range []struct {
		status int
		body   string
	}{{502, `{"error":{"message":"upstream failed after acceptance"}}`}, {200, `{"id":"chat_no_usage","choices":[{"index":0,"message":{"role":"assistant","content":"done"}}]}`}} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(fixture.status)
			_, _ = w.Write([]byte(fixture.body))
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		runtime := &Runtime{settings: Settings{OpenRouterAPIKey: "sk-local-unknown", AllowTestLoopback: true}}
		ctx = context.WithValue(ctx, ctxProviderBaseURLKey, upstream.URL+"/v1")
		cap := 32
		out, err := (BifrostAdapter{Runtime: runtime}).openRouterChat(ctx, ChatRequest{Model: "openai/gpt-4.1-mini", MaxTokens: &cap, Messages: []ChatMessage{{Role: "user", Content: "ping"}}})
		cancel()
		upstream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if fixture.status >= 500 && out.ErrorClass != "outcome_unknown" {
			t.Fatalf("5xx discarded accepted request fact: %+v", out)
		}
		if fixture.status == 200 && out.Body.Usage != nil {
			t.Fatalf("invented missing usage: %+v", out.Body.Usage)
		}
	}
}
