package modelruntime

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

// ValidateSubscriptionRequest preserves the public request contract on a
// subscription route. The native Codex endpoint does not accept the public
// API's requested output cap. Its verified model limit is reserved separately.
func ValidateSubscriptionRequest(kind string, outputLimit int64, r Request) error {
	if outputLimit <= 0 || string(r.Operation) != subscriptionauth.Operation(kind) {
		return ErrUnsupported
	}
	for _, input := range r.InputTypes {
		if input != "text" {
			return ErrUnsupported
		}
	}
	params := map[string]json.RawMessage{}
	if len(r.Params) > 0 && json.Unmarshal(r.Params, &params) != nil {
		return ErrInvalidRequest
	}
	switch kind {
	case subscriptionauth.Codex:
		for _, name := range []string{"max_output_tokens", "max_completion_tokens", "max_tool_calls", "temperature", "top_p",
			"background", "conversation", "metadata", "prompt", "prompt_cache_retention", "safety_identifier", "truncation", "user", "top_logprobs"} {
			if _, supplied := params[name]; supplied {
				return ErrUnsupported
			}
		}
	case subscriptionauth.Claude, subscriptionauth.Gemini, subscriptionauth.Antigravity:
		if n, supplied := params["n"]; supplied && string(n) != "1" {
			return ErrUnsupported
		}
		if !withinSubscriptionLimit(params["max_completion_tokens"], outputLimit) {
			return ErrUnsupported
		}
	case subscriptionauth.Grok:
		if !withinSubscriptionLimit(params["max_output_tokens"], outputLimit) {
			return ErrUnsupported
		}
	default:
		return ErrUnsupported
	}
	return nil
}

func withinSubscriptionLimit(raw json.RawMessage, limit int64) bool {
	var n int64
	return len(raw) == 0 || json.Unmarshal(raw, &n) == nil && n >= 1 && n <= limit
}

func subscriptionRetryAt(bc *schemas.BifrostContext) *time.Time {
	headers, _ := bc.Value(schemas.BifrostContextKeyProviderResponseHeaders).(map[string]string)
	for name, value := range headers {
		if !strings.EqualFold(name, "Retry-After") {
			continue
		}
		value = strings.TrimSpace(value)
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 && seconds <= math.MaxInt64/int64(time.Second) {
			at := time.Now().Add(time.Duration(seconds) * time.Second)
			return &at
		}
		if at, err := http.ParseTime(value); err == nil && at.After(time.Now()) {
			return &at
		}
	}
	return nil
}

func (b *Bifrost) subscriptionContext(ctx context.Context, bc *schemas.BifrostContext, a Attempt) error {
	if b.config.Subscriptions == nil || a.SubscriptionAccountID == "" || a.Provider != subscriptionauth.Protocol(a.SubscriptionKind) || a.BaseURL != "" {
		return ErrSubscriptionAuthUnavailable
	}
	credential, err := b.config.Subscriptions.ResolveSubscriptionCredential(ctx, a)
	if err != nil {
		return err
	}
	if credential.AccessToken == "" {
		return ErrSubscriptionReauth
	}
	endpoint := credential.Endpoint
	if endpoint == "" {
		switch a.SubscriptionKind {
		case subscriptionauth.Codex:
			endpoint = "https://chatgpt.com/backend-api/codex/responses"
		case subscriptionauth.Claude:
			endpoint = "https://api.anthropic.com/v1/messages"
		default:
			return ErrSubscriptionAuthUnavailable
		}
		if testEndpoint := b.config.subscriptionTestEndpoints[a.SubscriptionKind]; testEndpoint != "" && b.config.AllowTestLoopback {
			endpoint = testEndpoint
		}
	}
	if b.config.EndpointGuard == nil || b.config.EndpointGuard(ctx, endpoint) != nil {
		return ErrSubscriptionAuthUnavailable
	}
	bc.SetValue(schemas.BifrostContextKeyURLPath, endpoint)
	if a.SubscriptionKind == subscriptionauth.Claude {
		bc.SetValue(schemas.BifrostContextKeySkipKeySelection, true)
		headers := map[string][]string{"Authorization": {"Bearer " + credential.AccessToken}}
		if credential.Endpoint == "" {
			headers["anthropic-beta"] = []string{"oauth-2025-04-20"}
		}
		bc.SetValue(schemas.BifrostContextKeyPassthroughHeaders, headers)
		return nil
	}
	bc.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{ID: a.CredentialID, Name: a.CredentialID, Value: *schemas.NewSecretVar(credential.AccessToken), Models: schemas.WhiteList{a.UpstreamModel}, Weight: 1})
	if a.SubscriptionKind == subscriptionauth.Codex && credential.Endpoint == "" {
		if credential.AccountID == "" {
			return ErrSubscriptionReauth
		}
		bc.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{
			"ChatGPT-Account-Id": {credential.AccountID},
			"OpenAI-Beta":        {"responses=experimental"},
		})
	}
	return nil
}

func finishSubscriptionResponse(ctx context.Context, stream <-chan *schemas.BifrostStreamChunk, a Attempt) (*schemas.BifrostResponsesResponse, *schemas.BifrostError, error) {
	if stream == nil {
		return nil, nil, ErrUsageUnknown
	}
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case chunk, ok := <-stream:
			if !ok {
				return nil, nil, ErrUsageUnknown
			}
			if chunk == nil {
				continue
			}
			if chunk.BifrostError != nil {
				return nil, chunk.BifrostError, nil
			}
			if err := verifyRouting(chunk, a); err != nil {
				return nil, nil, err
			}
			event := chunk.BifrostResponsesStreamResponse
			if event != nil && event.Response != nil && (event.Type == schemas.ResponsesStreamResponseTypeCompleted || event.Type == schemas.ResponsesStreamResponseTypeIncomplete) {
				return event.Response, nil, nil
			}
		}
	}
}
