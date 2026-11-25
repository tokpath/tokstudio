package subscriptionauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfiguredSubscriptionRefreshKeepsSupplierAndRotatesToken(t *testing.T) {
	ctx := context.Background()
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("OAuth client followed a token endpoint redirect")
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	var redirect atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID, secret, ok := r.BasicAuth()
		if !ok || clientID != "approved-client" || secret != "rotated-secret" || r.ParseForm() != nil ||
			r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "current-refresh" {
			t.Error("refresh did not use the authorized client and refresh token")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, receiver.URL, http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"rotated-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	connection := Connection{ClientID: "approved-client", ClientSecret: "initial-secret", TokenAuthMethod: "client_secret_basic",
		TokenURL: server.URL, InferenceURL: server.URL + "/v1/responses"}
	current := Credential{AccountID: "original-account", RefreshToken: "current-refresh", Connection: connection.Fingerprint()}
	connection.ClientSecret = "rotated-secret"
	guard := func(_ context.Context, target string) error {
		if target != server.URL {
			t.Error("unexpected token destination")
			return ErrRefresh
		}
		return nil
	}
	rotated, err := connection.Refresh(ctx, current, guard)
	if err != nil || rotated.Connection != current.Connection || rotated.AccountID != current.AccountID ||
		rotated.RefreshToken != "rotated-refresh" || rotated.AccessToken != "rotated-access" || !rotated.ExpiresAt.After(time.Now()) {
		t.Fatal("refresh did not preserve the account and rotate both tokens", err)
	}
	redirect.Store(true)
	if _, err = connection.Refresh(ctx, current, guard); !errors.Is(err, ErrRefresh) {
		t.Fatalf("expected a refused redirect, got %v", err)
	}
	connection.InferenceURL = receiver.URL + "/v1/responses"
	if _, err = connection.Refresh(ctx, current, guard); !errors.Is(err, ErrReauth) {
		t.Fatalf("expected reauthorization after destination change, got %v", err)
	}
}

func TestDiscoveryUsesTheAuthorizedProviderCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer approved-access" {
			t.Error("discovery used a different token or made an inference call")
		}
		if r.URL.Path != "/v1/models" {
			t.Error("unknown model list", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		fmt.Fprint(w, `{"data":[{"id":"grok-text"},{"id":"../invalid"}],"models":[{"slug":"grok-text"},{"slug":"grok-fast"},{"slug":"local-only","supported_in_api":false}]}`)
	}))
	t.Cleanup(server.Close)
	connection := Connection{InferenceURL: server.URL + "/v1/responses"}
	guard := func(_ context.Context, target string) error {
		if !strings.HasPrefix(target, server.URL+"/v1/") {
			t.Error("model list left the approved inference origin")
			return ErrOAuthNotConfigured
		}
		return nil
	}
	grant := Credential{AccessToken: "approved-access", Connection: connection.Fingerprint()}
	want := []string{"grok-fast", "grok-text"}
	if got := connection.DiscoverModels(t.Context(), grant, guard); !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered %+v, expected %+v", got, want)
	}
	grant.Connection = "other"
	if got := connection.DiscoverModels(t.Context(), grant, guard); len(got) != 0 {
		t.Fatal("discovered models with an unrelated grant")
	}
}
