package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
)

type ctxKey string

const (
	bifrostProviderSlugKey ctxKey = "tokenhub.provider_slug"
	ctxRequestIDKey        ctxKey = "tokenhub.request_id"
)

// ContextWithRequestID 把账务 request_id 放进 context，供 Bifrost metadata 透传。
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if strings.TrimSpace(requestID) == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxRequestIDKey, requestID)
}

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestIDKey).(string)
	return v
}

// Runtime 是嵌在 TokenHub 进程里的 Bifrost 数据面。
// 沙箱模式用 plugin 短路回声，不打真实上游；live 模式才用 Account 里的 Key。
type Runtime struct {
	Client  *bifrost.Bifrost
	Sandbox bool
}

func (rt *Runtime) Close() {
	if rt == nil || rt.Client == nil {
		return
	}
	rt.Client.Shutdown()
}

// Settings 是进程内 Init Bifrost 所需的配置。
type Settings struct {
	Sandbox          bool
	LogLevel         string
	OpenAIAPIKey     string
	AnthropicAPIKey  string
	GeminiAPIKey     string
	OpenRouterAPIKey string
}

// Start 在当前进程初始化 Bifrost SDK。失败时调用方应让 Adapter 返回 provider_unavailable。
func Start(ctx context.Context, in Settings) (*Runtime, error) {
	account := &envAccount{settings: in}
	if !in.Sandbox {
		providers, err := account.GetConfiguredProviders()
		if err != nil {
			return nil, err
		}
		if len(providers) == 0 {
			return nil, fmt.Errorf("bifrost live mode requires at least one provider api key")
		}
	}
	cfg := schemas.BifrostConfig{
		Account:         account,
		Logger:          newBifrostLogger(in.LogLevel),
		InitialPoolSize: 0,
	}
	if in.Sandbox {
		cfg.LLMPlugins = []schemas.LLMPlugin{sandboxPlugin{}}
	}
	client, err := bifrost.Init(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Runtime{Client: client, Sandbox: in.Sandbox}, nil
}

// BifrostAdapter 在进程内调用 Bifrost SDK。client 为空时返回 503。
type BifrostAdapter struct {
	Runtime *Runtime
}

func (a BifrostAdapter) Name() string { return "bifrost" }

func (a BifrostAdapter) Chat(ctx context.Context, providerSlug, _ string, req ChatRequest) (AdapterResult, error) {
	if a.Runtime == nil || a.Runtime.Client == nil {
		return AdapterResult{HTTPStatus: 503, ErrorClass: "provider_unavailable"}, fmt.Errorf("bifrost unavailable")
	}
	messages := toBifrostMessages(req.Messages)
	if len(messages) == 0 {
		messages = []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("echo")},
		}}
	}
	bctx := schemas.NewBifrostContext(ctx, schemas.NoDeadline)
	bctx.SetValue(bifrostProviderSlugKey, providerSlug)
	resp, berr := a.Runtime.Client.ChatCompletionRequest(bctx, &schemas.BifrostChatRequest{
		Provider: resolveBifrostProvider(providerSlug),
		Model:    firstNonEmpty(req.Model, "tokenhub/echo-1"),
		Input:    messages,
		Params:   toBifrostParams(ctx, req, providerSlug),
	})
	if berr != nil {
		return mapBifrostError(berr), fmt.Errorf("%s", berr.GetErrorString())
	}
	out := fromBifrostChat(resp)
	if req.Stream && len(out.Body.Choices) > 0 {
		text := out.Body.Choices[0].Message.Content
		out.Stream = []string{
			`{"id":"` + out.Body.ID + `","object":"chat.completion.chunk","choices":[{"delta":{"content":"` + jsonEscape(text) + `"}}]}`,
		}
	}
	return out, nil
}

type envAccount struct {
	settings Settings
}

func (a *envAccount) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	if a.settings.Sandbox {
		return []schemas.ModelProvider{schemas.OpenAI}, nil
	}
	var out []schemas.ModelProvider
	if strings.TrimSpace(a.settings.OpenAIAPIKey) != "" {
		out = append(out, schemas.OpenAI)
	}
	if strings.TrimSpace(a.settings.AnthropicAPIKey) != "" {
		out = append(out, schemas.Anthropic)
	}
	if strings.TrimSpace(a.settings.GeminiAPIKey) != "" {
		out = append(out, schemas.Gemini)
	}
	if strings.TrimSpace(a.settings.OpenRouterAPIKey) != "" {
		out = append(out, schemas.OpenRouter)
	}
	return out, nil
}

