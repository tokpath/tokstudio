package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDocsExamplesUseBrandAndPlaceholderKey(t *testing.T) {
	examples := docsExamples("api.oem.localhost", "tokenhub/oem-demo")
	curl, _ := examples["curl"].(string)
	wantHeader := `"Authorization: Bearer ${TOKENHUB_API_KEY}"`
	for _, part := range []string{"https://api.oem.localhost/v1/chat/completions", "tokenhub/oem-demo", wantHeader} {
		if !strings.Contains(curl, part) {
			t.Fatalf("curl missing %q: %s", part, curl)
		}
	}
	if strings.Contains(curl, `'Authorization:`) {
		t.Fatalf("single-quoted auth would not expand: %s", curl)
	}
	if strings.Contains(curl, "sk-live") || strings.Contains(curl, "thsk_") {
		t.Fatal("examples must not embed a real key")
	}
	messages, _ := examples["messages"].(string)
	if !strings.Contains(messages, "/v1/messages") || !strings.Contains(messages, wantHeader) {
		t.Fatalf("messages: %s", messages)
	}
	notes := docsNotes()
	if notes["webhook"] == "" || notes["errors"] == "" {
		t.Fatalf("notes: %+v", notes)
	}
	auth, _ := notes["auth"].(string)
	if !strings.Contains(auth, "export TOKENHUB_API_KEY") || !strings.Contains(auth, "${TOKENHUB_API_KEY}") {
		t.Fatalf("auth note: %s", auth)
	}
}

func TestDocsCurlExpandsEnvAndPostsJSON(t *testing.T) {
	got := struct {
		method, path, auth, ctype, body string
	}{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.ctype = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		got.body = string(raw)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	curl, _ := docsExamples("api.oem.localhost", "tokenhub/oem-demo")["curl"].(string)
	rewritten := strings.Replace(curl, "https://api.oem.localhost", srv.URL, 1)
	cmd := exec.Command("bash", "-lc", rewritten)
	cmd.Env = append(envWithoutAPIKey(), "TOKENHUB_API_KEY=thk_virtual_test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("curl %v: %s\ncmd: %s", err, out, rewritten)
	}
	if got.method != http.MethodPost {
		t.Fatalf("method %q", got.method)
	}
	if got.path != "/v1/chat/completions" {
		t.Fatalf("path %q", got.path)
	}
	if got.auth != "Bearer thk_virtual_test" {
		t.Fatalf("authorization %q (literal env name leaked?)", got.auth)
	}
	if !strings.HasPrefix(got.ctype, "application/json") {
		t.Fatalf("content-type %q", got.ctype)
	}
	if got.body != `{"model":"tokenhub/oem-demo","messages":[{"role":"user","content":"hi"}]}` {
		t.Fatalf("body %q", got.body)
	}
}

func envWithoutAPIKey() []string {
	out := make([]string, 0, 16)
	for _, item := range os.Environ() {
		if strings.HasPrefix(item, "TOKENHUB_API_KEY=") {
			continue
		}
		out = append(out, item)
	}
	return out
}
