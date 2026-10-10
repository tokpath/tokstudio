package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

// 本文件的流式收集与 bifrost.go 的非流式转换共用同一套用量字段。

// StreamSink 在上游每个 chunk 到达时立刻接收一份 OpenAI chat chunk JSON。
// 调用方负责加上 data: 前缀并 Flush，不能攒到整段结束再写。
type StreamSink interface {
	Emit(chunkJSON string) error
}

type streamSinkKey struct{}

func WithStreamSink(ctx context.Context, sink StreamSink) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, streamSinkKey{}, sink)
}

func StreamSinkFrom(ctx context.Context) StreamSink {
	if ctx == nil {
		return nil
	}
	sink, _ := ctx.Value(streamSinkKey{}).(StreamSink)
	return sink
}

// publicModelSink 把 chunk 里的上游模型名换成客户请求的公开模型，再交给真正的写出端。
// started 只在写出端接受了这个 chunk 之后才为 true，这样第一个字节写出前仍允许换提供商。
type publicModelSink struct {
	model   string
	next    StreamSink
	chunks  []string
	started bool
}

func (s *publicModelSink) Emit(raw string) error {
	if s == nil {
		return nil
	}
	chunk := rewriteChunkModel(raw, s.model)
	if s.next != nil {
		if err := s.next.Emit(chunk); err != nil {
			return err
		}
	}
	s.chunks = append(s.chunks, chunk)
	s.started = true
	return nil
}

func emitChunk(ctx context.Context, payload string) error {
	sink := StreamSinkFrom(ctx)
	if sink == nil || payload == "" {
		return nil
	}
	return sink.Emit(payload)
}

type streamAssembly struct {
	id      string
	model   string
	content strings.Builder
	calls   []ToolCall
	finish  string
	usage   map[string]int
	raw     []string
}

func (a *streamAssembly) addChunk(body map[string]any, raw string) {
	if raw != "" {
		a.raw = append(a.raw, raw)
	}
	if id, _ := body["id"].(string); id != "" {
		a.id = id
	}
	if model, _ := body["model"].(string); model != "" {
		a.model = model
	}
	if usage := usageFromJSON(body["usage"]); usage != nil {
		a.usage = usage
	}
	choices, _ := body["choices"].([]any)
	for _, item := range choices {
		choice, _ := item.(map[string]any)
		if choice == nil {
			continue
		}
		if finish, _ := choice["finish_reason"].(string); finish != "" {
			a.finish = finish
		}
		delta, _ := choice["delta"].(map[string]any)
		if delta == nil {
			continue
		}
		if text, _ := delta["content"].(string); text != "" {
			a.content.WriteString(text)
		}
		calls, _ := delta["tool_calls"].([]any)
		for _, callItem := range calls {
			call, _ := callItem.(map[string]any)
			if call == nil {
				continue
			}
			index := jsonInt(call["index"])
			for len(a.calls) <= index {
				a.calls = append(a.calls, ToolCall{Type: "function"})
			}
			slot := &a.calls[index]
			if id, _ := call["id"].(string); id != "" {
				slot.ID = id
			}
			if kind, _ := call["type"].(string); kind != "" {
				slot.Type = kind
			}
			fn, _ := call["function"].(map[string]any)
			if fn == nil {
				continue
			}
			if name, _ := fn["name"].(string); name != "" {
				slot.Function.Name = name
			}
			if args, _ := fn["arguments"].(string); args != "" {
				slot.Function.Arguments += args
			}
		}
	}
}

func (a *streamAssembly) result() AdapterResult {
	msg := ChatMessage{Role: "assistant", Content: a.content.String(), ToolCalls: a.calls}
	resp := ChatResponse{ID: a.id, Object: "chat.completion", Model: a.model, Usage: a.usage}
	resp.Choices = append(resp.Choices, struct {
		Index   int         `json:"index"`
		Message ChatMessage `json:"message"`
	}{Index: 0, Message: msg})
	return AdapterResult{HTTPStatus: 200, Body: resp, Stream: append([]string(nil), a.raw...)}
}

func (a *streamAssembly) endedCleanly() bool {
	return a.finish != "" || a.usage != nil
}

