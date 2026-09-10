package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareReady(t *testing.T) {
	if NewCloudflare(CloudflareOptions{APIToken: "t"}).Ready() {
		t.Fatal("zone missing")
	}
	if !NewCloudflare(CloudflareOptions{APIToken: "t", ZoneID: "z"}).Ready() {
		t.Fatal("token+zone should be ready")
	}
}

func TestCloudflareEnsureCreates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("auth %s", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/custom_hostnames") {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if body["hostname"] != "oem.example.com" {
			t.Fatalf("hostname %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"id":"ch_1","status":"pending","ssl":{"status":"pending_validation","expires_on":"2027-01-02T03:04:05Z"}}}`))
	}))
	defer srv.Close()
	cf := NewCloudflare(CloudflareOptions{APIToken: "tok", ZoneID: "zone1", BaseURL: srv.URL, CNAMETarget: "edge.tokpath.com"})
	got, err := cf.Ensure(context.Background(), "oem.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ch_1" || got.Status != "pending" || got.Issuer != IssuerCloudflare || got.ExpiresAt == nil {
		t.Fatalf("%+v", got)
	}
}

func TestCloudflareEnsureConflictThenGet(t *testing.T) {
	var posts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"code":1406,"message":"The custom hostname already exists"}]}`))
			return
		}
		if r.URL.Query().Get("hostname") != "oem.example.com" {
			t.Fatalf("query %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"ch_dup","ssl":{"status":"active","expires_on":"2027-06-01T00:00:00Z"}}]}`))
	}))
	defer srv.Close()
	cf := NewCloudflare(CloudflareOptions{APIToken: "tok", ZoneID: "zone1", BaseURL: srv.URL})
	got, err := cf.Ensure(context.Background(), "oem.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "ch_dup" || got.Status != "issued" {
		t.Fatalf("posts=%d %+v", posts, got)
	}
}

func TestCloudflareGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"ch_get","ssl":{"status":"active","expires_on":"2027-06-01T00:00:00Z"}}]}`))
	}))
	defer srv.Close()
	cf := NewCloudflare(CloudflareOptions{APIToken: "tok", ZoneID: "zone1", BaseURL: srv.URL})
	got, err := cf.Get(context.Background(), "oem.example.com")
	if err != nil || got.Status != "issued" || got.ID != "ch_get" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestCloudflareEnsureRejectsLocalhost(t *testing.T) {
	cf := NewCloudflare(CloudflareOptions{APIToken: "tok", ZoneID: "zone1", BaseURL: "http://127.0.0.1:1"})
	if _, err := cf.Ensure(context.Background(), "oem.localhost"); err == nil {
		t.Fatal("localhost must not register")
	}
}
