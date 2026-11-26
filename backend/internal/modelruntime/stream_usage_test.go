package modelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestStreamZeroRequiresNativeProviderEvidence(t *testing.T) {
	c := testCapability(Chat, true)
	r := &schemas.BifrostStreamChunk{BifrostChatResponse: &schemas.BifrostChatResponse{Usage: &schemas.BifrostLLMUsage{}}}
	if extractUsage(r, c).Reliable {
		t.Fatal("SDK synthetic empty usage was accepted")
	}
	r.BifrostChatResponse.ExtraFields.RawResponse = `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`
	if u := extractUsage(r, c); !u.Reliable || len(u.Measurements) != 2 {
		t.Fatal("native zero must remain valid", u)
	}
	r.BifrostChatResponse.ExtraFields.RawResponse = `{"usage":{"prompt_tokens":0,"completion_tokens":0}}`
	if extractUsage(r, c).Reliable {
		t.Fatal("incomplete usage was accepted")
	}
}

func TestBifrostWebSocketResponsesRetainsTerminalOutputAndVerifiesUsage(t *testing.T) {
	const accrued = `{"input_tokens":8,"output_tokens":3,"total_tokens":11}`
	const final = `{"input_tokens":8,"output_tokens":5,"total_tokens":13}`
	const verifiedZero = `{"input_tokens":8,"output_tokens":0,"total_tokens":8}`
	const textOutput = `[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"retained text","annotations":[]}]}]`
	for _, sample := range []struct {
		name, earlierUsage, terminalUsage, status, output string
		wantTokens                                        int
	}{
		{name: "usage before completion", earlierUsage: accrued, status: "completed", output: textOutput, wantTokens: 3},
		{name: "usage before incomplete response", earlierUsage: accrued, status: "incomplete", output: textOutput, wantTokens: 3},
		{name: "terminal usage supersedes provisional counts", earlierUsage: accrued, terminalUsage: final, status: "completed", output: textOutput, wantTokens: 5},
		{name: "input counts alone are not verified", earlierUsage: `{"input_tokens":8,"total_tokens":8}`, status: "completed", output: textOutput, wantTokens: -1},
		{name: "provisional zero output is not verified", earlierUsage: verifiedZero, status: "completed", output: textOutput, wantTokens: -1},
		{name: "terminal zero output is verified", terminalUsage: verifiedZero, status: "completed", output: `[]`, wantTokens: 0},
	} {
		t.Run(sample.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/v1/responses" {
					t.Errorf("unexpected upstream URL %s", req.URL.Path)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				writeEvent := func(event, status, output, usage string) {
					if usage != "" {
						usage = `,"usage":` + usage
					}
					fmt.Fprintf(w, "data: "+`{"type":%q,"response":{"object":"response","id":"resp_test","created_at":123,"model":"upstream-exact-model","status":%q,"output":%s%s}}`+"\n\n",
						event, status, output, usage)
				}
				if sample.earlierUsage != "" {
					writeEvent("response.in_progress", "in_progress", `[]`, sample.earlierUsage)
				}
				writeEvent("response."+sample.status, sample.status, sample.output, sample.terminalUsage)
			}))
			t.Cleanup(server.Close)
			b := newTestRuntime(t)
			const body = `{"model":"public-model","input":"keep this turn","stream":true}`
			input, err := DecodeProtocol("request", Responses, []byte(body))
			if err != nil {
				t.Fatal(err)
			}
			input.WebSocketInput = json.RawMessage(body)
			attempt := testAttempt(server.URL+"/v1/responses", DefaultBilling(Responses).Capability(Responses, true))
			result, err := b.Execute(t.Context(), attempt, input, func(_ context.Context, _ Event) error { return nil })
			if result.State != "completed" || !strings.Contains(string(result.Output), `"type":"response.`+sample.status+`"`) ||
				sample.output == textOutput && !strings.Contains(string(result.Output), "retained text") {
				t.Fatalf("terminal state=%s output=%s error=%v", result.State, result.Output, err)
			}
			if sample.wantTokens < 0 {
				if !errors.Is(err, ErrUsageUnknown) || result.Usage.Reliable {
					t.Fatalf("input-only frame made the turn billable: %+v, %v", result.Usage, err)
				}
				return
			}
			if err != nil || !result.Usage.Reliable || len(result.Usage.Measurements) != 3 {
				t.Fatalf("missing verified usage: %+v, %v", result.Usage, err)
			}
			want := map[string]float64{"input": 8, "output": float64(sample.wantTokens), "cache": 0}
			for _, measurement := range result.Usage.Measurements {
				wanted, known := want[measurement.Component]
				quantity, err := strconv.ParseFloat(measurement.Quantity, 64)
				if !known || err != nil || quantity != wanted {
					t.Fatalf("incorrect measurement %+v, expected %v", measurement, want)
				}
			}
		})
	}
}
