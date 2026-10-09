package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	python, _ := examples["python"].(string)
	if !strings.Contains(python, `os.environ["TOKENHUB_API_KEY"]`) || strings.Contains(python, "api_key='...'") {
		t.Fatalf("python: %s", python)
	}
	if !strings.Contains(python, "仅标准库") || !strings.Contains(python, "python3 chat.py") {
		t.Fatalf("python run comments: %s", python)
	}
	node, _ := examples["node"].(string)
	if !strings.Contains(node, "process.env.TOKENHUB_API_KEY") || !strings.Contains(node, "await fetch(") {
		t.Fatalf("node: %s", node)
	}
	if !strings.Contains(node, "node chat.mjs") {
		t.Fatalf("node run comments: %s", node)
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
	if !strings.Contains(got.body, `"model":"tokenhub/oem-demo"`) || !strings.Contains(got.body, `"max_tokens":32`) {
		t.Fatalf("body %q", got.body)
	}
}

func TestDocsPythonAndNodeExamplesPostJSON(t *testing.T) {
	t.Run("python", func(t *testing.T) {
		runDocsSDKExample(t, "python")
	})
	t.Run("node", func(t *testing.T) {
		runDocsSDKExample(t, "node")
	})
}

type capturedChat struct {
	method, path, auth, ctype, body string
}

func runDocsSDKExample(t *testing.T, kind string) {
	t.Helper()
	got := capturedChat{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		got.ctype = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		got.body = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","created":0,"model":"tokenhub/oem-demo","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)

	examples := docsExamples("api.oem.localhost", "tokenhub/oem-demo")
	dir := t.TempDir()
	env := append(envWithoutAPIKey(), "TOKENHUB_API_KEY=thk_virtual_test", "HOME="+dir, "NPM_CONFIG_CACHE="+filepath.Join(dir, "npm-cache"))
	var cmd *exec.Cmd
	switch kind {
	case "python":
		src, _ := examples["python"].(string)
		src = strings.ReplaceAll(src, "https://api.oem.localhost", srv.URL)
		script := filepath.Join(dir, "chat.py")
		if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(lookPath(t, "python3", "python"), script)
	case "node":
		src, _ := examples["node"].(string)
		src = strings.ReplaceAll(src, "https://api.oem.localhost", srv.URL)
		script := filepath.Join(dir, "chat.mjs")
		if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(lookPath(t, "node"), script)
		cmd.Dir = dir
	default:
		t.Fatalf("kind %s", kind)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s example %v: %s", kind, err, out)
	}
	if got.method != http.MethodPost {
		t.Fatalf("method %q", got.method)
	}
	if got.path != "/v1/chat/completions" {
		t.Fatalf("path %q body %s", got.path, got.body)
	}
	if got.auth != "Bearer thk_virtual_test" {
		t.Fatalf("authorization %q", got.auth)
	}
	var payload struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(got.body), &payload); err != nil {
		t.Fatalf("json %v: %s", err, got.body)
	}
	if payload.Model != "tokenhub/oem-demo" {
		t.Fatalf("model %q", payload.Model)
	}
	if len(payload.Messages) != 1 || payload.Messages[0].Role != "user" || payload.Messages[0].Content != "hi" {
		t.Fatalf("messages %+v", payload.Messages)
	}
}

func lookPath(t *testing.T, names ...string) string {
	t.Helper()
	for _, name := range names {
		path, err := exec.LookPath(name)
		if err == nil {
			return path
		}
	}
	t.Fatalf("need %v on PATH", names)
	return ""
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

func TestDocsAPIBaseUsesLocalListenerAndPreservesPublicBrand(t *testing.T) {
	for _, tc := range []struct{ domain, public, want string }{
		{"localhost", "http://localhost:9080", "http://localhost:9080"},
		{"api.oem.localhost", "http://localhost:9080", "http://api.oem.localhost:9080"},
		{"localhost:9443", "https://localhost:9443", "https://localhost:9443"},
		{"[::1]:9443", "https://localhost:9443", "https://[::1]:9443"},
		{"api.customer.example", "http://localhost:9080", "https://api.customer.example"},
		{"api.customer.example:8443", "https://platform.example", "https://api.customer.example:8443"},
	} {
		t.Run(tc.domain+tc.public, func(t *testing.T) {
			base := docsAPIBase(tc.domain, tc.public)
			if base != tc.want {
				t.Fatalf("got %q, want %q", base, tc.want)
			}
			for _, language := range []string{"curl", "python", "node", "messages"} {
				if !strings.Contains(docsExamples(base, "test/model")[language].(string), tc.want+"/v1") {
					t.Fatalf("%s example does not use configured public endpoint", language)
				}
			}
		})
	}
}

func TestDocsAPIBaseRejectsMissingAndMalformedBrandDomain(t *testing.T) {
	for _, domain := range []string{"", " ", "https://api.customer.example", "http://api.customer.example", "user:password@api.customer.example", "api.customer.example/path", "api.customer.example/", "api.customer.example?x=1", "api.customer.example?", "api.customer.example#part", "api.customer.example#", "api.customer.example:bad", "api.customer.example:0", "api.customer.example:65536", "api.customer.example:", ":::::", "https://https://api.customer.example", "api.customer.example\\path"} {
		t.Run(domain, func(t *testing.T) {
			base := docsAPIBase(domain, "https://platform.example")
			if base != "" {
				t.Fatalf("malformed brand endpoint became copyable: %q", base)
			}
			if examples := docsExamplesFor(base, "test/model", []string{"/v1/chat/completions"}); len(examples) != 0 {
				t.Fatalf("missing endpoint generated examples: %+v", examples)
			}
		})
	}
}
