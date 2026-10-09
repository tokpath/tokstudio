package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"sync"
	"testing"
)

var oauthChallenges sync.Map

func startOAuthJSON(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	state, _ := body["state"].(string)
	for _, cookie := range response.Cookies() {
		if strings.HasPrefix(cookie.Name, "tokenhub_google_") {
			oauthChallenges.Store(state, cookie)
			t.Cleanup(func() { oauthChallenges.Delete(state) })
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 900 {
				t.Fatal("unsafe OAuth browser challenge cookie")
			}
		}
	}
	return body
}
func addOAuthChallenge(req *http.Request, state string) {
	if cookie, ok := oauthChallenges.Load(state); ok {
		req.AddCookie(cookie.(*http.Cookie))
	}
}
func callbackOAuthJSON(t *testing.T, endpoint string, payload map[string]string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	addOAuthChallenge(req, payload["state"])
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	json.NewDecoder(response.Body).Decode(&body)
	return response.StatusCode, body
}

func TestOAuthBrowserBindingAndAccountConflicts(t *testing.T) {
	a, server := newOAuthEnv(t)
	a.Config.GoogleClientID = "test-client"
	a.Config.GoogleClientSecret = "test-secret"
	a.Config.GoogleRedirect = "https://test.tokpath.com/login/oauth/google"
	email := "browser-" + t.Name() + "-" + crypto.HashToken(server.URL)[:12] + "@example.test"
	subject := "subject-" + email
	a.GoogleExchange = func(context.Context, string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: subject, Email: email}, nil
	}
	started := startOAuthJSON(t, server.URL+"/v1/auth/google/start?next=%2Fapp%2Freferral&promotion_code=THA1")
	state := started["state"].(string)
	payload := map[string]string{"state": state, "code": "synthetic"}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, payload); code != 400 {
		t.Fatalf("unbound callback %d", code)
	}
	code, body := callbackOAuthJSON(t, server.URL+"/v1/auth/google/callback", payload)
	if code != 200 {
		t.Fatalf("bound callback %d %+v", code, body)
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/v1/auth/google/callback", "", false, payload); code != 400 {
		t.Fatalf("unbound replay %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/v1/auth/login", "", false, map[string]string{"email": email, "password": "oauth-" + crypto.HashToken(subject)[:16] + "Xx"}); code == 200 {
		t.Fatal("public Google subject must not derive a usable password")
	}
	// A different Google subject cannot claim an email already bound to another subject.
	a.GoogleExchange = func(context.Context, string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: subject + "-foreign", Email: email}, nil
	}
	conflict := startOAuthJSON(t, server.URL+"/v1/auth/google/start")
	if code, body := callbackOAuthJSON(t, server.URL+"/v1/auth/google/callback", map[string]string{"state": conflict["state"].(string), "code": "foreign"}); code != 400 || body["session"] != nil {
		t.Fatalf("account conflict %d %+v", code, body)
	}
	// Failed account linking cannot silently issue a session or persist a binding.
	linkEmail := "link-" + email
	registered := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": linkEmail, "password": "password1"})
	a.GoogleExchange = func(context.Context, string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: "link-subject-" + email, Email: linkEmail}, nil
	}
	link := startOAuthJSON(t, server.URL+"/v1/auth/google/start")
	callback := "oauth-test-link-failure"
	if err := a.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_users" {
			tx.AddError(errors.New("injected binding failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	status, failed := callbackOAuthJSON(t, server.URL+"/v1/auth/google/callback", map[string]string{"state": link["state"].(string), "code": "link"})
	a.DB.Callback().Update().Remove(callback)
	if status < 400 || failed["session"] != nil {
		t.Fatalf("binding failure ignored %d %+v", status, failed)
	}
	var linked int64
	if err := a.DB.Table("identity_users").Where("id = ? AND google_sub IS NOT NULL", userIDOf(registered)).Count(&linked).Error; err != nil || linked != 0 {
		t.Fatalf("partial binding persisted %d %v", linked, err)
	}
}