// consumeOpenAIStream 按行读取上游 SSE。每读完一个 data 事件就 Emit，不等待后续 token。
func consumeOpenAIStream(ctx context.Context, body io.Reader) (AdapterResult, error) {
	reader := bufio.NewReader(io.LimitReader(body, 16<<20+1))
	var asm streamAssembly
	var read int
	for {
		if err := ctx.Err(); err != nil {
			return asm.interrupted(err)
		}
		line, err := reader.ReadString('\n')
		read += len(line)
		if read > 16<<20 {
			return asm.interrupted(fmt.Errorf("openrouter stream exceeded limit"))
		}
		payload, ok := sseData(line)
		if ok {
			if payload == "[DONE]" {
				return asm.result(), nil
			}
			var chunk map[string]any
			if json.Unmarshal([]byte(payload), &chunk) != nil || chunk["error"] != nil {
				if len(asm.raw) == 0 {
					return AdapterResult{HTTPStatus: 502, ErrorClass: "upstream_error"}, fmt.Errorf("openrouter stream rejected")
				}
				out := asm.result()
				out.HTTPStatus = 502
				out.ErrorClass = "outcome_unknown"
				return out, fmt.Errorf("openrouter stream interrupted")
			}
			if err := emitChunk(ctx, payload); err != nil {
				return asm.interrupted(err)
			}
			asm.addChunk(chunk, payload)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if len(asm.raw) == 0 {
					return AdapterResult{ErrorClass: "timeout"}, fmt.Errorf("openrouter outcome unknown")
				}
				if asm.endedCleanly() {
					return asm.result(), nil
				}
				out := asm.result()
				out.HTTPStatus = 502
				out.ErrorClass = "outcome_unknown"
				return out, fmt.Errorf("openrouter stream truncated")
			}
			return asm.interrupted(err)
		}
	}
}

func (a *streamAssembly) interrupted(err error) (AdapterResult, error) {
	if len(a.raw) == 0 {
		return AdapterResult{HTTPStatus: 408, ErrorClass: "timeout"}, err
	}
	out := a.result()
	out.HTTPStatus = 502
	out.ErrorClass = "outcome_unknown"
	return out, err
}

func sseData(line string) (string, bool) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
		return "", false
	}
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "data:")), true
}

func jsonInt(value any) int {
	switch n := value.(type) {
	case float64:
		if n < 0 {
			return 0
		}
		return int(n)
	case int:
		if n < 0 {
			return 0
		}
		return n
	default:
		return 0
	}
}

func usageFromJSON(value any) map[string]int {
	raw, _ := value.(map[string]any)
	if raw == nil {
		return nil
	}
	prompt, promptOK := jsonIntOK(raw["prompt_tokens"])
	completion, completionOK := jsonIntOK(raw["completion_tokens"])
	total, totalOK := jsonIntOK(raw["total_tokens"])
	if !promptOK && !completionOK && !totalOK {
		return nil
	}
	usage := map[string]int{"prompt_tokens": prompt, "completion_tokens": completion, "total_tokens": total}
	details, _ := raw["completion_tokens_details"].(map[string]any)
	if details != nil {
		if reasoning, ok := jsonIntOK(details["reasoning_tokens"]); ok && reasoning > 0 {
			usage["reasoning_tokens"] = reasoning
			usage["completion_includes_reasoning"] = 1
		}
	}
	return usage
}

func jsonIntOK(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int(number), true
}

func (a BifrostAdapter) chatStream(ctx context.Context, providerSlug string, req ChatRequest) (AdapterResult, error) {
	messages := toBifrostMessages(req.Messages)
	if len(messages) == 0 {
		messages = []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("")},
		}}
	}
	ctx = withProviderProtocolURL(ctx, providerSlug)
	bctx := schemas.NewBifrostContext(ctx, schemas.NoDeadline)
	bctx.SetValue(bifrostProviderSlugKey, providerSlug)
	params := toBifrostParams(ctx, req, providerSlug)
	if params == nil {
		params = &schemas.ChatParameters{}
	}
	params.StreamOptions = &schemas.ChatStreamOptions{IncludeUsage: schemas.Ptr(true)}
	chunks, berr := a.Runtime.Client.ChatCompletionStreamRequest(bctx, &schemas.BifrostChatRequest{
		Provider: resolveBifrostProvider(providerSlug),
		Model:    firstNonEmpty(req.Model, "gemini-2.0-flash"),
		Input:    messages,
		Params:   params,
	})
	if berr != nil {
		return mapBifrostError(berr), fmt.Errorf("%s", berr.GetErrorString())
	}
	if chunks == nil {
		return AdapterResult{HTTPStatus: 502, ErrorClass: "upstream_error"}, fmt.Errorf("bifrost stream unavailable")
	}
	return collectBifrostStream(ctx, chunks)
}

