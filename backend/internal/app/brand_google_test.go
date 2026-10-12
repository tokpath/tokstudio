package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
)

func brandRequest(t *testing.T, method, endpoint, host, token string, payload any, state string) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(payload)
	request, _ := http.NewRequest(method, endpoint, bytes.NewReader(raw))
	request.Host = host
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if state != "" {
		addOAuthChallenge(request, state)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if value, _ := body["state"].(string); value != "" {
		for _, cookie := range response.Cookies() {
			if strings.HasPrefix(cookie.Name, "tokenhub_google_") {
				oauthChallenges.Store(value, cookie)
				t.Cleanup(func() { oauthChallenges.Delete(value) })
			}
		}
	}
	for _, cookie := range response.Cookies() {
		if cookie.Name == "tokenhub_session" && body["test_only"] == true {
			t.Fatal("configuration test created a browser session")
		}
	}
	return response.StatusCode, body
}

func TestBrandGoogleConfigurationAndAccountIsolation(t *testing.T) {
	a, server := newOAuthEnv(t)
	ctx := context.Background()
	a.Config.EncryptionKey = "isolated-google-test-encryption-key"
	a.Config.GoogleClientID = "official.apps.googleusercontent.com"
	a.Config.GoogleClientSecret = "synthetic-official-secret"
	a.Config.GoogleRedirect = "https://localhost/login/oauth/google"
	actor := identity.Principal{Roles: []string{"platform_admin"}}
	session, err := a.Identity.Login(ctx, identity.LoginInput{Email: "admin@tokenhub.local", Password: "password1"})
	if err != nil {
		t.Fatal(err)
	}
	actor.UserID = session.User.ID
	marker := strconv.FormatInt(time.Now().UnixNano(), 10)
	create := func(name string) (*identity.BrandView, string) {
		item, err := a.Identity.CreateOEM(ctx, actor, identity.OEMCreateInput{OperationID: name + marker, SalesMode: "offline", Brand: &identity.BrandInput{Name: name, PrimaryDomain: name + marker + ".example.test", APIDomain: "api." + name + marker + ".example.test", AdminDomain: "admin." + name + marker + ".example.test"}})
		if err != nil {
			t.Fatal(err)
		}
		brand, err := a.Identity.OEMGoogleBrand(ctx, item.ChannelOrgID)
		if err != nil {
			t.Fatal(err)
		}
		return brand, item.ChannelOrgID
	}
	first, channelA := create("google-a")
	second, channelB := create("google-b")
	secret := "synthetic-oem-secret"
	path := func(id string) string { return server.URL + "/admin/oem-deliveries/" + id + "/google" }
	save := func(id, client string, version int64, enabled bool, password string) (int, map[string]any) {
		return doJSON(t, http.MethodPut, path(id), session.Token, true, map[string]any{"client_id": client, "client_secret": password, "enabled": enabled, "expected_version": version})
	}
	clientA := "a-" + marker + ".apps.googleusercontent.com"
	clientB := "b-" + marker + ".apps.googleusercontent.com"
	if status, _ := save(channelA, clientA, 0, true, secret); status != 409 {
		t.Fatalf("untested enable accepted: %d", status)
	}
	if status, _ := doJSON(t, http.MethodPut, path(channelA), session.Token, false, map[string]any{}); status != 409 {
		t.Fatalf("credential write without confirmation %d", status)
	}
	status, saved := save(channelA, clientA, 0, false, secret)
	if status != 200 {
		t.Fatalf("save config %d %+v", status, saved)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), secret) || saved["item"].(map[string]any)["secret_set"] != true {
		t.Fatal("secret leaked or was not saved")
	}
	var ciphertext string
	if err := a.DB.Table("identity_google_configs").Select("secret_ciphertext").Where("brand_id=?", first.ID).Scan(&ciphertext).Error; err != nil {
		t.Fatal(err)
	}
	plain, err := crypto.Open(a.Config.EncryptionKey, ciphertext)
	if err != nil || plain != secret || ciphertext == secret {
		t.Fatal("secret must be encrypted")
	}
	if status, _ := save(channelB, clientA, 0, false, secret); status != 409 {
		t.Fatalf("shared client accepted %d", status)
	}
	if status, _ := save(channelB, a.Config.GoogleClientID, 0, false, secret); status != 409 {
		t.Fatalf("official client reused %d", status)
	}
	if status, body := save(channelB, clientB, 0, false, secret); status != 200 {
		t.Fatalf("second save %d %+v", status, body)
	}
	if err := a.DB.Callback().Create().Before("gorm:create").Register("google_fail_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("injected audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	status, _ = save(channelA, clientA, 1, false, secret+"-must-rollback")
	a.DB.Callback().Create().Remove("google_fail_audit")
	view, err := a.Identity.GoogleConfig(ctx, first.ID)
	if status != 500 || err != nil || view.Version != 1 {
		t.Fatal("credential change survived failed audit")
	}
	a.GoogleExchange = func(context.Context, string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: "global-google-" + marker, Email: "same-" + marker + "@example.test"}, nil
	}
	verify := func(brand *identity.BrandView, channel, host string) {
		uri := "https://" + host + "/login/oauth/google"
		var usersBefore, tokensBefore int64
		a.DB.Table("identity_users").Count(&usersBefore)
		a.DB.Table("identity_access_tokens").Count(&tokensBefore)
		code, ticket := doJSON(t, http.MethodPost, path(channel)+"/test", session.Token, false, map[string]string{"redirect_uri": uri})
		if code != 200 {
			t.Fatalf("test ticket %d %+v", code, ticket)
		}
		parsed, _ := url.Parse(ticket["url"].(string))
		endpoint := server.URL + "/v1/auth/google/test/start?ticket=" + url.QueryEscape(parsed.Query().Get("ticket"))
		if code, _ := brandRequest(t, http.MethodGet, endpoint, "evil.example.test", "", nil, ""); code == 200 {
			t.Fatal("test ticket worked on wrong host")
		}
		code, started := brandRequest(t, http.MethodGet, endpoint, host, "", nil, "")
		if code != 200 {
			t.Fatalf("test start %d %+v", code, started)
		}
		state := started["state"].(string)
		if code, _ := brandRequest(t, http.MethodGet, endpoint, host, "", nil, ""); code == 200 {
			t.Fatal("test ticket replay")
		}
		code, finished := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", host, "", map[string]string{"state": state, "code": "synthetic-test"}, state)
		if code != 200 || finished["test_only"] != true || finished["session"] != nil {
			t.Fatalf("test callback %d %+v", code, finished)
		}
		var usersAfter, tokensAfter int64
		a.DB.Table("identity_users").Count(&usersAfter)
		a.DB.Table("identity_access_tokens").Count(&tokensAfter)
		if usersBefore != usersAfter || tokensBefore != tokensAfter {
			t.Fatal("test login created customer identity or access token")
		}
	}
	verify(first, channelA, first.PrimaryDomain)
	verify(second, channelB, second.PrimaryDomain)
	if status, body := save(channelA, clientA, 1, true, ""); status != 200 {
		t.Fatalf("enable %d %+v", status, body)
	}
	if status, body := save(channelB, clientB, 1, true, ""); status != 200 {
		t.Fatalf("enable B %d %+v", status, body)
	}
	if status, body := brandRequest(t, http.MethodGet, server.URL+"/v1/auth/google/status", first.AdminDomain, "", nil, ""); status != 200 || body["available"] != false {
		t.Fatal("untested admin callback exposed")
	}
	verify(first, channelA, first.AdminDomain)
	if _, body := brandRequest(t, http.MethodGet, server.URL+"/v1/auth/google/status", first.AdminDomain, "", nil, ""); body["available"] != true {
		t.Fatal("tested admin callback unavailable")
	}
	begin := func(brand *identity.BrandView) map[string]any {
		code, body := brandRequest(t, http.MethodGet, server.URL+"/v1/auth/google/start?next=%2Fapp%2Fkeys&host="+second.PrimaryDomain, brand.PrimaryDomain, "", nil, "")
		if code != 200 {
			t.Fatalf("start %d %+v", code, body)
		}
		target, _ := url.Parse(body["auth_url"].(string))
		wantClient := clientA
		if brand.ID == second.ID {
			wantClient = clientB
		}
		if target.Query().Get("client_id") != wantClient || target.Query().Get("redirect_uri") != "https://"+brand.PrimaryDomain+"/login/oauth/google" {
			t.Fatal("wrong client or callback selected")
		}
		return body
	}
	started := begin(first)
	state := started["state"].(string)
	payload := map[string]string{"state": state, "code": "synthetic-login"}
	if status, _ := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", second.PrimaryDomain, "", payload, state); status == 200 {
		t.Fatal("cross-brand callback accepted")
	}
	if status, _ := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", first.AdminDomain, "", payload, state); status == 200 {
		t.Fatal("same-brand cross-host callback accepted")
	}
	status, finishedA := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", first.PrimaryDomain, "", payload, state)
	if status != 200 {
		t.Fatalf("login A %d %+v", status, finishedA)
	}
	started = begin(second)
	state = started["state"].(string)
	status, finishedB := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", second.PrimaryDomain, "", map[string]string{"state": state, "code": "synthetic-login"}, state)
	if status != 200 {
		t.Fatalf("login B %d %+v", status, finishedB)
	}
	if userIDOf(finishedA) == userIDOf(finishedB) {
		t.Fatal("Google identities shared a user across brands")
	}
	tokenA := finishedA["session"].(map[string]any)["token"].(string)
	if status, _ := brandRequest(t, http.MethodGet, server.URL+"/v1/me", second.PrimaryDomain, tokenA, nil, ""); status != 403 {
		t.Fatalf("cross-brand session accepted %d", status)
	}
	if status, _ := brandRequest(t, http.MethodGet, server.URL+"/v1/me", first.PrimaryDomain, tokenA, nil, ""); status != 200 {
		t.Fatalf("own-brand session rejected %d", status)
	}
	for _, token := range []string{tokenA, session.Token} {
		if status, _ := brandRequest(t, http.MethodGet, path(channelA), first.PrimaryDomain, token, nil, ""); status != 403 {
			t.Fatal("customer or cross-host administrator read OEM credentials")
		}
	}
	if status, _ := doJSON(t, http.MethodGet, path(channelA), a.Config.BootstrapAdmin+"-tech", false, nil); status != 200 {
		t.Fatalf("platform technician cannot configure %d", status)
	}
	started = begin(first)
	state = started["state"].(string)
	if status, body := save(channelA, clientA, 2, false, secret+"-rotated"); status != 200 {
		t.Fatalf("rotate %d %+v", status, body)
	}
	if status, _ := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", first.PrimaryDomain, "", map[string]string{"state": state, "code": "old"}, state); status == 200 {
		t.Fatal("old state survived credential rotation")
	}
	if status, _ := save(channelA, clientA, 2, false, ""); status != 409 {
		t.Fatal("stale configuration overwritten")
	}
	if _, body := brandRequest(t, http.MethodGet, server.URL+"/v1/auth/google/status", first.PrimaryDomain, "", nil, ""); body["available"] != false {
		t.Fatal("disabled OEM fell back to official credentials")
	}
	if _, body := brandRequest(t, http.MethodGet, server.URL+"/v1/auth/google/status?host=localhost", "evil.example.test", "", nil, ""); body["available"] != false {
		t.Fatal("unknown host selected official credentials")
	}

	// Password registration is independent too, and account linking requires
	// authentication to that existing account in the same brand.
	email := "password-" + marker + "@example.test"
	register := func(brand *identity.BrandView, password string) map[string]any {
		status, body := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/register", brand.PrimaryDomain, "", map[string]string{"email": email, "password": password}, "")
		if status != 201 {
			t.Fatalf("password register %d %+v", status, body)
		}
		return body
	}
	passwordA := register(first, "password-a")
	passwordB := register(second, "password-b")
	if userIDOf(passwordA) == userIDOf(passwordB) {
		t.Fatal("email registration merged brands")
	}
	if code, _ := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/login", second.PrimaryDomain, "", map[string]string{"email": email, "password": "password-a"}, ""); code == 200 {
		t.Fatal("other brand password logged in")
	}
	if code, _ := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/login", second.PrimaryDomain, "", map[string]string{"email": strings.ToUpper(email), "password": "password-b"}, ""); code != 200 {
		t.Fatal("brand email normalization failed")
	}
	a.GoogleExchange = func(context.Context, string) (identity.GoogleProfile, error) {
		return identity.GoogleProfile{Subject: "link-" + marker, Email: email}, nil
	}
	started = begin(second)
	state = started["state"].(string)
	code, body := brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", second.PrimaryDomain, "", map[string]string{"state": state, "code": "unbound"}, state)
	if code != 400 || body["error"].(map[string]any)["param"] != "account_link_required" {
		t.Fatalf("email auto-linked %d %+v", code, body)
	}
	passwordToken := passwordB["session"].(map[string]any)["token"].(string)
	code, started = brandRequest(t, http.MethodGet, server.URL+"/v1/me/google/start", second.PrimaryDomain, passwordToken, nil, "")
	if code != 200 {
		t.Fatalf("link start %d %+v", code, started)
	}
	state = started["state"].(string)
	code, body = brandRequest(t, http.MethodPost, server.URL+"/v1/auth/google/callback", second.PrimaryDomain, "", map[string]string{"state": state, "code": "link"}, state)
	if code != 200 || userIDOf(body) != userIDOf(passwordB) {
		t.Fatalf("link did not retain original account %d %+v", code, body)
	}
	var otherBindings int64
	a.DB.Table("identity_users").Where("id = ? AND google_sub IS NOT NULL", userIDOf(passwordA)).Count(&otherBindings)
	if otherBindings != 0 {
		t.Fatal("Google linking changed other brand")
	}
}
