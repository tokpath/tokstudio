package payment

import (
	"encoding/json"
	"net/url"
	"strings"
)

func peekPaymentOrderID(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	raw := strings.TrimSpace(string(body))
	if strings.HasPrefix(raw, "{") {
		var payload map[string]any
		if json.Unmarshal(body, &payload) == nil {
			if id := asString(payload["order_id"]); id != "" {
				return id
			}
			if id := asString(payload["out_trade_no"]); id != "" {
				return id
			}
			if data, ok := payload["data"].(map[string]any); ok {
				if obj, ok := data["object"].(map[string]any); ok {
					if meta, ok := obj["metadata"].(map[string]any); ok {
						if id := asString(meta["order_id"]); id != "" {
							return id
						}
					}
				}
			}
		}
	}
	if vals, err := url.ParseQuery(raw); err == nil {
		if id := strings.TrimSpace(vals.Get("out_trade_no")); id != "" {
			return id
		}
		if id := strings.TrimSpace(vals.Get("order_id")); id != "" {
			return id
		}
	}
	return ""
}
