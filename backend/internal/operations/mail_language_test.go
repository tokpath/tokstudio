package operations

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

type languageMailBox struct{ messages []mail.Message }

func (b *languageMailBox) Send(_ context.Context, m mail.Message) error {
	b.messages = append(b.messages, m)
	return nil
}

func TestNotificationMailLanguagePreservesConsentScopeAndAuthoredContent(t *testing.T) {
	s, _, _, operator, customer := opsFixture(t)
	ctx := t.Context()
	box := &languageMailBox{}
	s.Mail = box
	if _, err := s.SetPreferences(ctx, customer, operator, true, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMailLanguage(ctx, customer, uuid.NewString(), "en"); err == nil {
		t.Fatal("other operator accepted")
	}
	if err := s.SetMailLanguage(ctx, customer, operator, "xx"); err == nil {
		t.Fatal("invalid language accepted")
	}
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		if err := s.SetMailLanguage(ctx, customer, operator, locale); err != nil {
			t.Fatal(err)
		}
		pref, err := s.Preferences(ctx, customer, operator)
		if err != nil || !pref.Marketing || pref.UsageEmail || pref.Language != locale {
			t.Fatal("language changed consent", pref, err)
		}
		source := Notification{OperatorID: operator, RecipientID: customer, BusinessKey: "commerce:" + locale, Category: "mandatory", Severity: "info", Title: "邮件服务测试", Body: "这是一封测试邮件。收到此邮件，说明当前品牌的邮件服务已能向你的邮箱发送邮件。"}
		notice, err := s.Emit(ctx, nil, source)
		if err != nil {
			t.Fatal(err)
		}
		if notice.Title != source.Title || notice.Body != source.Body {
			t.Fatal("stored notification source changed", notice)
		}
		if deliveries, err := s.RunNotifications(ctx, 1); err != nil || len(deliveries) != 1 || deliveries[0].State != "accepted" {
			t.Fatal(deliveries, err)
		}
		got := box.messages[len(box.messages)-1]
		if got.Subject != httpx.LocalizedText(locale, source.Title) || got.Text != httpx.LocalizedText(locale, source.Body) {
			t.Fatal(locale, got)
		}
	}
	// Updating subscriptions must preserve the language selected in the UI.
	if _, err := s.SetPreferences(ctx, customer, operator, false, true); err != nil {
		t.Fatal(err)
	}
	pref, err := s.Preferences(ctx, customer, operator)
	if err != nil || pref.Language != "ru" {
		t.Fatal(pref, err)
	}
	authored := "付款已完成。This is operator-authored text."
	if _, err = s.Emit(ctx, nil, Notification{OperatorID: operator, RecipientID: customer, BusinessKey: "notice:authored", Category: "mandatory", Severity: "info", Title: "付款已完成", Body: authored}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RunNotifications(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if got := box.messages[len(box.messages)-1]; got.Subject != "付款已完成" || !strings.Contains(got.Text, authored) {
		t.Fatal("authored content translated", got)
	}
}
