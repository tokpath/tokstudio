package usage

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

// Output ceiling fields recognized across common model APIs. The request body
// is only read; nothing here renames or removes a field before forwarding.
var outputLimitKeys = map[string]bool{
	"max_tokens":            true,
	"max_completion_tokens": true,
	"max_output_tokens":     true,
	"maxOutputTokens":       true,
	"maxTokens":             true,
	"max_tokens_to_sample":  true,
	"max_new_tokens":        true,
	"max_gen_len":           true,
	"num_predict":           true,
}

// How many images or videos this call asks for. Names differ by provider:
// OpenAI n, Vertex Imagen sampleCount, Bedrock numberOfImages, Ideogram and
// fal num_images, Replicate num_outputs, Stability num_samples, diffusers
// num_images_per_prompt, Wan n, Wan and Seedream max_images.
var outputCountKeys = map[string]bool{
	"n":                     true,
	"sampleCount":           true,
	"sample_count":          true,
	"numberOfImages":        true,
	"number_of_images":      true,
	"num_images":            true,
	"numImages":             true,
	"num_samples":           true,
	"samples":               true,
	"num_outputs":           true,
	"numOutputs":            true,
	"batch_size":            true,
	"batchSize":             true,
	"num_images_per_prompt": true,
	"numImagesPerPrompt":    true,
	"max_images":            true,
	"maxImages":             true,
	"image_count":           true,
	"imageCount":            true,
	"candidateCount":        true,
	"candidate_count":       true,
}

// These switches mean the model may emit several outputs, but the exact count
// is chosen later. Without a count field above, the registered bound is used.
var openEndedCountKeys = map[string]bool{
	"sequential_image_generation": true,
	"enable_interleave":           true,
}

// Duration in seconds. OpenAI videos use seconds, Runway Kling Wan Seedance
// MiniMax and Luma use duration, Veo uses durationSeconds, ElevenLabs sound
// effects use duration_seconds. Luma may send a string such as "5s".
var durationSecondKeys = map[string]bool{
	"seconds":          true,
	"duration":         true,
	"durationSeconds":  true,
	"duration_seconds": true,
	"video_duration":   true,
	"videoDuration":    true,
}

var durationMillisecondKeys = map[string]bool{
	"duration_ms":           true,
	"durationMs":            true,
	"duration_milliseconds": true,
}

// Frame counts are not seconds. They convert only when a frame rate is also present.
var frameCountKeys = map[string]bool{
	"num_frames":   true,
	"numFrames":    true,
	"frame_count":  true,
	"frameCount":   true,
	"video_length": true,
	"videoLength":  true,
}

var frameRateKeys = map[string]bool{
	"fps":               true,
	"frame_rate":        true,
	"frameRate":         true,
	"frames_per_second": true,
}

// Spoken text. OpenAI uses input, ElevenLabs and MiniMax use text, Google
// uses text, ssml, or markup, Cartesia uses transcript, Polly uses Text.
// Sibling alternatives use the longer text. Turns in an array are all spoken.
var speechTextKeys = map[string]bool{
	"input":      true,
	"text":       true,
	"ssml":       true,
	"markup":     true,
	"transcript": true,
	"Text":       true,
}

