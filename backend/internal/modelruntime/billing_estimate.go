package modelruntime

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"
	"unicode/utf8"
)

func EstimateUsage(c Capability, r Request) map[string]string {
	out := map[string]string{}
	if r.Operation == VideoRetrieve || r.Operation == VideoDownload {
		for key := range c.BillableBounds {
			out[key] = "0"
		}
		return out
	}
	if c.Billing == nil {
		return out
	}
	var params map[string]any
	decoder := json.NewDecoder(bytes.NewReader(r.Params))
	decoder.UseNumber()
	_ = decoder.Decode(&params)
	count := big.NewRat(1, 1)
	if n, ok := numeric(params["n"]); ok && n.Sign() > 0 && n.IsInt() {
		count = n
	}
	limit := new(big.Rat)
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if n, ok := numeric(params[key]); ok && n.Cmp(limit) > 0 {
			limit = n
		}
	}
	layout := sources(r)
	facts := r.InputFacts
	if facts == nil {
		facts = (*Bifrost)(nil).inputFacts(nil, r, *c.Billing)
	}
	for _, direction := range []string{"input", "output"} {
		for _, item := range c.Billing.Items(direction) {
			selected := ""
			if direction == "output" {
				selected, _ = item.SelectVariant(r.Params)
			}
			for _, component := range item.Components(direction) {
				q, _ := new(big.Rat).SetString(c.BillableBounds[component.Key])
				if q == nil {
					q = big.NewRat(1, 1)
				}
				if direction == "input" && item.Template != "tokens" {
					if value, ok := facts[component.Key]; ok {
						out[component.Key] = value
						continue
					}
				}
				if selected != "" && !strings.HasSuffix(component.Key, ":"+selected) || selected == "" && len(item.Variants) > 0 && strings.Contains(component.Key, ":") {
					out[component.Key] = "0"
					continue
				}
				switch component.Unit {
				case "token":
					if strings.HasPrefix(component.Key, "cache") {
						q = new(big.Rat)
					} else if direction == "output" {
						if limit.Sign() > 0 {
							q = new(big.Rat).Mul(limit, count)
						}
					} else if item.Modality == "text" || len(layout.kinds()) <= 1 && len(layout.text) > 0 {
						q = big.NewRat(layout.characters()*2+24+toolEstimate(params), 1)
					}
				case "image":
					if direction == "output" {
						q = new(big.Rat).Set(count)
					}
				case "megapixel":
					if direction == "input" {
						q.Mul(q, big.NewRat(int64(len(layout.images)), 1))
					}
					if direction == "output" {
						if size, ok := params["size"].(string); ok {
							var w, h int64
							if parseSize(size, &w, &h) {
								q = new(big.Rat).Mul(big.NewRat(w*h, 1000000), count)
							}
						}
					}
				case "second":
					if direction == "output" {
						if duration, ok := numeric(params["seconds"]); ok && duration.Sign() > 0 {
							q = new(big.Rat).Mul(duration, count)
						}
					}
				}
				out[component.Key] = q.FloatString(18)
			}
		}
	}
	return out
}
func parseSize(size string, w, h *int64) bool {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return false
	}
	a, ok := new(big.Int).SetString(parts[0], 10)
	if !ok || !a.IsInt64() {
		return false
	}
	b, ok := new(big.Int).SetString(parts[1], 10)
	if !ok || !b.IsInt64() {
		return false
	}
	*w, *h = a.Int64(), b.Int64()
	return *w > 0 && *h > 0 && *w <= 65536 && *h <= 65536
}

func toolEstimate(params map[string]any) int64 {
	value, exists := params["tools"]
	if !exists {
		return 0
	}
	raw, _ := json.Marshal(value)
	return int64(utf8.RuneCount(raw)) * 2
}
