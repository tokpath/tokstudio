package modelruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"regexp"
	"strings"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
)

// BifrostConfig permits platform-owned credentials only. EndpointGuard must
// enforce the platform's URL allowlist and DNS/private-network policy for a
// configured endpoint. It is deliberately not supplied by customer requests.
type BifrostConfig struct {
	Costs             CostMeter
	Providers         []string
	Credentials       CredentialSource
	EndpointGuard     func(context.Context, string) error
	ResourceGuard     func(context.Context, string) error
	AllowTestLoopback bool
}
type Bifrost struct {
	client *bifrost.Bifrost
	config BifrostConfig
	egress *egressProxy
}

func NewBifrost(ctx context.Context, c BifrostConfig) (*Bifrost, error) {
	if c.Credentials == nil {
		return nil, ErrInvalidRequest
	}
	providers := map[schemas.ModelProvider]bool{}
	for _, name := range c.Providers {
		p := schemas.ModelProvider(name)
		switch p {
		case schemas.OpenAI, schemas.Anthropic, schemas.Gemini, schemas.OpenRouter, schemas.Elevenlabs:
		default:
			return nil, fmt.Errorf("unsupported configured provider %q", name)
		}
		providers[p] = true
	}
	egress, e := newEgressProxy(c.AllowTestLoopback)
	if e != nil {
		return nil, e
	}
	client, e := bifrost.Init(ctx, schemas.BifrostConfig{Account: runtimeAccount{providers, egress.config()}, Logger: privateLogger{}, Tracer: &schemas.NoOpTracer{}, InitialPoolSize: 0})
	if e != nil {
		egress.close()
		return nil, fmt.Errorf("initialize bifrost: %w", e)
	}
	return &Bifrost{client: client, config: c, egress: egress}, nil
}
func (b *Bifrost) Close() {
	if b != nil && b.client != nil {
		b.client.Shutdown()
		if b.egress != nil {
			b.egress.close()
		}
	}
}

type runtimeAccount struct {
	providers map[schemas.ModelProvider]bool
	proxy     *schemas.ProxyConfig
}

func (a runtimeAccount) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	out := make([]schemas.ModelProvider, 0, len(a.providers))
	for p := range a.providers {
		out = append(out, p)
	}
	return out, nil
}
func (a runtimeAccount) GetKeysForProvider(context.Context, schemas.ModelProvider) ([]schemas.Key, error) {
	return nil, fmt.Errorf("an explicitly selected attempt credential is required")
}
func (a runtimeAccount) GetConfigForProvider(p schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	if !a.providers[p] {
		return nil, ErrUnsupported
	}
	network := schemas.DefaultNetworkConfig
	network.MaxRetries = 0
	return &schemas.ProviderConfig{NetworkConfig: network, ProxyConfig: a.proxy, ConcurrencyAndBufferSize: schemas.DefaultConcurrencyAndBufferSize}, nil
}

// The SDK may format bodies, credentials and provider error strings into logs.
// Technical observability comes from Result/attempt facts, never SDK messages.
type privateLogger struct{}

func (privateLogger) Debug(string, ...any)                   {}
func (privateLogger) Info(string, ...any)                    {}
func (privateLogger) Warn(string, ...any)                    {}
func (privateLogger) Error(string, ...any)                   {}
func (privateLogger) Fatal(string, ...any)                   {}
func (privateLogger) SetLevel(schemas.LogLevel)              {}
func (privateLogger) SetOutputType(schemas.LoggerOutputType) {}
func (privateLogger) LogHTTPRequest(schemas.LogLevel, string) schemas.LogEventBuilder {
	return schemas.NoopLogEvent
}

