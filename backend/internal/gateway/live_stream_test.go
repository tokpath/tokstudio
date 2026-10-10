package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type sinkFunc func(string) error

func (f sinkFunc) Emit(chunk string) error { return f(chunk) }

func TestConsumeOpenAIStreamEmitsBeforeUpstreamFinishes(t *testing.T) {
	first := `{"id":"c1","model":"upstream-hidden","choices":[{"index":0,"delta":{"content":"He"}}]}`
	rest := "data: " + `{"id":"c1","model":"upstream-hidden","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":"stop"}]}` + "\n\n" +
		"data: " + `{"id":"c1","model":"upstream-hidden","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}` + "\n\n" +
		"data: [DONE]\n\n"
	reader, writer := io.Pipe()
	release := make(chan struct{})
	go func() {
		defer writer.Close()
		_, _ = io.WriteString(writer, "data: "+first+"\n\n")
		<-release
		_, _ = io.WriteString(writer, rest)
	}()
	arrived := make(chan string, 1)
	relay := &publicModelSink{model: "brand/public-model", next: sinkFunc(func(chunk string) error {
		select {
		case arrived <- chunk:
		default:
		}
		return nil
	})}
	errCh := make(chan error, 1)
	var result AdapterResult
	go func() {
		var err error
		result, err = consumeOpenAIStream(WithStreamSink(context.Background(), relay), reader)
		errCh <- err
	}()
	select {
	case chunk := <-arrived:
		if !strings.Contains(chunk, `"content":"He"`) {
			t.Fatalf("first chunk = %s", chunk)
		}
		if strings.Contains(chunk, "upstream-hidden") || !strings.Contains(chunk, "brand/public-model") {
			t.Fatalf("public chunk leaked upstream model: %s", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first token was buffered until the upstream finished")
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if result.Body.Choices[0].Message.Content != "Hello" {
		t.Fatalf("content=%q", result.Body.Choices[0].Message.Content)
	}
	if result.Body.Usage["total_tokens"] != 5 || result.Body.Model != "upstream-hidden" {
		t.Fatalf("assembly=%+v", result.Body)
	}
}

func TestOpenRouterChatEmitsFirstTokenBeforeUpstreamFinishes(t *testing.T) {
	release := make(chan struct{})
	arrived := make(chan struct{}, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["stream"] != true {
			t.Errorf("upstream was asked for a buffered completion: %#v", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"id\":\"c\",\"model\":\"up\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\n")
		flusher.Flush()
		<-release
		_, _ = io.WriteString(w, "data: {\"id\":\"c\",\"model\":\"up\",\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, ctxProviderBaseURLKey, upstream.URL+"/v1")
	ctx = WithStreamSink(ctx, sinkFunc(func(string) error {
		select {
		case arrived <- struct{}{}:
		default:
		}
		return nil
	}))
	capTokens := 32
	errCh := make(chan error, 1)
	go func() {
		_, err := (BifrostAdapter{Runtime: &Runtime{settings: Settings{OpenRouterAPIKey: "sk-live", AllowTestLoopback: true}}}).openRouterChat(ctx, ChatRequest{
			Stream: true, Model: "openai/gpt-4.1-mini", MaxTokens: &capTokens,
			Messages: []ChatMessage{{Role: "user", Content: "hi"}},
		})
		errCh <- err
	}()
	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		t.Fatal("openrouter completion was buffered until the upstream finished")
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestConsumeOpenAIStreamKeepsSplitToolCallIdentity(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		`data: {"id":"chat_tool","model":"gpt","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_local","type":"function","function":{"name":"read_file","arguments":"{\"path\":"}}]}}]}`,
		``,
		`data: {"id":"chat_tool","model":"gpt","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n"))
	result, err := consumeOpenAIStream(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	call := result.Body.Choices[0].Message.ToolCalls[0]
	if call.ID != "call_local" || call.Function.Name != "read_file" || call.Function.Arguments != `{"path":"a.txt"}` {
		t.Fatalf("tool identity lost: %+v", call)
	}
}

func TestPublicModelSinkDoesNotStartWhenWriteFails(t *testing.T) {
	relay := &publicModelSink{model: "brand/public", next: sinkFunc(func(string) error {
		return io.ErrClosedPipe
	})}
	if err := relay.Emit(`{"model":"hidden","choices":[]}`); err == nil {
		t.Fatal("expected write failure")
	}
	if relay.started {
		t.Fatal("failed first byte must still allow provider fallback")
	}
}
