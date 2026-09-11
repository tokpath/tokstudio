package gateway

import (
	"context"
	"encoding/json"
	"fmt"
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

// UnavailableAdapter 诚实失败：缺真实上游时返回 503，禁止回声/假成功。
type UnavailableAdapter struct {
	AdapterName string
}

func (a UnavailableAdapter) Name() string {
	if a.AdapterName == "" {
		return "unavailable"
	}
	return a.AdapterName
}

func (a UnavailableAdapter) Chat(context.Context, string, string, ChatRequest) (AdapterResult, error) {
	return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("%s provider unavailable", a.Name())
}

// GeminiAdapter 仅在 live Runtime 可用时走 Bifrost Gemini；否则 503，禁止沙箱回声。
type GeminiAdapter struct {
	Runtime *Runtime
}

func (GeminiAdapter) Name() string { return "gemini" }

func (a GeminiAdapter) Chat(ctx context.Context, providerSlug, behavior string, req ChatRequest) (AdapterResult, error) {
	if !GeminiLiveEnabled(a.Runtime) && !(a.Runtime != nil && !a.Runtime.Sandbox && a.Runtime.Client != nil) {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("gemini live bifrost unavailable")
	}
	if a.Runtime == nil || a.Runtime.Client == nil {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("gemini live bifrost unavailable")
	}
	return BifrostAdapter{Runtime: a.Runtime}.Chat(ctx, firstNonEmpty(providerSlug, "gemini"), behavior, req)
}
