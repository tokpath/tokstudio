package gateway

import (
	"strings"
)

const (
	SandboxFixed     = "fixed"
	SandboxContent   = "content"
	SandboxReasoning = "reasoning"
	SandboxOmit      = "omit"
)

// NormalizeSandboxMode 把请求头和 Omit-Usage 收成一种模式。
// 缺省是 fixed：8/4/12，保证现有 CI 对 echo usage 的断言不变。
func NormalizeSandboxMode(mode string, omitUsage bool) string {
	if omitUsage {
		return SandboxOmit
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SandboxContent, SandboxReasoning, SandboxOmit, SandboxFixed:
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return SandboxFixed
	}
}

// ApplySandboxUsage 按沙箱模式覆盖上游 usage，方便按 Key / 内容核对账本。
func ApplySandboxUsage(mode string, chat ChatRequest, completion string, current map[string]int) map[string]int {
	switch NormalizeSandboxMode(mode, false) {
	case SandboxOmit:
		return map[string]int{}
	case SandboxContent:
		return sandboxUsageFromContent(chat, completion, false)
	case SandboxReasoning:
		return sandboxUsageFromContent(chat, completion, true)
	default:
		if len(current) == 0 {
			return map[string]int{"prompt_tokens": 8, "completion_tokens": 4, "total_tokens": 12}
		}
		return current
	}
}

func sandboxUsageFromContent(chat ChatRequest, completion string, reasoning bool) map[string]int {
	prompt := 0
	for _, msg := range chat.Messages {
		prompt += len([]rune(msg.Content))
	}
	if prompt < 8 {
		prompt = 8
	}
	comp := len([]rune(completion))
	if comp < 4 {
		comp = 4
	}
	usage := map[string]int{
		"prompt_tokens":     prompt,
		"completion_tokens": comp,
		"total_tokens":      prompt + comp,
	}
	if reasoning {
		usage["reasoning_tokens"] = 3
		usage["total_tokens"] += 3
	}
	return usage
}

func completionText(resp ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}
