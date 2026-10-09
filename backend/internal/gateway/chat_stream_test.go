package gateway

import (
	"encoding/json"
	"testing"
)

func TestStreamKeepsPublicModelIdentityAndUsage(t *testing.T) {
	var response ChatResponse
	if err := json.Unmarshal([]byte(`{"id":"chat-local","model":"internal-upstream-model","choices":[{"index":0,"message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":7,"completion_tokens":9,"total_tokens":16}}`), &response); err != nil {
		t.Fatal(err)
	}
	chunks := publicChatStreamChunks(chatStreamChunks(response), "brand/public-model")
	if len(chunks) != 3 {
		t.Fatalf("stream chunks=%v", chunks)
	}
	for _, chunk := range chunks {
		var body map[string]any
		if err := json.Unmarshal([]byte(chunk), &body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "brand/public-model" {
			t.Fatalf("public stream leaked upstream identity: %+v", body)
		}
	}
	var usage map[string]any
	_ = json.Unmarshal([]byte(chunks[2]), &usage)
	if usage["usage"].(map[string]any)["total_tokens"] != float64(16) || len(usage["choices"].([]any)) != 0 {
		t.Fatalf("usage chunk invalid: %+v", usage)
	}
}