func (a *envAccount) GetKeysForProvider(_ context.Context, provider schemas.ModelProvider) ([]schemas.Key, error) {
	key := ""
	switch provider {
	case schemas.OpenAI:
		key = a.settings.OpenAIAPIKey
		if a.settings.Sandbox && strings.TrimSpace(key) == "" {
			key = "sk-tokenhub-sandbox"
		}
	case schemas.Anthropic:
		key = a.settings.AnthropicAPIKey
	case schemas.Gemini:
		key = a.settings.GeminiAPIKey
	case schemas.OpenRouter:
		key = a.settings.OpenRouterAPIKey
	default:
		return nil, fmt.Errorf("provider %s not configured", provider)
	}
	if strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("provider %s has no api key", provider)
	}
	return []schemas.Key{{
		ID:     "tokenhub-" + string(provider),
		Name:   "tokenhub-" + string(provider),
		Value:  *schemas.NewSecretVar(key),
		Models: schemas.WhiteList{"*"},
		Weight: 1.0,
	}}, nil
}

func (a *envAccount) GetConfigForProvider(provider schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	switch provider {
	case schemas.OpenAI, schemas.Anthropic, schemas.Gemini, schemas.OpenRouter:
		return &schemas.ProviderConfig{
			NetworkConfig:            schemas.DefaultNetworkConfig,
			ConcurrencyAndBufferSize: schemas.DefaultConcurrencyAndBufferSize,
		}, nil
	default:
		return nil, fmt.Errorf("provider %s not configured", provider)
	}
}

// sandboxPlugin 在 PreLLMHook 短路，保持本地/CI 不依赖真实 Provider Key。
type sandboxPlugin struct{}

func (sandboxPlugin) GetName() string { return "tokenhub-sandbox" }

func (sandboxPlugin) Cleanup() error { return nil }

func (sandboxPlugin) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	return nil
}

func (sandboxPlugin) PreLLMHook(ctx *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	slug, _ := ctx.Value(bifrostProviderSlugKey).(string)
	text := "bifrost:" + lastChatText(req)
	if slug != "" {
		text += " via " + slug
	}
	model := "tokenhub/echo-1"
	if req != nil && req.ChatRequest != nil && req.ChatRequest.Model != "" {
		model = req.ChatRequest.Model
	}
	id := fmt.Sprintf("bifrost_%d", time.Now().UnixNano())
	return req, &schemas.LLMPluginShortCircuit{
		Response: &schemas.BifrostResponse{
			ChatResponse: &schemas.BifrostChatResponse{
				ID:     id,
				Object: "chat.completion",
				Model:  model,
				Usage: &schemas.BifrostLLMUsage{
					PromptTokens:     8,
					CompletionTokens: 4,
					TotalTokens:      12,
				},
				Choices: []schemas.BifrostResponseChoice{{
					Index: 0,
					ChatNonStreamResponseChoice: &schemas.ChatNonStreamResponseChoice{
						Message: &schemas.ChatMessage{
							Role: schemas.ChatMessageRoleAssistant,
							Content: &schemas.ChatMessageContent{
								ContentStr: schemas.Ptr(text),
							},
						},
					},
				}},
			},
		},
	}, nil
}

func (sandboxPlugin) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, bifrostErr *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, bifrostErr, nil
}

func lastChatText(req *schemas.BifrostRequest) string {
	if req == nil || req.ChatRequest == nil || len(req.ChatRequest.Input) == 0 {
		return "echo"
	}
	last := req.ChatRequest.Input[len(req.ChatRequest.Input)-1]
	if last.Content != nil && last.Content.ContentStr != nil {
		return *last.Content.ContentStr
	}
	return "echo"
}

func toBifrostMessages(in []ChatMessage) []schemas.ChatMessage {
	out := make([]schemas.ChatMessage, 0, len(in))
	for _, msg := range in {
		item := schemas.ChatMessage{
			Role:    schemas.ChatMessageRole(msg.Role),
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr(msg.Content)},
		}
		if item.Role == "" {
			item.Role = schemas.ChatMessageRoleUser
		}
		out = append(out, item)
	}
	return out
}

