package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
)

func TestUpstreamURLPolicy(t *testing.T) {
	production := newUpstreamHTTP(Settings{Production: true, UpstreamURLAllowlist: []string{"compatible.example"}, AllowTestLoopback: true})
	defer production.client.CloseIdleConnections()
	for _, tc := range []struct {
		url     string
		allowed bool
	}{
		{"https://openrouter.ai/api/v1/chat/completions", true},
		{"https://tenant.compatible.example/custom/v1/chat/completions", true},
		{"https://tenant.compatible.example:8443/v1/chat/completions", true},
		{"http://openrouter.ai/api/v1/chat/completions", false},
		{"https://openrouter.ai.evil.example/v1/chat/completions", false},
		{"https://evilcompatible.example/v1/chat/completions", false},
		{"https://evil.example/v1/chat/completions", false},
		{"https://user:secret@openrouter.ai/v1/chat/completions", false},
		{"https://openrouter.ai@127.0.0.1/v1/chat/completions", false},
		{"https://openrouter.ai/v1/chat/completions?redirect=http://127.0.0.1", false},
		{"https://openrouter.ai/v1/chat/completions#fragment", false},
		{"https://openrouter.ai:0/v1/chat/completions", false},
		{"https://openrouter.ai:65536/v1/chat/completions", false},
		{"https://169.254.169.254/latest/meta-data", false},
		{"https://metadata.google.internal/v1/chat/completions", false},
		{"https://127.0.0.1:8443/v1/chat/completions", false},
		{"https://[::ffff:127.0.0.1]/v1/chat/completions", false},
		{"file://openrouter.ai/v1/chat/completions", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			got := production.allowedURL.MatchString(tc.url) && production.validateURL(tc.url) == nil
			if got != tc.allowed {
				t.Fatalf("allowed=%v want=%v", got, tc.allowed)
			}
		})
	}
	// The explicit test exception only admits loopback, never metadata/private
	// networks, and cannot bypass the production check even when mis-set.
	local := newUpstreamHTTP(Settings{AllowTestLoopback: true})
	defer local.client.CloseIdleConnections()
	if !local.allowedURL.MatchString("http://127.0.0.1:1234/v1/chat/completions") || local.validateURL("http://127.0.0.1:1234/v1/chat/completions") != nil {
		t.Fatal("httptest exception unavailable")
	}
	for _, raw := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "168.63.129.16", "::1", "::ffff:10.0.0.1", "fc00::1", "fe80::1", "64:ff9b::a9fe:a9fe"} {
		address := netip.MustParseAddr(raw)
		if production.addressAllowed(address) {
			t.Fatalf("production allows protected address %s", raw)
		}
		if !address.IsLoopback() && local.addressAllowed(address) {
			t.Fatalf("test exception allows protected address %s", raw)
		}
	}
}

func TestUpstreamDialRejectsDNSPrivateAndPinsPublicAnswer(t *testing.T) {
	guard := newUpstreamHTTP(Settings{Production: true})
	defer guard.client.CloseIdleConnections()
	var calls int
	guard.dial = func(_ context.Context, _, authority string) (net.Conn, error) {
		calls++
		if authority != "93.184.216.34:443" {
			t.Fatalf("unvalidated hostname/IP dialed %s", authority)
		}
		return nil, errors.New("fixture connection failure")
	}
	for _, answers := range [][]netip.Addr{
		{netip.MustParseAddr("10.0.0.1")},
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("169.254.169.254")},
		{netip.MustParseAddr("::ffff:127.0.0.1")},
	} {
		guard.lookup = func(context.Context, string, string) ([]netip.Addr, error) { return answers, nil }
		if _, err := guard.dialContext(context.Background(), "tcp", "openrouter.ai:443"); !errors.Is(err, errBlockedUpstream) {
			t.Fatalf("private DNS answer not blocked %v", err)
		}
	}
	if calls != 0 {
		t.Fatalf("dialed before validating all answers: %d", calls)
	}
	var lookups int
	guard.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		if lookups > 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	_, _ = guard.dialContext(context.Background(), "tcp", "openrouter.ai:443")
	if calls != 1 || lookups != 1 {
		t.Fatalf("address was resolved again after inspection: dial=%d lookup=%d", calls, lookups)
	}
	if guard.client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("environment proxy bypasses pinned DNS")
	}
}

