package portal

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

func TestPostgresSubscriptionBrowserOAuth(t *testing.T) {
	f := setupPortal(t)
	f.a.Config.WebOrigin = f.server.URL
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	f.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	otherJar, _ := cookiejar.New(nil)
	other := &testApp{a: f.a, server: f.server, client: &http.Client{Jar: otherJar, CheckRedirect: f.client.CheckRedirect}}
	other.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + f.a.Installation.OperatorID
	pools := base + "/subscription-pools"
	post := func(path string, input any) map[string]any {
		t.Helper()
		status, out, _ := f.call(t, "POST", path, input, "")
		mustOK(t, status, out)
		return out
	}
	callback := func(client *http.Client, kind string, query url.Values, want string) {
		t.Helper()
		resp, err := client.Get(f.server.URL + "/v1/subscription-oauth/" + kind + "/callback?" + query.Encode())
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		location, _ := url.Parse(resp.Header.Get("Location"))
		if resp.StatusCode != http.StatusSeeOther || location == nil || location.Path != "/manage/"+f.a.Installation.OperatorID+"/subscription-pools" ||
			location.Query().Get("oauth") != want || location.Query().Has("code") || location.Query().Has("state") ||
			resp.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("callback %d %q, expected %s", resp.StatusCode, resp.Header.Get("Location"), want)
		}
	}
	for _, sample := range []struct{ kind, path, model, pricingProvider string }{
		{subscriptionauth.Grok, "/v1/responses", "grok-test-model", "xai"},
		{subscriptionauth.Gemini, "/v1/chat/completions", "google/gemini-test-model", "gemini"},
		{subscriptionauth.Antigravity, "/v1/chat/completions", "antigravity-test-model", "gemini"},
	} {
		t.Run(sample.kind, func(t *testing.T) {
			var exchanges atomic.Int64
			supplier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/userinfo" && r.Header.Get("Authorization") == "Bearer private-access" {
					fmt.Fprint(w, `{"sub":"supplier-account"}`)
					return
				}
				if r.URL.Path != "/token" || r.Method != http.MethodPost || r.ParseForm() != nil {
					http.NotFound(w, r)
					return
				}
				exchanges.Add(1)
				parts := strings.Split(r.Form.Get("code"), ".")
				challenge := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if len(parts) != 3 || parts[0] != "test-code" || parts[1] != base64.RawURLEncoding.EncodeToString(challenge[:]) ||
					r.Form.Get("client_id") != "registered-client" || r.Form.Get("client_secret") != "test-client-secret" ||
					r.Form.Get("redirect_uri") != f.server.URL+"/api/v1/subscription-oauth/"+sample.kind+"/callback" {
					t.Error("OAuth token exchange did not use the registered client and PKCE")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				now := time.Now()
				claims, _ := json.Marshal(map[string]any{"sub": "supplier-account", "aud": "registered-client", "nonce": parts[2],
					"iss": "http://" + r.Host, "exp": now.Add(time.Hour).Unix(), "iat": now.Unix()})
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "private-access", "refresh_token": "private-refresh",
					"token_type": "Bearer", "expires_in": 3600, "id_token": "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".provider"})
			}))
			defer supplier.Close()
			connection := subscriptionauth.Connection{
				AuthorizationURL: supplier.URL + "/authorize?prompt=select_account",
				TokenURL:         supplier.URL + "/token", InferenceURL: supplier.URL + sample.path,
				ClientID: "registered-client", ClientSecret: "test-client-secret", Scope: "openid inference",
				Issuer: supplier.URL,
			}
			if sample.kind == subscriptionauth.Grok {
				connection.Issuer, connection.UserInfoURL = "", supplier.URL+"/userinfo"
			}
			config, _ := json.Marshal(subscriptionauth.Connections{sample.kind: connection})
			profiles, err := subscriptionauth.LoadConnections(string(config), true)
			if err != nil {
				t.Fatal(err)
			}
			f.a.Config.SubscriptionConnections = profiles
			f.a.Catalog.SubscriptionConnections = profiles

			pool := post(pools, map[string]any{"action": "save_pool", "pool": map[string]any{
				"name": sample.kind, "subscription_kind": sample.kind, "enabled": true,
			}})["id"].(string)
			draft := map[string]any{"pool_id": pool, "name": "Browser account", "enabled": true, "expected_version": 0}
			start := func() url.Values {
				t.Helper()
				out := post(pools+"/oauth/start", draft)
				u, err := url.Parse(out["url"].(string))
				if err != nil || u.Host != strings.TrimPrefix(supplier.URL, "http://") || u.Path != "/authorize" {
					t.Fatal("unexpected authorize destination", u, err)
				}
				q := u.Query()
				if len(q.Get("state")) != 43 || q.Get("code_challenge_method") != "S256" ||
					q.Get("prompt") != "select_account" || q.Has("client_secret") {
					t.Fatal("invalid authorization request", q)
				}
				return url.Values{"state": {q.Get("state")}, "code": {"test-code." + q.Get("code_challenge") + "." + q.Get("nonce")}}
			}
			cancelled := start()
			cancelled.Del("code")
			cancelled.Set("error", "access_denied")
			callback(f.client, sample.kind, cancelled, "cancelled")
			query := start()
			callback(other.client, sample.kind, query, "failed")
			callback(f.client, sample.kind, query, "saved")
			callback(f.client, sample.kind, query, "failed")
			callback(f.client, sample.kind, start(), "duplicate")
			if exchanges.Load() != 2 {
				t.Fatalf("expected one exchange for each accepted state, got %d", exchanges.Load())
			}

			imported, _ := json.Marshal(subscriptionauth.Credential{
				AccountID: "imported-account", AccessToken: "private-import", ExpiresAt: time.Now().Add(time.Hour),
			})
			post(pools, map[string]any{"action": "save_account", "account": map[string]any{
				"pool_id": pool, "name": "Imported account", "credential": string(imported), "enabled": true,
			}})
			status, page, _ := f.call(t, "GET", pools, nil, "")
			mustOK(t, status, page)
			found := false
			for _, item := range page["items"].([]any) {
				current := item.(map[string]any)
				if current["id"] == pool {
					found = current["browser_oauth"] == true && len(current["accounts"].([]any)) == 2
				}
			}
			if !found {
				t.Fatal("pool did not show both authorized accounts")
			}
			raw, _ := json.Marshal(page)
			if strings.Contains(string(raw), "private-") || strings.Contains(string(raw), "ciphertext") {
				t.Fatal("pool exposed authorization data")
			}

			if err := f.a.DB.Exec(`INSERT INTO model_pricing_rows (id,provider,model,mode,definition)
				VALUES (?,?,?,'chat','{"max_input_tokens":32768,"max_output_tokens":2048}'::jsonb)`,
				sample.kind, sample.pricingProvider, strings.TrimPrefix(sample.model, "google/")).Error; err != nil {
				t.Fatal(err)
			}
			operation := subscriptionauth.Operation(sample.kind)
			model := post(base+"/providers", map[string]any{"action": "model", "supplier_model": map[string]any{
				"id": uuid.NewString(), "credential_id": pool, "provider": subscriptionauth.Protocol(sample.kind), "model": sample.model, "enabled": true,
				"capabilities": []map[string]any{{"operation": operation, "streaming": true}},
			}})["id"].(string)
			public := "oauth-" + sample.kind
			post(base+"/catalog", map[string]any{"model": map[string]any{"id": public, "capabilities": []map[string]any{{"operation": operation}}}})
			post(base+"/routes", map[string]any{"route": map[string]any{
				"public_model_id": public, "strategy": "priority", "bindings": []map[string]any{{"supplier_model_id": model, "enabled": true}},
			}})
			now := time.Now()
			post(base+"/supplier-costs", map[string]any{"provider_id": pool, "statement_reference": "OAUTH-STATEMENT",
				"starts_at": now.Add(-time.Hour), "ends_at": now, "currency": "EUR", "amount": "21.30"})
		})
	}
}