func (b *Bifrost) Execute(ctx context.Context, a Attempt, r Request, emit Emit) (out Result, err error) {
	out = Result{AttemptID: a.ID, State: "not_dispatched", ErrorCode: "request_not_dispatched", Usage: Usage{Source: "unknown"}}
	if b == nil || b.client == nil {
		return out, fmt.Errorf("bifrost unavailable")
	}
	if err = ValidateRequest(a, r); err != nil {
		return out, err
	}
	if !time.Now().Before(a.Deadline) {
		return out, context.DeadlineExceeded
	}
	if r.Stream && emit == nil {
		return out, ErrInvalidRequest
	}
	if !member(b.config.Providers, a.Provider) {
		return out, ErrUnsupported
	}
	if err = b.guardResources(ctx, r.Input); err != nil {
		return out, err
	}
	if err = b.guardResources(ctx, r.Params); err != nil {
		return out, err
	}
	if a.Capability.Billing != nil {
		if r.InputFacts == nil {
			r.InputFacts = b.inputFacts(ctx, r, *a.Capability.Billing)
		}
		out.InputFacts = r.InputFacts
		if r.Operation != VideoRetrieve && r.Operation != VideoDownload {
			for _, item := range a.Capability.Billing.Input {
				if item.Template == "tokens" || r.Operation == Transcription && item.Modality == "audio" && item.Template == "seconds" {
					continue
				}
				for _, component := range item.Components("input") {
					if _, known := r.InputFacts[component.Key]; !known {
						out.State = "not_dispatched"
						out.ErrorCode = "input_quantity_unavailable"
						out.Cost = &CostObservation{Source: "not_dispatched"}
						return out, ErrUsageUnknown
					}
				}
			}
		}
	}
	credential, err := b.config.Credentials.ResolveCredential(ctx, a.UpstreamID, a.CredentialID)
	if err != nil || credential.ID != a.CredentialID || strings.TrimSpace(credential.Secret) == "" {
		return out, fmt.Errorf("selected platform credential unavailable")
	}
	ctx, cancel := context.WithDeadline(ctx, a.Deadline)
	defer cancel()
	bc := schemas.NewBifrostContext(ctx, schemas.NoDeadline)
	if r.Operation == Video || r.Operation == VideoRetrieve || a.Provider == "openrouter" || (r.Operation == Chat && r.Stream) {
		bc.SetValue(schemas.BifrostContextKeyAllowPerRequestRawOverride, true)
		bc.SetValue(schemas.BifrostContextKeySendBackRawResponse, true)
	}
	bc.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{ID: credential.ID, Name: credential.ID, Value: *schemas.NewSecretVar(credential.Secret), Models: schemas.WhiteList{a.UpstreamModel}, Weight: 1})
	if a.BaseURL != "" {
		u, e := url.Parse(a.BaseURL)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && u.Scheme != "http") || b.config.EndpointGuard == nil {
			return out, ErrInvalidRequest
		}
		if e = b.config.EndpointGuard(ctx, a.BaseURL); e != nil {
			return out, e
		}
		// BaseURL is the exact endpoint URL selected by the verified route.
		bc.SetValue(schemas.BifrostContextKeyURLPath, a.BaseURL)
	}
	provider := schemas.ModelProvider(a.Provider)
	var response any
	var stream chan *schemas.BifrostStreamChunk
	var failure *schemas.BifrostError
	switch r.Operation {
	case Chat:
		q := schemas.BifrostChatRequest{Provider: provider, Model: a.UpstreamModel, Params: &schemas.ChatParameters{}}
		if err = decodePreserved(r.Input, &q.Input); err != nil {
			return out, err
		}
		if len(q.Input) == 0 {
			return out, ErrInvalidRequest
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		if r.Stream {
			out.State = "failed"
			out.ErrorCode = ""
			stream, failure = b.client.ChatCompletionStreamRequest(bc, &q)
		} else {
			out.State = "failed"
			out.ErrorCode = ""
			response, failure = b.client.ChatCompletionRequest(bc, &q)
		}
	case Responses:
		q := schemas.BifrostResponsesRequest{Provider: provider, Model: a.UpstreamModel, Params: &schemas.ResponsesParameters{}}
		if err = decodePreserved(r.Input, &q.Input); err != nil {
			return out, err
		}
		if len(q.Input) == 0 {
			return out, ErrInvalidRequest
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		if r.Stream {
			out.State = "failed"
			out.ErrorCode = ""
			stream, failure = b.client.ResponsesStreamRequest(bc, &q)
		} else {
			out.State = "failed"
			out.ErrorCode = ""
			response, failure = b.client.ResponsesRequest(bc, &q)
		}
	case Embedding:
		if r.Stream {
			return out, ErrUnsupported
		}
		q := schemas.BifrostEmbeddingRequest{Provider: provider, Model: a.UpstreamModel, Params: &schemas.EmbeddingParameters{}}
		if q.Input, err = embeddingInputs(r.Input); err != nil {
			return out, err
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		out.State = "failed"
		out.ErrorCode = ""
		response, failure = b.client.EmbeddingRequest(bc, &q)
	case Image:
		q := schemas.BifrostImageGenerationRequest{Provider: provider, Model: a.UpstreamModel, Input: &schemas.ImageGenerationInput{}, Params: &schemas.ImageGenerationParameters{}}
		if err = decodePreserved(r.Input, q.Input); err != nil {
			return out, err
		}
		if strings.TrimSpace(q.Input.Prompt) == "" {
			return out, ErrInvalidRequest
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		if r.Stream {
			out.State = "failed"
			out.ErrorCode = ""
			stream, failure = b.client.ImageGenerationStreamRequest(bc, &q)
		} else {
			out.State = "failed"
			out.ErrorCode = ""
			response, failure = b.client.ImageGenerationRequest(bc, &q)
		}
	case Speech:
		q := schemas.BifrostSpeechRequest{Provider: provider, Model: a.UpstreamModel, Input: &schemas.SpeechInput{}, Params: &schemas.SpeechParameters{}}
		if err = decodePreserved(r.Input, q.Input); err != nil {
			return out, err
		}
		if q.Input.Input == "" {
			return out, ErrInvalidRequest
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		if r.Stream {
			out.State = "failed"
			out.ErrorCode = ""
			stream, failure = b.client.SpeechStreamRequest(bc, &q)
		} else {
			out.State = "failed"
			out.ErrorCode = ""
			response, failure = b.client.SpeechRequest(bc, &q)
		}
	case Transcription:
		q := schemas.BifrostTranscriptionRequest{Provider: provider, Model: a.UpstreamModel, Input: &schemas.TranscriptionInput{}, Params: &schemas.TranscriptionParameters{}}
		if err = decodePreserved(r.Input, q.Input); err != nil {
			return out, err
		}
		if len(q.Input.File) == 0 {
			return out, ErrInvalidRequest
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		if r.Stream {
			out.State = "failed"
			out.ErrorCode = ""
			stream, failure = b.client.TranscriptionStreamRequest(bc, &q)
		} else {
			out.State = "failed"
			out.ErrorCode = ""
			response, failure = b.client.TranscriptionRequest(bc, &q)
		}
	case Video:
		if r.Stream {
			return out, ErrUnsupported
		}
		q := schemas.BifrostVideoGenerationRequest{Provider: provider, Model: a.UpstreamModel, Input: &schemas.VideoGenerationInput{}, Params: &schemas.VideoGenerationParameters{}}
		if err = decodePreserved(r.Input, q.Input); err != nil {
			return out, err
		}
		if q.Input.Prompt == "" {
			return out, ErrInvalidRequest
		}
		if err = decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		out.State = "failed"
		out.ErrorCode = ""
		response, failure = b.client.VideoGenerationRequest(bc, &q)
	case VideoRetrieve:
		if r.Stream || a.TaskID == "" {
			return out, ErrInvalidRequest
		}
		out.State = "failed"
		out.ErrorCode = ""
		response, failure = b.client.VideoRetrieveRequest(bc, &schemas.BifrostVideoRetrieveRequest{Provider: provider, ID: a.TaskID})
	case VideoDownload:
		if r.Stream || a.TaskID == "" {
			return out, ErrInvalidRequest
		}
		out.State = "failed"
		out.ErrorCode = ""
		response, failure = b.client.VideoDownloadRequest(bc, &schemas.BifrostVideoDownloadRequest{Provider: provider, ID: a.TaskID})
	default:
		return out, ErrUnsupported
	}
	if failure != nil {
		return executionFailure(out, failure)
	}
	if r.Stream {
		if stream == nil {
			out.State = "outcome_unknown"
			return out, fmt.Errorf("empty upstream stream")
		}
		measurements := billingStream{}
		for {
			select {
			case <-ctx.Done():
				out.State = "outcome_unknown"
				return out, ctx.Err()
			case chunk, ok := <-stream:
				if !ok {
					out.State = "completed"
					if !out.Usage.Reliable {
						return out, ErrUsageUnknown
					}
					return out, nil
				}
				if chunk == nil {
					continue
				}
				if chunk.BifrostError != nil {
					return executionFailure(out, chunk.BifrostError)
				}
				if err = verifyRouting(chunk, a); err != nil {
					out.State = "outcome_unknown"
					out.ErrorCode = "route_contract_violation"
					return out, err
				}
				body, e := publicOutput(chunk, r.PublicModel)
				if e != nil {
					return out, e
				}
				usage := measurements.measure(ctx, b, chunk, a.Capability, r)
				if b.config.Costs != nil {
					observation := measurements.cost(ctx, b.config.Costs, a, chunk)
					if observation != nil && (observation.ReportedCost != nil || observation.ReferenceCost != nil || chunk.BifrostImageGenerationStreamResponse != nil) {
						out.Cost = observation
					}
				}
				if usage.Reliable {
					out.Usage = usage
				}
				event := Event{Data: body}
				if usage.Reliable {
					event.Usage = &usage
				}
				if e = emit(ctx, event); e != nil {
					out.State = "outcome_unknown"
					return out, e
				}
				out.OutputDelivered = true
			}
		}
	}
	if err = verifyRouting(response, a); err != nil {
		out.State = "outcome_unknown"
		out.ErrorCode = "route_contract_violation"
		return out, err
	}
	out.Output, err = publicOutput(response, r.PublicModel)
	if err != nil {
		out.State = "outcome_unknown"
		return out, err
	}
	measured := b.measurableResponse(ctx, response, a.Capability)
	out.Usage = extractBilledUsage(measured, a.Capability, r)
	if b.config.Costs != nil {
		costResponse := measured
		if _, ok := measured.(map[string]any); ok {
			costResponse = response
		}
		out.Cost = b.config.Costs.Observe(ctx, a, costResponse)
	}
	out.State = "completed"
	if speech, ok := response.(*schemas.BifrostSpeechResponse); ok && speech != nil {
		out.Binary = speech.Audio
		out.ContentType = "audio/mpeg"
		var p struct {
			Format string `json:"response_format"`
		}
		_ = json.Unmarshal(r.Params, &p)
		switch p.Format {
		case "wav":
			out.ContentType = "audio/wav"
		case "flac":
			out.ContentType = "audio/flac"
		case "opus":
			out.ContentType = "audio/opus"
		case "aac":
			out.ContentType = "audio/aac"
		case "pcm":
			out.ContentType = "audio/pcm"
		}
	}
	if download, ok := response.(*schemas.BifrostVideoDownloadResponse); ok && download != nil {
		out.Binary = download.Content
		out.ContentType = download.ContentType
		return out, nil
	}
	if video, ok := response.(*schemas.BifrostVideoGenerationResponse); ok && video != nil {
		out.TaskID = NativeVideoTaskID(video.ID, a.Provider)
		if video.Status != "completed" {
			out.State = "submitted"
			if video.Status == "failed" {
				out.State = "failed"
			}
			return out, nil
		}
	}
	if !out.Usage.Reliable {
		return out, ErrUsageUnknown
	}
	return out, nil
}

// NativeVideoTaskID removes the SDK's provider suffix/URL encoding. Persist this
// native ID together with the pinned upstream and credential for lifecycle calls.
func NativeVideoTaskID(id, provider string) string {
	raw := strings.TrimSuffix(id, ":"+provider)
	decoded, e := url.PathUnescape(raw)
	if e == nil {
		return decoded
	}
	return raw
}
func executionFailure(out Result, f *schemas.BifrostError) (Result, error) {
	status, detail := supplierFailureDetail(f)
	out.ProviderStatus = status
	out.ProviderError = detail
	out.ErrorCode = "upstream_error"
	if status == 429 {
		out.ErrorCode = "rate_limited"
	}
	if status == 0 || status == 408 || status >= 500 {
		out.State = "outcome_unknown"
	} else {
		out.State = "failed"
	}
	return out, &UpstreamFailure{Status: status, Detail: detail}
}

var providerSecretPattern = regexp.MustCompile(`(?i)bearer\s+\S+|sk-[A-Za-z0-9_\-.]{8,}|"[^"]*(?:api[_-]?key|authorization|secret)[^"]*"\s*:\s*"[^"]*"`)

// Diagnostic is the failure text stored on a request. Supplier details are
// kept; credential material is removed.
func Diagnostic(err error) string {
	if err == nil {
		return ""
	}
	var upstream *UpstreamFailure
	if errors.As(err, &upstream) {
		return upstream.Error()
	}
	return boundProviderDetail(redactProviderDetail(err.Error()))
}

func supplierFailureDetail(f *schemas.BifrostError) (int, string) {
	if f == nil {
		return 0, ""
	}
	status := 0
	if f.StatusCode != nil {
		status = *f.StatusCode
	}
	message, kind, code := "", "", ""
	if f.Error != nil {
		message = f.Error.Message
		if f.Error.Type != nil {
			kind = *f.Error.Type
		}
		if f.Error.Code != nil {
			code = *f.Error.Code
		}
	}
	body := strings.TrimSpace(string(f.ExtraFields.NativeErrorResponse))
	if body == "" && f.ExtraFields.RawResponse != nil {
		switch value := f.ExtraFields.RawResponse.(type) {
		case string:
			body = value
		case []byte:
			body = string(value)
		case json.RawMessage:
			body = string(value)
		default:
			encoded, err := json.Marshal(value)
			if err == nil {
				body = string(encoded)
			}
		}
	}
	detail := strings.TrimSpace(body)
	if detail == "" {
		parts := []string{}
		if status != 0 {
			parts = append(parts, fmt.Sprintf("HTTP %d", status))
		}
		if kind != "" {
			parts = append(parts, kind)
		}
		if code != "" {
			parts = append(parts, code)
		}
		if message != "" {
			parts = append(parts, message)
		}
		detail = strings.Join(parts, " ")
	} else if status != 0 {
		detail = fmt.Sprintf("HTTP %d %s", status, detail)
	}
	return status, boundProviderDetail(redactProviderDetail(detail))
}

func redactProviderDetail(value string) string {
	return providerSecretPattern.ReplaceAllString(value, "[redacted]")
}

func boundProviderDetail(value string) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "")
	if len(value) <= 4000 {
		return value
	}
	return value[:4000] + "…"
}
func decodePreserved(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	if e := json.Unmarshal(raw, dst); e != nil {
		return fmt.Errorf("%w: invalid SDK schema", ErrInvalidRequest)
	}
	encoded, e := json.Marshal(dst)
	if e != nil {
		return e
	}
	var before, after any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if e = d.Decode(&before); e != nil {
		return e
	}
	d = json.NewDecoder(bytes.NewReader(encoded))
	d.UseNumber()
	if e = d.Decode(&after); e != nil {
		return e
	}
	if !preserved(before, after) {
		return fmt.Errorf("%w: SDK would change or drop an input field", ErrUnsupported)
	}
	return nil
}
func preserved(before, after any) bool {
	switch b := before.(type) {
	case map[string]any:
		a, ok := after.(map[string]any)
		if !ok {
			return false
		}
		for k, v := range b {
			av, ok := a[k]
			if !ok || !preserved(v, av) {
				return false
			}
		}
		return true
	case []any:
		a, ok := after.([]any)
		if !ok || len(a) != len(b) {
			return false
		}
		for i := range b {
			if !preserved(b[i], a[i]) {
				return false
			}
		}
		return true
	case json.Number:
		a, ok := after.(json.Number)
		if !ok {
			return false
		}
		br, bok := new(big.Rat).SetString(string(b))
		ar, aok := new(big.Rat).SetString(string(a))
		return bok && aok && br.Cmp(ar) == 0
	case string:
		a, ok := after.(string)
		return ok && b == a
	case bool:
		a, ok := after.(bool)
		return ok && b == a
	case nil:
		return after == nil
	default:
		return false
	}
}
func publicOutput(response any, publicModel string) (json.RawMessage, error) {
	raw, e := json.Marshal(response)
	if e != nil {
		return nil, e
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, fmt.Errorf("empty upstream response")
	}
	delete(object, "extra_fields")
	delete(object, "prompt")
	if _, ok := object["model"]; ok {
		object["model"], _ = json.Marshal(publicModel)
	}
	if nested, ok := object["response"]; ok && string(nested) != "null" {
		var child map[string]json.RawMessage
		if json.Unmarshal(nested, &child) == nil && child != nil {
			delete(child, "extra_fields")
			if _, ok := child["model"]; ok {
				child["model"], _ = json.Marshal(publicModel)
			}
			object["response"], _ = json.Marshal(child)
		}
	}
	return json.Marshal(object)
}
func verifyRouting(response any, a Attempt) error {
	raw, e := json.Marshal(response)
	if e != nil {
		return e
	}
	var body struct {
		Extra schemas.BifrostResponseExtraFields `json:"extra_fields"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return ErrInvalidRequest
	}
	r := body.Extra.RoutingInfo
	if r.IsFallback || (r.Provider != "" && string(r.Provider) != a.Provider) || (r.Model != "" && r.Model != a.UpstreamModel) || (r.Key != "" && r.Key != a.CredentialID) || r.ResolvedKeyAlias != nil || (r.ServerSideFallbackModel != nil && *r.ServerSideFallbackModel != a.UpstreamModel) || body.Extra.ConvertedRequestType != "" || len(body.Extra.DroppedCompatPluginParams) > 0 || len(body.Extra.DroppedUnsupportedTools) > 0 {
		return fmt.Errorf("upstream violated verified route or parameter contract")
	}
	return nil
}
func (b *Bifrost) guardResources(ctx context.Context, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var root any
	if json.Unmarshal(raw, &root) != nil {
		return ErrInvalidRequest
	}
	var walk func(any, string) error
	walk = func(v any, key string) error {
		switch value := v.(type) {
		case map[string]any:
			for k, item := range value {
				if e := walk(item, k); e != nil {
					return e
				}
			}
		case []any:
			for _, item := range value {
				if e := walk(item, key); e != nil {
					return e
				}
			}
		case string:
			if (key == "url" || key == "image_url" || key == "video_url" || key == "input_reference" || key == "video_uri" || key == "input_images") && (strings.HasPrefix(value, "http:") || strings.HasPrefix(value, "https:")) {
				u, e := url.Parse(value)
				if e != nil || u.Scheme != "https" || u.User != nil || b.config.ResourceGuard == nil {
					return fmt.Errorf("resource URL is not permitted")
				}
				if e = b.config.ResourceGuard(ctx, value); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return walk(root, "")
}
