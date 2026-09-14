package catalog

import "testing"

func TestValidateUpstreamURL(t *testing.T) {
	if err := ValidateUpstreamURL("", true, nil); err != nil {
		t.Fatalf("empty url should be allowed: %v", err)
	}
	if err := ValidateUpstreamURL("http://169.254.169.254/latest", false, nil); err == nil {
		t.Fatal("metadata IP must be blocked even in development")
	}
	if err := ValidateUpstreamURL("http://127.0.0.1:8080", false, nil); err == nil {
		t.Fatal("loopback IP must be blocked")
	}
	if err := ValidateUpstreamURL("http://api.openai.com", false, nil); err != nil {
		t.Fatalf("dev may use http public host: %v", err)
	}
	if err := ValidateUpstreamURL("http://api.openai.com", true, nil); err == nil {
		t.Fatal("production must require https")
	}
	if err := ValidateUpstreamURL("https://evil.example", true, nil); err == nil {
		t.Fatal("production must require allowlist")
	}
	if err := ValidateUpstreamURL("https://api.openai.com/v1", true, nil); err != nil {
		t.Fatalf("openai https should pass: %v", err)
	}
	if err := ValidateUpstreamURL("https://coding.dashscope.aliyuncs.com/v1", true, nil); err != nil {
		t.Fatalf("dashscope https should pass: %v", err)
	}
	if err := ValidateUpstreamURL("https://proxy.mycorp.test", true, []string{"mycorp.test"}); err != nil {
		t.Fatalf("extra allowlist should pass: %v", err)
	}
}
