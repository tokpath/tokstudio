package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func googleLiveConfigured() bool {
	return strings.TrimSpace(os.Getenv("TOKENHUB_GOOGLE_CLIENT_ID")) != "" &&
		strings.TrimSpace(os.Getenv("TOKENHUB_GOOGLE_CLIENT_SECRET")) != "" &&
		strings.TrimSpace(os.Getenv("TOKENHUB_GOOGLE_REDIRECT_URL")) != ""
}

func newOAuthEnv(t *testing.T) (*app.App, *httptest.Server) {
	t.Helper()
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Env = "test"
	cfg.PublicBaseURL = "http://localhost:8080"
	cfg.WebOrigin = "http://localhost:3000"
	cfg.GoogleClientID = ""
	cfg.GoogleClientSecret = ""
	cfg.GoogleRedirect = ""
	cfg.GoogleAllowMock = false
	cfg.BootstrapAdmin = "oauth_admin"
	cfg.BootstrapUser = "oauth_user"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	t.Cleanup(server.Close)
	return application, server
}

func TestW1OAuthStatusAndStartUnavailableWithoutTriad(t *testing.T) {
	_, server := newOAuthEnv(t)

	status := getJSON(t, server.URL+"/v1/auth/google/status", "")
	if status["available"] != false || status["configured"] != false || status["mock"] != false {
		t.Fatalf("default must be unavailable: %+v", status)
	}

	code, body := doJSON(t, http.MethodGet, server.URL+"/v1/auth/google/start?promotion_code=THC1", "", false, nil)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("start without triad: %d %+v", code, body)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "provider_unavailable" || errObj["message"] != "未配置 Google 登录" {
		t.Fatalf("structured unavailable: %+v", body)
	}
	if errObj["retryable"] != false {
		t.Fatalf("config miss is not retryable: %+v", body)
	}
	if body["session"] != nil || strings.Contains(mustJSONString(body), "mock:") {
		t.Fatalf("must not silently mock-succeed: %+v", body)
	}

	cbCode, cb := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, map[string]string{
		"state": "nope", "code": "mock:oem@example.test",
	})
	if cbCode != http.StatusServiceUnavailable {
		t.Fatalf("callback without triad: %d %+v", cbCode, cb)
	}
	if cb["session"] != nil {
		t.Fatalf("callback must not issue a session: %+v", cb)
	}
}

func TestW1OAuthStartBuildsRealGoogleURLWhenTriadPresent(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleClientID = "id.apps.googleusercontent.com"
	application.Config.GoogleClientSecret = "not-a-real-secret"
	application.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"

	status := getJSON(t, server.URL+"/v1/auth/google/status", "")
	if status["available"] != true || status["configured"] != true || status["mock"] != false {
		t.Fatalf("triad status: %+v", status)
	}

	started := getJSON(t, server.URL+"/v1/auth/google/start?promotion_code=THC1", "")
	if started["mock"] == true {
		t.Fatalf("triad start must not be mock: %+v", started)
	}
	authURL, _ := started["auth_url"].(string)
	if !strings.HasPrefix(authURL, identity.GoogleAuthorizeURL) {
		t.Fatalf("auth url: %s", authURL)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "id.apps.googleusercontent.com" {
		t.Fatalf("client_id: %s", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "https://test.tokpath.com/login/oauth/google" {
		t.Fatalf("redirect_uri: %s", q.Get("redirect_uri"))
	}
	if q.Get("state") == "" || q.Get("response_type") != "code" {
		t.Fatalf("oauth query: %v", q)
	}
	if strings.Contains(authURL, "not-a-real-secret") {
		t.Fatal("auth URL leaked client secret")
	}
}

func TestW1OAuthFakeExchangerKeepsPromotionAndHttpOnlyCookie(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleClientID = "id.apps.googleusercontent.com"
	application.Config.GoogleClientSecret = "not-a-real-secret"
	application.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"
	email := "oauth-fake-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	application.GoogleExchange = func(_ context.Context, code string) (identity.GoogleProfile, error) {
		if strings.Contains(code, "ya29") || strings.HasPrefix(code, "mock:") {
			return identity.GoogleProfile{}, errors.New("fake exchanger received a token-like code")
		}
		return identity.GoogleProfile{Subject: "google_fake_" + email, Email: email}, nil
	}

	started := getJSON(t, server.URL+"/v1/auth/google/start?promotion_code=THC1", "")
	state, _ := started["state"].(string)
	if state == "" {
		t.Fatalf("state: %+v", started)
	}

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/auth/google/callback", bytes.NewReader(mustJSON(map[string]string{
		"state": state, "code": "auth-code-from-google",
	})))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("callback %d %s", resp.StatusCode, raw)
	}
	if strings.Contains(string(raw), "auth-code-from-google") {
		t.Fatalf("response leaked authorization code: %s", raw)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if channelOf(body) != identity.OEMChannelID {
		t.Fatalf("promotion THC1 must stay on OEM: %+v", body)
	}
	user, _ := body["session"].(map[string]any)["user"].(map[string]any)
	if user["email"] != email {
		t.Fatalf("session email: %+v", body)
	}

	found := false
	for _, cookie := range resp.Cookies() {
		if cookie.Name != "tokenhub_session" {
			continue
		}
		found = true
		if !cookie.HttpOnly {
			t.Fatal("session cookie must be HttpOnly")
		}
		if cookie.Value == "" {
			t.Fatal("empty session cookie")
		}
	}
	if !found {
		t.Fatalf("missing tokenhub_session cookie: %v", resp.Header.Values("Set-Cookie"))
	}
}

