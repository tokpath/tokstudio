package modelruntime

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

func extractUsage(response any, c Capability) Usage {
	unknown := Usage{Source: "unknown"}
	encoded, e := json.Marshal(response)
	if e != nil {
		return unknown
	}
	var body map[string]json.RawMessage
	if json.Unmarshal(encoded, &body) != nil {
		return unknown
	}
	raw := body["usage"]
	if c.Operation == Responses && (len(raw) == 0 || string(raw) == "null") {
		var response map[string]json.RawMessage
		if json.Unmarshal(body["response"], &response) == nil {
			raw = response["usage"]
		}
	}
	source := "provider_usage"
	var videoNativeSeconds any
	if c.Operation == Video {
		var extra struct {
			Raw json.RawMessage `json:"raw_response"`
		}
		_ = json.Unmarshal(body["extra_fields"], &extra)
		var native map[string]any
		d := json.NewDecoder(bytes.NewReader(extra.Raw))
		d.UseNumber()
		if d.Decode(&native) == nil && native["status"] == "completed" {
			videoNativeSeconds = native["seconds"]
		}
		if len(raw) == 0 || string(raw) == "null" {
			raw = json.RawMessage(`{}`)
		}
	}
	// Bifrost omits zero-valued token fields. Its typed usage object preserves
	// their presence as one complete provider usage object; restore canonical
	// zeros before applying the provider's verified measurement mapping.
	var llm *schemas.BifrostLLMUsage
	switch r := response.(type) {
	case *schemas.BifrostChatResponse:
		if r != nil {
			llm = r.Usage
		}
	case *schemas.BifrostEmbeddingResponse:
		if r != nil {
			llm = r.Usage
		}
	case *schemas.BifrostStreamChunk:
		if r != nil && r.BifrostChatResponse != nil {
			llm = r.BifrostChatResponse.Usage
		}
	}
	if llm != nil {
		if llm.PromptTokens < 0 || llm.CompletionTokens < 0 || llm.TotalTokens != llm.PromptTokens+llm.CompletionTokens {
			return unknown
		}
		if _, stream := response.(*schemas.BifrostStreamChunk); stream && llm.TotalTokens == 0 {
			// Bifrost synthesizes a terminal chat usage object even when the
			// provider sent no usage. Typed zero fields alone are not evidence.
			// A real zero measurement is accepted only with its native envelope;
			// do not retain that envelope or any generated content in the ledger.
			var extra struct {
				Raw json.RawMessage `json:"raw_response"`
			}
			_ = json.Unmarshal(body["extra_fields"], &extra)
			native := extra.Raw
			var text string
			if json.Unmarshal(native, &text) == nil {
				native = json.RawMessage(text)
			}
			var envelope map[string]json.RawMessage
			if json.Unmarshal(native, &envelope) != nil {
				return unknown
			}
			var nativeUsage struct {
				Prompt     *int `json:"prompt_tokens"`
				Completion *int `json:"completion_tokens"`
				Total      *int `json:"total_tokens"`
			}
			if json.Unmarshal(envelope["usage"], &nativeUsage) != nil {
				return unknown
			}
			if nativeUsage.Prompt == nil || nativeUsage.Completion == nil || nativeUsage.Total == nil || *nativeUsage.Prompt != 0 || *nativeUsage.Completion != 0 || *nativeUsage.Total != 0 {
				return unknown
			}
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
		fields["prompt_tokens"] = json.RawMessage(strconv.Itoa(llm.PromptTokens))
		fields["completion_tokens"] = json.RawMessage(strconv.Itoa(llm.CompletionTokens))
		raw, _ = json.Marshal(fields)
	}
	if (len(raw) == 0 || string(raw) == "null") && c.Operation == Image {
		var data []json.RawMessage
		if json.Unmarshal(body["data"], &data) != nil || len(data) == 0 {
			return unknown
		}
		raw, _ = json.Marshal(map[string]int{"output_count": len(data)})
		source = "verified_output"
	}
	if len(raw) == 0 || string(raw) == "null" {
		return unknown
	}
	var facts map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&facts) != nil || facts == nil {
		return unknown
	}
	// SDK prices use float64 and may be estimates. They never enter a money
	// decision or the persisted evidence accepted by the billing domain.
	delete(facts, "cost")
	delete(facts, "cost_in_usd_ticks")
	if videoNativeSeconds != nil {
		facts["provider"] = map[string]any{"seconds": videoNativeSeconds}
		source = "provider_metadata"
	}
	if c.Operation == Image {
		var data []json.RawMessage
		if json.Unmarshal(body["data"], &data) == nil && len(data) > 0 {
			facts["output_count"] = json.Number(strconv.Itoa(len(data)))
			for _, rule := range c.UsageRules {
				if rule.Path == "/output_count" {
					source = "verified_output"
				}
			}
		}
	}
	if c.Operation == Speech {
		for _, rule := range c.UsageRules {
			if rule.Path == "/input_chars" {
				source = "verified_input"
			}
		}
	}
	measurements := make([]Measurement, 0, len(c.UsageRules))
	for _, rule := range c.UsageRules {
		v, exists := pointer(facts, rule.Path)
		if !exists {
			if rule.ZeroWhenAbsent {
				v = json.Number("0")
			} else {
				return unknown
			}
		}
		var text string
		switch value := v.(type) {
		case json.Number:
			text = string(value)
		case string:
			text = value
		default:
			return unknown
		}
		q, ok := new(big.Rat).SetString(text)
		if !ok || q.Sign() < 0 || strings.ContainsAny(text, "/eE+-") {
			return unknown
		}
		for _, path := range c.SubtractPaths[rule.Component] {
			included, exists := pointer(facts, path)
			if !exists {
				return unknown
			}
			var value string
			switch n := included.(type) {
			case json.Number:
				value = string(n)
			case string:
				value = n
			default:
				return unknown
			}
			part, ok := new(big.Rat).SetString(value)
			if !ok || part.Sign() < 0 {
				return unknown
			}
			q.Sub(q, part)
			if q.Sign() < 0 {
				return unknown
			}
		}
		if len(c.SubtractPaths[rule.Component]) > 0 {
			text = q.FloatString(24)
			roundtrip, ok := new(big.Rat).SetString(text)
			if !ok || roundtrip.Cmp(q) != 0 {
				return unknown
			}
			text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
			if text == "" {
				text = "0"
			}
		}
		// Zero or nonzero evidence exceeding the promised maximum is retained
		// as reliable usage. The coordinator records the technical bound failure;
		// it must never turn it into negative procurement balances.
		measurements = append(measurements, Measurement{Component: rule.Component, Unit: rule.Unit, Quantity: text})
	}
	if len(measurements) == 0 {
		return unknown
	}
	evidence, _ := json.Marshal(facts)
	return Usage{Source: source, Evidence: evidence, Measurements: measurements, Reliable: true}
}
func pointer(root map[string]any, path string) (any, bool) {
	if !strings.HasPrefix(path, "/") {
		return nil, false
	}
	var value any = root
	for _, part := range strings.Split(path[1:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch v := value.(type) {
		case map[string]any:
			next, ok := v[part]
			if !ok {
				return nil, false
			}
			value = next
		case []any:
			n, e := strconv.Atoi(part)
			if e != nil || n < 0 || n >= len(v) {
				return nil, false
			}
			value = v[n]
		default:
			return nil, false
		}
	}
	return value, true
}

// CheckUsageBounds is independent of charging. Reliable evidence is preserved
// on failure so the coordinator can classify platform technical losses.
func CheckUsageBounds(c Capability, u Usage) error {
	if !u.Reliable {
		return ErrUsageUnknown
	}
	seen := map[string]bool{}
	units := map[string]string{}
	for _, r := range c.UsageRules {
		units[r.Component] = r.Unit
	}
	for _, m := range u.Measurements {
		if seen[m.Component] || units[m.Component] != m.Unit {
			return ErrInvalidRequest
		}
		seen[m.Component] = true
		q, ok := new(big.Rat).SetString(m.Quantity)
		if !ok || q.Sign() < 0 {
			return ErrInvalidRequest
		}
		b, ok := new(big.Rat).SetString(c.BillableBounds[m.Component])
		if !ok {
			return ErrInvalidRequest
		}
		if q.Cmp(b) > 0 {
			return ErrBoundExceeded
		}
	}
	if len(seen) != len(c.BillableBounds) {
		return ErrUsageUnknown
	}
	return nil
}
