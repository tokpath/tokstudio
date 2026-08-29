package logx

import "testing"

func TestIsSensitiveKey(t *testing.T) {
	cases := []string{"password", "Authorization", "api-key", "secret_ciphertext", "TOKENHUB_BOOTSTRAP_ADMIN_TOKEN"}
	for _, key := range cases {
		if !IsSensitiveKey(key) {
			t.Fatalf("expected %q to be sensitive", key)
		}
	}
	if IsSensitiveKey("request_id") {
		t.Fatal("request_id must not be treated as secret")
	}
}

func TestRedactString(t *testing.T) {
	if RedactString("Bearer super-secret") != "[REDACTED]" {
		t.Fatal("bearer token must be redacted")
	}
	if RedactString("health check ok") != "health check ok" {
		t.Fatal("ordinary text must stay intact")
	}
}
