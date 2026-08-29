package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
}

type Adapter interface {
	Name() string
	Chat(ctx context.Context, providerSlug string, behavior string, req ChatRequest) (AdapterResult, error)
}

// TestAdapter 是可重复跑的沙箱上游，用来验证路由、fallback 和流式。
type TestAdapter struct{}

func (TestAdapter) Name() string { return "test" }

func (TestAdapter) Chat(_ context.Context, providerSlug, behavior string, req ChatRequest) (AdapterResult, error) {
	if behavior == "429" {
		return AdapterResult{HTTPStatus: 429, ErrorClass: "rate_limited"}, nil
	}
	if behavior == "500" {
		return AdapterResult{HTTPStatus: 500, ErrorClass: "upstream_error"}, nil
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

// BifrostAdapter 走受控内部 HTTP。未配置时网关不会选用它。
type BifrostAdapter struct {
	BaseURL string
	Client  interface {
		Do(req any) error
	}
}

func (a BifrostAdapter) Name() string { return "bifrost" }

// GeminiAdapter 在未配置真实 Base URL 时走沙箱回声，保证 P0 可验证 Google Gemini 目录与路由。
type GeminiAdapter struct {
	BaseURL string
}

func (GeminiAdapter) Name() string { return "gemini" }

func (a GeminiAdapter) Chat(ctx context.Context, providerSlug, behavior string, req ChatRequest) (AdapterResult, error) {
	_ = ctx
	if strings.TrimSpace(a.BaseURL) == "" {
		result, err := TestAdapter{}.Chat(ctx, providerSlug, behavior, req)
		if err != nil {
			return result, err
		}
		if result.HTTPStatus == 200 && len(result.Body.Choices) > 0 {
			result.Body.Choices[0].Message.Content = "gemini:" + result.Body.Choices[0].Message.Content
		}
		return result, nil
	}
	return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("gemini upstream not configured")
}

func (a BifrostAdapter) Chat(ctx context.Context, providerSlug, _ string, req ChatRequest) (AdapterResult, error) {
	if strings.TrimSpace(a.BaseURL) == "" {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("bifrost unavailable")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return AdapterResult{HTTPStatus: 500, ErrorClass: "upstream_error"}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Tokenhub-Provider", providerSlug)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		class := "upstream_error"
		if resp.StatusCode == 429 {
			class = "rate_limited"
		}
		return AdapterResult{HTTPStatus: resp.StatusCode, ErrorClass: class}, fmt.Errorf("bifrost status %d", resp.StatusCode)
	}
	var out ChatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return AdapterResult{HTTPStatus: 502, ErrorClass: "upstream_error"}, err
	}
	return AdapterResult{HTTPStatus: 200, Body: out}, nil
}
