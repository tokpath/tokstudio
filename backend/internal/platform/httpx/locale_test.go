package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLocaleMatchesInterfaceSelection(t *testing.T) {
	for _, test := range []struct{ cookie, accept, want string }{
		{"en", "ja-JP,zh;q=0.9", "en"},
		{"ru-RU", "ja-JP,en;q=0.9", "ru"},
		{"", "ru-RU;q=0.9,en;q=0.2", "ru"},
		{"", "ru;q=NaN,ja;q=0.5", "ja"},
		{"ja", "en-US,en;q=0.9", "ja"},
		{"zh", "en-US", "zh"},
		{"", "zh;q=0,en-US;q=0.8,ja;q=0.5", "en"},
		{"auto", "en;q=0.2,ja-JP;q=0.9", "ja"},
		{"fr", "fr-FR,en;q=0.7", "en"},
		{"", "ja;q=0,en;q=bad", "zh"},
		{"", "", "zh"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Language", test.accept)
		if test.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "NEXT_LOCALE", Value: test.cookie})
		}
		if got := RequestLocale(req); got != test.want {
			t.Fatalf("cookie=%q accept=%q: got %s, want %s", test.cookie, test.accept, got, test.want)
		}
	}
}

func TestLocalizedErrorsPreserveMachineContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		t.Run(locale, func(t *testing.T) {
			r := gin.New()
			r.GET("/validation", func(c *gin.Context) {
				c.Set(ContextRequestID, "request-locale-test")
				AbortParam(c, http.StatusConflict, "invalid_request", "邮箱已注册", map[string]string{"field": "email"}, false)
			})
			req := httptest.NewRequest(http.MethodGet, "/validation", nil)
			req.AddCookie(&http.Cookie{Name: "NEXT_LOCALE", Value: locale})
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			var response struct {
				Error APIError `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusConflict || response.Error.Code != "invalid_request" || response.Error.RequestID != "request-locale-test" || response.Error.Retryable || response.Error.Param.(map[string]any)["field"] != "email" {
				t.Fatalf("locale changed the error contract: %+v", response)
			}
			want := map[string]string{"zh": "邮箱已注册", "en": "This email is already registered for this brand.", "ja": "このメールはこのブランドで登録済みです。", "ru": errorCatalog.Messages["邮箱已注册"][2]}[locale]
			if response.Error.Message != want {
				t.Fatalf("got %q, want %q", response.Error.Message, want)
			}
		})
	}
}

func TestNewErrorMessagesUseLocalizedCodeFallback(t *testing.T) {
	for index, locale := range []string{"en", "ja", "ru"} {
		if got := localizedErrorMessage(locale, "insufficient_balance", "新的中文余额提示"); got != errorCatalog.Codes["insufficient_balance"][index] {
			t.Fatal("untranslated error escaped the localized fallback")
		}
	}
	if got := localizedErrorMessage("zh", "insufficient_balance", "原始提示"); got != "原始提示" {
		t.Fatal("Chinese error text changed")
	}
}

func TestClosureRequirementsTranslateWithoutChangingBalances(t *testing.T) {
	amount := "0.000000000000000001"
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		text := LocalizedErrorMessage(locale, "unmet_requirements", "仍有未满足事项：frozen commission USD: "+amount+"；all requested, approved and uncertain withdrawals must be resolved")
		want := LocalizedText(locale, "仍有未满足事项：") + strings.ReplaceAll(LocalizedText(locale, "frozen commission USD: {amount}"), "{amount}", amount) + "\n" + LocalizedText(locale, "all requested, approved and uncertain withdrawals must be resolved")
		if text != want || !strings.Contains(text, amount) {
			t.Fatalf("locale %s lost a detailed requirement or exact amount: %q", locale, text)
		}
		if locale != "en" && strings.Contains(text, "frozen commission") {
			t.Fatalf("untranslated requirement in %s", locale)
		}
	}
}
