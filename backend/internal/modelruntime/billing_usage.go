package modelruntime

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"github.com/maximhq/bifrost/core/schemas"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math/big"
	"strings"
)

func extractBilledUsage(response any, c Capability, r Request) Usage {
	if c.Billing == nil {
		return extractUsage(response, c)
	}
	raw, e := json.Marshal(response)
	if e != nil {
		return Usage{Source: "unknown"}
	}
	var body map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&body) != nil {
		return Usage{Source: "unknown"}
	}
	tokens := readTokenFacts(response, c, body)
	facts := map[string]*big.Rat{}
	for _, direction := range []string{"input", "output"} {
		for _, item := range c.Billing.Items(direction) {
			for _, component := range item.Components(direction) {
				facts[component.Key] = new(big.Rat)
			}
			if item.Template == "tokens" {
				total, cache := tokens.part(direction, item.Modality, r, body)
				if total == nil {
					return Usage{Source: "unknown"}
				}
				if direction == "input" {
					if cache == nil || total.Cmp(cache) < 0 {
						return Usage{Source: "unknown"}
					}
					total = new(big.Rat).Sub(total, cache)
					components := item.Components(direction)
					facts[components[1].Key] = cache
				}
				facts[item.Key(direction)] = total
				continue
			}
			if direction == "input" {
				input := r.InputFacts
				if input == nil {
					input = (*Bifrost)(nil).inputFacts(nil, r, *c.Billing)
				}
				for _, component := range item.Components(direction) {
					text, exists := input[component.Key]
					// Transcription providers can report the portion actually processed.
					if item.Modality == "audio" && item.Template == "seconds" && c.Operation == Transcription {
						usage, _ := body["usage"].(map[string]any)
						if q, ok := numeric(usage["seconds"]); ok {
							text = q.FloatString(24)
							exists = true
						}
					}
					value, ok := new(big.Rat).SetString(text)
					if !exists || !ok || value.Sign() < 0 {
						return Usage{Source: "unknown"}
					}
					facts[component.Key] = value
				}
				continue
			}
			if item.Modality == "image" {
				if !imageOutputFacts(item, r, body, facts) {
					return Usage{Source: "unknown"}
				}
				continue
			}
			var quantity *big.Rat
			switch item.Template {
			case "characters":
				text, ok := outputText(body, c.Operation)
				if count, known := numeric(body["_measured_output_characters"]); known {
					quantity = count
				} else if ok {
					quantity = big.NewRat(int64(len([]rune(text))), 1)
				}
			case "seconds":
				if item.Modality == "video" {
					quantity = KnownVideoSeconds(response)
				} else if item.Modality == "audio" {
					if audio, ok := response.(*schemas.BifrostSpeechResponse); ok && audio != nil {
						quantity = audioSeconds(audio.Audio)
					}
					if quantity == nil && c.Operation == Chat {
						quantity = chatAudioSeconds(body)
					}
					if quantity == nil && c.Operation == Speech {
						native := nativeBody(body)
						usage, _ := native["usage"].(map[string]any)
						for _, key := range []string{"audio_seconds", "seconds", "duration"} {
							if q, ok := numeric(usage[key]); ok {
								quantity = q
								break
							}
						}
					}
				}
			}
			if quantity == nil {
				return Usage{Source: "unknown"}
			}
			selected, e := item.SelectVariant(r.Params)
			if e != nil {
				return Usage{Source: "unknown"}
			}
			key := item.Key(direction)
			if selected != "" {
				key += ":" + selected
			}
			facts[key] = quantity
		}
	}
	measurements := []Measurement{}
	evidence := map[string]string{}
	for _, component := range c.Billing.Components(c.Operation) {
		value := facts[component.Key]
		if value == nil || value.Sign() < 0 {
			return Usage{Source: "unknown"}
		}
		text := value.FloatString(24)
		measurements = append(measurements, Measurement{Component: component.Key, Unit: component.Unit, Quantity: text})
		evidence[component.Key] = text
	}
	encoded, _ := json.Marshal(evidence)
	return Usage{Reliable: true, Source: "normalized_quantities", Measurements: measurements, Evidence: encoded}
}
func imageOutputFacts(item BillingItem, r Request, body map[string]any, facts map[string]*big.Rat) bool {
	data, ok := outputImages(body)
	if !ok {
		return false
	}
	for _, raw := range data {
		entry, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		quantity := big.NewRat(1, 1)
		selector := r.Params
		w, h := imageDimensions(entry)
		if item.Template == "megapixels" {
			if w <= 0 || h <= 0 {
				return false
			}
			quantity = big.NewRat(int64(w)*int64(h), 1000000)
		}
		if len(item.Variants) > 0 {
			var params map[string]json.RawMessage
			_ = json.Unmarshal(r.Params, &params)
			needsSize := false
			for _, v := range item.Variants {
				needsSize = needsSize || v.Size != ""
			}
			if needsSize {
				if w <= 0 || h <= 0 {
					return false
				}
				size := new(big.Int).SetInt64(int64(w)).String() + "x" + new(big.Int).SetInt64(int64(h)).String()
				var requested string
				_ = json.Unmarshal(params["size"], &requested)
				if requested != "" && requested != "auto" && requested != size {
					return false
				}
				params["size"], _ = json.Marshal(size)
			}
			selector, _ = json.Marshal(params)
		}
		selected, e := item.SelectVariant(selector)
		if e != nil {
			return false
		}
		key := item.Key("output")
		if selected != "" {
			key += ":" + selected
		}
		facts[key].Add(facts[key], quantity)
	}
	return true
}
func nativeBody(body map[string]any) map[string]any {
	extra, _ := body["extra_fields"].(map[string]any)
	native := extra["raw_response"]
	if text, ok := native.(string); ok {
		var value any
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if decoder.Decode(&value) == nil {
			native = value
		}
	}
	object, _ := native.(map[string]any)
	return object
}
func outputText(body map[string]any, op Operation) (string, bool) {
	if op == Transcription {
		text, ok := body["text"].(string)
		return text, ok
	}
	if op == Chat {
		choices, ok := body["choices"].([]any)
		if !ok {
			return "", false
		}
		var out strings.Builder
		for _, raw := range choices {
			choice, _ := raw.(map[string]any)
			message, _ := choice["message"].(map[string]any)
			if message == nil {
				return "", false
			}
			text, ok := message["content"].(string)
			if ok {
				out.WriteString(text)
			} else {
				parts, ok := message["content"].([]any)
				if !ok {
					return "", false
				}
				for _, raw := range parts {
					part, _ := raw.(map[string]any)
					if part["type"] == "text" || part["type"] == "output_text" {
						text, _ := part["text"].(string)
						out.WriteString(text)
					}
				}
			}
		}
		return out.String(), true
	}
	if op == Responses {
		if response, ok := body["response"].(map[string]any); ok {
			body = response
		}
		outputs, ok := body["output"].([]any)
		if !ok {
			return "", false
		}
		var out strings.Builder
		for _, raw := range outputs {
			item, _ := raw.(map[string]any)
			if item["type"] != "message" {
				continue
			}
			parts, _ := item["content"].([]any)
			for _, raw := range parts {
				part, _ := raw.(map[string]any)
				if part["type"] == "output_text" {
					text, _ := part["text"].(string)
					out.WriteString(text)
				}
			}
		}
		return out.String(), true
	}
	return "", false
}
func numeric(v any) (*big.Rat, bool) {
	var str string
	switch n := v.(type) {
	case json.Number:
		str = n.String()
	case string:
		str = n
	default:
		return nil, false
	}
	q, ok := new(big.Rat).SetString(str)
	return q, ok && q.Sign() >= 0
}
func imageDimensions(entry map[string]any) (int, int) {
	if encoded, ok := entry["b64_json"].(string); ok && len(encoded) <= 48<<20 {
		if raw, e := base64.StdEncoding.DecodeString(encoded); e == nil {
			if cfg, _, e := image.DecodeConfig(bytes.NewReader(raw)); e == nil && cfg.Width > 0 && cfg.Height > 0 && cfg.Width <= 65536 && cfg.Height <= 65536 {
				return cfg.Width, cfg.Height
			}
		}
	}
	w, wok := numeric(entry["width"])
	h, hok := numeric(entry["height"])
	if wok && hok && w.IsInt() && h.IsInt() && w.Num().IsInt64() && h.Num().IsInt64() {
		width, height := w.Num().Int64(), h.Num().Int64()
		if width > 0 && height > 0 && width <= 65536 && height <= 65536 {
			return int(width), int(height)
		}
	}
	return 0, 0
}
func wavSeconds(data []byte) *big.Rat {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil
	}
	var rate, size uint32
	for pos := 12; pos+8 <= len(data); {
		n := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		if n < 0 || n > len(data)-pos-8 {
			return nil
		}
		if string(data[pos:pos+4]) == "fmt " && n >= 16 {
			rate = binary.LittleEndian.Uint32(data[pos+16 : pos+20])
		}
		if string(data[pos:pos+4]) == "data" {
			size += uint32(n)
		}
		pos += 8 + n + (n % 2)
	}
	if rate == 0 {
		return nil
	}
	return new(big.Rat).SetFrac(big.NewInt(int64(size)), big.NewInt(int64(rate)))
}

