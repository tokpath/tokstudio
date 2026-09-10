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

func TestLoginMethodsOf(t *testing.T) {
	hash := "hashed"
	sub := "google-sub-1"
	if got := loginMethodsOf(userRow{}); len(got) != 0 {
		t.Fatalf("empty user: %v", got)
	}
	got := loginMethodsOf(userRow{PasswordHash: &hash})
	if len(got) != 1 || got[0] != "password" {
		t.Fatalf("password only: %v", got)
	}
	got = loginMethodsOf(userRow{GoogleSub: &sub})
	if len(got) != 1 || got[0] != "google" {
		t.Fatalf("google only: %v", got)
	}
	got = loginMethodsOf(userRow{PasswordHash: &hash, GoogleSub: &sub})
	if len(got) != 2 || got[0] != "password" || got[1] != "google" {
		t.Fatalf("both: %v", got)
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
