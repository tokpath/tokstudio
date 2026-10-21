package usage

import (
	"bytes"
	"encoding/json"
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

// reservationQuantities is what customer credit and procurement both reserve.
// A quantity visible on this request is used as-is. Several output ceilings
// use the larger number. Anything we cannot see uses the registered bound.
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
	seconds, hasSeconds := topLevelInt(r.Params, "seconds")
	images := int64(1)
	if n, ok := topLevelInt(r.Params, "n"); ok && n > 0 {
		images = n
	}
	chars := textRunes(r.Input)
	inputTokens := chars*2 + 16
	for key, bound := range q.Bounds {
		unit := strings.ToLower(units[key])
		lower := strings.ToLower(key)
		quantity := bound
		switch {
		case unit == "token" && (strings.Contains(lower, "input") || strings.Contains(lower, "prompt")):
			quantity = strconv.FormatInt(inputTokens*samples, 10)
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
			if hasSeconds {
				quantity = strconv.FormatInt(seconds, 10)
			}
		case unit == "image" || strings.Contains(lower, "image"):
			quantity = strconv.FormatInt(images, 10)
		case unit == "character" || unit == "char" || strings.Contains(lower, "char"):
			quantity = strconv.FormatInt(chars*samples, 10)
		}
		out[key] = quantity
	}
	return out
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
