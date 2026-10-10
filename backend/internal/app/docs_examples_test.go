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
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestDocsToolAndSDKExamplesFollowActualEndpoints(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/videos", "/v1/images/generations"} {
		examples := docsExamplesFor("http://api.brand.localhost:9080", "vendor/a'b", []string{path})
		entry := examples[path].(gin.H)
		_, sdk := entry["python_sdk"]
		if sdk != (path == "/v1/chat/completions" || path == "/v1/responses") {
			t.Fatalf("wrong SDK support for %s", path)
		}
		_, aider := entry["aider"]
		if aider != (path == "/v1/chat/completions") {
			t.Fatalf("wrong Agent support for %s", path)
		}
		for _, language := range []string{"python_sdk", "node_sdk"} {
			if source, ok := entry[language].(string); ok {
				if !strings.Contains(source, "http://api.brand.localhost:9080/v1") || !strings.Contains(source, "TOKENHUB_API_KEY") || strings.Contains(source, "api.openai.com") {
					t.Fatalf("wrong brand/secret in %s: %s", language, source)
				}
				if path == "/v1/responses" && (strings.Contains(source, "store=") || strings.Contains(source, "stream=") || strings.Contains(source, "previous_response_id")) {
					t.Fatalf("unsupported Responses lifecycle in example: %s", source)
				}
			}
		}
	}
	if strings.Contains(docsNotes()["errors"].(string), "key_budget_unbounded") || !strings.Contains(docsNotes()["errors"].(string), "price_estimate_unavailable") {
		t.Fatal("outdated budget errors")
	}
}

func TestDocsAiderCommandKeepsBrandAndFullModelID(t *testing.T) {
	dir := t.TempDir()
	// This stub checks shell argument/env encoding, not Agent integration.
	stub := filepath.Join(dir, "aider")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s\\n' \"$OPENAI_API_BASE\" \"$OPENAI_API_KEY\" \"$1\" \"$2\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	model := "vendor/quote'$(bad-command) model"
	source := docsExamplesFor("http://api.brand.localhost:9080", model, []string{"/v1/chat/completions"})["/v1/chat/completions"].(gin.H)["aider"].(string)
	command := exec.Command("sh", "-c", source)
	command.Env = append(envWithoutAPIKey(), "PATH="+dir+":"+os.Getenv("PATH"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("shell: %v %s", err, output)
	}
	want := "http://api.brand.localhost:9080/v1\nPASTE_YOUR_KEY\n--model\nopenai/" + model + "\n"
	if string(output) != want {
		t.Fatalf("got %q, want %q", output, want)
	}
}

// Opt-in SDK dependencies live outside the repository. This exercises the real
// SDK packages against a local server and never sends a paid model request.
func TestDocsOfficialSDKsExecuteLocally(t *testing.T) {
	sdkDir := os.Getenv("TOKENHUB_DOCS_SDK_DIR")
	if sdkDir == "" {
		t.Skip("set TOKENHUB_DOCS_SDK_DIR to isolated official Node/Python SDK dependencies")
	}
	for _, language := range []string{"python_sdk", "node_sdk"} {
		for _, path := range []string{"/v1/chat/completions", "/v1/responses"} {
			for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable, 0} {
				t.Run(language+path+http.StatusText(status), func(t *testing.T) {
					var count atomic.Int32
					requests := make(chan capturedChat, 4)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						count.Add(1)
						body, _ := io.ReadAll(r.Body)
						requests <- capturedChat{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"), body: string(body)}
						if status == 0 {
							conn, _, err := w.(http.Hijacker).Hijack()
							if err == nil {
								_ = conn.Close()
							}
							return
						}
						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("x-request-id", "req-guide")
						w.WriteHeader(status)
						if status != http.StatusOK {
							_, _ = w.Write([]byte(`{"error":{"code":"provider_unavailable","message":"temporary","request_id":"req-guide"}}`))
						} else if path == "/v1/responses" {
							_, _ = w.Write([]byte(`{"id":"resp-local","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"request_id":"req-guide"}`))
						} else {
							_, _ = w.Write([]byte(`{"id":"chat-local","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"request_id":"req-guide"}`))
						}
					}))
					defer server.Close()
					model := "vendor/quoted'\" model"
					source := docsExamplesFor(server.URL, model, []string{path})[path].(gin.H)[language].(string)
					dir := t.TempDir()
					var command *exec.Cmd
					if language == "python_sdk" {
						file := filepath.Join(dir, "chat.py")
						if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
							t.Fatal(err)
						}
						command = exec.Command(lookPath(t, "python3"), file)
					} else {
						file := filepath.Join(dir, "chat.mjs")
						if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
							t.Fatal(err)
						}
						if err := os.Symlink(filepath.Join(sdkDir, "node_modules"), filepath.Join(dir, "node_modules")); err != nil {
							t.Fatal(err)
						}
						command = exec.Command(lookPath(t, "node"), file)
					}
					command.Env = append(envWithoutAPIKey(), "TOKENHUB_API_KEY=virtual-guide-key", "PYTHONPATH="+filepath.Join(sdkDir, "python"))
					output, err := command.CombinedOutput()
					if status == http.StatusOK && (err != nil || !strings.Contains(string(output), "ok") || !strings.Contains(string(output), "req-guide")) {
						t.Fatalf("example failed: %v %s", err, output)
					}
					if status != http.StatusOK && err == nil {
						t.Fatal("HTTP error treated as success")
					}
					if count.Load() != 1 {
						t.Fatalf("unexpected automatic retry: %d: %s", count.Load(), output)
					}
					got := <-requests
					if got.method != "POST" || got.path != path || got.auth != "Bearer virtual-guide-key" {
						t.Fatalf("wrong request: %+v", got)
					}
					var body map[string]any
					if err := json.Unmarshal([]byte(got.body), &body); err != nil {
						t.Fatal(err)
					}
					if body["model"] != model {
						t.Fatalf("model changed: %s", got.body)
					}
					if path == "/v1/responses" && (body["input"] != "hi" || body["max_output_tokens"] != float64(32)) {
						t.Fatalf("Responses: %s", got.body)
					}
					if path == "/v1/chat/completions" && body["max_tokens"] != float64(32) {
						t.Fatalf("Chat: %s", got.body)
					}
				})
			}
		}
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

func TestDocsHTTPExamplesKeepFailureCodeAndRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"price_estimate_unavailable","message":"cannot estimate","request_id":"req-example-error"}}`))
	}))
	defer server.Close()
	entry := docsExamplesFor(server.URL, "vendor/text", []string{"/v1/chat/completions"})["/v1/chat/completions"].(gin.H)
	for _, language := range []string{"curl", "python", "node"} {
		t.Run(language, func(t *testing.T) {
			var command *exec.Cmd
			if language == "curl" {
				command = exec.Command("sh", "-c", entry[language].(string))
			} else {
				file := filepath.Join(t.TempDir(), "chat.py")
				binary := "python3"
				if language == "node" {
					file = strings.TrimSuffix(file, ".py") + ".mjs"
					binary = "node"
				}
				if err := os.WriteFile(file, []byte(entry[language].(string)), 0o600); err != nil {
					t.Fatal(err)
				}
				command = exec.Command(lookPath(t, binary), file)
			}
			command.Env = append(envWithoutAPIKey(), "TOKENHUB_API_KEY=virtual-guide-key")
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "price_estimate_unavailable") || !strings.Contains(string(output), "req-example-error") {
				t.Fatalf("failure hidden or counted as success: %v %s", err, output)
			}
		})
	}
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
