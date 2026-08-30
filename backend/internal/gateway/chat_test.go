package gateway

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestChatMessageVisionAndTools(t *testing.T) {
	var msg ChatMessage
	if err := json.Unmarshal([]byte(`{"role":"user","content":[{"type":"text","text":"see"},{"type":"image_url","image_url":{"url":"https://x/a.png"}}]}`), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Content != "see" || visionCount(ChatRequest{Messages: []ChatMessage{msg}}) != 1 {
		t.Fatalf("vision flatten: %+v", msg)
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) || string(raw) == "" {
		t.Fatalf("marshal: %s", raw)
	}
}

func TestValidateChat(t *testing.T) {
	caps := map[string]any{"supported_parameters": []string{"tools", "vision", "json", "reasoning"}, "unsupported_parameters": []string{"logit_bias"}}
	if err := ValidateChat(ChatRequest{LogitBias: []byte(`{"1":1}`)}, caps); !errors.Is(err, ErrUnsupportedParam) {
		t.Fatalf("logit_bias should be unsupported: %v", err)
	}
	tools, _ := json.Marshal([]map[string]any{{"type": "function", "function": map[string]string{"name": "lookup"}}})
	if err := ValidateChat(ChatRequest{Tools: tools}, caps); err != nil {
		t.Fatal(err)
	}
	oem := map[string]any{"supported_parameters": []string{"messages", "model"}}
	if err := ValidateChat(ChatRequest{Tools: tools}, oem); !errors.Is(err, ErrUnsupportedParam) {
		t.Fatalf("oem should reject tools: %v", err)
	}
}
