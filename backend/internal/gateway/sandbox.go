package gateway

import (
	"strings"
)

const (
	// UsageOmit 表示请求方要求不写入 usage（账本 pending），不再提供固定/内容估算填空。
	UsageOmit = "omit"
)

// NormalizeUsageMode 把请求头和 Omit-Usage 收成一种模式。
// 仅支持 omit；其余一律透传上游 usage，禁止 8/4/12 等沙箱填空。
func NormalizeUsageMode(mode string, omitUsage bool) string {
	if omitUsage {
		return UsageOmit
	}
	if strings.EqualFold(strings.TrimSpace(mode), UsageOmit) {
		return UsageOmit
	}
	return ""
}

// NormalizeSandboxMode 保留旧名，供迁移期调用方编译；行为同 NormalizeUsageMode。
func NormalizeSandboxMode(mode string, omitUsage bool) string {
	return NormalizeUsageMode(mode, omitUsage)
}

func completionText(resp ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}
