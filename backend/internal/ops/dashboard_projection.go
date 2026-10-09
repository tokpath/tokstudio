package ops

import "encoding/json"

// Omit unavailable numbers instead of serializing default zero values.
func (d *Dashboard) SafeTotals() map[string]any {
	raw, _ := json.Marshal(d.Totals)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	if d.ModuleErrors["finance"] != "" {
		for _, key := range []string{"revenue_minor", "upstream_cost_minor", "wholesale_minor", "commission_liability_minor", "refund_minor", "gross_profit_minor", "pending_reconciliation_count", "low_balance_wallets", "wallet_reserved_minor", "channel_spend_minor", "preauth_failed", "prompt_tokens", "completion_tokens", "reasoning_tokens", "video_seconds", "image_count", "audio_seconds"} {
			delete(out, key)
		}
	}
	if d.ModuleErrors["traffic"] != "" {
		for _, key := range []string{"success_rate", "latency_p50_ms", "latency_p95_ms", "latency_p99_ms", "fallbacks", "http_429", "http_5xx", "upstream_errors", "timeouts"} {
			delete(out, key)
		}
	}
	if d.ModuleErrors["callbacks"] != "" {
		delete(out, "callback_latency_p95_ms")
	}
	if d.ModuleErrors["errors"] != "" {
		delete(out, "error_codes")
	}
	return out
}
func (d *Dashboard) SafeView() map[string]any {
	raw, _ := json.Marshal(d)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	out["totals"] = d.SafeTotals()
	for _, key := range []string{"alerts", "canary", "runbooks", "thresholds"} {
		if d.ModuleErrors[key] != "" {
			delete(out, key)
		}
	}
	if d.ModuleErrors["backup"] != "" {
		delete(out, "last_backup_drill")
	}
	return out
}
