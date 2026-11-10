package httpx

import (
	"strings"
	"testing"
	"unicode"
)

func TestSystemMailTranslatesDynamicMessagesWithoutChangingFacts(t *testing.T) {
	for _, locale := range []string{"en", "ja", "ru"} {
		source := "这笔提现申请已被拒绝，占用已释放。 本次申请金额 15.000000000000000001 USD。拒绝理由：Account unavailable。"
		got := LocalizedSystemMessage(locale, source)
		if got == source || !strings.Contains(got, "15.000000000000000001 USD") || !strings.Contains(got, "Account unavailable") || strings.Contains(got, "{amount}") {
			t.Fatal(locale, got)
		}
		if locale != "ja" {
			for _, r := range got {
				if unicode.Is(unicode.Han, r) {
					t.Fatal("untranslated system copy", locale, got)
				}
			}
		}
		source = "公开模型 Example 的价格或使用条件将于 2026-11-11T12:00:00Z 生效。通知已在提交时发送，生效时间按填写执行。已受理请求继续按原快照结算。"
		got = LocalizedSystemMessage(locale, source)
		if got == source || !strings.Contains(got, "Example") || !strings.Contains(got, "2026-11-11T12:00:00Z") {
			t.Fatal(locale, got)
		}
	}
}
