package app_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestW1UserShellLogoutRevokesSession(t *testing.T) {
	_, server := newOAuthEnv(t)
	email := "shell-logout-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postJSONRaw(t, server.URL+"/v1/auth/register", "", map[string]any{
		"email": email, "password": "password1", "promotion_code": "THA1",
	})
	token := tokenOf(reg)
	if token == "" {
		t.Fatalf("register session: %+v", reg)
	}
	me := getAuthJSON(t, server.URL+"/v1/me", token)
	user, _ := me["user"].(map[string]any)
	if user["email"] != email {
		t.Fatalf("me before logout: %+v", me)
	}
	methods, _ := user["login_methods"].([]any)
	if len(methods) == 0 {
		t.Fatalf("login_methods must come from real columns: %+v", user)
	}

	code, body := doJSON(t, http.MethodPost, server.URL+"/v1/auth/logout", token, false, nil)
	if code != http.StatusOK || body["ok"] != true {
		t.Fatalf("logout: %d %+v", code, body)
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/v1/me", token, nil) != http.StatusForbidden {
		t.Fatalf("revoked session must not read /v1/me")
	}
}

func TestW1UserShellMeLoginMethodsAreReal(t *testing.T) {
	_, server := newOAuthEnv(t)
	email := "shell-me-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postJSONRaw(t, server.URL+"/v1/auth/register", "", map[string]any{
		"email": email, "password": "password1", "promotion_code": "THA1",
	})
	user := getAuthJSON(t, server.URL+"/v1/me", tokenOf(reg))["user"].(map[string]any)
	if user["display_name"] != "" {
		t.Fatalf("new local user display_name must be empty, not invented: %+v", user)
	}
	methods, _ := user["login_methods"].([]any)
	if len(methods) != 1 || methods[0] != "password" {
		t.Fatalf("local register is password only: %+v", user)
	}
	if user["email"] != email {
		t.Fatalf("email: %+v", user)
	}
}

func TestW1UserShellBalanceAvailableField(t *testing.T) {
	_, server := newOAuthEnv(t)
	email := "shell-bal-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postJSONRaw(t, server.URL+"/v1/auth/register", "", map[string]any{
		"email": email, "password": "password1", "promotion_code": "THA1",
	})
	body := getAuthJSON(t, server.URL+"/v1/me/balance", tokenOf(reg))
	bal, _ := body["balance"].(map[string]any)
	if bal == nil {
		t.Fatalf("balance missing: %+v", body)
	}
	if _, ok := bal["available"]; !ok {
		t.Fatalf("topbar nails balance.available; field missing: %+v", bal)
	}
	if strings.Contains(mustJSONString(bal), "fake") {
		t.Fatalf("balance must not invent copy: %+v", bal)
	}
}
