package catalog

import (
	"testing"
	"time"
)

func TestAccountUsable(t *testing.T) {
	now := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)

	cases := []struct {
		name     string
		row      accountRow
		publicID string
		want     bool
	}{
		{"active open", accountRow{Status: AccountActive}, "tokenhub/echo-1", true},
		{"disabled", accountRow{Status: AccountDisabled}, "tokenhub/echo-1", false},
		{"invalid", accountRow{Status: AccountInvalid}, "tokenhub/echo-1", false},
		{"rotated", accountRow{Status: AccountRotated}, "tokenhub/echo-1", false},
		{"exhausted", accountRow{Status: AccountExhausted}, "tokenhub/echo-1", false},
		{"cooling", accountRow{Status: AccountCooldown, CooldownUntil: &future}, "tokenhub/echo-1", false},
		{"cooldown expired", accountRow{Status: AccountCooldown, CooldownUntil: &past}, "tokenhub/echo-1", true},
		{"cooldown no until", accountRow{Status: AccountCooldown}, "tokenhub/echo-1", false},
		{"tag miss", accountRow{Status: AccountActive, ModelTags: "other/model"}, "tokenhub/echo-1", false},
		{"tag hit", accountRow{Status: AccountActive, ModelTags: "tokenhub/echo-1"}, "tokenhub/echo-1", true},
	}
	for _, tc := range cases {
		if got := accountUsable(tc.row, now, tc.publicID); got != tc.want {
			t.Fatalf("%s: usable=%v want %v", tc.name, got, tc.want)
		}
	}
	if accountFingerprint("abcdef12zzzz") != "abcdef12" {
		t.Fatal("fingerprint should be first 8 hex chars")
	}
}
