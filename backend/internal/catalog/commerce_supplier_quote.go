package catalog

import (
	"math/big"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

type supplierQuoteLine struct {
	Key, Unit, Per, Path     string
	Optional, ZeroWhenAbsent bool
	SubtractFrom             string
}

// supplierQuoteLines is the provider price for one call. The server owns the
// unit, the quantity each price covers, and where usage is read. A text call
// is priced per million tokens. Images, speech, transcription, and video use
// their own quantities.
func supplierQuoteLines(op modelruntime.Operation) ([]supplierQuoteLine, bool) {
	token := func(key, path string, optional bool) supplierQuoteLine {
		return supplierQuoteLine{Key: key, Unit: "token", Per: "1000000", Path: path, Optional: optional, ZeroWhenAbsent: optional}
	}
	switch op {
	case modelruntime.Chat:
		return []supplierQuoteLine{
			token("input", "/prompt_tokens", false),
			token("output", "/completion_tokens", false),
			{Key: "cache", Unit: "token", Per: "1000000", Path: "/prompt_tokens_details/cached_read_tokens", Optional: true, ZeroWhenAbsent: true, SubtractFrom: "input"},
		}, true
	case modelruntime.Responses:
		return []supplierQuoteLine{
			token("input", "/input_tokens", false),
			token("output", "/output_tokens", false),
			{Key: "cache", Unit: "token", Per: "1000000", Path: "/input_tokens_details/cached_tokens", Optional: true, ZeroWhenAbsent: true, SubtractFrom: "input"},
		}, true
	case modelruntime.Embedding:
		return []supplierQuoteLine{token("input", "/prompt_tokens", false)}, true
	case modelruntime.Image:
		return []supplierQuoteLine{{Key: "image", Unit: "image", Per: "1", Path: "/output_count"}}, true
	case modelruntime.Speech:
		return []supplierQuoteLine{{Key: "characters", Unit: "character", Per: "1000000", Path: "/input_chars"}}, true
	case modelruntime.Transcription:
		return []supplierQuoteLine{{Key: "audio_seconds", Unit: "second", Per: "1", Path: "/seconds"}}, true
	case modelruntime.Video:
		return []supplierQuoteLine{{Key: "video_seconds", Unit: "second", Per: "1", Path: "/provider/seconds"}}, true
	default:
		return nil, false
	}
}

// normalizeSupplierModel replaces submitted metering with the price lines for
// each capability. Client-supplied paths, units, and "per" quantities are not
// kept. The same usage has one provider rate across every capability.
func normalizeSupplierModel(in *SupplierModel) error {
	if len(in.Capabilities) == 0 {
		return ErrInvalidInput
	}
	priceByKey := map[string]ComponentRate{}
	out := make([]modelruntime.Capability, 0, len(in.Capabilities))
	for _, capability := range in.Capabilities {
		lines, ok := supplierQuoteLines(capability.Operation)
		if !ok {
			return ErrInvalidInput
		}
		submitted := map[string]modelruntime.CapabilityQuote{}
		if len(capability.Quotes) > 0 {
			for _, quote := range capability.Quotes {
				if quote.Key == "" {
					return ErrInvalidInput
				}
				if _, exists := submitted[quote.Key]; exists {
					return ErrInvalidInput
				}
				submitted[quote.Key] = quote
			}
		} else {
			for _, rule := range capability.UsageRules {
				known := false
				for _, line := range lines {
					if line.Key == rule.Component {
						known = true
					}
				}
				if !known {
					return ErrInvalidInput
				}
			}
			for _, line := range lines {
				bound := strings.TrimSpace(capability.BillableBounds[line.Key])
				rate := ""
				for _, price := range in.Prices {
					if price.Key == line.Key {
						rate = strings.TrimSpace(price.Rate)
					}
				}
				if rate == "" && bound == "" {
					continue
				}
				submitted[line.Key] = modelruntime.CapabilityQuote{Key: line.Key, Rate: rate, Bound: bound}
			}
		}
		for key := range submitted {
			known := false
			for _, line := range lines {
				if line.Key == key {
					known = true
				}
			}
			if !known {
				return ErrInvalidInput
			}
		}
		rules := make([]modelruntime.UsageRule, 0, len(lines))
		bounds := map[string]string{}
		subtract := map[string][]string{}
		for _, line := range lines {
			quote, exists := submitted[line.Key]
			rate, bound := strings.TrimSpace(quote.Rate), strings.TrimSpace(quote.Bound)
			if !exists || rate == "" && bound == "" {
				if line.Optional {
					continue
				}
				return ErrInvalidInput
			}
			if rate == "" || bound == "" {
				return ErrInvalidInput
			}
			if _, e := PriceDecimal(line.Per); e != nil {
				return ErrInvalidInput
			}
			parsedRate, e := PriceDecimal(rate)
			if e != nil || parsedRate.Sign() < 0 {
				return ErrInvalidInput
			}
			parsedBound, ok := new(big.Rat).SetString(bound)
			if !ok || parsedBound.Sign() < 0 {
				return ErrInvalidInput
			}
			if previous, exists := priceByKey[line.Key]; exists && previous.Rate != rate {
				return ErrInvalidInput
			}
			priceByKey[line.Key] = ComponentRate{Key: line.Key, Unit: line.Unit, Per: line.Per, Rate: rate}
			rules = append(rules, modelruntime.UsageRule{Component: line.Key, Unit: line.Unit, Path: line.Path, ZeroWhenAbsent: line.ZeroWhenAbsent})
			bounds[line.Key] = bound
			if line.SubtractFrom != "" {
				subtract[line.SubtractFrom] = append(subtract[line.SubtractFrom], line.Path)
			}
		}
		if len(rules) == 0 {
			return ErrInvalidInput
		}
		normalized := modelruntime.Capability{Operation: capability.Operation, Streaming: capability.Streaming, UsageRules: rules, BillableBounds: bounds}
		if len(subtract) > 0 {
			normalized.SubtractPaths = subtract
		}
		out = append(out, normalized)
	}
	prices := make([]ComponentRate, 0, len(priceByKey))
	seen := map[string]bool{}
	for _, capability := range out {
		for _, rule := range capability.UsageRules {
			if seen[rule.Component] {
				continue
			}
			seen[rule.Component] = true
			prices = append(prices, priceByKey[rule.Component])
		}
	}
	in.Capabilities = out
	in.Prices = prices
	return nil
}
