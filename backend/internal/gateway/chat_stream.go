package gateway

import "encoding/json"

// The current adapter buffers the upstream response, then emits valid Chat SSE
// chunks. Tool deltas must retain call identity and arguments for the next turn.
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
		var body map[string]any
		if err := json.Unmarshal([]byte(chunk), &body); err != nil {
			out[i] = chunk
			continue
		}
		body["model"] = publicModel
		raw, _ := json.Marshal(body)
		out[i] = string(raw)
	}
	return out
}
