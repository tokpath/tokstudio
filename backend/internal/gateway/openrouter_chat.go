package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenRouter's SDK adapter concatenates URLPath to a fixed origin. Use its
// documented OpenAI-compatible HTTP contract so an account's actual BaseURL is
// honored, with the total max_completion_tokens ceiling preserved.
func (a BifrostAdapter) openRouterChat(ctx context.Context, req ChatRequest) (AdapterResult, error) {
	if req.MaxTokens != nil && *req.MaxTokens < 16 {
		return AdapterResult{HTTPStatus: 400, ErrorClass: "invalid_request"}, ParamError{Param: "max_tokens (OpenRouter minimum 16)"}
	}
	secret := contextString(ctx, ctxAccountSecretKey)
	if secret == "" {
		secret = a.Runtime.settings.OpenRouterAPIKey
	}
	if secret == "" && a.Runtime.settings.Keys != nil {
		keys, err := a.Runtime.settings.Keys.ListPlainKeys(ctx, a.Runtime.settings.EncryptionKey, "openrouter")
		if err != nil {
			return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, err
		}
		for _, key := range keys {
			if key.Value != "" {
				secret = key.Value
				break
			}
		}
	}
	if secret == "" {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("openrouter account unavailable")
	}
	base := strings.TrimRight(contextString(ctx, ctxProviderBaseURLKey), "/")
	if base == "" {
		base = "https://openrouter.ai/api/v1"
	}
	if base == "https://openrouter.ai" {
		base += "/api/v1"
	}
	endpoint := chatCompletionsURL(base)
	guard := a.Runtime.upstreamHTTP
	if guard == nil {
		guard = newUpstreamHTTP(a.Runtime.settings)
		defer guard.client.CloseIdleConnections()
	}
	// Only server-allowed URL authorities can reach the request constructor.
	// The transport separately pins public DNS results and rejects redirects.
	if !guard.allowedURL.MatchString(endpoint) {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, errBlockedUpstream
	}
	if err := guard.validateURL(endpoint); err != nil {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, err
	}
	raw, _ := json.Marshal(req)
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return AdapterResult{}, err
	}
	delete(body, "max_tokens")
	body["max_completion_tokens"] = req.MaxTokens
	body["stream"] = req.Stream
	if req.Stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if req.ReasoningEffort != "" {
		delete(body, "reasoning_effort")
		body["reasoning"] = map[string]any{"effort": req.ReasoningEffort}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return AdapterResult{}, err
	}
	call, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(raw))
	if err != nil {
		return AdapterResult{}, err
	}
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("Authorization", "Bearer "+secret)
	response, err := guard.client.Do(call)
	if err != nil {
		if errors.Is(err, errBlockedUpstream) {
			return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, errBlockedUpstream
		}
		return AdapterResult{ErrorClass: "timeout"}, fmt.Errorf("openrouter outcome unknown")
	}
	defer response.Body.Close()
	result := AdapterResult{HTTPStatus: response.StatusCode}
	if response.StatusCode >= 400 {
		result.ErrorClass = "upstream_error"
		if response.StatusCode >= 500 {
			result.ErrorClass = "outcome_unknown"
		}
		if response.StatusCode == 408 {
			result.ErrorClass = "timeout"
		}
		if response.StatusCode == 429 {
			result.ErrorClass = "rate_limited"
		}
		return result, nil
	}
	if req.Stream && strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		return consumeOpenAIStream(ctx, response.Body)
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 16<<20+1))
	if err != nil || len(responseBody) > 16<<20 {
		return AdapterResult{ErrorClass: "timeout"}, fmt.Errorf("openrouter outcome unknown")
	}
	var rawResponse map[string]json.RawMessage
	if err := json.Unmarshal(responseBody, &rawResponse); err != nil {
		return AdapterResult{ErrorClass: "timeout"}, fmt.Errorf("openrouter outcome unknown")
	}
	var usage struct {
		Prompt     int `json:"prompt_tokens"`
		Completion int `json:"completion_tokens"`
		Total      int `json:"total_tokens"`
		Details    struct {
			Reasoning int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	}
	if err := json.Unmarshal(rawResponse["usage"], &usage); err != nil && len(rawResponse["usage"]) > 0 {
		return AdapterResult{ErrorClass: "timeout"}, fmt.Errorf("openrouter usage unconfirmed")
	}
	delete(rawResponse, "usage")
	sanitized, _ := json.Marshal(rawResponse)
	if err := json.Unmarshal(sanitized, &result.Body); err != nil {
		return AdapterResult{ErrorClass: "timeout"}, fmt.Errorf("openrouter outcome unknown")
	}
	if len(responseBody) > 0 && usage.Total > 0 {
		result.Body.Usage = map[string]int{"prompt_tokens": usage.Prompt, "completion_tokens": usage.Completion, "total_tokens": usage.Total}
		if usage.Details.Reasoning > 0 {
			result.Body.Usage["reasoning_tokens"] = usage.Details.Reasoning
			result.Body.Usage["completion_includes_reasoning"] = 1
		}
	}
	if req.Stream {
		result.Stream = chatStreamChunks(result.Body)
		for _, chunk := range result.Stream {
			if err := emitChunk(ctx, chunk); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}
