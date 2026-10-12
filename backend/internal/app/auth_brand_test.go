package app

import "testing"

func TestAuthAuthorityRejectsUntrustedURLShapes(t *testing.T) {
	for _, raw := range []string{"", "evil.test/path", "user@brand.test", "brand.test?host=other.test", "brand.test#fragment", "brand.test,evil.test", "brand.test\\evil", "brand.test\n"} {
		if got := authAuthority(raw); got != "" {
			t.Errorf("accepted %q as %q", raw, got)
		}
	}
	if got := authAuthority("Brand.Example:8443"); got != "brand.example:8443" {
		t.Fatal(got)
	}
	if !loopbackAuthHost("127.0.0.1:3000") || loopbackAuthHost("127.0.0.1.evil.test") {
		t.Fatal("invalid loopback detection")
	}
}
