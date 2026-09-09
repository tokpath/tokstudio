package billing

import "testing"

func TestUsageMissingDetectsOmitPayload(t *testing.T) {
	if !usageMissing([]byte(`{"missing":true}`)) {
		t.Fatal("missing:true must be a usage gap")
	}
	if !usageMissing(nil) {
		t.Fatal("empty usage is a gap")
	}
	if usageMissing([]byte(`{"prompt_tokens":8,"completion_tokens":4}`)) {
		t.Fatal("real tokens are not a gap")
	}
}

func TestUniqueNonEmptyKeepsOrder(t *testing.T) {
	got := uniqueNonEmpty([]string{"a", "", "b", "a", "c"})
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("got %+v", got)
	}
}
