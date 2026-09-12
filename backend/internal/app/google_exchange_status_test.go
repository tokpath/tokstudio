package app

import (
	"net/http"
	"testing"
)

func TestGoogleExchangeHTTPStatusAvoidsBadGateway(t *testing.T) {
	cases := map[string]int{
		"network_error":         http.StatusServiceUnavailable,
		"invalid_grant":         http.StatusBadRequest,
		"redirect_uri_mismatch": http.StatusBadRequest,
		"invalid_client":        http.StatusBadRequest,
		"invalid_request":       http.StatusBadRequest,
		"access_denied":         http.StatusBadRequest,
		"empty_profile":         http.StatusBadRequest,
	}
	for reason, want := range cases {
		got := googleExchangeHTTPStatus(reason)
		if got == http.StatusBadGateway {
			t.Fatalf("%s must not use 502 (Cloudflare masks JSON body)", reason)
		}
		if got != want {
			t.Fatalf("%s: got %d want %d", reason, got, want)
		}
	}
}
