package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func TestPortalLocalizedErrorsPreserveDetailedBusinessContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		t.Run(locale, func(t *testing.T) {
			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest("POST", "/v1/auth/login", nil)
			c.Request.Header.Set("Accept-Language", locale)
			c.Set("request_id", "unchanged-request")
			handleError(c, organization.ErrInvalidInviteCode)
			var got struct {
				Error struct {
					Code, Message string
					RequestID     string `json:"request_id"`
					Retryable     bool
				}
			}
			if err := json.Unmarshal(writer.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			source := "邀请码无效，请更正或显式移除；不会自动更改经营归属"
			if writer.Code != 400 || got.Error.Code != "invalid_invite_code" || got.Error.RequestID != "unchanged-request" || got.Error.Retryable {
				t.Fatalf("business contract changed: %s", writer.Body)
			}
			if got.Error.Message != httpx.LocalizedText(locale, source) || (locale != "zh" && got.Error.Message == source) {
				t.Fatalf("lost detailed error: %s", writer.Body)
			}
		})
	}
}

func TestUpstreamFailureIsPrivateAndLocalized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		for _, status := range []int{0, 400, 401, 403, 429, 500, 504} {
			writer := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(writer)
			c.Request = httptest.NewRequest("POST", "/v1/me/playground", nil)
			c.Request.Header.Set("Accept-Language", locale)
			c.Set("request_id", "http-request")
			// Wrappers themselves may include a key or a credential-bearing URL.
			handleError(c, fmt.Errorf("provider-secret: %w", &modelruntime.UpstreamFailure{Status: status}))
			var got struct {
				Error struct {
					Code, Message string
					RequestID     string `json:"request_id"`
					Retryable     bool   `json:"retryable"`
				}
			}
			if err := json.Unmarshal(writer.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			source := "模型服务暂时不可用，请稍后重试或联系经营方，并提供请求编号"
			want := httpx.LocalizedErrorMessage(locale, "upstream_error", source)
			if writer.Code != 502 || got.Error.Code != "upstream_error" || got.Error.Message != want || got.Error.RequestID != "http-request" || got.Error.Retryable != (status == 0 || status == 408 || status >= 500) {
				t.Fatal(locale, status, writer.Code, writer.Body.String())
			}
			if strings.Contains(writer.Body.String(), "provider-secret") || (locale != "zh" && got.Error.Message == source) {
				t.Fatal("supplier text escaped or translation missing", locale)
			}
		}
	}
}

func TestPortalLocaleThroughRealAuthAndVerificationMail(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	address, _ := url.Parse(f.server.URL)
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		t.Run(locale, func(t *testing.T) {
			f.client.Jar.SetCookies(address, []*http.Cookie{{Name: "NEXT_LOCALE", Value: locale, Path: "/"}})
			status, body, _ := f.call(t, "POST", "/v1/auth/login", map[string]any{"kind": "staff", "email": "owner@example.test", "password": "wrong-password"}, uuid.NewString())
			failure := body["error"].(map[string]any)
			if status != 401 || failure["code"] != "invalid_credentials" || failure["message"] != httpx.LocalizedText(locale, "邮箱或密码错误，或此身份不可用") {
				t.Fatalf("real authentication response %d: %v", status, body)
			}
			status, body, _ = f.call(t, "POST", "/v1/auth/registrations", map[string]any{"email": locale + "@locale.example.test", "password": "customer-password-test", "display_name": "客户"}, uuid.NewString())
			mustOK(t, status, body)
			if err := f.a.RunMailJobs(context.Background(), "locale-mail", 1); err != nil {
				t.Fatal(err)
			}
			f.box.mu.Lock()
			message := f.box.messages[len(f.box.messages)-1]
			f.box.mu.Unlock()
			if message.Subject != httpx.LocalizedText(locale, "验证邮箱并完成注册") || !strings.Contains(message.Text, "/verify-email?token=") || !strings.Contains(message.Text, httpx.LocalizedText(locale, "\n链接过期后请重新发起。若非本人操作，请忽略。")) {
				t.Fatal("verification language or opaque verification link changed")
			}
		})
	}
}
