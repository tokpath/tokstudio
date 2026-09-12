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

func TestNormalizeSecretValueStripsQuotes(t *testing.T) {
	cases := map[string]string{
		`GOCSPX-plain`:        "GOCSPX-plain",
		`"GOCSPX-quoted"`:     "GOCSPX-quoted",
		`'GOCSPX-single'`:     "GOCSPX-single",
		`  "GOCSPX-padded"  `: "GOCSPX-padded",
		`""`:                  "",
		`'`:                   "'",
		`856270696615-abc.apps.googleusercontent.com`: "856270696615-abc.apps.googleusercontent.com",
	}
	for in, want := range cases {
		if got := normalizeSecretValue(in); got != want {
			t.Fatalf("normalizeSecretValue(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRedactedMapGoogleShape(t *testing.T) {
	cfg := &Config{
		GoogleClientID:     "856270696615-oduqir6in2hl1u64gcrbgk30rpddap3b.apps.googleusercontent.com",
		GoogleClientSecret: "GOCSPX-example-not-real",
		GoogleRedirect:     "https://test.tokpath.com/login/oauth/google",
	}
	m := cfg.RedactedMap()
	if m["google_triad"] != true {
		t.Fatal("expected triad true")
	}
	if m["google_secret_looks_web"] != true {
		t.Fatal("expected GOCSPX prefix flag")
	}
	if m["google_secret_len"] != len(cfg.GoogleClientSecret) {
		t.Fatal("unexpected secret len")
	}
	if m["google_secret_has_space"] != false {
		t.Fatal("secret should not report spaces")
	}
	wantSuffix := suffixToken(cfg.GoogleClientID, 24)
	if got, _ := m["google_client_id_suffix"].(string); got != wantSuffix {
		t.Fatalf("suffix=%q want %q", got, wantSuffix)
	}
	if m["google_redirect"] != cfg.GoogleRedirect {
		t.Fatal("redirect should be logged (public)")
	}
	if _, ok := m["google_client_secret"]; ok {
		t.Fatal("must never redact-map the raw secret")
	}
}
