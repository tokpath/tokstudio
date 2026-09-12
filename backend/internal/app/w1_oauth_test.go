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

func TestW1OAuthExchangeFailureAvoidsBadGateway(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleClientID = "id.apps.googleusercontent.com"
	application.Config.GoogleClientSecret = "not-a-real-secret"
	application.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"

	t.Run("invalid_grant", func(t *testing.T) {
		application.GoogleExchange = func(_ context.Context, _ string) (identity.GoogleProfile, error) {
			return identity.GoogleProfile{}, identity.NewGoogleExchangeError("invalid_grant")
		}
		started := getJSON(t, server.URL+"/v1/auth/google/start", "")
		state, _ := started["state"].(string)
		code, body := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, map[string]string{
			"state": state, "code": "auth-code-used",
		})
		if code == http.StatusBadGateway {
			t.Fatalf("must not use 502 (Cloudflare masks body): %d %+v", code, body)
		}
		if code != http.StatusBadRequest {
			t.Fatalf("invalid_grant status: %d %+v", code, body)
		}
		errObj, _ := body["error"].(map[string]any)
		if errObj["code"] != "provider_unavailable" || errObj["param"] != "invalid_grant" {
			t.Fatalf("structured exchange error: %+v", body)
		}
	})

	t.Run("network_error", func(t *testing.T) {
		application.GoogleExchange = func(_ context.Context, _ string) (identity.GoogleProfile, error) {
			return identity.GoogleProfile{}, identity.NewGoogleExchangeError("network_error")
		}
		started := getJSON(t, server.URL+"/v1/auth/google/start", "")
		state, _ := started["state"].(string)
		code, body := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, map[string]string{
			"state": state, "code": "auth-code-net",
		})
		if code == http.StatusBadGateway {
			t.Fatalf("must not use 502 (Cloudflare masks body): %d %+v", code, body)
		}
		if code != http.StatusServiceUnavailable {
			t.Fatalf("network_error status: %d %+v", code, body)
		}
		errObj, _ := body["error"].(map[string]any)
		if errObj["param"] != "network_error" {
			t.Fatalf("structured network error: %+v", body)
		}
	})
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
		if cookie.Secure {
			t.Fatal("http PublicBaseURL must not force Secure")
		}
		if cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("SameSite want Lax, got %v", cookie.SameSite)
		}
	}
	if !found {
		t.Fatalf("missing tokenhub_session cookie: %v", resp.Header.Values("Set-Cookie"))
	}

	meCode, me := doJSON(t, http.MethodGet, server.URL+"/v1/me", body["session"].(map[string]any)["token"].(string), false, nil)
	if meCode != http.StatusOK || me["user"] == nil {
		t.Fatalf("cookie session must read /v1/me: %d %+v", meCode, me)
	}
}

func TestW1OAuthSessionCookieSecureWhenPublicHTTPS(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.PublicBaseURL = "https://test.tokpath.com"
	application.Config.WebOrigin = "https://test.tokpath.com"
	application.Config.Env = "development"
	application.Config.GoogleClientID = "id.apps.googleusercontent.com"
	application.Config.GoogleClientSecret = "not-a-real-secret"
	application.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"
	email := "oauth-secure-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	application.GoogleExchange = func(_ context.Context, _ string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: "google_secure_" + email, Email: email}, nil
	}

	started := getJSON(t, server.URL+"/v1/auth/google/start", "")
	state, _ := started["state"].(string)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/auth/google/callback", bytes.NewReader(mustJSON(map[string]string{
		"state": state, "code": "auth-code-secure",
	})))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("callback %d %s", resp.StatusCode, raw)
	}
	var secureCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "tokenhub_session" {
			secureCookie = cookie
			break
		}
	}
	if secureCookie == nil {
		t.Fatalf("missing tokenhub_session: %v", resp.Header.Values("Set-Cookie"))
	}
	if !secureCookie.Secure {
		t.Fatal("https PublicBaseURL must set Secure even when ENV=development")
	}
	if !secureCookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly")
	}
	if secureCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("SameSite want Lax, got %v", secureCookie.SameSite)
	}

	anon := mustStatusJSON(t, http.MethodGet, server.URL+"/v1/me", "", nil)
	if anon != http.StatusUnauthorized {
		t.Fatalf("anonymous /v1/me must be 401 未登录, got %d", anon)
	}
}

