package payments

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

// The independent container calls back to an explicitly advertised host. The
// default remains a loopback-only httptest listener for local lab processes.
func externalCallbackServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	host := os.Getenv("TOKENHUB_MOCK_CALLBACK_HOST")
	if host != "" {
		listener, e := net.Listen("tcp4", "0.0.0.0:0")
		if e != nil {
			t.Fatal(e)
		}
		server.Listener = listener
	}
	server.Start()
	if host != "" {
		u, e := url.Parse(server.URL)
		if e != nil {
			t.Fatal(e)
		}
		u.Host = net.JoinHostPort(host, u.Port())
		server.URL = u.String()
	}
	t.Cleanup(server.Close)
	return server
}
