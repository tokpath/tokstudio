package modelruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestExternalBifrostMediaStreamingContracts(t *testing.T) {
	base := strings.TrimRight(os.Getenv("TOKENHUB_MODEL_MOCK_URL"), "/")
	if base == "" {
		t.Skip("independent model HTTP protocol lab required")
	}
	origin, e := url.Parse(base)
	if e != nil || origin.Hostname() != "127.0.0.1" {
		t.Fatal("explicit test loopback origin required")
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
	for _, sample := range []struct {
		op          Operation
		path, input string
	}{
		{Image, "/v1/images/generations", `{"model":"public-model","stream":true,"prompt":"a blue dot","n":1,"size":"1024x1024","quality":"high","output_format":"png","partial_images":1}`},
		{Speech, "/v1/audio/speech", `{"model":"public-model","stream":true,"input":"hello","voice":"alloy","response_format":"wav"}`},
		{Transcription, "/v1/audio/transcriptions", `{"model":"public-model","stream":true,"file":"UklGRiQAAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQAAAAA=","response_format":"json","language":"en"}`},
	} {
		for _, mode := range []string{"ok", "stream-drop", "usage-missing"} {
			t.Run(string(sample.op)+"/"+mode, func(t *testing.T) {
				r, e := DecodeProtocol("request", sample.op, []byte(sample.input))
				if e != nil {
					t.Fatal(e)
				}
				c := testCapability(sample.op, true)
				c.InputTypes = append([]string{}, r.InputTypes...)
				c.Parameters = nil
				c.RequiredParameters = nil
				c.OutputLimitParameter = ""
				c.MaxOutput = 0
				c.ParameterConstraints = nil
				var params map[string]json.RawMessage
				_ = json.Unmarshal(r.Params, &params)
				for p := range params {
					c.Parameters = append(c.Parameters, p)
					c.RequiredParameters = append(c.RequiredParameters, p)
				}
				c.UsageRules = []UsageRule{{Component: "input", Unit: "token", Path: "/input_tokens"}, {Component: "output", Unit: "token", Path: "/output_tokens"}}
				c.BillableBounds = map[string]string{"input": "12", "output": "5"}
				if sample.op == Image {
					c.ParameterConstraints = map[string]ParameterConstraint{"n": {Min: "1", Max: "1"}, "size": {Values: []string{"1024x1024"}}, "quality": {Values: []string{"high"}}, "output_format": {Values: []string{"png"}}, "partial_images": {Min: "1", Max: "1"}}
				}
				a := testAttempt(base+sample.path, c)
				a.UpstreamModel = "test-" + string(sample.op) + "-" + mode
				events := 0
				completed := false
				binary := false
				out, e := b.Execute(context.Background(), a, r, func(_ context.Context, event Event) error {
					events++
					if strings.Contains(string(event.Data), "extra_fields") || strings.Contains(string(event.Data), "raw_response") {
						t.Error("private SDK metadata escaped media stream")
					}
					var v struct {
						Type    string `json:"type"`
						Audio   string `json:"audio"`
						B64JSON string `json:"b64_json"`
					}
					if json.Unmarshal(event.Data, &v) != nil {
						return ErrInvalidRequest
					}
					if strings.HasSuffix(v.Type, ".done") || strings.HasSuffix(v.Type, ".completed") {
						completed = true
					}
					encoded := v.Audio
					if encoded == "" {
						encoded = v.B64JSON
					}
					if encoded != "" {
						if bytes, e := base64.StdEncoding.DecodeString(encoded); e != nil || len(bytes) == 0 {
							t.Error("invalid streamed media bytes")
						} else {
							binary = true
						}
					}
					return nil
				})
				if mode == "ok" {
					if e != nil || !out.Usage.Reliable || !completed || events < 2 {
						t.Fatalf("real SDK media stream incomplete: %+v events=%d complete=%v err=%v", out, events, completed, e)
					}
					if sample.op != Transcription && !binary {
						t.Fatal("media bytes missing")
					}
					if e = CheckUsageBounds(c, out.Usage); e != nil {
						t.Fatal(e)
					}
				} else if e == nil || out.Usage.Reliable {
					t.Fatalf("truncated or unmetered stream pretended to complete: %+v %v", out, e)
				}
				if !out.OutputDelivered || events == 0 {
					t.Fatal("partial output not recorded as delivered")
				}
			})
		}
	}
}
