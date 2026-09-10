package config

import "testing"

func TestGoogleTriadAndMockGate(t *testing.T) {
	empty := &Config{}
	if empty.GoogleTriad() || empty.GoogleMockAllowed() {
		t.Fatal("empty config must not look configured or allow mock")
	}

	partial := &Config{GoogleClientID: "id.apps.googleusercontent.com", GoogleRedirect: "https://grok.tokpath.com/login/oauth/google"}
	if partial.GoogleTriad() {
		t.Fatal("missing secret is not a triad")
	}

	triad := &Config{
		GoogleClientID:     "id.apps.googleusercontent.com",
		GoogleClientSecret: "secret",
		GoogleRedirect:     "https://grok.tokpath.com/login/oauth/google",
	}
	if !triad.GoogleTriad() {
		t.Fatal("triad must be complete")
	}

	devMock := &Config{Env: "development", GoogleAllowMock: true, PublicBaseURL: "http://localhost:8080"}
	if !devMock.GoogleMockAllowed() {
		t.Fatal("local development may opt into mock")
	}

	prodMock := &Config{Env: "production", GoogleAllowMock: true, PublicBaseURL: "https://www.tokpath.com"}
	if prodMock.GoogleMockAllowed() {
		t.Fatal("production must forbid mock even if ALLOW_MOCK=true")
	}

	grokMock := &Config{Env: "development", GoogleAllowMock: true, PublicBaseURL: "https://grok.tokpath.com"}
	if grokMock.GoogleMockAllowed() {
		t.Fatal("grok preview must forbid mock")
	}

	testMock := &Config{Env: "development", GoogleAllowMock: true, WebOrigin: "https://test.tokpath.com"}
	if testMock.GoogleMockAllowed() {
		t.Fatal("test preview must forbid mock")
	}

	redacted := triad.RedactedMap()
	if redacted["google_secret_set"] != true || redacted["google_triad"] != true {
		t.Fatalf("redacted must flag secret without printing it: %+v", redacted)
	}
	for _, value := range redacted {
		if s, ok := value.(string); ok && (s == "secret" || s == triad.GoogleClientSecret) {
			t.Fatalf("secret leaked in redacted map: %+v", redacted)
		}
	}
}
