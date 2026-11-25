package modelruntime

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/maximhq/bifrost/core/providers/gemini"
	"github.com/maximhq/bifrost/core/schemas"
)

// The SDK's Gemini transport joins paths to its own API-key origin. Reuse its
// generateContent conversions for the approved OAuth endpoint instead.
func (b *Bifrost) geminiMedia(ctx context.Context, a Attempt, r Request, credential SubscriptionCredential, out Result) (Result, error) {
	model := strings.TrimPrefix(a.UpstreamModel, "google/")
	if r.Stream {
		return out, ErrUnsupported
	}
	var input *gemini.GeminiGenerationRequest
	switch r.Operation {
	case Image:
		q := schemas.BifrostImageGenerationRequest{Provider: schemas.Gemini, Model: model, Input: &schemas.ImageGenerationInput{}, Params: &schemas.ImageGenerationParameters{}}
		if err := decodePreserved(r.Input, q.Input); err != nil {
			return out, err
		}
		if strings.TrimSpace(q.Input.Prompt) == "" {
			return out, ErrInvalidRequest
		}
		if err := decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		input = gemini.ToGeminiImageGenerationRequest(&q)
	case ImageEdit:
		q := schemas.BifrostImageEditRequest{Provider: schemas.Gemini, Model: model, Input: &schemas.ImageEditInput{}, Params: &schemas.ImageEditParameters{}}
		if err := decodePreserved(r.Input, q.Input); err != nil {
			return out, err
		}
		if err := decodePreserved(r.Params, q.Params); err != nil {
			return out, err
		}
		if len(q.Params.Mask) > 0 {
			return out, ErrUnsupported
		}
		if err := b.loadEditImages(ctx, q.Input); err != nil {
			return out, err
		}
		input = gemini.ToGeminiImageEditRequest(&q)
	default:
		return out, ErrUnsupported
	}
	if input == nil {
		return out, ErrUnsupported
	}
	body, err := json.Marshal(input)
	if err != nil {
		return out, ErrInvalidRequest
	}
	raw, _, out, err := b.requestSubscriptionMedia(ctx, credential, http.MethodPost, body, out)
	if err != nil {
		return out, err
	}
	var data gemini.GenerateContentResponse
	if json.Unmarshal(raw, &data) != nil {
		return out, &UpstreamFailure{}
	}
	response, failure := data.ToBifrostImageGenerationResponse()
	if failure != nil || response == nil || len(response.Data) == 0 {
		return out, &UpstreamFailure{}
	}
	return b.finishResponse(ctx, response, a, r, out)
}
