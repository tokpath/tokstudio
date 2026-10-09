package endpoints

import (
	"net"
	"strings"
	"testing"
)

func TestRejectNonPublicEndpoints(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.1.2.3", "169.254.169.254", "100.64.1.1", "198.18.1.2", "192.0.2.1", "::1", "fc00::1", "2001:db8::1"} {
		if publicIP(net.ParseIP(address)) {
			t.Errorf("accepted non-public %s", address)
		}
	}
	if !publicIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public address rejected")
	}
}
func TestBrandRouteRequiresMatchingEvidence(t *testing.T) {
	for _, sample := range []struct {
		path, body string
		want       bool
	}{
		{"/v1/public/brand", `{"brand":{"id":"oem-test"}}`, true},
		{"/v1/public/brand", `{"brand":{"id":"another"}}`, false},
		{"/login", `<html><meta name="tokstudio-brand-id" content="oem-test"></html>`, true},
		{"/", `<html><meta name="tokstudio-brand-id" content="another"></html>`, false},
		{"/", `<html><title>Unrelated site</title></html>`, false},
	} {
		if got := responseBrandMatches(strings.NewReader(sample.body), sample.path, "oem-test"); got != sample.want {
			t.Errorf("path=%s got=%v", sample.path, got)
		}
	}
}
