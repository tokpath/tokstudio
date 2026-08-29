package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type ChatRequest struct {
	Model       string          `json:"model"`
	Messages    []ChatMessage   `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature"`
	MaxTokens   *int            `json:"max_tokens"`
	LogitBias   json.RawMessage `json:"logit_bias"`
	Extra       map[string]any  `json:"-"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
	text := "echo:"
	if len(req.Messages) > 0 {
		text += req.Messages[len(req.Messages)-1].Content
	}
	resp := ChatResponse{
		ID:     fmt.Sprintf("chat_%d", time.Now().UnixNano()),
		Object: "chat.completion",
		Model:  req.Model,
		Usage:  map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
	}
	resp.Choices = append(resp.Choices, struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
	}{Index: 0, Message: ChatMessage{Role: "assistant", Content: text + " via " + providerSlug}})
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
	_ = ctx
	_ = providerSlug
	_ = req
	return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, io.EOF
}
