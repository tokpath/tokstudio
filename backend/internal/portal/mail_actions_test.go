package portal

import (
	"errors"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func TestMailTestErrorsDoNotExposeSMTPResponses(t *testing.T) {
	for _, err := range []error{mail.ErrUnavailable, &net.DNSError{Err: "secret host credential"}, &textproto.Error{Code: 535, Msg: "secret credential"}, &textproto.Error{Code: 550, Msg: "secret recipient"}, errors.New("secret token TLS error")} {
		got := mailTestFailure(err)
		if strings.Contains(got, "secret") || got == "" {
			t.Fatal(got)
		}
		for _, locale := range []string{"en", "ja", "ru"} {
			if httpx.LocalizedText(locale, got) == got {
				t.Fatal("untranslated error", locale, got)
			}
		}
	}
}

func TestPostgresMailTestUsesCurrentAccountBrandAndIdempotency(t *testing.T) {
	f := setupPortal(t)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	u, _ := url.Parse(f.server.URL)
	f.client.Jar.SetCookies(u, []*http.Cookie{{Name: "NEXT_LOCALE", Value: "ru", Path: "/"}})
	op := f.a.Installation.OperatorID
	status, out, _ := f.call(t, "POST", "/v1/auth/language", map[string]any{"operator_id": op, "language": "ru"}, "")
	mustOK(t, status, out)
	pref, err := f.a.Operations.Preferences(t.Context(), f.a.Installation.OwnerStaffID, op)
	if err != nil || pref.Language != "ru" {
		t.Fatal(pref, err)
	}
	path := "/v1/operators/" + op + "/mail/test"
	status, page, _ := f.call(t, "GET", "/v1/operators/"+op+"/mail", nil, "")
	mustOK(t, status, page)
	operationFields(t, page, "test")
	key := uuid.NewString()
	for i := 0; i < 2; i++ {
		status, out, _ = f.call(t, "POST", path, map[string]any{}, key)
		mustOK(t, status, out)
		if out["status"] != "accepted" {
			t.Fatal(out)
		}
	}
	f.box.mu.Lock()
	messages := append([]mail.Message{}, f.box.messages...)
	f.box.mu.Unlock()
	if len(messages) != 1 || messages[0].To != "owner@example.test" || messages[0].BrandID != f.a.Installation.BrandID || messages[0].Subject != httpx.LocalizedText("ru", "邮件服务测试") {
		t.Fatal(messages)
	}
	f.a.Mail.SetSender(nil)
	status, out, _ = f.call(t, "POST", path, map[string]any{}, uuid.NewString())
	if status != 400 {
		t.Fatal("unavailable service reported success", status, out)
	}
}
