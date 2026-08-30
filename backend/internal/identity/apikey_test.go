package identity

import "testing"

func TestNormalizeAllowlist(t *testing.T) {
	got := normalizeAllowlist([]string{" tokenhub/echo-1 ", "", "google/gemini-flash", "tokenhub/echo-1", "  "})
	if len(got) != 2 || got[0] != "tokenhub/echo-1" || got[1] != "google/gemini-flash" {
		t.Fatalf("normalizeAllowlist: %+v", got)
	}
	if got := normalizeAllowlist(nil); len(got) != 0 {
		t.Fatalf("empty allowlist should stay empty: %+v", got)
	}
}
