package modelpricing

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/modelcatalog/datasheet"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func (s *Service) Observe(ctx context.Context, a modelruntime.Attempt, response any) *modelruntime.CostObservation {
	out := &modelruntime.CostObservation{Source: "unavailable"}
	var snapshot Snapshot
	if json.Unmarshal(a.CostSnapshot, &snapshot) == nil && len(snapshot.Rows) > 0 {
		out.Version = snapshot.Version
		if store, e := rowStore(ctx, snapshot); e == nil {
			provider := schemas.ModelProvider(snapshot.Rows[0].Provider)
			store.SetIgnoreProviderCost(provider, true)
			wrapped := costResponse(response)
			if wrapped != nil && wrapped.GetExtraFields() != nil {
				extra := wrapped.GetExtraFields()
				extra.RoutingInfo = schemas.RoutingInfo{Provider: provider, Model: snapshot.Rows[0].Model}
				if video := wrapped.VideoGenerationResponse; video != nil {
					seconds := modelruntime.KnownVideoSeconds(response)
					if seconds == nil || !seconds.IsInt() {
						return out
					}
					// Retrieval is settlement of the original generation, never a new cost.
					value := seconds.Num().String()
					video.Seconds = &value
					video.Videos = nil
					extra.RequestType = schemas.VideoGenerationRequest
					extra.PricingRequestType = ""
				}
				if cost := store.CalculateCostBreakdown(wrapped, &datasheet.LookupScopes{Provider: string(provider), BilledAt: snapshot.AcceptedAt}); cost != nil {
					if amount := nonnegative(cost.TotalCost); amount != nil {
						out.ReferenceCost = &modelruntime.Money{Currency: "USD", Amount: *amount}
					}
				}
			}
		}
	}
	// Only an adapter whose documented amount is USD can report USD here.
	// A generic compatible provider's field named `cost` is not sufficient.
	if a.Provider == "openrouter" {
		if amount := reportedMoney(response); amount != nil {
			out.ReportedCost = &modelruntime.Money{Currency: "USD", Amount: *amount}
		}
	}
	if out.ReportedCost != nil {
		out.Source = "provider_reported"
	} else if out.ReferenceCost != nil {
		out.Source = "bifrost_calculated"
	}
	return out
}

// Clone response envelopes before setting the pinned cost identity. Supplier
// aliases and prices must never change the user's public response identity.
func costResponse(response any) *schemas.BifrostResponse {
	switch v := response.(type) {
	case *schemas.BifrostChatResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{ChatResponse: &c}
		}
	case *schemas.BifrostResponsesResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{ResponsesResponse: &c}
		}
	case *schemas.BifrostEmbeddingResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{EmbeddingResponse: &c}
		}
	case *schemas.BifrostImageGenerationResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{ImageGenerationResponse: &c}
		}
	case *schemas.BifrostSpeechResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{SpeechResponse: &c}
		}
	case *schemas.BifrostTranscriptionResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{TranscriptionResponse: &c}
		}
	case *schemas.BifrostVideoGenerationResponse:
		if v != nil {
			c := *v
			return &schemas.BifrostResponse{VideoGenerationResponse: &c}
		}
	case *schemas.BifrostStreamChunk:
		if v != nil {
			if v.BifrostSpeechStreamResponse != nil {
				c := *v.BifrostSpeechStreamResponse
				if c.Type != schemas.SpeechStreamResponseTypeDone {
					return nil
				}
				return &schemas.BifrostResponse{SpeechStreamResponse: &c}
			}
			if v.BifrostTranscriptionStreamResponse != nil {
				c := *v.BifrostTranscriptionStreamResponse
				return &schemas.BifrostResponse{TranscriptionStreamResponse: &c}
			}
			if v.BifrostImageGenerationStreamResponse != nil {
				c := *v.BifrostImageGenerationStreamResponse
				if c.Type != schemas.ImageGenerationEventTypeCompleted || c.URL == "" && c.B64JSON == "" {
					return nil
				}
				// The SDK stream calculator does not infer an image count. A
				// completed indexed event represents one verified output.
				return &schemas.BifrostResponse{ImageGenerationResponse: &schemas.BifrostImageGenerationResponse{
					Data: []schemas.ImageData{{B64JSON: c.B64JSON, URL: c.URL}}, Usage: c.Usage,
					ImageGenerationResponseParameters: &schemas.ImageGenerationResponseParameters{Size: c.Size, Quality: c.Quality},
					ExtraFields:                       c.ExtraFields,
				}}
			}
			if v.BifrostChatResponse != nil {
				if v.BifrostChatResponse.Usage == nil || v.BifrostChatResponse.Usage.TotalTokens == 0 {
					return nil
				}
				return costResponse(v.BifrostChatResponse)
			}
			if v.BifrostResponsesStreamResponse != nil {
				c := *v.BifrostResponsesStreamResponse
				return &schemas.BifrostResponse{ResponsesStreamResponse: &c}
			}
		}
	}
	return nil
}
func reportedMoney(response any) *string {
	raw, e := json.Marshal(response)
	if e != nil {
		return nil
	}
	var envelope struct {
		Extra struct {
			Raw json.RawMessage `json:"raw_response"`
		} `json:"extra_fields"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Extra.Raw) == 0 {
		return nil
	}
	native := envelope.Extra.Raw
	var str string
	if json.Unmarshal(native, &str) == nil {
		native = []byte(str)
	}
	var body struct {
		Usage struct {
			Cost *json.Number `json:"cost"`
		} `json:"usage"`
	}
	decoder := json.NewDecoder(bytes.NewReader(native))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil || body.Usage.Cost == nil {
		return nil
	}
	value := body.Usage.Cost.String()
	amount, e := ledger.ParseDecimal(value)
	if e != nil || amount.Sign() < 0 {
		return nil
	}
	return &value
}
