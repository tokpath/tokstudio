package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
)

type ctxKey string

const (
	bifrostProviderSlugKey ctxKey = "tokenhub.provider_slug"
	ctxRequestIDKey        ctxKey = "tokenhub.request_id"
	ctxAPIKeyIDKey         ctxKey = "tokenhub.api_key_id"
	ctxUserIDKey           ctxKey = "tokenhub.user_id"
	ctxChannelIDKey        ctxKey = "tokenhub.channel_org_id"
	ctxPublicModelKey      ctxKey = "tokenhub.public_model_id"
	ctxAttemptIDKey        ctxKey = "tokenhub.attempt_id"
	ctxAccountSecretKey    ctxKey = "tokenhub.account_secret"
	ctxProviderBaseURLKey  ctxKey = "tokenhub.provider_base_url"
)

// ContextWithRequestID 把账务 request_id 放进 context，供 Bifrost metadata 透传。
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return ContextWithMeta(ctx, map[string]string{"request_id": requestID})
}

// ContextWithMeta 把 request / API Key / 用户 / 渠道写进 context，Bifrost 只读 metadata。
func ContextWithMeta(ctx context.Context, meta map[string]string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	for key, value := range meta {
		if strings.TrimSpace(value) == "" {
			continue
		}
		switch key {
		case "request_id":
			ctx = context.WithValue(ctx, ctxRequestIDKey, value)
		case "api_key_id":
			ctx = context.WithValue(ctx, ctxAPIKeyIDKey, value)
		case "user_id":
			ctx = context.WithValue(ctx, ctxUserIDKey, value)
		case "channel_org_id":
			ctx = context.WithValue(ctx, ctxChannelIDKey, value)
		case "public_model_id":
			ctx = context.WithValue(ctx, ctxPublicModelKey, value)
		case "attempt_id":
			ctx = context.WithValue(ctx, ctxAttemptIDKey, value)
		}
	}
	return ctx
}

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxRequestIDKey).(string)
	return v
}

func contextString(ctx context.Context, key ctxKey) string {
	v, _ := ctx.Value(key).(string)
	return v
}

// Runtime 是嵌在 TokenHub 进程里的 Bifrost 数据面（仅 live，禁止 sandbox echo plugin）。
type Runtime struct {
	Client       *bifrost.Bifrost
	Sandbox      bool // 恒为 false；保留字段避免调用方大面积改动。
	GeminiAPIKey string
}

// GeminiLiveEnabled 仅在配置了非空 Gemini Key 且 Runtime 非沙箱时打真实上游。
func GeminiLiveEnabled(rt *Runtime) bool {
	return rt != nil && !rt.Sandbox && strings.TrimSpace(rt.GeminiAPIKey) != ""
}

func (rt *Runtime) Close() {
	if rt == nil || rt.Client == nil {
		return
	}
	rt.Client.Shutdown()
}

// Settings 是进程内 Init Bifrost 所需的配置。
// Sandbox 字段已废弃：即使传入 true 也会被忽略，始终走 live，缺 Key 则 Init 失败。
type Settings struct {
	Sandbox          bool // deprecated: ignored
	LogLevel         string
	OpenAIAPIKey     string
	AnthropicAPIKey  string
	GeminiAPIKey     string
	OpenRouterAPIKey string
	EncryptionKey    string
	Keys             AccountKeys
}

// AccountKeys 由 catalog 实现：把目录账号池解密给 Bifrost。
type AccountKeys interface {
	ListPlainKeys(ctx context.Context, encKey, providerKind string) ([]catalog.PlainKey, error)
	HasPlainKeys(ctx context.Context, encKey, providerKind string) bool
}

