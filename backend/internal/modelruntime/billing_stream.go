package modelruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	"math/big"
	"strconv"
	"unicode/utf8"
)

type billingStream struct {
	images           map[int]schemas.ImageData
	audio            []byte
	audioOverflow    bool
	imageCosts       map[int]*CostObservation
	textCharacters   int64
	outputModalities map[string]bool
}

func (s *billingStream) measure(ctx context.Context, b *Bifrost, chunk *schemas.BifrostStreamChunk, c Capability, r Request) Usage {
	if c.Billing == nil {
		return extractUsage(chunk, c)
	}
	if r.Operation == Chat || r.Operation == Responses {
		encoded, _ := json.Marshal(chunk)
		var body map[string]any
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		_ = decoder.Decode(&body)
		if r.Operation == Chat {
			choices, _ := body["choices"].([]any)
			for _, raw := range choices {
				choice, _ := raw.(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				text, _ := delta["content"].(string)
				s.textCharacters += int64(utf8.RuneCountInString(text))
			}
		}
		if r.Operation == Responses && body["type"] == "response.output_text.delta" {
			text, _ := body["delta"].(string)
			s.textCharacters += int64(utf8.RuneCountInString(text))
		}
		if s.outputModalities == nil {
			s.outputModalities = map[string]bool{}
		}
		for _, kind := range outputKinds(body) {
			s.outputModalities[kind] = true
		}
		if choices, ok := body["choices"].([]any); ok {
			for _, raw := range choices {
				choice, _ := raw.(map[string]any)
				delta, _ := choice["delta"].(map[string]any)
				if delta["audio"] != nil {
					s.outputModalities["audio"] = true
				}
			}
		}
		kinds := []any{}
		for _, kind := range []string{"text", "image", "audio", "video"} {
			if s.outputModalities[kind] {
				kinds = append(kinds, kind)
			}
		}
		body["_meter_output_kinds"] = kinds
		if c.Billing.Has("output", "text", "characters") && s.textCharacters == 0 {
			return Usage{Source: "unknown"}
		}
		tokens := readTokenFacts(chunk, c, body)
		body["_meter_token_evidence"] = tokens.input != nil
		if tokens.input != nil {
			body["_meter_token_input"] = tokens.input.FloatString(24)
		}
		if tokens.output != nil {
			body["_meter_token_output"] = tokens.output.FloatString(24)
		}
		if tokens.cache != nil {
			body["_meter_token_cache"] = tokens.cache.FloatString(24)
		}
		body["_measured_output_characters"] = json.Number(strconv.FormatInt(s.textCharacters, 10))
		return extractBilledUsage(body, c, r)
	}
	if image := chunk.BifrostImageGenerationStreamResponse; image != nil && !c.Billing.Has("output", "image", "tokens") {
		if image.Type != schemas.ImageGenerationEventTypeCompleted {
			return Usage{Source: "unknown"}
		}
		if s.images == nil {
			s.images = map[int]schemas.ImageData{}
		}
		s.images[image.Index] = schemas.ImageData{B64JSON: image.B64JSON, URL: image.URL}
		images := make([]schemas.ImageData, 0, len(s.images))
		for _, image := range s.images {
			images = append(images, image)
		}
		response := &schemas.BifrostImageGenerationResponse{Data: images, Usage: image.Usage}
		return extractBilledUsage(b.measurableResponse(ctx, response, c), c, r)
	}
	if speech := chunk.BifrostSpeechStreamResponse; speech != nil {
		if !s.audioOverflow && len(s.audio)+len(speech.Audio) <= 32<<20 {
			s.audio = append(s.audio, speech.Audio...)
		} else {
			s.audio = nil
			s.audioOverflow = true
		}
		if speech.Type != schemas.SpeechStreamResponseTypeDone {
			return Usage{Source: "unknown"}
		}
		return extractBilledUsage(&schemas.BifrostSpeechResponse{Audio: s.audio, Usage: speech.Usage}, c, r)
	}
	return extractBilledUsage(chunk, c, r)
}

// Completed image events are indexed, rather than cumulative usage snapshots.
// Replayed events replace the same index and never add a second charge.
func (s *billingStream) cost(ctx context.Context, meter CostMeter, a Attempt, chunk *schemas.BifrostStreamChunk) *CostObservation {
	observation := meter.Observe(ctx, a, chunk)
	image := chunk.BifrostImageGenerationStreamResponse
	if image == nil || image.Type != schemas.ImageGenerationEventTypeCompleted {
		return observation
	}
	if s.imageCosts == nil {
		s.imageCosts = map[int]*CostObservation{}
	}
	s.imageCosts[image.Index] = observation
	total := &CostObservation{Source: "unavailable"}
	sum := func(field func(*CostObservation) *string) *string {
		value := new(big.Rat)
		for _, item := range s.imageCosts {
			if item == nil || field(item) == nil {
				return nil
			}
			n, ok := new(big.Rat).SetString(*field(item))
			if !ok || n.Sign() < 0 {
				return nil
			}
			value.Add(value, n)
		}
		text := value.FloatString(24)
		return &text
	}
	total.AmountUSD = sum(func(v *CostObservation) *string { return v.AmountUSD })
	total.CalculatedUSD = sum(func(v *CostObservation) *string { return v.CalculatedUSD })
	total.ReportedUSD = sum(func(v *CostObservation) *string { return v.ReportedUSD })
	if observation != nil {
		total.Version = observation.Version
		if total.AmountUSD != nil {
			total.Source = observation.Source
		}
	}
	return total
}
