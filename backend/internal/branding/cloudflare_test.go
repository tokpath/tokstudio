package branding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type txtLookup func(context.Context, string) ([]string, error)

func (f txtLookup) LookupTXT(ctx context.Context, name string) ([]string, error) { return f(ctx, name) }

// The isolated server checks the actual Cloudflare HTTP wire contract. It is
// deliberately not evidence of a real account's entitlement or public TLS.
func TestCloudflareProvisionRetryAndSeparateHostnameTLSStatus(t *testing.T) {
	var posts, gets atomic.Int64
	var metadata map[string]string
	var active atomic.Bool
	var hostnameActive atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-test-token" || r.URL.Path != "/zones/0123456789abcdef0123456789abcdef/custom_hostnames" {
			t.Error("wrong authenticated provider target")
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		host := map[string]any{"id": "host-1", "hostname": "tenant.example", "status": "pending", "custom_metadata": metadata, "ownership_verification": map[string]string{"type": "txt", "name": "_cf-custom-hostname.tenant.example", "value": "provider-proof"}, "ssl": map[string]any{"status": "pending_validation", "validation_records": []map[string]string{{"txt_name": "_acme-challenge.tenant.example", "txt_value": "certificate-proof"}}}}
		if active.Load() {
			host["ssl"] = map[string]string{"status": "active"}
		}
		if hostnameActive.Load() {
			host["status"] = "active"
		}
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			if r.URL.Query().Get("hostname") != "tenant.example" {
				t.Error("missing exact hostname filter")
			}
			rows := []any{}
			if posts.Load() > 0 {
				rows = append(rows, host)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": rows})
		case http.MethodPost:
			posts.Add(1)
			var in struct {
				Hostname string            `json:"hostname"`
				Metadata map[string]string `json:"custom_metadata"`
				SSL      struct {
					Method, Type string
					Settings     map[string]string
				} `json:"ssl"`
			}
			if e := json.NewDecoder(r.Body).Decode(&in); e != nil {
				t.Error(e)
			}
			if in.Hostname != "tenant.example" || in.SSL.Method != "txt" || in.SSL.Type != "dv" || in.SSL.Settings["min_tls_version"] != "1.2" || len(in.Metadata["tokenhub_verification_hash"]) != 64 {
				t.Errorf("wrong provisioning payload %+v", in)
			}
			metadata = in.Metadata
			host["custom_metadata"] = metadata
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": host})
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	resolver := txtLookup(func(_ context.Context, name string) ([]string, error) {
		if name != "_tokenhub.tenant.example" {
			t.Fatal(name)
		}
		return []string{"tokenhub-verification=private-challenge"}, nil
	})
	v, e := NewCloudflareVerifier(CloudflareSettings{APIToken: "private-test-token", ZoneID: "0123456789abcdef0123456789abcdef", APIBaseURL: server.URL, Resolver: resolver, CNAME: "saas.example"}, server.Client())
	if e != nil {
		t.Fatal(e)
	}
	first, e := v.Verify(context.Background(), "tenant.example", "private-challenge")
	if e != nil || !first.OwnershipVerified || first.TLSReady || len(first.Instructions) != 3 || first.ProviderReference == "" {
		t.Fatalf("pending result %+v %v", first, e)
	}
	active.Store(true)
	tlsOnly, e := v.Verify(context.Background(), "tenant.example", "private-challenge")
	if e != nil || tlsOnly.TLSReady {
		t.Fatalf("TLS alone enabled hostname %+v %v", tlsOnly, e)
	}
	hostnameActive.Store(true)
	ready, e := v.Verify(context.Background(), "tenant.example", "private-challenge")
	if e != nil || !ready.TLSReady || posts.Load() != 1 || gets.Load() != 3 {
		t.Fatalf("not idempotent ready=%+v posts=%d gets=%d err=%v", ready, posts.Load(), gets.Load(), e)
	}
	_, e = v.Verify(context.Background(), "tenant.example", "foreign-challenge")
	if e != nil {
		t.Fatal(e)
	} // DNS mismatch must avoid even a provider lookup.
	if gets.Load() != 3 {
		t.Fatal("provisioned without DNS ownership proof")
	}
}

func TestCloudflareRejectsForeignBindingAndRedactsProviderErrors(t *testing.T) {
	var broken atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken.Load() {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("secret-token must never appear in returned errors"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []map[string]any{{"id": "foreign", "hostname": "tenant.example", "status": "active", "custom_metadata": map[string]string{"tokenhub_verification_hash": "foreign"}, "ssl": map[string]string{"status": "active"}}}})
	}))
	defer server.Close()
	v, e := NewCloudflareVerifier(CloudflareSettings{APIToken: "secret-token", ZoneID: "0123456789abcdef0123456789abcdef", APIBaseURL: server.URL, Resolver: txtLookup(func(context.Context, string) ([]string, error) { return []string{"tokenhub-verification=proof"}, nil })}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = v.Verify(context.Background(), "tenant.example", "proof"); !errors.Is(e, ErrConflict) {
		t.Fatal("adopted foreign metadata", e)
	}
	broken.Store(true)
	if _, e = v.Verify(context.Background(), "tenant.example", "proof"); !errors.Is(e, ErrVerificationUnavailable) || strings.Contains(e.Error(), "secret-token") {
		t.Fatal("provider error leaked", e)
	}
}
