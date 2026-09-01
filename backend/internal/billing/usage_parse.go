package billing

import (
	"encoding/json"
	"strconv"
)

// ParseUnitUsage 从 usage 快照 JSON 读出 token。缺字段当 0，方便列表和 CSV 直接展示。
func ParseUnitUsage(raw json.RawMessage) (prompt, completion, reasoning int64) {
	if len(raw) == 0 {
		return 0, 0, 0
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return 0, 0, 0
	}
	return jsonInt64(m, "prompt_tokens"), jsonInt64(m, "completion_tokens"), jsonInt64(m, "reasoning_tokens")
}

func jsonInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}