func TestW1OAuthTestPreviewForbidsMockEvenIfAllowFlag(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleAllowMock = true
	application.Config.PublicBaseURL = "https://test.tokpath.com"
	application.Config.WebOrigin = "https://test.tokpath.com"

	status := getJSON(t, server.URL+"/v1/auth/google/status", "")
	if status["available"] != false || status["mock"] != false {
		t.Fatalf("test preview must forbid mock: %+v", status)
	}
	code, body := doJSON(t, http.MethodGet, server.URL+"/v1/auth/google/start", "", false, nil)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("test preview start: %d %+v", code, body)
	}
}

func TestW1OAuthAllowMockOnlyWhenExplicitAndLocal(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleAllowMock = true
	application.Config.PublicBaseURL = "http://127.0.0.1:8080"
	application.Config.WebOrigin = "http://127.0.0.1:3000"

	status := getJSON(t, server.URL+"/v1/auth/google/status", "")
	if status["available"] != true || status["mock"] != true || status["configured"] != false {
		t.Fatalf("local ALLOW_MOCK: %+v", status)
	}
	started := getJSON(t, server.URL+"/v1/auth/google/start?promotion_code=THC1", "")
	if started["mock"] != true {
		t.Fatalf("mock start: %+v", started)
	}
	email := "oauth-mock-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	finished := postBody(t, server.URL+"/v1/auth/google/callback", "", map[string]string{
		"state": started["state"].(string), "code": "mock:" + email,
	})
	if channelOf(finished) != identity.OEMChannelID {
		t.Fatalf("mock attribution: %+v", finished)
	}
}

func TestW1OAuthLiveGoogleExchange(t *testing.T) {
	if !googleLiveConfigured() {
		t.Skip("live Google requires TOKENHUB_GOOGLE_CLIENT_ID + TOKENHUB_GOOGLE_CLIENT_SECRET + TOKENHUB_GOOGLE_REDIRECT_URL")
	}
	ex := identity.NewGoogleExchange(identity.GoogleOAuthConfig{
		ClientID:     os.Getenv("TOKENHUB_GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("TOKENHUB_GOOGLE_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("TOKENHUB_GOOGLE_REDIRECT_URL"),
	})
	profile, err := ex(context.Background(), "not-a-real-google-code")
	if err == nil {
		t.Fatal("dummy code must not succeed against live Google")
	}
	if errors.Is(err, identity.ErrGoogleUnavailable) {
		t.Fatal("triad present must attempt a real exchange")
	}
	if !errors.Is(err, identity.ErrGoogleExchange) {
		t.Fatalf("live dummy code: %v", err)
	}
	if profile.Email != "" {
		t.Fatalf("live failure must not invent a profile: %+v", profile)
	}
	if strings.Contains(err.Error(), os.Getenv("TOKENHUB_GOOGLE_CLIENT_SECRET")) {
		t.Fatal("live error leaked client secret")
	}
}

func mustJSONString(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
