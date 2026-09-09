package catalog

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// PriceSnapshot 是账务模块允许看到的公开价格视图，不含内部 ORM。
type PriceSnapshot struct {
	VersionID   string          `json:"version_id"`
	PublicID    string          `json:"public_id"`
	Raw         json.RawMessage `json:"unit_prices"`
	EffectiveAt time.Time       `json:"effective_at"`
}

type PriceBookView struct {
	ID          string          `json:"id"`
	PublicID    string          `json:"public_id"`
	Status      string          `json:"status"`
	UnitPrices  json.RawMessage `json:"unit_prices"`
	EffectiveAt time.Time       `json:"effective_at"`
	Upstream    string          `json:"upstream"`
	Wholesale   string          `json:"wholesale"`
	Sell        string          `json:"sell"`
	Channel     string          `json:"channel,omitempty"`
}

var ErrEmptyUnitPrices = errors.New("unit prices required")

// NormalizeUnitPrices 把发布体收成账务已认识的扁平键，并保留 docs/03 的维度对象。
// 接受 nested upstream_cost / wholesale / customer_sell / channel_override，
// 以及既有 input/output 与 *_input/*_output 别名。渠道覆盖未建模时可以缺省。
func NormalizeUnitPrices(in map[string]any) (map[string]any, error) {
	if in == nil {
		return nil, ErrEmptyUnitPrices
	}
	out := map[string]any{}

	sellIn, sellOut := pickDim(in, "customer_sell", "customer_sell_input", "customer_sell_output", "input", "output")
	costIn, costOut := pickDim(in, "upstream_cost", "upstream_cost_input", "upstream_cost_output")
	wholeIn, wholeOut := pickDim(in, "wholesale", "wholesale_input", "wholesale_output")
	chIn, chOut := pickDim(in, "channel_override", "channel_customer_input", "channel_customer_output")
	if chIn == "" && chOut == "" {
		chIn, chOut = pickDim(in, "channel_customer", "channel_customer_price_input", "channel_customer_price_output")
	}

	writeDim(out, "customer_sell", "customer_sell_input", "customer_sell_output", sellIn, sellOut)
	if sellIn != "" {
		out["input"] = sellIn
	}
	if sellOut != "" {
		out["output"] = sellOut
	}
	writeDim(out, "upstream_cost", "upstream_cost_input", "upstream_cost_output", costIn, costOut)
	writeDim(out, "wholesale", "wholesale_input", "wholesale_output", wholeIn, wholeOut)
	writeDim(out, "channel_override", "channel_customer_input", "channel_customer_output", chIn, chOut)

	if curr := stringifyPrice(in["currency"]); curr != "" {
		out["currency"] = curr
	} else {
		out["currency"] = "USD"
	}
	for _, key := range []string{
		"video_second", "image_count", "audio_second",
		"video_second_cost", "image_count_cost",
		"reasoning", "reasoning_output", "upstream_cost_reasoning",
	} {
		if v := stringifyPrice(in[key]); v != "" {
			out[key] = v
		}
	}

	if !hasPricedUnit(out) {
		return nil, ErrEmptyUnitPrices
	}
	return out, nil
}

func mergeUnitPrices(prev, incoming map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range prev {
		out[k] = v
	}
	for k, v := range incoming {
		out[k] = v
	}
	return out
}

func priceBookFromRow(row priceRow, publicID string) PriceBookView {
	dims := decodeUnitPrices(row.UnitPrices)
	return PriceBookView{
		ID:          row.ID,
		PublicID:    publicID,
		Status:      row.Status,
		UnitPrices:  json.RawMessage(row.UnitPrices),
		EffectiveAt: row.EffectiveAt,
		Upstream:    formatIO(dims["upstream_cost_input"], dims["upstream_cost_output"]),
		Wholesale:   formatIO(dims["wholesale_input"], dims["wholesale_output"]),
		Sell:        formatIO(firstNonEmpty(dims["input"], dims["customer_sell_input"]), firstNonEmpty(dims["output"], dims["customer_sell_output"])),
		Channel:     formatIO(dims["channel_customer_input"], dims["channel_customer_output"]),
	}
}

func decodeUnitPrices(raw []byte) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return out
	}
	sellIn, sellOut := pickDim(m, "customer_sell", "customer_sell_input", "customer_sell_output", "input", "output")
	costIn, costOut := pickDim(m, "upstream_cost", "upstream_cost_input", "upstream_cost_output")
	wholeIn, wholeOut := pickDim(m, "wholesale", "wholesale_input", "wholesale_output")
	chIn, chOut := pickDim(m, "channel_override", "channel_customer_input", "channel_customer_output")
	if chIn == "" && chOut == "" {
		chIn, chOut = pickDim(m, "channel_customer", "channel_customer_price_input", "channel_customer_price_output")
	}
	set := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	set("input", sellIn)
	set("output", sellOut)
	set("customer_sell_input", sellIn)
	set("customer_sell_output", sellOut)
	set("upstream_cost_input", costIn)
	set("upstream_cost_output", costOut)
	set("wholesale_input", wholeIn)
	set("wholesale_output", wholeOut)
	set("channel_customer_input", chIn)
	set("channel_customer_output", chOut)
	return out
}

func pickDim(in map[string]any, nested string, keys ...string) (input, output string) {
	if nested != "" {
		if obj, ok := asObject(in[nested]); ok {
			input = stringifyPrice(firstKey(obj, "input", "in"))
			output = stringifyPrice(firstKey(obj, "output", "out"))
		}
	}
	for _, key := range keys {
		if strings.HasSuffix(key, "_output") || key == "output" {
			if output == "" {
				output = stringifyPrice(in[key])
			}
			continue
		}
		if input == "" {
			input = stringifyPrice(in[key])
		}
	}
	return input, output
}

func writeDim(out map[string]any, nested, inKey, outKey, input, output string) {
	if input == "" && output == "" {
		return
	}
	dim := map[string]any{}
	if input != "" {
		out[inKey] = input
		dim["input"] = input
	}
	if output != "" {
		out[outKey] = output
		dim["output"] = output
	}
	out[nested] = dim
}

func asObject(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func firstKey(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func hasPricedUnit(out map[string]any) bool {
	for _, key := range []string{
		"input", "output", "customer_sell_input", "customer_sell_output",
		"upstream_cost_input", "upstream_cost_output",
		"wholesale_input", "wholesale_output",
		"channel_customer_input", "channel_customer_output",
		"video_second", "image_count", "audio_second",
	} {
		if stringifyPrice(out[key]) != "" {
			return true
		}
	}
	return false
}

func formatIO(input, output string) string {
	switch {
	case input == "" && output == "":
		return ""
	case output == "":
		return input
	case input == "":
		return output
	default:
		return input + "/" + output
	}
}
