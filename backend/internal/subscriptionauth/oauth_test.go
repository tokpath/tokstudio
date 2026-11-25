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

func TestMediaPresetsKeepTheApprovedConnection(t *testing.T) {
	for _, sample := range []struct {
		kind, preset, inference, operation, endpoint string
	}{
		{Codex, "codex", "https://chatgpt.com/backend-api/codex/responses", ImageEdit, "https://chatgpt.com/backend-api/codex/images/edits"},
		{Grok, "xai", "https://api.x.ai/v1/responses", VideoRetrieve, "https://api.x.ai/v1/videos/example"},
		{Gemini, "gemini", "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", ImageEdit, "https://generativelanguage.googleapis.com/v1beta/models/example:generateContent"},
		{Antigravity, "gemini", "https://aiplatform.googleapis.com/v1/projects/approved/locations/global/endpoints/openapi/chat/completions", ImageGeneration, "https://aiplatform.googleapis.com/v1/projects/approved/locations/global/publishers/google/models/example:generateContent"},
	} {
		t.Run(sample.kind, func(t *testing.T) {
			c := Connection{MediaPreset: sample.preset, InferenceURL: sample.inference}
			if err := c.validateMedia(sample.kind, false); err != nil {
				t.Fatal(err)
			}
			endpoint, ok := c.MediaEndpoint(sample.operation)
			if got, err := endpoint.Resolve("example", "example"); !ok || err != nil || got != sample.endpoint {
				t.Fatalf("derived endpoint %q, expected %q: %v", got, sample.endpoint, err)
			}
		})
	}
	c := Connection{MediaPreset: "codex", InferenceURL: "https://api.openai.com/v1/responses"}
	if err := c.validateMedia(Codex, false); !errors.Is(err, ErrOAuthNotConfigured) {
		t.Fatal("public Responses authorization became a native Codex media profile")
	}
}

func TestDiscoveryUsesTheAuthorizedProviderCatalog(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer approved-access" {
			t.Error("discovery used a different token or made an inference call")
		}
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"grok-text"},{"id":"grok-image","capabilities":{"unknown":true}},{"id":"../invalid"}],"models":[{"slug":"local-only","supported_in_api":false}]}`)
		case "/v1/image-generation-models":
			fmt.Fprint(w, `{"models":[{"id":"grok-image","input_modalities":["text","image"]}]}`)
		case "/v1/video-generation-models":
			fmt.Fprint(w, `{"models":[{"id":"grok-video"}]}`)
		default:
			t.Error("unknown model list", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	connection := Connection{MediaPreset: "xai", InferenceURL: server.URL + "/v1/responses"}
	guard := func(_ context.Context, target string) error {
		if !strings.HasPrefix(target, server.URL+"/v1/") {
			t.Error("model list left the approved inference origin")
			return ErrOAuthNotConfigured
		}
		return nil
	}
	grant := Credential{AccessToken: "approved-access", Connection: connection.Fingerprint()}
	want := []DiscoveredModel{
		{ID: "grok-image", Capabilities: []string{ImageGeneration, ImageEdit}},
		{ID: "grok-text"},
		{ID: "grok-video", Capabilities: []string{VideoGeneration}},
	}
	if got := connection.DiscoverModels(t.Context(), grant, guard); !reflect.DeepEqual(got, want) {
		t.Fatalf("discovered %+v, expected %+v", got, want)
	}
	grant.Connection = "other"
	if got := connection.DiscoverModels(t.Context(), grant, guard); len(got) != 0 {
		t.Fatal("discovered models with an unrelated grant")
	}
}
