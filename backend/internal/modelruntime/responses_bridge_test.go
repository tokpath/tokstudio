package modelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

func TestCodexResponsesBridgePreservesClientProtocolAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		if r.URL.Path != "/backend-api/codex/responses" || r.Header.Get("ChatGPT-Account-Id") != "mock-account" ||
			json.NewDecoder(r.Body).Decode(&body) != nil || string(body["model"]) != "\"upstream-exact-model\"" ||
			string(body["store"]) != "false" || string(body["stream"]) != "true" || body["max_output_tokens"] != nil ||
			strings.Contains(string(body["input"]), `"role":"system"`) ||
			!strings.Contains(string(body["input"])+string(body["instructions"]), "reply briefly") || !strings.Contains(string(body["input"]), "hello") {
			t.Error("wrong Codex request", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"type":"response.created","response":{"id":"resp_test","object":"response","created_at":123,"status":"in_progress","model":"upstream-exact-model","output":[]}}`,
			`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_test","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
			`{"type":"response.content_part.added","item_id":"msg_test","output_index":0,"content_index":0,"part":{"type":"output_text","text":"","annotations":[]}}`,
			`{"type":"response.output_text.delta","item_id":"msg_test","output_index":0,"content_index":0,"delta":"ok"}`,
			`{"type":"response.output_text.done","item_id":"msg_test","output_index":0,"content_index":0,"text":"ok"}`,
			`{"type":"response.completed","response":{"id":"resp_test","object":"response","created_at":123,"model":"upstream-exact-model","status":"completed","output":[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":6,"input_tokens_details":{"cached_tokens":2},"output_tokens":3,"total_tokens":9}}}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	t.Cleanup(server.Close)
	endpoint := server.URL + "/backend-api/codex/responses"
	b, err := NewBifrost(t.Context(), BifrostConfig{
		Providers: []string{"openai"}, Credentials: testCredentials{}, Subscriptions: testSubscriptionCredential{},
		AllowTestLoopback: true, subscriptionTestEndpoints: map[string]string{subscriptionauth.Codex: endpoint},
		EndpointGuard: func(_ context.Context, raw string) error {
			if raw != endpoint {
				return ErrInvalidRequest
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	for _, sample := range []struct {
		protocol                    Operation
		body, response, streamEvent string
	}{
		{Chat, `"messages":[{"role":"system","content":"reply briefly"},{"role":"user","content":"hello"}],"max_completion_tokens":20`,
			`"object":"chat.completion"`, `"object":"chat.completion.chunk"`},
		{Messages, `"messages":[{"role":"user","content":"hello"}],"system":"reply briefly","max_tokens":20`,
			`"type":"message"`, "message_stop"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", sample.protocol, stream), func(t *testing.T) {
				options := ""
				if stream && sample.protocol == Chat {
					options = `,"stream_options":{"include_usage":true}`
				}
				body := fmt.Sprintf(`{"model":"public-model","stream":%t,%s%s}`, stream, sample.body, options)
				r, err := DecodeProtocol("request", sample.protocol, []byte(body))
				if err != nil {
					t.Fatal(err)
				}
				r, err = ResponsesFromClient(r)
				if err != nil {
					t.Fatal(err)
				}
				cap := DefaultBilling(Responses).Capability(Responses, stream)
				a := testAttempt("", cap)
				a.SubscriptionKind, a.SubscriptionAccountID, a.SubscriptionOutputLimit = subscriptionauth.Codex, "account", 20
				var output strings.Builder
				out, err := b.Execute(t.Context(), a, r, func(_ context.Context, event Event) error {
					output.WriteString(event.Type)
					output.Write(event.Data)
					return nil
				})
				if err != nil || out.State != "completed" || !out.Usage.Reliable || out.Cost != nil {
					t.Fatalf("bridge state=%s usage=%+v err=%v", out.State, out.Usage, err)
				}
				marker := sample.response
				if stream {
					marker = sample.streamEvent
				} else {
					output.Write(out.Output)
				}
				if raw := output.String(); !strings.Contains(raw, marker) || !strings.Contains(raw, "ok") || !strings.Contains(raw, "public-model") ||
					strings.Contains(raw, "upstream-exact-model") {
					t.Fatalf("unexpected client output %s", raw)
				}
				if options != "" && (!strings.Contains(output.String(), `"choices":[]`) || strings.Count(output.String(), `"prompt_tokens":`) != 1) {
					t.Fatal("Chat Completions did not emit a separate, single usage frame", output.String())
				}
				r.Params = json.RawMessage(`{"max_output_tokens":10}`)
				if err := ValidateSubscriptionRequest(subscriptionauth.Codex, 20, r); !errors.Is(err, ErrUnsupported) {
					t.Fatalf("silently changed a requested token cap: %v", err)
				}
			})
		}
	}
}
