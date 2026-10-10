package identity

import "testing"

func TestSafeReturnPath(t *testing.T) {
	for _, value := range []string{"/", "/app/referral?tab=settlements", "/models/openai/gpt?tab=agent&next=%2Fapp"} {
		if got := SafeReturnPath(value); got != value {
			t.Fatalf("valid %q => %q", value, got)
		}
	}
	for _, value := range []string{"https://evil.test", "//evil.test", "/\\evil.test", "/%5cevil.test", "/%255cevil.test", "/%2f%2fevil.test", "/\n/evil.test", "/%0d%0aLocation:evil", "/%invalid"} {
		if got := SafeReturnPath(value); got != "" {
			t.Fatalf("unsafe %q => %q", value, got)
		}
	}
}
