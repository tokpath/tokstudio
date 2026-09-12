package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFinishGoogleNilExchangeIsUnavailable(t *testing.T) {
	svc := &Service{}
	session, err := svc.FinishGoogle(context.Background(), "state", "code", nil)
	if session != nil {
		t.Fatalf("nil exchanger must not issue a session: %+v", session)
	}
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("nil exchanger: %v", err)
	}
}

func TestNewGoogleExchangeUsesTokenAndProfile(t *testing.T) {
	var sawCode, leaked bool
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") == "good-code" && r.Form.Get("client_secret") != "" {
			sawCode = true
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "ya29.test-token", "token_type": "Bearer"})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "missing bearer", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"sub": "google-sub-1", "email": "Ada@Example.Test"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ex := NewGoogleExchange(GoogleOAuthConfig{
		ClientID:     "id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RedirectURL:  "https://test.tokpath.com/login/oauth/google",
		TokenURL:     srv.URL + "/token",
		UserInfoURL:  srv.URL + "/userinfo",
		HTTPClient:   srv.Client(),
	})
	profile, err := ex(context.Background(), "good-code")
	if err != nil {
		t.Fatal(err)
	}
	if !sawCode {
		t.Fatal("token endpoint must receive the authorization code")
	}
	if profile.Subject != "google-sub-1" || profile.Email != "ada@example.test" {
		t.Fatalf("profile: %+v", profile)
	}
	if strings.Contains(profile.Subject, "ya29") || strings.Contains(profile.Email, "client-secret") {
		leaked = true
	}
	if leaked {
		t.Fatal("profile must not carry token or secret")
	}
}

func TestNewGoogleExchangeRejectsBadCodeWithoutLeaking(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	secretCode := "4/0AanRRrs-this-must-not-appear"
	ex := NewGoogleExchange(GoogleOAuthConfig{
		ClientID:     "id.apps.googleusercontent.com",
		ClientSecret: "client-secret",
		RedirectURL:  "https://test.tokpath.com/login/oauth/google",
		TokenURL:     srv.URL + "/token",
		UserInfoURL:  srv.URL + "/userinfo",
		HTTPClient:   srv.Client(),
	})
	profile, err := ex(context.Background(), secretCode)
	if !errors.Is(err, ErrGoogleExchange) {
		t.Fatalf("bad code: %v", err)
	}
	var exchangeErr *GoogleExchangeError
	if !errors.As(err, &exchangeErr) || exchangeErr.Reason != "invalid_grant" {
		t.Fatalf("expected invalid_grant reason, got %v", err)
	}
	if profile.Email != "" || profile.Subject != "" {
		t.Fatalf("failed exchange must not invent a profile: %+v", profile)
	}
	if strings.Contains(err.Error(), secretCode) || strings.Contains(err.Error(), "client-secret") {
		t.Fatalf("exchange error leaked a secret: %v", err)
	}
}

func TestNewGoogleExchangeRequiresTriad(t *testing.T) {
	ex := NewGoogleExchange(GoogleOAuthConfig{ClientID: "only-id"})
	if _, err := ex(context.Background(), "code"); !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("incomplete triad: %v", err)
	}
}
