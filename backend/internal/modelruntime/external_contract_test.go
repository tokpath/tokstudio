package modelruntime

// These tests execute the real embedded SDK against the separately deployed
// integration-lab server. They are opt-in and never target a vendor account.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExternalBifrostProtocolContracts(t *testing.T) {
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	if base == "" {
		t.Skip("TOKENHUB_MODEL_MOCK_URL required for external integration-lab contracts")
	}
	origin, e := url.Parse(base)
	if e != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("external contract test requires explicit test loopback origin")
	}
	b, e := NewBifrost(context.Background(), BifrostConfig{AllowTestLoopback: true, Providers: []string{"openai"}, Credentials: testCredentials{}, EndpointGuard: func(_ context.Context, target string) error {
		u, e := url.Parse(target)
		if e != nil || u.Host != origin.Host {
			return ErrInvalidRequest
		}
		return nil
	}})
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	type sample struct {
		operation             Operation
		endpoint, model, body string
		rules                 []UsageRule
		bounds                map[string]string
		required              []string
		constraints           map[string]ParameterConstraint
	}
	samples := []sample{
		{Chat, "/v1/chat/completions", "test-chat", `{"model":"public-model","messages":[{"role":"user","content":"hello"}],"max_tokens":20,"temperature":0.3}`, []UsageRule{{"input", "token", "/prompt_tokens", false}, {"output", "token", "/completion_tokens", false}}, map[string]string{"input": "100000", "output": "100000"}, []string{"max_completion_tokens"}, nil},
		{Responses, "/v1/responses", "test-responses", `{"model":"public-model","input":"hello","max_output_tokens":20}`, []UsageRule{{"input", "token", "/input_tokens", false}, {"output", "token", "/output_tokens", false}}, map[string]string{"input": "100000", "output": "100000"}, []string{"max_output_tokens"}, nil},
		{Embedding, "/v1/embeddings", "test-embedding", `{"model":"public-model","input":["hello","world"],"dimensions":3}`, []UsageRule{{"input", "token", "/prompt_tokens", false}}, map[string]string{"input": "100000"}, nil, nil},
		{Image, "/v1/images/generations", "test-image", `{"model":"public-model","prompt":"a blue dot","n":1,"size":"1024x1024","response_format":"b64_json"}`, []UsageRule{{"image", "image", "/output_count", false}}, map[string]string{"image": "1"}, []string{"n", "size"}, map[string]ParameterConstraint{"n": {Min: "1", Max: "1"}, "size": {Values: []string{"1024x1024"}}}},
		{Speech, "/v1/audio/speech", "test-speech", `{"model":"public-model","input":"hello","voice":"alloy","response_format":"wav"}`, []UsageRule{{"input_chars", "character", "/input_chars", false}}, map[string]string{"input_chars": "1000"}, []string{"voice", "response_format"}, nil},
	}
	for _, s := range samples {
		t.Run(string(s.operation), func(t *testing.T) {
			r, e := DecodeProtocol("request", s.operation, []byte(s.body))
			if e != nil {
				t.Fatal(e)
			}
			c := testCapability(s.operation, false)
			c.Parameters = nil
			var params map[string]json.RawMessage
			_ = json.Unmarshal(r.Params, &params)
			for key := range params {
				c.Parameters = append(c.Parameters, key)
			}
			c.RequiredParameters = s.required
			c.ParameterConstraints = s.constraints
			c.OutputLimitParameter = ""
			c.BillableBounds = s.bounds
			c.UsageRules = s.rules
			a := testAttempt(base+s.endpoint, c)
			a.UpstreamModel = s.model
			out, e := b.Execute(context.Background(), a, r, nil)
			if e != nil {
				t.Fatalf("SDK protocol %s: state=%s error=%v", s.operation, out.State, e)
			}
			if !out.Usage.Reliable {
				t.Fatal("missing verified usage")
			}
			if e = CheckUsageBounds(c, out.Usage); e != nil {
				t.Fatal(e)
			}
			if s.operation == Speech && len(out.Binary) == 0 {
				t.Fatal("speech bytes missing")
			}
		})
	}
	for _, model := range []string{"test-chat-stream", "test-chat-stream-drop", "test-chat-503", "test-chat-usage-missing"} {
		t.Run(model, func(t *testing.T) {
			stream := strings.Contains(model, "stream")
			body, _ := json.Marshal(map[string]any{"model": "public-model", "messages": []map[string]string{{"role": "user", "content": "contract failure path"}}, "max_tokens": 20, "stream": stream})
			r, e := DecodeProtocol("request", Chat, body)
			if e != nil {
				t.Fatal(e)
			}
			c := testCapability(Chat, stream)
			a := testAttempt(base+"/v1/chat/completions", c)
			a.UpstreamModel = model
			events := 0
			out, e := b.Execute(context.Background(), a, r, func(context.Context, Event) error { events++; return nil })
			switch model {
			case "test-chat-stream":
				if e != nil || !out.Usage.Reliable || events == 0 {
					t.Fatalf("stream result=%#v err=%v", out, e)
				}
			case "test-chat-stream-drop":
				if e == nil || !out.OutputDelivered {
					t.Fatalf("truncated stream state=%s delivered=%v err=%v", out.State, out.OutputDelivered, e)
				}
			case "test-chat-503":
				if e == nil || out.State != "outcome_unknown" {
					t.Fatalf("503 state=%s err=%v", out.State, e)
				}
			case "test-chat-usage-missing":
				if !errors.Is(e, ErrUsageUnknown) || out.Usage.Reliable {
					t.Fatalf("usage missing state=%s err=%v", out.State, e)
				}
			}
		})
	}
	for _, model := range []string{"test-responses-stream", "test-responses-stream-drop"} {
		t.Run(model, func(t *testing.T) {
			r, e := DecodeProtocol("request", Responses, []byte(`{"model":"public-model","input":"hello","stream":true,"max_output_tokens":20}`))
			if e != nil {
				t.Fatal(e)
			}
			c := testCapability(Responses, true)
			c.Parameters = []string{"max_output_tokens"}
			c.RequiredParameters = []string{"max_output_tokens"}
			c.OutputLimitParameter = "max_output_tokens"
			c.UsageRules = []UsageRule{{Component: "input", Unit: "token", Path: "/input_tokens"}, {Component: "output", Unit: "token", Path: "/output_tokens"}}
			a := testAttempt(base+"/v1/responses", c)
			a.UpstreamModel = model
			events := 0
			out, e := b.Execute(context.Background(), a, r, func(_ context.Context, event Event) error {
				events++
				if strings.Contains(string(event.Data), model) {
					t.Error("upstream model leaked into public response stream")
				}
				return nil
			})
			if strings.HasSuffix(model, "drop") {
				if e == nil || !out.OutputDelivered {
					t.Fatalf("Responses truncation state=%s err=%v", out.State, e)
				}
			} else if e != nil || !out.Usage.Reliable || events == 0 {
				t.Fatalf("Responses stream state=%s err=%v", out.State, e)
			}
		})
	}
	t.Run("transcription", func(t *testing.T) {
		wav, _ := base64.StdEncoding.DecodeString("UklGRiQAAABXQVZFZm10IBAAAAABAAEAgD4AAAB9AAACABAAZGF0YQAAAAA=")
		body, _ := json.Marshal(map[string]any{"model": "public-model", "file": wav, "filename": "input.wav", "response_format": "verbose_json"})
		r, e := DecodeProtocol("request", Transcription, body)
		if e != nil {
			t.Fatal(e)
		}
		c := testCapability(Transcription, false)
		c.InputTypes = []string{"audio"}
		c.Parameters = []string{"response_format"}
		c.RequiredParameters = []string{"response_format"}
		c.OutputLimitParameter = ""
		c.UsageRules = []UsageRule{{Component: "seconds", Unit: "second", Path: "/seconds"}}
		c.BillableBounds = map[string]string{"seconds": "100"}
		a := testAttempt(base+"/v1/audio/transcriptions", c)
		a.UpstreamModel = "test-transcription"
		out, e := b.Execute(context.Background(), a, r, nil)
		if e != nil || !out.Usage.Reliable {
			t.Fatalf("transcription state=%s error=%v", out.State, e)
		}
	})
	t.Run("video_submit_lookup_download", func(t *testing.T) {
		r, e := DecodeProtocol("request", Video, []byte(`{"model":"public-model","prompt":"a blue dot","seconds":"4","size":"720x1280"}`))
		if e != nil {
			t.Fatal(e)
		}
		c := testCapability(Video, false)
		c.Parameters = []string{"seconds", "size"}
		c.RequiredParameters = []string{"seconds", "size"}
		c.OutputLimitParameter = ""
		c.ParameterConstraints = map[string]ParameterConstraint{"seconds": {Values: []string{"4"}}, "size": {Values: []string{"720x1280"}}}
		c.UsageRules = []UsageRule{{Component: "video_second", Unit: "second", Path: "/actual_seconds"}}
		c.BillableBounds = map[string]string{"video_second": "4"}
		a := testAttempt(base+"/v1/videos", c)
		a.UpstreamModel = "test-video"
		out, e := b.Execute(context.Background(), a, r, nil)
		if e != nil || out.State != "submitted" || out.TaskID == "" {
			t.Fatalf("video submit state=%s task=%s err=%v", out.State, out.TaskID, e)
		}
		a.TaskID = out.TaskID
		a.BaseURL = base + "/v1/videos/" + url.PathEscape(out.TaskID)
		client := &http.Client{Timeout: 5 * time.Second}
		advanceBody := bytes.NewBufferString(`{"seconds":30}`)
		advance, e := client.Post(base+"/__control/advance", "application/json", advanceBody)
		if e != nil {
			t.Fatal(e)
		}
		_ = advance.Body.Close()
		r.Operation = VideoRetrieve
		r.Input = json.RawMessage(`{}`)
		r.Params = json.RawMessage(`{}`)
		out, e = b.Execute(context.Background(), a, r, nil)
		if e != nil && !errors.Is(e, ErrUsageUnknown) {
			t.Fatal(e)
		}
		if out.State != "completed" {
			t.Fatalf("video lookup state=%s", out.State)
		}
		// The SDK's current video usage exposes monetary float cost only. It is
		// deliberately not accepted as actual seconds or exact supplier cost.
		if out.Usage.Reliable {
			t.Fatal("SDK backfilled seconds claimed as provider measurement")
		}
		if !errors.Is(e, ErrUsageUnknown) {
			t.Fatal("missing video metering was hidden")
		}
		// Native metadata is captured separately from SDK backfill. Only the
		// actual completed provider response's seconds field can settle this
		// verified duration-based combination; the whole raw body is discarded.
		a.Capability.UsageRules = []UsageRule{{Component: "video_second", Unit: "second", Path: "/provider/seconds"}}
		out, e = b.Execute(context.Background(), a, r, nil)
		if e != nil || !out.Usage.Reliable || out.Usage.Source != "provider_metadata" {
			t.Fatalf("native video usage=%#v err=%v", out.Usage, e)
		}
		if e = CheckUsageBounds(a.Capability, out.Usage); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(string(out.Usage.Evidence), "prompt") {
			t.Fatal("video prompt retained in usage evidence")
		}
		r.Operation = VideoDownload
		a.BaseURL += "/content"
		out, e = b.Execute(context.Background(), a, r, nil)
		if e != nil || len(out.Binary) == 0 {
			t.Fatalf("video download bytes=%d err=%v", len(out.Binary), e)
		}
	})
	t.Run("request_evidence", func(t *testing.T) {
		response, e := http.Get(base + "/__control/requests")
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		var state any
		if e = json.NewDecoder(response.Body).Decode(&state); e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(state)
		if !strings.Contains(string(raw), "test-chat") || !strings.Contains(string(raw), "max_completion_tokens") {
			t.Fatal("external server did not record full SDK parameters")
		}
		if strings.Contains(string(raw), "platform-secret-do-not-log") {
			t.Fatal("external server exposed raw credential")
		}
	})
}