func toBifrostParams(ctx context.Context, req ChatRequest, providerSlug string) *schemas.ChatParameters {
	meta := map[string]any{
		"tokenhub_provider": providerSlug,
	}
	if requestID := requestIDFromContext(ctx); requestID != "" {
		meta["request_id"] = requestID
	}
	params := &schemas.ChatParameters{
		Temperature:         req.Temperature,
		MaxCompletionTokens: req.MaxTokens,
		Metadata:            &meta,
	}
	if len(req.LogitBias) > 0 {
		var bias map[string]float64
		if err := json.Unmarshal(req.LogitBias, &bias); err == nil {
			params.LogitBias = &bias
		}
	}
	if len(req.ResponseFormat) > 0 {
		var rf any
		if err := json.Unmarshal(req.ResponseFormat, &rf); err == nil {
			params.ResponseFormat = &rf
		}
	}
	if len(req.Tools) > 0 {
		var tools []schemas.ChatTool
		if err := json.Unmarshal(req.Tools, &tools); err == nil {
			params.Tools = tools
		}
	}
	if len(req.ToolChoice) > 0 {
		var toolChoice schemas.ChatToolChoice
		if err := json.Unmarshal(req.ToolChoice, &toolChoice); err == nil {
			params.ToolChoice = &toolChoice
		}
	}
	if req.ReasoningEffort != "" {
		params.Reasoning = &schemas.ChatReasoning{Effort: schemas.Ptr(req.ReasoningEffort)}
	}
	return params
}

func fromBifrostChat(resp *schemas.BifrostChatResponse) AdapterResult {
	out := ChatResponse{Object: "chat.completion"}
	if resp == nil {
		return AdapterResult{HTTPStatus: 502, ErrorClass: "upstream_error", Body: out}
	}
	out.ID = resp.ID
	out.Model = resp.Model
	if resp.Object != "" {
		out.Object = resp.Object
	}
	if resp.Usage != nil {
		out.Usage = map[string]int{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		}
	}
	for _, choice := range resp.Choices {
		msg := ChatMessage{Role: "assistant"}
		if choice.ChatNonStreamResponseChoice != nil && choice.ChatNonStreamResponseChoice.Message != nil {
			src := choice.ChatNonStreamResponseChoice.Message
			msg.Role = string(src.Role)
			if src.Content != nil && src.Content.ContentStr != nil {
				msg.Content = *src.Content.ContentStr
			}
		}
		out.Choices = append(out.Choices, struct {
			Index   int         `json:"index"`
			Message ChatMessage `json:"message"`
		}{Index: choice.Index, Message: msg})
	}
	return AdapterResult{HTTPStatus: 200, Body: out}
}

func mapBifrostError(berr *schemas.BifrostError) AdapterResult {
	status := 502
	class := "upstream_error"
	if berr != nil && berr.StatusCode != nil && *berr.StatusCode > 0 {
		status = *berr.StatusCode
	}
	switch status {
	case 408:
		class = "timeout"
	case 429:
		class = "rate_limited"
	case 503:
		class = "provider_unavailable"
	}
	return AdapterResult{HTTPStatus: status, ErrorClass: class}
}

func resolveBifrostProvider(slug string) schemas.ModelProvider {
	s := strings.ToLower(slug)
	switch {
	case strings.Contains(s, "anthropic"), strings.Contains(s, "claude"):
		return schemas.Anthropic
	case strings.Contains(s, "gemini"), strings.Contains(s, "google"):
		return schemas.Gemini
	case strings.Contains(s, "openrouter"):
		return schemas.OpenRouter
	default:
		return schemas.OpenAI
	}
}

func newBifrostLogger(level string) schemas.Logger {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return bifrost.NewDefaultLogger(schemas.LogLevelDebug)
	case "warn", "warning":
		return bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	case "error":
		return bifrost.NewDefaultLogger(schemas.LogLevelError)
	default:
		return bifrost.NewDefaultLogger(schemas.LogLevelInfo)
	}
}

func jsonEscape(v string) string {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	if len(b) >= 2 {
		return string(b[1 : len(b)-1])
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
