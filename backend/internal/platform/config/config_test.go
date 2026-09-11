package config

import "testing"

func TestGoogleTriad(t *testing.T) {
	empty := &Config{}
	if empty.GoogleTriad() || empty.GoogleConfigured() {
		t.Fatal("empty config must not look configured")
	}

	full := &Config{
		GoogleClientID:     "cid",
		GoogleClientSecret: "sec",
		GoogleRedirect:     "https://example.com/cb",
	}
	if !full.GoogleTriad() || !full.GoogleConfigured() {
		t.Fatal("triad must be configured")
	}

	partial := &Config{GoogleClientID: "cid", GoogleRedirect: "https://example.com/cb"}
	if partial.GoogleTriad() {
		t.Fatal("missing secret must not look configured")
	}
}
