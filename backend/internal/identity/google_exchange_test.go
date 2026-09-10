package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGoogleOAuthExchange(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("token method %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("code") != "ok-code" || r.Form.Get("client_id") != "cid" || r.Form.Get("redirect_uri") != "https://app.example/login" {
			http.Error(w, "bad token request", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "atok"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer atok" {
			http.Error(w, "no bearer", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "gid-1", "email": "User@Example.com", "email_verified": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	oauth := GoogleOAuth{
		ClientID: "cid", ClientSecret: "sec", RedirectURI: "https://app.example/login",
		TokenURL: srv.URL + "/token", UserInfoURL: srv.URL + "/userinfo",
		HTTPClient: srv.Client(),
	}
	got, err := oauth.Exchange(context.Background(), "ok-code")
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != "gid-1" || got.Email != "user@example.com" {
		t.Fatalf("profile %+v", got)
	}
}

func TestGoogleOAuthExchangeRejectsUnverified(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "atok"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": "gid-1", "email": "user@example.com", "email_verified": false})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	oauth := GoogleOAuth{
		ClientID: "cid", ClientSecret: "sec", RedirectURI: "https://app.example/login",
		TokenURL: srv.URL + "/token", UserInfoURL: srv.URL + "/userinfo",
		HTTPClient: srv.Client(),
	}
	if _, err := oauth.Exchange(context.Background(), "ok-code"); err != ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}

func TestGoogleOAuthExchangeRejectsTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)
	oauth := GoogleOAuth{
		ClientID: "cid", ClientSecret: "sec", RedirectURI: "https://app.example/login",
		TokenURL: srv.URL, UserInfoURL: srv.URL,
		HTTPClient: srv.Client(),
	}
	if _, err := oauth.Exchange(context.Background(), "bad"); err != ErrInvalidCredentials {
		t.Fatalf("got %v", err)
	}
}
