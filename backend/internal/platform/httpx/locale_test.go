package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestLocaleMatchesInterfaceSelection(t *testing.T) {
	for _, test := range []struct{ cookie, accept, want string }{
		{"en", "ja-JP,zh;q=0.9", "en"},
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
	for _, locale := range []string{"zh", "en", "ja"} {
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
			want := map[string]string{"zh": "邮箱已注册", "en": "This email is already registered for this brand.", "ja": "このメールはこのブランドで登録済みです。"}[locale]
			if response.Error.Message != want {
				t.Fatalf("got %q, want %q", response.Error.Message, want)
			}
		})
	}
}

func TestNewErrorMessagesUseLocalizedCodeFallback(t *testing.T) {
	for _, locale := range []string{"en", "ja"} {
		index := 0
		if locale == "ja" {
			index = 1
		}
		if got := localizedErrorMessage(locale, "insufficient_balance", "新的中文余额提示"); got != errorCatalog.Codes["insufficient_balance"][index] {
			t.Fatal("untranslated error escaped the localized fallback")
		}
	}
	if got := localizedErrorMessage("zh", "insufficient_balance", "原始提示"); got != "原始提示" {
		t.Fatal("Chinese error text changed")
	}
}
