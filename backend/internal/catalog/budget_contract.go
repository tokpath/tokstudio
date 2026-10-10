package catalog

import (
	"encoding/json"
	"math/big"
	"strings"
)

// Estimate support follows the deployed adapter registry, not a vendor/host
// whitelist. Admission is an estimate; reliable actual usage always settles.
func TextBudgetCandidate(c RouteCandidate) bool {
	switch strings.ToLower(strings.TrimSpace(c.Adapter)) {
	case "test", "bifrost", "openai", "anthropic", "openrouter", "google", "gemini":
		return true
	default:
		return false
	}
}

func BudgetableTextPrices(raw []byte) bool  { return estimablePrices(raw, false) }
func BudgetableMediaPrices(raw []byte) bool { return estimablePrices(raw, true) }

// Unknown charged dimensions cannot be ignored to manufacture a cheap estimate.
// Supplier/wholesale terms are snapshots, not customer billing dimensions.
func estimablePrices(raw []byte, media bool) bool {
	var prices map[string]any
	if json.Unmarshal(raw, &prices) != nil || len(prices) == 0 {
		return false
	}
	allowed := map[string]bool{"input": true, "output": true, "reasoning": true, "reasoning_output": true, "customer_sell_input": true, "customer_sell_output": true}
	if media {
		for _, key := range []string{"image_count", "video_second", "audio_second"} {
			allowed[key] = true
		}
	}
	found := false
	for field, value := range prices {
		if field == "currency" {
			if value != "USD" {
				return false
			}
			continue
		}
		if strings.HasPrefix(field, "upstream_cost") || strings.HasPrefix(field, "wholesale") || strings.HasPrefix(field, "channel_customer") || strings.HasSuffix(field, "_cost") || field == "customer_sell" {
			continue
		}
		text, ok := value.(string)
		if !ok {
			if n, ok := value.(float64); ok {
				raw, _ := json.Marshal(n)
				text = string(raw)
			} else {
				return false
			}
		}
		rate, ok := new(big.Rat).SetString(text)
		if !ok || rate.Sign() < 0 {
			return false
		}
		if !allowed[field] && rate.Sign() != 0 {
			return false
		}
		if allowed[field] {
			found = true
		}
	}
	return found
}
