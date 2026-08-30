package identity

import (
	"strings"
	"testing"
)

func TestNormalizeDisplayName(t *testing.T) {
	name, err := NormalizeDisplayName("  Ada  ")
	if err != nil || name != "Ada" {
		t.Fatalf("got %q %v", name, err)
	}
	long := strings.Repeat("名", MaxDisplayNameLen+1)
	if _, err := NormalizeDisplayName(long); err != ErrInvalidProfile {
		t.Fatalf("expected ErrInvalidProfile, got %v", err)
	}
}

func TestNormalizeLocale(t *testing.T) {
	got, err := NormalizeLocale("EN")
	if err != nil || got != "en" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizeLocale("fr"); err != ErrInvalidLocale {
		t.Fatalf("expected ErrInvalidLocale, got %v", err)
	}
}