// reservationQuantities is what customer credit and procurement both reserve.
// A quantity visible on this request is used. Several visible quantities use
// the larger number. Anything we cannot see uses the registered bound.
func reservationQuantities(q catalog.ModelQuote, r modelruntime.Request) map[string]string {
	out := map[string]string{}
	if r.Operation == modelruntime.VideoRetrieve || r.Operation == modelruntime.VideoDownload {
		for key := range q.Bounds {
			out[key] = "0"
		}
		return out
	}
	units := map[string]string{}
	for _, component := range q.Retail.Components {
		units[component.Key] = component.Unit
	}
	samples := int64(1)
	if n, ok := topLevelInt(r.Params, "n"); ok && n > 1 {
		samples = n
	}
	cap, hasCap := largestOutputLimit(r.Params)
	copies, hasCopies := outputCount(r.Input, r.Params)
	if copies < 1 {
		copies, hasCopies = 1, false
	}
	seconds, hasSeconds := mediaDurationSeconds(r.Input, r.Params)
	chars := spokenAcross(r.Input, r.Params)
	inputTokens := textRunes(r.Input)*2 + 16
	for key, bound := range q.Bounds {
		unit := strings.ToLower(units[key])
		lower := strings.ToLower(key)
		quantity := bound
		switch {
		case unit == "token" && (strings.Contains(lower, "input") || strings.Contains(lower, "prompt")):
			n := inputTokens * samples
			if inputHasUnsizedMedia(r.Input) {
				if limit := positiveInt(bound); limit > n {
					n = limit
				}
			}
			quantity = strconv.FormatInt(n, 10)
		case unit == "token" && (strings.Contains(lower, "output") || strings.Contains(lower, "completion")):
			n := positiveInt(bound)
			if hasCap {
				n = cap
			}
			if n < 1 {
				n = 1
			}
			quantity = strconv.FormatInt(n*samples, 10)
		case unit == "second" || strings.Contains(lower, "second"):
			n := positiveInt(bound)
			if hasSeconds {
				n = seconds
			}
			if n < 1 {
				n = 1
			}
			if hasCopies {
				n *= copies
			}
			quantity = strconv.FormatInt(n, 10)
		case unit == "image" || strings.Contains(lower, "image"):
			switch {
			case hasCopies:
				quantity = strconv.FormatInt(copies, 10)
			case openEndedCount(r.Input, r.Params):
				quantity = bound
			default:
				quantity = "1"
			}
		case unit == "character" || unit == "char" || strings.Contains(lower, "char"):
			if chars < 1 {
				break
			}
			n := chars
			if hasCopies {
				n *= copies
			}
			quantity = strconv.FormatInt(n, 10)
		}
		out[key] = quantity
	}
	return out
}

func outputCount(blobs ...json.RawMessage) (int64, bool) {
	best, found := int64(0), false
	for _, raw := range blobs {
		walkJSON(parseJSON(raw), func(key string, child any) {
			if !outputCountKeys[key] {
				return
			}
			if n, ok := positiveCeil(child); ok && (!found || n > best) {
				best, found = n, true
			}
		})
	}
	return best, found
}

func openEndedCount(blobs ...json.RawMessage) bool {
	open := false
	for _, raw := range blobs {
		walkJSON(parseJSON(raw), func(key string, child any) {
			if openEndedCountKeys[key] && openEndedValue(child) {
				open = true
			}
		})
	}
	return open
}

func openEndedValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "auto", "true", "enabled", "on", "yes":
			return true
		}
	}
	return false
}

func mediaDurationSeconds(blobs ...json.RawMessage) (int64, bool) {
	best, found := 0.0, false
	frames, framesFound := 0.0, false
	fps, fpsFound := 0.0, false
	for _, raw := range blobs {
		walkJSON(parseJSON(raw), func(key string, child any) {
			if durationSecondKeys[key] {
				if n, ok := scaledNumber(child, 1); ok && (!found || n > best) {
					best, found = n, true
				}
			}
			if durationMillisecondKeys[key] {
				if n, ok := scaledNumber(child, 0.001); ok && (!found || n > best) {
					best, found = n, true
				}
			}
			if frameCountKeys[key] {
				if n, ok := scaledNumber(child, 1); ok && (!framesFound || n > frames) {
					frames, framesFound = n, true
				}
			}
			if frameRateKeys[key] {
				if n, ok := scaledNumber(child, 1); ok && (!fpsFound || n < fps) {
					fps, fpsFound = n, true
				}
			}
		})
	}
	if framesFound && fpsFound && fps > 0 {
		computed := frames / fps
		if !found || computed > best {
			best, found = computed, true
		}
	}
	if !found || best <= 0 {
		return 0, false
	}
	return int64(math.Ceil(best)), true
}

