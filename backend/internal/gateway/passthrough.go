package gateway

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	FactSourceSandbox = "sandbox"
	FactSourceLive    = "live"
)

// PassthroughMeta 是每次调用必须带进 Bifrost、再写回 TokenHub 的不可变账务键。
// 缺字段保持空串，调用方不得填估算值。
type PassthroughMeta struct {
	RequestID     string `json:"request_id,omitempty"`
	AttemptID     string `json:"attempt_id,omitempty"`
	UserID        string `json:"user_id,omitempty"`
	APIKeyID      string `json:"api_key_id,omitempty"`
	ChannelOrgID  string `json:"channel_org_id,omitempty"`
	PublicModelID string `json:"public_model_id,omitempty"`
	FactSource    string `json:"fact_source,omitempty"`
}

func (m PassthroughMeta) Map() map[string]string {
	out := map[string]string{}
	put := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		out[key] = value
	}
	put("request_id", m.RequestID)
	put("attempt_id", m.AttemptID)
	put("user_id", m.UserID)
	put("api_key_id", m.APIKeyID)
	put("channel_org_id", m.ChannelOrgID)
	put("public_model_id", m.PublicModelID)
	put("fact_source", m.FactSource)
	return out
}

func passthroughFromContext(ctx context.Context) PassthroughMeta {
	return PassthroughMeta{
		RequestID:     contextString(ctx, ctxRequestIDKey),
		AttemptID:     contextString(ctx, ctxAttemptIDKey),
		UserID:        contextString(ctx, ctxUserIDKey),
		APIKeyID:      contextString(ctx, ctxAPIKeyIDKey),
		ChannelOrgID:  contextString(ctx, ctxChannelIDKey),
		PublicModelID: contextString(ctx, ctxPublicModelKey),
	}
}

func normalizeFactSource(raw string, _ bool) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case FactSourceSandbox, "echo":
		// 仍识别历史 sandbox/echo 标签，便于徽章诚实展示；生产路径不再主动写入。
		return FactSourceSandbox
	case FactSourceLive, "upstream":
		return FactSourceLive
	default:
		return ""
	}
}

func usageTokenPtrs(usage map[string]int) (prompt, completion, total *int) {
	if len(usage) == 0 {
		return nil, nil, nil
	}
	if v, ok := usage["prompt_tokens"]; ok {
		p := v
		prompt = &p
	}
	if v, ok := usage["completion_tokens"]; ok {
		c := v
		completion = &c
	}
	if v, ok := usage["total_tokens"]; ok {
		t := v
		total = &t
	}
	return prompt, completion, total
}

func metadataJSON(meta map[string]string) []byte {
	if len(meta) == 0 {
		return nil
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	return raw
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// resolveAttemptUsage 决定写入账本的 usage。
// 只透传上游 usage 或按 omit 清空；禁止按内容估算或固定 8/4/12 填空。
func resolveAttemptUsage(_, mode string, _ ChatRequest, _ string, current map[string]int) map[string]int {
	if NormalizeUsageMode(mode, false) == UsageOmit {
		return map[string]int{}
	}
	if current == nil {
		return map[string]int{}
	}
	return current
}