func TestW1OAuthDoubleCallbackIsIdempotent(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleClientID = "id.apps.googleusercontent.com"
	application.Config.GoogleClientSecret = "not-a-real-secret"
	application.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"
	email := "oauth-dup-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	var exchanges int
	application.GoogleExchange = func(_ context.Context, code string) (identity.GoogleProfile, error) {
		exchanges++
		return identity.GoogleProfile{Subject: "google_dup_" + email, Email: email}, nil
	}

	started := getJSON(t, server.URL+"/v1/auth/google/start?promotion_code=THC1", "")
	state, _ := started["state"].(string)
	if state == "" {
		t.Fatalf("state: %+v", started)
	}

	payload := map[string]string{"state": state, "code": "auth-code-once"}
	firstCode, first := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, payload)
	if firstCode != http.StatusOK {
		t.Fatalf("first callback: %d %+v", firstCode, first)
	}
	if first["session"] == nil {
		t.Fatalf("first must issue session: %+v", first)
	}

	secondCode, second := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, payload)
	if secondCode != http.StatusOK {
		t.Fatalf("second callback must be idempotent success, got %d %+v", secondCode, second)
	}
	if second["session"] == nil {
		t.Fatalf("second must replay session: %+v", second)
	}
	if second["idempotent"] != true {
		t.Fatalf("second should mark idempotent: %+v", second)
	}
	firstUser, _ := first["session"].(map[string]any)["user"].(map[string]any)
	secondUser, _ := second["session"].(map[string]any)["user"].(map[string]any)
	if firstUser["email"] != email || secondUser["email"] != email {
		t.Fatalf("users: first=%+v second=%+v", firstUser, secondUser)
	}
	if exchanges != 1 {
		t.Fatalf("Google exchange must run once, got %d", exchanges)
	}
}

func TestW1OAuthConcurrentDoubleCallback(t *testing.T) {
	application, server := newOAuthEnv(t)
	application.Config.GoogleClientID = "id.apps.googleusercontent.com"
	application.Config.GoogleClientSecret = "not-a-real-secret"
	application.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"
	email := "oauth-race-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	var exchanges int
	application.GoogleExchange = func(_ context.Context, code string) (identity.GoogleProfile, error) {
		exchanges++
		time.Sleep(80 * time.Millisecond)
		return identity.GoogleProfile{Subject: "google_race_" + email, Email: email}, nil
	}

	started := getJSON(t, server.URL+"/v1/auth/google/start?promotion_code=THC1", "")
	state, _ := started["state"].(string)
	payload := mustJSON(map[string]string{"state": state, "code": "auth-code-race"})

	type result struct {
		code int
		body map[string]any
	}
	ch := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/auth/google/callback", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				ch <- result{code: 0, body: map[string]any{"err": err.Error()}}
				return
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			ch <- result{code: resp.StatusCode, body: body}
		}()
	}

	a, b := <-ch, <-ch
	if a.code != http.StatusOK || b.code != http.StatusOK {
		t.Fatalf("both callbacks must succeed: a=%d %+v b=%d %+v", a.code, a.body, b.code, b.body)
	}
	if a.body["session"] == nil || b.body["session"] == nil {
		t.Fatalf("both must carry session: a=%+v b=%+v", a.body, b.body)
	}
	if exchanges != 1 {
		t.Fatalf("Google exchange must run once under race, got %d", exchanges)
	}
}

func TestW1OAuthMockPathRemoved(t *testing.T) {
	_, server := newOAuthEnv(t)

	status := getJSON(t, server.URL+"/v1/auth/google/status", "")
	if status["available"] != false || status["mock"] != false {
		t.Fatalf("without triad must stay unavailable, no mock: %+v", status)
	}
	code, body := doJSON(t, http.MethodGet, server.URL+"/v1/auth/google/start", "", false, nil)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("start without triad: %d %+v", code, body)
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