func collectBifrostStream(ctx context.Context, chunks <-chan *schemas.BifrostStreamChunk) (AdapterResult, error) {
	var asm streamAssembly
	for {
		select {
		case <-ctx.Done():
			go drainBifrostStream(chunks)
			return asm.interrupted(ctx.Err())
		case chunk, ok := <-chunks:
			if !ok {
				if len(asm.raw) == 0 {
					return AdapterResult{HTTPStatus: 502, ErrorClass: "upstream_error"}, fmt.Errorf("empty upstream stream")
				}
				if asm.endedCleanly() {
					return asm.result(), nil
				}
				out := asm.result()
				out.HTTPStatus = 502
				out.ErrorClass = "outcome_unknown"
				return out, fmt.Errorf("bifrost stream truncated")
			}
			if chunk == nil {
				continue
			}
			if chunk.BifrostError != nil && len(asm.raw) == 0 {
				return mapBifrostError(chunk.BifrostError), fmt.Errorf("%s", chunk.BifrostError.GetErrorString())
			}
			if chunk.BifrostError != nil {
				out := asm.result()
				out.HTTPStatus = 502
				out.ErrorClass = "outcome_unknown"
				return out, fmt.Errorf("%s", chunk.BifrostError.GetErrorString())
			}
			if chunk.BifrostChatResponse == nil {
				continue
			}
			raw, err := openAIChunkJSON(chunk.BifrostChatResponse)
			if err != nil {
				return asm.interrupted(err)
			}
			if raw == "" {
				continue
			}
			if err := emitChunk(ctx, raw); err != nil {
				go drainBifrostStream(chunks)
				return asm.interrupted(err)
			}
			var body map[string]any
			if json.Unmarshal([]byte(raw), &body) != nil {
				continue
			}
			asm.addChunk(body, raw)
		}
	}
}

func drainBifrostStream(chunks <-chan *schemas.BifrostStreamChunk) {
	for range chunks {
	}
}

func openAIChunkJSON(resp *schemas.BifrostChatResponse) (string, error) {
	if resp == nil {
		return "", nil
	}
	usage := llmUsageMap(resp.Usage)
	choices := make([]any, 0, len(resp.Choices))
	for _, choice := range resp.Choices {
		delta := map[string]any{}
		if choice.ChatStreamResponseChoice != nil && choice.ChatStreamResponseChoice.Delta != nil {
			src := choice.ChatStreamResponseChoice.Delta
			if src.Role != nil {
				delta["role"] = *src.Role
			}
			if src.Content != nil {
				delta["content"] = *src.Content
			}
			if len(src.ToolCalls) > 0 {
				calls := make([]any, 0, len(src.ToolCalls))
				for _, call := range src.ToolCalls {
					item := map[string]any{"index": call.Index}
					if call.ID != nil {
						item["id"] = *call.ID
					}
					if call.Type != nil {
						item["type"] = *call.Type
					}
					fn := map[string]any{}
					if call.Function.Name != nil {
						fn["name"] = *call.Function.Name
					}
					if call.Function.Arguments != "" {
						fn["arguments"] = call.Function.Arguments
					}
					item["function"] = fn
					calls = append(calls, item)
				}
				delta["tool_calls"] = calls
			}
		}
		item := map[string]any{"index": choice.Index, "delta": delta, "finish_reason": nil}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			item["finish_reason"] = *choice.FinishReason
		}
		choices = append(choices, item)
	}
	if len(choices) == 0 && usage == nil {
		return "", nil
	}
	body := map[string]any{
		"id":      resp.ID,
		"object":  "chat.completion.chunk",
		"model":   resp.Model,
		"choices": choices,
	}
	if usage != nil {
		body["usage"] = usage
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func llmUsageMap(usage *schemas.BifrostLLMUsage) map[string]int {
	if usage == nil {
		return nil
	}
	reasoning := 0
	if usage.CompletionTokensDetails != nil {
		reasoning = usage.CompletionTokensDetails.ReasoningTokens
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 && reasoning == 0 {
		return nil
	}
	out := map[string]int{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	if reasoning > 0 {
		out["reasoning_tokens"] = reasoning
		out["completion_includes_reasoning"] = 1
	}
	return out
}
