package httpx

import "testing"

func TestOriginAllowedLoopbackAlias(t *testing.T) {
	if !originAllowed("http://127.0.0.1:3000", "http://localhost:3000") {
		t.Fatal("127.0.0.1 should match configured localhost on the same port")
	}
	if !originAllowed("http://localhost:3000", "http://127.0.0.1:3000") {
		t.Fatal("localhost should match configured 127.0.0.1 on the same port")
	}
	if originAllowed("http://127.0.0.1:3001", "http://localhost:3000") {
		t.Fatal("different ports must not match")
	}
	if originAllowed("https://evil.example", "http://localhost:3000") {
		t.Fatal("foreign origin must not match")
	}
	if originAllowed("http://oem.localhost:3000", "http://localhost:3000") {
		t.Fatal("oem.localhost is a different host, not a loopback alias")
	}
}
