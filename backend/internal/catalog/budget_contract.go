package catalog

import (
	"encoding/json"
	"net/url"
	"strings"
)

// OpenAIChatBudgetCandidate describes a concrete adapter/endpoint contract, not
// an editable model capability. Official Chat max_completion_tokens includes
// reasoning; arbitrary compatible servers need their own verified contract.
func OpenAIChatBudgetCandidate(c RouteCandidate) bool {
	switch c.Adapter {
	case "bifrost", "openai", "anthropic", "openrouter", "google":
	default:
		return false
	}
	slug := strings.ToLower(c.ProviderSlug)
	for _, other := range []string{"anthropic", "claude", "gemini", "google", "openrouter"} {
		if strings.Contains(slug, other) {
			return false
		}
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return true
	} // Bifrost's concrete OpenAI default
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && u.Hostname() == "api.openai.com" && (u.Port() == "" || u.Port() == "443") && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || strings.TrimRight(u.Path, "/") == "/v1" || u.Path == "/v1/chat/completions")
}

// TextBudgetCandidate mirrors the concrete Bifrost provider selection. The
// official endpoints enforce a total output ceiling, including hidden thinking.
// OpenRouter dynamic routers and Gemini are intentionally excluded.
func TextBudgetCandidate(c RouteCandidate) bool {
	switch c.Adapter {
	case "bifrost", "openai", "anthropic", "openrouter", "google":
	default:
		return false
	}
	slug := strings.ToLower(c.ProviderSlug)
	switch {
	case strings.Contains(slug, "anthropic"), strings.Contains(slug, "claude"):
		return officialEndpoint(c.BaseURL, "api.anthropic.com", []string{"", "/v1", "/v1/messages"})
	case strings.Contains(slug, "gemini"), strings.Contains(slug, "google"):
		return false
	case strings.Contains(slug, "openrouter"):
		model := strings.ToLower(c.UpstreamModelID)
		return (strings.HasPrefix(model, "openai/") || strings.HasPrefix(model, "anthropic/")) && officialEndpoint(c.BaseURL, "openrouter.ai", []string{"", "/api/v1", "/api/v1/chat/completions"})
	default:
		return OpenAIChatBudgetCandidate(c)
	}
}
func officialEndpoint(base, host string, paths []string) bool {
	if strings.TrimSpace(base) == "" {
		return true
	}
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Scheme != "https" || u.Hostname() != host || (u.Port() != "" && u.Port() != "443") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	for _, path := range paths {
		if strings.TrimRight(u.Path, "/") == path {
			return true
		}
	}
	return false
}

// BudgetableTextPrices rejects dimensions the adapter cannot bound. This is
// applied to the effective customer price, including brand price overrides.
func BudgetableTextPrices(raw []byte) bool {
	var prices map[string]json.RawMessage
	if json.Unmarshal(raw, &prices) != nil {
		return false
	}
	allowed := map[string]bool{"input": true, "output": true, "reasoning": true, "reasoning_output": true, "currency": true, "customer_sell_input": true, "customer_sell_output": true, "customer_sell": true}
	for field, value := range prices {
		if strings.HasPrefix(field, "upstream_cost") || strings.HasPrefix(field, "wholesale") {
			continue
		}
		if !allowed[field] && string(value) != "0" && string(value) != "\"0\"" {
			return false
		}
	}
	return true
}

func BudgetableMediaPrices(raw []byte) bool {
	var prices map[string]json.RawMessage
	if json.Unmarshal(raw, &prices) != nil {
		return false
	}
	allowed := map[string]bool{"currency": true, "customer_sell": true, "image_count": true, "video_second": true, "audio_second": true}
	for field, value := range prices {
		if strings.HasPrefix(field, "upstream_cost") || strings.HasPrefix(field, "wholesale") || strings.HasSuffix(field, "_cost") {
			continue
		}
		if !allowed[field] && string(value) != "0" && string(value) != "\"0\"" {
			return false
		}
	}
	return true
}
