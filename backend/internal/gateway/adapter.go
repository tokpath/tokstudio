package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type ChatRequest struct {
	Model           string          `json:"model"`
	Messages        []ChatMessage   `json:"messages"`
	Stream          bool            `json:"stream"`
	Temperature     *float64        `json:"temperature"`
	MaxTokens       *int            `json:"max_tokens"`
	LogitBias       json.RawMessage `json:"logit_bias"`
	Tools           json.RawMessage `json:"tools"`
	ToolChoice      json.RawMessage `json:"tool_choice"`
	ResponseFormat  json.RawMessage `json:"response_format"`
	Reasoning       json.RawMessage `json:"reasoning"`
	ReasoningEffort string          `json:"reasoning_effort"`
	Extra           map[string]any  `json:"-"`
}

type ChatMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	Name       string        `json:"name,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	Parts      []ContentPart `json:"-"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Usage     map[string]int `json:"usage"`
	RequestID string         `json:"request_id"`
	Provider  string         `json:"provider,omitempty"`
}

type AdapterResult struct {
	HTTPStatus int
	ErrorClass string
	Body       ChatResponse
	Stream     []string
	FactSource string
	EchoedMeta map[string]string
}

type Adapter interface {
	Name() string
	Chat(ctx context.Context, providerSlug string, behavior string, req ChatRequest) (AdapterResult, error)
}

// TestAdapter 是可重复跑的沙箱上游，用来验证路由、fallback 和流式。
type TestAdapter struct{}

func (TestAdapter) Name() string { return "test" }

func (TestAdapter) Chat(ctx context.Context, providerSlug, behavior string, req ChatRequest) (AdapterResult, error) {
	if err := ctx.Err(); err != nil {
		return AdapterResult{HTTPStatus: 408, ErrorClass: "timeout"}, err
	}
	if behavior == "429" {
		return AdapterResult{HTTPStatus: 429, ErrorClass: "rate_limited"}, nil
	}
	if behavior == "500" {
		return AdapterResult{HTTPStatus: 500, ErrorClass: "upstream_error"}, nil
	}
	if behavior == "timeout" {
		return AdapterResult{HTTPStatus: 408, ErrorClass: "timeout"}, context.DeadlineExceeded
	}
	last := ChatMessage{}
	if len(req.Messages) > 0 {
		last = req.Messages[len(req.Messages)-1]
	}
	text := "echo:" + last.Content
	usage := map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12}
	msg := ChatMessage{Role: "assistant", Content: text + " via " + providerSlug}
	if n := visionCount(req); n > 0 {
		msg.Content = fmt.Sprintf("vision:%s images=%d via %s", last.Content, n, providerSlug)
	}
	if jsonMode(req.ResponseFormat) {
		payload, _ := json.Marshal(map[string]any{"ok": true, "echo": last.Content})
		msg.Content = string(payload)
	}
	if presentRaw(req.Tools) && last.Role != "tool" && last.Role != "assistant" {
		msg.Content = ""
		msg.ToolCalls = []ToolCall{{
			ID: "call_echo", Type: "function",
			Function: ToolFunction{Name: firstToolName(req.Tools), Arguments: `{"echo":true}`},
		}}
	}
	if last.Role == "tool" {
		msg.Content = "tool-result:" + last.Content + " via " + providerSlug
	}
	if req.ReasoningEffort != "" || presentRaw(req.Reasoning) {
		usage["reasoning_tokens"] = 3
		usage["total_tokens"] = 15
	}
	resp := ChatResponse{
		ID: fmt.Sprintf("chat_%d", time.Now().UnixNano()), Object: "chat.completion",
		Model: req.Model, Usage: usage,
	}
	resp.Choices = append(resp.Choices, struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
	}{Index: 0, Message: msg})
	result := AdapterResult{HTTPStatus: 200, Body: resp}
	if req.Stream {
		result.Stream = []string{
			`{"id":"` + resp.ID + `","object":"chat.completion.chunk","choices":[{"delta":{"content":"` + text + `"}}]}`,
			`{"id":"` + resp.ID + `","object":"chat.completion.chunk","choices":[{"delta":{"content":" via ` + providerSlug + `"}}]}`,
		}
	}
	return result, nil
}

// GeminiAdapter 沙箱回声；live 且 Runtime 可用时改走 Bifrost Gemini。
// W1-Gemini：sandbox 或空 Key 且无 live Runtime 只回声；live+Key 无 Client 返回 503。
// 回声不得把 fact_source 标成 live，也不得伪造 Google 上游身份。
type GeminiAdapter struct {
	Runtime *Runtime
}

func (GeminiAdapter) Name() string { return "gemini" }

func (a GeminiAdapter) Chat(ctx context.Context, providerSlug, behavior string, req ChatRequest) (AdapterResult, error) {
	if GeminiLiveEnabled(a.Runtime) || (a.Runtime != nil && !a.Runtime.Sandbox && a.Runtime.Client != nil) {
		if a.Runtime == nil || a.Runtime.Client == nil {
			return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("gemini live bifrost unavailable")
		}
		return BifrostAdapter{Runtime: a.Runtime}.Chat(ctx, firstNonEmpty(providerSlug, "gemini"), behavior, req)
	}
	result, err := TestAdapter{}.Chat(ctx, providerSlug, behavior, req)
	if err != nil {
		result.FactSource = FactSourceSandbox
		return result, err
	}
	if result.HTTPStatus == 200 && len(result.Body.Choices) > 0 {
		result.Body.Choices[0].Message.Content = "gemini:" + result.Body.Choices[0].Message.Content
	}
	result.FactSource = FactSourceSandbox
	if result.EchoedMeta == nil {
		result.EchoedMeta = map[string]string{}
	}
	result.EchoedMeta["fact_source"] = FactSourceSandbox
	return result, nil
}