func inputHasUnsizedMedia(raw json.RawMessage) bool {
	found := false
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if kind, ok := x["type"].(string); ok {
				switch kind {
				case "image_url", "input_image", "input_audio", "audio", "video_url", "input_video":
					found = true
				}
			}
			for _, child := range x {
				visit(child)
			}
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(parseJSON(raw))
	return found
}

func spokenAcross(blobs ...json.RawMessage) int64 {
	best := int64(0)
	for _, raw := range blobs {
		if n := spokenRunes(parseJSON(raw)); n > best {
			best = n
		}
	}
	return best
}

func spokenRunes(v any) int64 {
	switch x := v.(type) {
	case string:
		return int64(utf8.RuneCountInString(x))
	case []any:
		var n int64
		for _, child := range x {
			n += spokenRunes(child)
		}
		return n
	case map[string]any:
		best, found := int64(0), false
		for key, child := range x {
			if !speechTextKeys[key] {
				continue
			}
			n := spokenRunes(child)
			if !found || n > best {
				best, found = n, true
			}
		}
		if found {
			return best
		}
		var n int64
		for _, child := range x {
			switch child.(type) {
			case map[string]any, []any:
				n += spokenRunes(child)
			}
		}
		return n
	default:
		return 0
	}
}

func walkJSON(v any, visit func(key string, child any)) {
	switch x := v.(type) {
	case map[string]any:
		for key, child := range x {
			visit(key, child)
			walkJSON(child, visit)
		}
	case []any:
		for _, child := range x {
			walkJSON(child, visit)
		}
	}
}

func positiveCeil(v any) (int64, bool) {
	n, ok := scaledNumber(v, 1)
	if !ok {
		return 0, false
	}
	out := int64(math.Ceil(n))
	if out < 1 {
		return 0, false
	}
	return out, true
}

func scaledNumber(v any, scale float64) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		if err != nil || f <= 0 {
			return 0, false
		}
		return f * scale, true
	case string:
		s := strings.TrimSpace(n)
		mult := scale
		lower := strings.ToLower(s)
		switch {
		case strings.HasSuffix(lower, "ms"):
			s = strings.TrimSpace(s[:len(s)-2])
			mult = 0.001
		case strings.HasSuffix(lower, "s"):
			s = strings.TrimSpace(s[:len(s)-1])
			mult = 1
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f <= 0 {
			return 0, false
		}
		return f * mult, true
	default:
		return 0, false
	}
}

func largestOutputLimit(raw json.RawMessage) (int64, bool) {
	best, found := int64(0), false
	var visit func(any, bool)
	visit = func(v any, reasoning bool) {
		switch x := v.(type) {
		case map[string]any:
			for key, child := range x {
				if !reasoning && outputLimitKeys[key] {
					if n, ok := positiveJSONInt(child); ok && (!found || n > best) {
						best, found = n, true
					}
				}
				visit(child, reasoning || key == "reasoning")
			}
		case []any:
			for _, child := range x {
				visit(child, reasoning)
			}
		}
	}
	visit(parseJSON(raw), false)
	return best, found
}

func topLevelInt(raw json.RawMessage, key string) (int64, bool) {
	obj, ok := parseJSON(raw).(map[string]any)
	if !ok {
		return 0, false
	}
	return positiveJSONInt(obj[key])
}

func positiveInt(raw string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func positiveJSONInt(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil || i < 1 {
			f, ferr := n.Float64()
			if ferr != nil || f < 1 {
				return 0, false
			}
			return int64(f), true
		}
		return i, true
	case string:
		i := positiveInt(n)
		return i, i > 0
	default:
		return 0, false
	}
}

func textRunes(raw json.RawMessage) int64 {
	var count int64
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case string:
			count += int64(utf8.RuneCountInString(x))
		case map[string]any:
			for _, child := range x {
				visit(child)
			}
		case []any:
			for _, child := range x {
				visit(child)
			}
		}
	}
	visit(parseJSON(raw))
	return count
}

func parseJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if dec.Decode(&v) != nil {
		return nil
	}
	return v
}
