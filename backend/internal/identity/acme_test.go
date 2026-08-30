package identity

import (
	"context"
	"testing"
)

func TestResolveACMEDirectory(t *testing.T) {
	if got := ResolveACMEDirectory(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := ResolveACMEDirectory("sandbox"); got != "" {
		t.Fatalf("sandbox: %q", got)
	}
	if got := ResolveACMEDirectory("letsencrypt"); got != DirectoryLetsEnc {
		t.Fatalf("le: %q", got)
	}
	if got := ResolveACMEDirectory("staging"); got != DirectoryLEStaging {
		t.Fatalf("staging: %q", got)
	}
	if got := ResolveACMEDirectory("https://pebble:14000/dir"); got != "https://pebble:14000/dir" {
		t.Fatalf("custom: %q", got)
	}
}

func TestUsePublicACME(t *testing.T) {
	if UsePublicACME("") || UsePublicACME("127.0.0.1") || UsePublicACME("oem.localhost") || UsePublicACME("edge.tokenhub.local") {
		t.Fatal("empty / IP / .localhost / .local must stay sandbox")
	}
	if UsePublicACME("foo.test") || UsePublicACME("bar.invalid") {
		t.Fatal("reserved TLDs must stay sandbox")
	}
	if !UsePublicACME("oem.example.com") {
		t.Fatal("public-form hostname should use ACME when a directory is set")
	}
}

func TestACMEChallengeStore(t *testing.T) {
	a := NewACME("", false, false)
	if a.Enabled() {
		t.Fatal("empty directory must disable ACME")
	}
	a.putChallenge("tok", "auth")
	if a.LookupChallenge("tok") != "auth" {
		t.Fatal("lookup")
	}
	a.deleteChallenge("tok")
	if a.LookupChallenge("tok") != "" {
		t.Fatal("deleted")
	}
}

func TestACMEIssueRejectsIP(t *testing.T) {
	a := NewACME("https://example.test/dir", true, true)
	if _, err := a.Issue(context.Background(), "127.0.0.1"); err == nil {
		t.Fatal("IP must be rejected")
	}
}
