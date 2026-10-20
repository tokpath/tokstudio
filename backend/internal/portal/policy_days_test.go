package portal

import "testing"

func TestPolicyDaysRoundTrip(t *testing.T) {
	seconds, ok := policySecondsFromDays(30, 0, 365)
	if !ok || seconds != 30*86400 || policyDays(seconds) != 30 {
		t.Fatal(seconds, ok)
	}
	reward, ok := policySecondsFromDays(30, 1, 3650)
	if !ok || reward != 30*86400 {
		t.Fatal(reward, ok)
	}
	if _, ok = policySecondsFromDays(0, 1, 3650); ok {
		t.Fatal("reward validity must be at least one day")
	}
	if _, ok = policySecondsFromDays(366, 0, 365); ok {
		t.Fatal("confirmation wait cannot exceed 365 days")
	}
	if policyDays(60) != 1 || policyDays(0) != 0 || policyDays(86400) != 1 {
		t.Fatal(policyDays(60), policyDays(0), policyDays(86400))
	}
}