// Start 在当前进程初始化 Bifrost SDK（仅 live）。失败时调用方应让 Adapter 返回 provider_unavailable。
func Start(ctx context.Context, in Settings) (*Runtime, error) {
	in.Sandbox = false
	account := &envAccount{settings: in}
	providers, err := account.GetConfiguredProviders()
	if err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return nil, fmt.Errorf("bifrost live mode requires at least one provider api key")
	}
	cfg := schemas.BifrostConfig{
		Account:         account,
		Logger:          newBifrostLogger(in.LogLevel),
		InitialPoolSize: 0,
	}
	client, err := bifrost.Init(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Runtime{Client: client, Sandbox: false, GeminiAPIKey: in.GeminiAPIKey}, nil
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
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("")},
		}}
	}
	ctx = withProviderChatURL(ctx)
	bctx := schemas.NewBifrostContext(ctx, schemas.NoDeadline)
	bctx.SetValue(bifrostProviderSlugKey, providerSlug)
	resp, berr := a.Runtime.Client.ChatCompletionRequest(bctx, &schemas.BifrostChatRequest{
		Provider: resolveBifrostProvider(providerSlug),
		Model:    firstNonEmpty(req.Model, "gemini-2.0-flash"),
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
	var out []schemas.ModelProvider
	for _, item := range []struct {
		key   string
		kind  string
		model schemas.ModelProvider
	}{
		{a.settings.OpenAIAPIKey, "openai", schemas.OpenAI},
		{a.settings.AnthropicAPIKey, "anthropic", schemas.Anthropic},
		{a.settings.GeminiAPIKey, "gemini", schemas.Gemini},
		{a.settings.OpenRouterAPIKey, "openrouter", schemas.OpenRouter},
	} {
		if strings.TrimSpace(item.key) != "" || a.hasCatalogKeys(item.kind) {
			out = append(out, item.model)
		}
	}
	return out, nil
}

func (a *envAccount) hasCatalogKeys(kind string) bool {
	if a.settings.Keys == nil {
		return false
	}
	return a.settings.Keys.HasPlainKeys(context.Background(), a.settings.EncryptionKey, kind)
}

func (a *envAccount) GetKeysForProvider(ctx context.Context, provider schemas.ModelProvider) ([]schemas.Key, error) {
	kind := bifrostKind(provider)
	envKey := ""
	switch provider {
	case schemas.OpenAI:
		envKey = a.settings.OpenAIAPIKey
	case schemas.Anthropic:
		envKey = a.settings.AnthropicAPIKey
	case schemas.Gemini:
		envKey = a.settings.GeminiAPIKey
	case schemas.OpenRouter:
		envKey = a.settings.OpenRouterAPIKey
	default:
		return nil, fmt.Errorf("provider %s not configured", provider)
	}
	var out []schemas.Key
	if preferred := contextString(ctx, ctxAccountSecretKey); strings.TrimSpace(preferred) != "" {
		out = append(out, schemas.Key{
			ID:     firstNonEmpty(contextString(ctx, ctxAttemptIDKey), "tokenhub-preferred"),
			Name:   "tokenhub-preferred",
			Value:  *schemas.NewSecretVar(preferred),
			Models: schemas.WhiteList{"*"},
			Weight: 1.0,
		})
	}
	if strings.TrimSpace(envKey) != "" {
		out = append(out, schemas.Key{
			ID:     "tokenhub-" + string(provider),
			Name:   "tokenhub-" + string(provider),
			Value:  *schemas.NewSecretVar(envKey),
			Models: schemas.WhiteList{"*"},
			Weight: 0.5,
		})
	}
	if a.settings.Keys != nil && strings.TrimSpace(a.settings.EncryptionKey) != "" {
		rows, err := a.settings.Keys.ListPlainKeys(ctx, a.settings.EncryptionKey, kind)
		if err == nil {
			for _, row := range rows {
				if strings.TrimSpace(row.Value) == "" {
					continue
				}
				out = append(out, schemas.Key{
					ID:     row.ID,
					Name:   row.ID,
					Value:  *schemas.NewSecretVar(row.Value),
					Models: schemas.WhiteList{"*"},
					Weight: 0.5,
				})
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider %s has no api key", provider)
	}
	return out, nil
}

func bifrostKind(provider schemas.ModelProvider) string {
	switch provider {
	case schemas.Anthropic:
		return "anthropic"
	case schemas.Gemini:
		return "gemini"
	case schemas.OpenRouter:
		return "openrouter"
	default:
		return "openai"
	}
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
	if v := contextString(ctx, ctxAPIKeyIDKey); v != "" {
		meta["api_key_id"] = v
	}
	if v := contextString(ctx, ctxUserIDKey); v != "" {
		meta["user_id"] = v
	}
	if v := contextString(ctx, ctxChannelIDKey); v != "" {
		meta["channel_org_id"] = v
	}
	if v := contextString(ctx, ctxPublicModelKey); v != "" {
		meta["public_model_id"] = v
	}
	if v := contextString(ctx, ctxAttemptIDKey); v != "" {
		meta["attempt_id"] = v
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
	echoed := echoedMetaFromExtra(resp.ExtraFields)
	fact := normalizeFactSource(echoed["fact_source"], false)
	// live 只在 Bifrost 回了真实路由事实时标记；空 Extra 不得冒充上游。
	if fact == "" && (echoed["bifrost_provider"] != "" || echoed["upstream_model"] != "") {
		fact = FactSourceLive
	}
	return AdapterResult{
		HTTPStatus: 200,
		Body:       out,
		FactSource: fact,
		EchoedMeta: echoed,
	}
}

func echoedMetaFromExtra(extra schemas.BifrostResponseExtraFields) map[string]string {
	out := map[string]string{}
	for key, value := range extra.ProviderResponseHeaders {
		name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(key)), "x-tokenhub-")
		if name == "" || strings.TrimSpace(value) == "" {
			continue
		}
		out[name] = value
	}
	if extra.RoutingInfo.Model != "" {
		out["upstream_model"] = extra.RoutingInfo.Model
	}
	if extra.RoutingInfo.Provider != "" {
		out["bifrost_provider"] = string(extra.RoutingInfo.Provider)
	}
	return out
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

func withProviderChatURL(ctx context.Context) context.Context {
	u := chatCompletionsURL(contextString(ctx, ctxProviderBaseURLKey))
	if u == "" {
		return ctx
	}
	return context.WithValue(ctx, schemas.BifrostContextKeyURLPath, u)
}

func chatCompletionsURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
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
