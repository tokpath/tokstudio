package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"
)

// Verify browser session cookies, rather than bootstrap bearer tokens, for every
// company role and each consumer/promotion account type on its own brand host.
func TestRoleLoginPageContracts(t *testing.T) {
	_, server := newOAuthEnv(t)
	admin := tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": "admin@tokenhub.local", "password": "password1"}))
	oem := tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": "channel.c@tokenhub.local", "password": "password1"}, "oem.localhost"))
	prefix := "role-login-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	type account struct {
		name, email, host, role string
		allowed, denied         []string
	}
	accounts := []account{
		{name: "reseller", email: "channel.b@tokenhub.local", host: "localhost", role: "channel_admin", allowed: []string{"/channel/me", "/channel/customers", "/channel/models", "/v1/me/referral"}, denied: []string{"/admin/ops/dashboard", "/channel/delivery"}},
		{name: "consumer", email: "user@tokenhub.local", host: "localhost", role: "end_user", allowed: []string{"/v1/me/api-keys", "/v1/me/balance", "/v1/me/referral"}, denied: []string{"/admin/ops/dashboard", "/channel/me"}},
		{name: "agent", email: "agent.b@tokenhub.local", host: "localhost", role: "end_user", allowed: []string{"/v1/me/api-keys", "/v1/me/referral", "/v1/partner/me"}, denied: []string{"/admin/ops/dashboard", "/channel/me"}},
		{name: "promoter", email: "kol1.b@tokenhub.local", host: "localhost", role: "end_user", allowed: []string{"/v1/me/api-keys", "/v1/me/referral", "/v1/partner/me"}, denied: []string{"/admin/ops/dashboard", "/channel/me"}},
	}
	for _, role := range []string{"platform_admin", "finance_admin", "ops_admin", "tech_admin", "audit_readonly", "channel_admin", "oem_ops", "oem_finance", "oem_audit"} {
		a := account{name: role, email: prefix + "-" + role + "@example.test", role: role, host: "localhost"}
		path, token := "/admin/staff", admin
		if role == "channel_admin" || role == "oem_ops" || role == "oem_finance" || role == "oem_audit" {
			path, token, a.host = "/channel/staff", oem, "oem.localhost"
			a.allowed = []string{"/channel/me", "/channel/delivery", "/channel/models", "/channel/customers", "/channel/customer-scopes"}
			a.denied = []string{"/admin/ops/dashboard", "/admin/providers"}
			if role == "oem_finance" {
				a.denied = append(a.denied, "/channel/api-keys", "/channel/subchannels")
			} else {
				a.allowed = append(a.allowed, "/channel/api-keys", "/channel/subchannels")
			}
		} else {
			a.allowed = []string{"/admin/ops/dashboard", "/admin/metrics/series?days=7"}
			if role == "tech_admin" {
				a.allowed = append(a.allowed, "/admin/providers")
				a.denied = []string{"/admin/customers", "/admin/billing/report"}
			} else {
				a.allowed = append(a.allowed, "/admin/customers", "/admin/customer-scopes", "/admin/billing/report")
			}
			if role == "finance_admin" {
				a.denied = append(a.denied, "/admin/providers", "/admin/models")
			}
		}
		code, created := doJSON(t, http.MethodPost, server.URL+path, token, true, map[string]any{"email": a.email, "display_name": "Role login audit", "password": "password1", "roles": []string{role}})
		if code != http.StatusCreated {
			t.Fatalf("create isolated %s fixture: status %d", role, code)
		}
		id := created["item"].(map[string]any)["user_id"].(string)
		t.Cleanup(func() {
			code, _ := doJSON(t, http.MethodPatch, server.URL+path+"/"+id, token, true, map[string]any{"status": "disabled"})
			if code != http.StatusOK {
				t.Errorf("disable isolated %s fixture: status %d", role, code)
			}
		})
		accounts = append(accounts, a)
	}
	for _, a := range accounts {
		t.Run(a.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]string{"email": a.email, "password": "password1"})
			req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/auth/login", bytes.NewReader(payload))
			req.Host = a.host
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("password login: status %d", res.StatusCode)
			}
			var session *http.Cookie
			for _, cookie := range res.Cookies() {
				if cookie.Name == "tokenhub_session" {
					session = cookie
				}
			}
			if session == nil || !session.HttpOnly || session.Path != "/" || session.Domain != "" {
				t.Fatal("missing browser-safe host-only session cookie")
			}
			read := func(path, host string, want int) map[string]any {
				t.Helper()
				req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
				req.Host = host
				req.AddCookie(session)
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer res.Body.Close()
				if res.StatusCode != want {
					t.Fatalf("%s on %s: status %d, want %d", path, host, res.StatusCode, want)
				}
				var body map[string]any
				json.NewDecoder(res.Body).Decode(&body)
				return body
			}
			me := read("/v1/me", a.host, http.StatusOK)["user"].(map[string]any)
			found := false
			for _, role := range me["roles"].([]any) {
				found = found || role == a.role
			}
			if !found {
				t.Fatalf("login lost assigned role %s", a.role)
			}
			for _, path := range append(a.allowed, "/v1/me/notifications", "/v1/me/notifications/unread-count", "/v1/me/notification-preferences") {
				read(path, a.host, http.StatusOK)
			}
			for _, path := range a.denied {
				read(path, a.host, http.StatusForbidden)
			}
			if a.host == "oem.localhost" {
				read("/v1/me", "localhost", http.StatusForbidden)
			}
		})
	}
}
