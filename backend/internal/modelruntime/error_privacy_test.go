package modelruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

type privacyCredential string

func (s privacyCredential) ResolveCredential(context.Context, string, string) (Credential, error) {
	return Credential{ID: "credential", Secret: string(s)}, nil
}

// Exercise the real SDK and HTTP boundary. The mock echoes the actual outbound
// credential in ordinary text, arbitrary fields, headers and encoded forms.
func TestBifrostSupplierErrorsNeverEscape(t *testing.T) {
	const secret = "opaque+supplier/key=with-no-recognizable-prefix"
	const marker = "supplier-private-diagnostic"
	for _, tc := range []struct {
		name      string
		op        Operation
		stream    bool
		status    int
		late      bool
		embedded  bool
		plainText bool
	}{
		{name: "chat_authentication", op: Chat, status: 401},
		{name: "chat_rate_limit", op: Chat, status: 429},
		{name: "chat_plain_failure", op: Chat, status: 503, plainText: true},
		{name: "chat_stream_rejection", op: Chat, stream: true, status: 401},
		{name: "chat_stream_after_output", op: Chat, stream: true, status: 200, late: true},
		{name: "responses", op: Responses, status: 403},
		{name: "responses_stream", op: Responses, stream: true, status: 401},
		{name: "video_submission", op: Video, status: 401},
		{name: "video_polling", op: VideoRetrieve, status: 401},
		{name: "video_download", op: VideoDownload, status: 401},
		{name: "video_task_failure_in_http_200", op: VideoRetrieve, status: 200, embedded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				if key != secret {
					t.Error("the selected credential was not sent to the mock")
				}
				message := marker + ": credential " + key + " encoded " + url.QueryEscape(key) + " base64 " + base64.StdEncoding.EncodeToString([]byte(key))
				failure := map[string]any{"message": message, "code": message, "type": message, "unknown_debug_field": message}
				w.Header().Set("X-Provider-Debug", message)
				w.Header().Set("Content-Type", "application/json")
				if tc.late {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"id\":\"stream\",\"object\":\"chat.completion.chunk\",\"model\":\"upstream-exact-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
					w.(http.Flusher).Flush()
					raw, _ := json.Marshal(map[string]any{"error": failure})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", raw)
					return
				}
				w.WriteHeader(tc.status)
				if tc.plainText {
					fmt.Fprint(w, message)
					return
				}
				payload := map[string]any{"error": failure}
				if tc.embedded {
					payload["id"], payload["model"], payload["status"] = "task", "upstream-exact-model", "failed"
				}
				_ = json.NewEncoder(w).Encode(payload)
			}))
			defer server.Close()
			b := newTestRuntime(t)
			b.config.Credentials = privacyCredential(secret)
			op := tc.op
			if op == VideoRetrieve || op == VideoDownload {
				op = Video
			}
			a := testAttempt(server.URL, testCapability(op, tc.stream))
			r := Request{ID: "request", PublicModel: "public-model", Operation: tc.op, Stream: tc.stream, Input: json.RawMessage(`{}`), Params: json.RawMessage(`{}`), InputTypes: []string{"text"}}
			switch tc.op {
			case Chat:
				r.Input = json.RawMessage(`[{"role":"user","content":"hi"}]`)
			case Responses:
				r.Input = json.RawMessage(`[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]`)
			case Video:
				r.Input = json.RawMessage(`{"prompt":"a blue dot"}`)
			case VideoRetrieve, VideoDownload:
				a.TaskID = "task"
			}
			var events []json.RawMessage
			out, err := b.Execute(context.Background(), a, r, func(_ context.Context, event Event) error {
				events = append(events, event.Data)
				return nil
			})
			if err == nil || calls.Load() != 1 {
				t.Fatalf("expected one rejected upstream attempt, calls=%d err=%v", calls.Load(), err)
			}
			if tc.late && (!out.OutputDelivered || len(events) == 0) {
				t.Fatal("late failure did not exercise an already-started stream")
			}
			raw, _ := json.Marshal([]any{out, events, out.Output, err.Error(), Diagnostic(err)})
			for _, private := range []string{secret, marker, url.QueryEscape(secret), base64.StdEncoding.EncodeToString([]byte(secret))} {
				if strings.Contains(string(raw), private) {
					t.Fatal("supplier-controlled content escaped the runtime")
				}
			}
			if tc.status != 200 && out.ProviderStatus != tc.status {
				t.Fatalf("numeric supplier status lost: %d", out.ProviderStatus)
			}
		})
	}
}

func TestDiagnosticsDiscardTransportMessagesAndWrappedSecrets(t *testing.T) {
	const secret = "ordinary-opaque-credential"
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{errors.New(secret), "model_execution_failed"},
		{&url.Error{Op: "POST", URL: "https://user:" + secret + "@example.invalid/?key=" + secret, Err: errors.New(secret)}, "model_execution_failed"},
		{fmt.Errorf("%s: %w", secret, context.DeadlineExceeded), "upstream_timeout"},
		{fmt.Errorf("%s: %w", secret, ErrInvalidRequest), "invalid_model_request"},
		{fmt.Errorf("%s: %w", secret, &UpstreamFailure{Status: 401}), "upstream_authentication_failed"},
	} {
		if got := Diagnostic(tc.err); got != tc.want {
			t.Fatalf("unsafe diagnostic: %q", got)
		}
	}
}

func TestProtocolFailureBodiesAreNeverPublished(t *testing.T) {
	for _, body := range []string{
		`{"error":{"message":"private-key","code":"private-key"}}`,
		`{"type":"error","message":"private-key","code":"private-key"}`,
		`{"type":"response.failed","response":{"error":{"message":"private-key"}}}`,
		`{"response":{"status":"failed","error":{"message":"private-key"}}}`,
		`{"status":"failed","error":"private-key"}`,
	} {
		var input any
		_ = json.Unmarshal([]byte(body), &input)
		out, err := publicOutput(input, "public-model")
		var failure *UpstreamFailure
		if len(out) != 0 || !errors.As(err, &failure) || strings.Contains(err.Error(), "private-key") {
			t.Fatal("embedded provider failure was published")
		}
	}
	// A normal response has a null error field; user-authored content remains intact.
	out, err := publicOutput(map[string]any{"error": nil, "model": "supplier-model", "output": "ordinary model content"}, "public-model")
	if err != nil || !strings.Contains(string(out), "ordinary model content") {
		t.Fatal("ordinary successful output was rejected")
	}
}

func TestBifrostErrorMetadataCannotBecomeDiagnostics(t *testing.T) {
	status := 401
	private := "unrecognizable-provider-credential"
	for _, raw := range []any{private, []byte(private), json.RawMessage(`{"debug":"` + private + `"}`), map[string]any{"debug": private}} {
		failure := &schemas.BifrostError{StatusCode: &status, Error: &schemas.ErrorField{Message: private, Type: &private, Code: &private}, ExtraFields: schemas.BifrostErrorExtraFields{RawResponse: raw}}
		result, err := executionFailure(Result{}, failure)
		if result.ErrorCode != "upstream_authentication_failed" || err.Error() != result.ErrorCode {
			t.Fatal("metadata was not discarded")
		}
	}
}