func outputImages(body map[string]any) ([]any, bool) {
	if data, ok := body["data"].([]any); ok {
		return data, true
	}
	out := []any{}
	found := false
	invalid := false
	add := func(value string) {
		if value == "" {
			invalid = true
			return
		}
		entry := map[string]any{}
		if strings.HasPrefix(value, "data:") {
			index := strings.Index(value, ",")
			if index < 0 || !strings.HasSuffix(value[:index], ";base64") {
				invalid = true
				return
			}
			entry["b64_json"] = value[index+1:]
		} else {
			entry["url"] = value
		}
		out = append(out, entry)
		found = true
	}
	choices, _ := body["choices"].([]any)
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		if _, ok := choice["message"].(map[string]any); ok {
			found = true
		}
	}
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		message, _ := choice["message"].(map[string]any)
		parts, _ := message["content"].([]any)
		for _, raw := range parts {
			part, _ := raw.(map[string]any)
			if part["type"] == "image_url" || part["type"] == "output_image" {
				value, _ := part["image_url"].(string)
				if nested, ok := part["image_url"].(map[string]any); ok {
					value, _ = nested["url"].(string)
				}
				add(value)
			}
		}
	}
	if nested, ok := body["response"].(map[string]any); ok {
		body = nested
	}
	outputs, complete := body["output"].([]any)
	if complete {
		found = true
	}
	for _, raw := range outputs {
		item, _ := raw.(map[string]any)
		if item["type"] == "image_generation_call" {
			if value, ok := item["result"].(string); ok && value != "" {
				out = append(out, map[string]any{"b64_json": value})
				found = true
			}
		}
	}
	return out, found && !invalid
}

// Chat-level aggregate audio_seconds can include input audio. Measure the
// returned audio instead, so input and output seconds never reuse one total.
func chatAudioSeconds(body map[string]any) *big.Rat {
	choices, ok := body["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil
	}
	total := new(big.Rat)
	for _, raw := range choices {
		choice, _ := raw.(map[string]any)
		message, ok := choice["message"].(map[string]any)
		if !ok {
			return nil
		}
		if message["audio"] == nil {
			continue
		}
		audio, ok := message["audio"].(map[string]any)
		if !ok {
			return nil
		}
		data, _ := audio["data"].(string)
		duration := audioSeconds(assetBytes(data))
		if duration == nil {
			return nil
		}
		total.Add(total, duration)
	}
	return total
}
