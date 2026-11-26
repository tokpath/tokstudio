package usage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

func TestPostgresWebSocketSettlesOnlyReplayableResponseTurns(t *testing.T) {
	capability := modelruntime.DefaultBilling(modelruntime.Responses).Capability(modelruntime.Responses, true)
	f := setup(t, "100", nil, capability)
	store := imageTestStore{}
	f.s.ResultStore = store
	if url := os.Getenv("TOKENHUB_TEST_REDIS_URL"); url != "" {
		options, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		cache := redis.NewClient(options)
		t.Cleanup(func() { _ = cache.Close() })
		f.s.ResponseReplayCache = cache
	}
	body := []byte(`{"model":"public-model","input":"test input","stream":true}`)
	request, err := modelruntime.DecodeProtocol("", modelruntime.Responses, body)
	if err != nil {
		t.Fatal(err)
	}
	request.WebSocketInput = body
	for _, sample := range []struct {
		name       string
		output     json.RawMessage
		replayable bool
	}{
		{name: "missing_terminal_event"},
		{name: "invalid_terminal_event", output: json.RawMessage(`{"type":"response.completed"}`)},
		{name: "incomplete_response", output: json.RawMessage(`{"type":"response.incomplete","response":{"id":"resp_original","status":"incomplete"}}`), replayable: true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			f.s.Runtime = subscriptionTestRuntime(func(ctx context.Context, _ modelruntime.Attempt, _ modelruntime.Request, emit modelruntime.Emit) (modelruntime.Result, error) {
				err := emit(ctx, modelruntime.Event{Data: json.RawMessage(`{"type":"response.output_text.delta","delta":"partial output"}`)})
				return modelruntime.Result{State: "outcome_unknown", Output: sample.output, Usage: measured("12", "5")}, err
			})
			key := "responses-ws:" + sample.name
			before := len(store)
			execution, err := f.s.Execute(f.ctx, f.key.Key, key, request, func(context.Context, modelruntime.Event) error { return nil })
			if sample.replayable {
				if err != nil || execution.Request.Status != "completed" || execution.Request.ChargedATG.String() != "0.017" {
					t.Fatalf("replayable partial response did not settle: %+v %v", execution.Request, err)
				}
				replay, found, err := f.s.ReplayResponseTurn(f.ctx, f.key.Key, key, request)
				if err != nil || !found || string(replay.Result.Output) != string(sample.output) {
					t.Fatalf("incomplete response could not be replayed: %v %t", err, found)
				}
				return
			}
			if !errors.Is(err, objectstore.ErrUnavailable) || execution.Request.Status != "free" || !execution.Request.ChargedATG.IsZero() || len(store) != before {
				t.Fatalf("a turn without a replayable result was retained or billed: %+v %v", execution.Request, err)
			}
			if cache := f.s.ResponseReplayCache; cache != nil {
				if count, err := cache.Exists(f.ctx, responseReplayKey(execution.Request.ID)).Result(); err != nil || count != 0 {
					t.Fatalf("invalid result was written to Redis: %d %v", count, err)
				}
			}
		})
	}
}
