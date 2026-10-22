package modelruntime

import (
	"encoding/json"
	"math/big"
)

type tokenFacts struct {
	input, output, cache        *big.Rat
	inputDetails, outputDetails map[string]any
}

func readTokenFacts(response any, c Capability, body map[string]any) tokenFacts {
	op := c.Operation
	input, output, cache := "/prompt_tokens", "/completion_tokens", "/prompt_tokens_details/cached_read_tokens"
	if op != Chat && op != Embedding {
		input, output, cache = "/input_tokens", "/output_tokens", "/input_tokens_details/cached_tokens"
	}
	capability := Capability{Operation: op, UsageRules: []UsageRule{{Component: "input", Unit: "token", Path: input}, {Component: "cache", Unit: "token", Path: cache, ZeroWhenAbsent: true}}}
	if op != Embedding {
		capability.UsageRules = append(capability.UsageRules, UsageRule{Component: "output", Unit: "token", Path: output})
	}
	if op != Chat && op != Embedding && op != Responses {
		fields, _ := body["usage"].(map[string]any)
		if total, ok := numeric(fields["total_tokens"]); !ok || total.Sign() == 0 {
			native := nativeBody(body)
			if reported, ok := native["usage"].(map[string]any); ok {
				fields = reported
			}
		}
		total, ok := numeric(fields["total_tokens"])
		if !ok || total.Sign() == 0 {
			return tokenFacts{}
		}
		normalized := map[string]any{}
		for key, value := range fields {
			normalized[key] = value
		}
		for _, key := range []string{"input_tokens", "output_tokens"} {
			if _, exists := normalized[key]; !exists {
				normalized[key] = json.Number("0")
			}
		}
		copy := map[string]any{}
		for key, value := range body {
			copy[key] = value
		}
		copy["usage"] = normalized
		body = copy
		response = copy
	}
	if c.Streaming && op == Chat {
		usageBody, _ := body["usage"].(map[string]any)
		if total, ok := numeric(usageBody["total_tokens"]); !ok || total.Sign() == 0 {
			native := nativeBody(body)
			reported, _ := native["usage"].(map[string]any)
			if _, ok := reported["total_tokens"]; !ok {
				return tokenFacts{}
			}
		}
	}
	usage := extractUsage(response, capability)
	if confirmed, exists := body["_meter_token_evidence"].(bool); exists && !confirmed {
		usage.Reliable = false
	}
	out := tokenFacts{}
	if usage.Reliable {
		for _, m := range usage.Measurements {
			q, ok := new(big.Rat).SetString(m.Quantity)
			if !ok {
				return tokenFacts{}
			}
			switch m.Component {
			case "input":
				out.input = q
			case "output":
				out.output = q
			case "cache":
				out.cache = q
			}
		}
	}
	if q, ok := numeric(body["_meter_token_input"]); ok {
		out.input = q
	}
	if q, ok := numeric(body["_meter_token_output"]); ok {
		out.output = q
	}
	if q, ok := numeric(body["_meter_token_cache"]); ok {
		out.cache = q
	}
	if nested, ok := body["response"].(map[string]any); ok {
		body = nested
	}
	fields, _ := body["usage"].(map[string]any)
	if op == Chat || op == Embedding {
		out.inputDetails, _ = fields["prompt_tokens_details"].(map[string]any)
		out.outputDetails, _ = fields["completion_tokens_details"].(map[string]any)
	} else {
		out.inputDetails, _ = fields["input_tokens_details"].(map[string]any)
		out.outputDetails, _ = fields["output_tokens_details"].(map[string]any)
	}
	return out
}
func (f tokenFacts) part(direction, modality string, r Request, body map[string]any) (*big.Rat, *big.Rat) {
	total, details := f.input, f.inputDetails
	if direction == "output" {
		total, details = f.output, f.outputDetails
	}
	if modality == "all" {
		return total, f.cache
	}
	kinds := sources(r).kinds()
	if direction == "output" {
		switch r.Operation {
		case Image:
			kinds = []string{"image"}
		case Speech:
			kinds = []string{"audio"}
		case Transcription:
			kinds = []string{"text"}
		case Video, VideoRetrieve:
			kinds = []string{"video"}
		default:
			kinds = outputKinds(body)
			for _, kind := range []string{"image", "audio", "video"} {
				if n, ok := numeric(details[kind+"_tokens"]); ok && n.Sign() > 0 {
					kinds = append(kinds, kind)
				}
			}
		}
	}
	if direction == "output" {
		if observed, ok := body["_meter_output_kinds"].([]any); ok {
			for _, raw := range observed {
				if kind, ok := raw.(string); ok && !member(kinds, kind) {
					kinds = append(kinds, kind)
				}
			}
		}
	}
	present := false
	for _, kind := range kinds {
		if kind == modality {
			present = true
		}
	}
	if !present {
		return new(big.Rat), new(big.Rat)
	}
	value, ok := numeric(details[modality+"_tokens"])
	if !ok && len(kinds) == 1 {
		value = total
	}
	if !ok && modality == "text" && total != nil && len(kinds) > 1 {
		remainder := new(big.Rat).Set(total)
		known := true
		for _, kind := range kinds {
			if kind == "text" {
				continue
			}
			part, ok := numeric(details[kind+"_tokens"])
			if !ok {
				known = false
				break
			}
			remainder.Sub(remainder, part)
		}
		if known && remainder.Sign() >= 0 {
			value = remainder
		}
	}
	if total == nil || value == nil || value.Cmp(total) > 0 {
		return nil, nil
	}
	if direction == "output" {
		return value, nil
	}
	if f.cache == nil {
		return nil, nil
	}
	if f.cache.Sign() == 0 {
		return value, new(big.Rat)
	}
	if len(kinds) == 1 {
		return value, f.cache
	}
	cached, _ := details["cached_tokens_details"].(map[string]any)
	part, ok := numeric(cached[modality+"_tokens"])
	if !ok {
		return nil, nil
	}
	return value, part
}

func outputKinds(body map[string]any) []string {
	seen := map[string]bool{"text": true}
	inspect := func(parts []any) {
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			switch part["type"] {
			case "audio", "output_audio":
				seen["audio"] = true
			case "image", "output_image", "image_url":
				seen["image"] = true
			case "video", "output_video":
				seen["video"] = true
			}
		}
	}
	choices, _ := body["choices"].([]any)
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		message, _ := choice["message"].(map[string]any)
		if message["audio"] != nil {
			seen["audio"] = true
		}
		parts, _ := message["content"].([]any)
		inspect(parts)
	}
	if nested, ok := body["response"].(map[string]any); ok {
		body = nested
	}
	outputs, _ := body["output"].([]any)
	for _, raw := range outputs {
		item, _ := raw.(map[string]any)
		if item["type"] == "image_generation_call" {
			seen["image"] = true
		}
		parts, _ := item["content"].([]any)
		inspect(parts)
	}
	out := []string{}
	for _, kind := range []string{"text", "image", "audio", "video"} {
		if seen[kind] {
			out = append(out, kind)
		}
	}
	return out
}
