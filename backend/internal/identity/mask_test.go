package identity

import "testing"

func TestMaskEmail(t *testing.T) {
	if got := MaskEmail("alice@example.test"); got != "a***@example.test" {
		t.Fatalf("got %s", got)
	}
}
