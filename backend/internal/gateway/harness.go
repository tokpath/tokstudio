package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// HarnessAdapter 仅供测试注入：可重复、确定性的上游替身，用来跑路由/账单/计量契约。
// 生产 New() 不会注册它；必须由 InstallTestHarness 显式挂上。
type HarnessAdapter struct {
	NameValue string
}

func (a HarnessAdapter) Name() string {
	if a.NameValue == "" {
		return "test"
	}
	return a.NameValue
}

func (a HarnessAdapter) Chat(ctx context.Context, providerSlug, behavior string, req ChatRequest) (AdapterResult, error) {
	if err := ctx.Err(); err != nil {
		return AdapterResult{HTTPStatus: 408, ErrorClass: "timeout"}, err
	}
	switch behavior {
	case "429":
		return AdapterResult{HTTPStatus: 429, ErrorClass: "rate_limited"}, nil
	case "500":
		return AdapterResult{HTTPStatus: 500, ErrorClass: "upstream_error"}, nil
	case "timeout":
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
	result := AdapterResult{HTTPStatus: 200, Body: resp, EchoedMeta: map[string]string{}}
	meta := passthroughFromContext(ctx)
	for key, value := range meta.Map() {
		result.EchoedMeta[key] = value
	}
	if req.Stream {
		result.Stream = []string{
			`{"id":"` + resp.ID + `","object":"chat.completion.chunk","choices":[{"delta":{"content":"` + text + `"}}]}`,
			`{"id":"` + resp.ID + `","object":"chat.completion.chunk","choices":[{"delta":{"content":" via ` + providerSlug + `"}}]}`,
		}
	}
	return result, nil
}

// InstallTestHarness 把可注入的测试上游挂到 Service 上。
// 只影响当前进程内的 Service 实例；生产启动路径不得调用。
// - test：始终替换为 HarnessAdapter（覆盖 catalog echo 种子）
// - bifrost：仅在尚无 live Client 时替换，避免 CI 无 Key 时整条链路 503
// - gemini：保持 live-only（无 Key 仍 503），满足「不得假装 live Google」契约
func (s *Service) InstallTestHarness() {
	if s == nil {
		return
	}
	if s.adapters == nil {
		s.adapters = map[string]Adapter{}
	}
	s.adapters["test"] = HarnessAdapter{NameValue: "test"}
	if s.runtime == nil || s.runtime.Client == nil {
		s.adapters["bifrost"] = HarnessAdapter{NameValue: "bifrost"}
	}
}
