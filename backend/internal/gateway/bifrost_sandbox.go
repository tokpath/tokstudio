package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SandboxHandler 是 P0 的 Bifrost 数据面沙箱：只做协议回声，不碰账务。
func SandboxHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"service":"bifrost-sandbox"}`))
	})
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		text := "bifrost:echo"
		if len(req.Messages) > 0 {
			text = "bifrost:" + req.Messages[len(req.Messages)-1].Content
		}
		slug := r.Header.Get("X-Tokenhub-Provider")
		if slug != "" {
			text += " via " + slug
		}
		resp := ChatResponse{
			ID:     fmt.Sprintf("bifrost_%d", time.Now().UnixNano()),
			Object: "chat.completion",
			Model:  req.Model,
			Usage:  map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12},
		}
		resp.Choices = append(resp.Choices, struct {
			Index   int         `json:"index"`
			Message ChatMessage `json:"message"`
		}{Index: 0, Message: ChatMessage{Role: "assistant", Content: text}})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}