func TestOpenRouterBlocksUnsafeDestinationsBeforeSending(t *testing.T) {
	cap := 32
	for _, base := range []string{"https://evil.example/v1", "https://user:secret@openrouter.ai/v1", "http://169.254.169.254/latest", "http://127.0.0.1:1/v1"} {
		runtime := &Runtime{settings: Settings{Production: true, OpenRouterAPIKey: "fixture-secret"}}
		ctx := context.WithValue(context.Background(), ctxProviderBaseURLKey, base)
		out, err := (BifrostAdapter{Runtime: runtime}).openRouterChat(ctx, ChatRequest{MaxTokens: &cap})
		if !errors.Is(err, errBlockedUpstream) || out.ErrorClass != "provider_unavailable" {
			t.Fatalf("destination was not refused before sending %s: %+v %v", base, out, err)
		}
	}
	guard := newUpstreamHTTP(Settings{Production: true})
	defer guard.client.CloseIdleConnections()
	guard.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("10.0.0.1")}, nil
	}
	guard.dial = func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("private DNS answer reached dial")
		return nil, nil
	}
	runtime := &Runtime{settings: Settings{Production: true, OpenRouterAPIKey: "fixture-secret"}, upstreamHTTP: guard}
	out, err := (BifrostAdapter{Runtime: runtime}).openRouterChat(context.Background(), ChatRequest{MaxTokens: &cap})
	if !errors.Is(err, errBlockedUpstream) || out.ErrorClass != "provider_unavailable" {
		t.Fatalf("pre-send DNS refusal was treated as accepted/unknown: %+v %v", out, err)
	}
}

func TestOpenRouterConfiguredCompatibilityURL(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/compat/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Errorf("configured path/credentials not preserved: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chat_fixture","choices":[{"index":0,"message":{"role":"assistant","content":"done"}}],"usage":{"prompt_tokens":7,"completion_tokens":9,"total_tokens":16}}`))
	}))
	defer upstream.Close()
	guard := newUpstreamHTTP(Settings{Production: true, UpstreamURLAllowlist: []string{"example.com"}})
	defer guard.client.CloseIdleConnections()
	guard.client.Transport.(*http.Transport).TLSClientConfig = upstream.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	_, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var resolutions int
	guard.lookup = func(context.Context, string, string) ([]netip.Addr, error) {
		resolutions++
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	guard.dial = func(ctx context.Context, network, authority string) (net.Conn, error) {
		if authority != net.JoinHostPort("93.184.216.34", port) {
			t.Fatalf("DNS pin lost %s", authority)
		}
		// This package-local fixture redirects the verified public IP's socket
		// to our TLS server, retaining real certificate verification for example.com.
		return (&net.Dialer{}).DialContext(ctx, network, upstream.Listener.Addr().String())
	}
	runtime := &Runtime{settings: Settings{Production: true, OpenRouterAPIKey: "fixture-secret"}, upstreamHTTP: guard}
	ctx := context.WithValue(context.Background(), ctxProviderBaseURLKey, "https://example.com:"+port+"/compat/v1")
	cap := 32
	out, err := (BifrostAdapter{Runtime: runtime}).openRouterChat(ctx, ChatRequest{Model: "compatible/test", MaxTokens: &cap})
	if err != nil || out.Body.Usage["completion_tokens"] != 9 || calls.Load() != 1 || resolutions != 1 {
		t.Fatalf("legitimate configured endpoint failed %+v %v calls=%d DNS=%d", out, err, calls.Load(), resolutions)
	}
}

func TestOpenRouterNeverFollowsRedirectToLoopback(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetCalls.Add(1) }))
	defer target.Close()
	cap := 32
	for _, status := range []int{301, 302, 303, 307, 308} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-secret" {
					t.Error("original provider secret missing")
				}
				http.Redirect(w, r, target.URL, status)
			}))
			defer upstream.Close()
			runtime := &Runtime{settings: Settings{OpenRouterAPIKey: "fixture-secret", AllowTestLoopback: true}}
			ctx := context.WithValue(context.Background(), ctxProviderBaseURLKey, upstream.URL+"/v1")
			_, _ = (BifrostAdapter{Runtime: runtime}).openRouterChat(ctx, ChatRequest{Model: "openai/test", MaxTokens: &cap})
			if targetCalls.Load() != 0 {
				t.Fatal("followed redirect and exposed request to second target")
			}
		})
	}
}

type idleCloseFixture struct{ closed int }

func (f *idleCloseFixture) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("fixture has no upstream")
}
func (f *idleCloseFixture) CloseIdleConnections() { f.closed++ }

func TestRuntimeCloseReleasesHTTPPoolWithoutSDK(t *testing.T) {
	transport := &idleCloseFixture{}
	runtime := &Runtime{upstreamHTTP: &upstreamHTTP{client: &http.Client{Transport: transport}}}
	runtime.Close()
	if transport.closed != 1 {
		t.Fatal("HTTP pool survived runtime without SDK client")
	}
	(*Runtime)(nil).Close()
}
