package gateway

import (
	"encoding/json"
	"strings"
)

// chatStreamChunks 只用于上游没有逐 token 流、只给了完整 JSON 的兼容回退。
// 真正的实时路径在 consumeOpenAIStream / chatStream：每个上游事件立刻 Emit。
// 工具增量必须保留 call 的 index、id、函数名和参数，下一轮才能对上。
func chatStreamChunks(response ChatResponse) []string {
	chunks := make([]string, 0, len(response.Choices)*2+1)
	appendChunk := func(choices []any, usage map[string]int) {
		body := map[string]any{"id": response.ID, "object": "chat.completion.chunk", "model": response.Model, "choices": choices}
		if usage != nil {
			body["usage"] = usage
		}
		raw, _ := json.Marshal(body)
		chunks = append(chunks, string(raw))
	}
	for _, choice := range response.Choices {
		delta := map[string]any{"role": "assistant"}
		if choice.Message.Content != "" {
			delta["content"] = choice.Message.Content
		}
		finish := "stop"
		if len(choice.Message.ToolCalls) > 0 {
			finish = "tool_calls"
			calls := make([]map[string]any, 0, len(choice.Message.ToolCalls))
			for index, call := range choice.Message.ToolCalls {
				calls = append(calls, map[string]any{"index": index, "id": call.ID, "type": "function", "function": call.Function})
			}
			delta["tool_calls"] = calls
		}
		appendChunk([]any{map[string]any{"index": choice.Index, "delta": delta, "finish_reason": nil}}, nil)
		appendChunk([]any{map[string]any{"index": choice.Index, "delta": map[string]any{}, "finish_reason": finish}}, nil)
	}
	if response.Usage != nil {
		appendChunk([]any{}, response.Usage)
	}
	return chunks
}

func publicChatStreamChunks(chunks []string, publicModel string) []string {
	out := make([]string, len(chunks))
	for i, chunk := range chunks {
		out[i] = rewriteChunkModel(chunk, publicModel)
	}
	return out
}

func rewriteChunkModel(chunk, publicModel string) string {
	if strings.TrimSpace(publicModel) == "" {
		return chunk
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(chunk), &body); err != nil {
		return chunk
	}
	body["model"] = publicModel
	raw, err := json.Marshal(body)
	if err != nil {
		return chunk
	}
	return string(raw)
}
