package httpx

import (
	"strings"
	"testing"
)

func TestUpstreamErrorsCannotBypassPrivacyThroughLocalization(t *testing.T) {
	for _, locale := range []string{"zh", "en", "ja", "ru"} {
		got := LocalizedErrorMessage(locale, "upstream_error", "HTTP 401: opaque-supplier-secret")
		if got == "" || strings.Contains(got, "opaque-supplier-secret") || got != LocalizedErrorMessage(locale, "upstream_error", UpstreamUnavailableMessage) {
			t.Fatal("supplier text escaped the response boundary", locale)
		}
	}
}
