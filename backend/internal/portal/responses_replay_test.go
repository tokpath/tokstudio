package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
	"github.com/tokpath/tokstudio/backend/internal/usage"
)

type unavailableResponsesStore struct {
	modelTaskTestStore
	puts atomic.Int64
}

func (s *unavailableResponsesStore) Put(context.Context, string, []byte, string, time.Time) error {
	s.puts.Add(1)
	return objectstore.ErrUnavailable
}

func TestPostgresResponsesWebSocketRecoversFromResultStoreFailure(t *testing.T) {
	f := setupPortal(t)
	if f.a.Redis == nil {
		t.Skip("TOKENHUB_TEST_REDIS_URL required for cross-instance WebSocket replay")
	}
	var calls atomic.Int64
	upstreamStarted := make(chan struct{})
	completed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			close(upstreamStarted)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: "+`{"type":"response.created","response":{"object":"response","id":"response-%d","created_at":123,"model":"test-chat","status":"in_progress","output":[]}}`+"\n\n", call)
		w.(http.Flusher).Flush()
		select {
		case <-completed:
		case <-r.Context().Done():
			return
		case <-t.Context().Done():
			return
		}
		fmt.Fprintf(w, "data: "+`{"type":"response.in_progress","response":{"object":"response","id":"response-%d","created_at":123,"model":"test-chat","status":"in_progress","output":[],"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"input_tokens_details":{"cached_tokens":2}}}}`+"\n\n", call)
		fmt.Fprintf(w, "data: "+`{"type":"response.completed","response":{"object":"response","id":"response-%d","created_at":123,"model":"test-chat","status":"completed","output":[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"original retained output","annotations":[]}]}]}}`+"\n\n", call)
	}))
	t.Cleanup(upstream.Close)
	model := f.configureExternalCatalog(t, upstream.URL, modelruntime.Responses)
	customer, secret := f.modelTaskClient(t, "replay@model-task.test")
	store := &unavailableResponsesStore{}
	f.a.Usage.ResultStore = store
	secondInstance := func(cached bool) *httptest.Server {
		t.Helper()
		a, err := newApplication(t.Context(), f.a.Config, f.a.DB)
		if err != nil {
			t.Fatal(err)
		}
		a.Usage.ResultStore = store
		if !cached {
			a.Usage.ResponseReplayCache = nil
		}
		server := httptest.NewServer(a.Router())
		t.Cleanup(func() { server.Close(); a.Close() })
		return server
	}
	replayHost := secondInstance(true)
	noFallbackHost := secondInstance(false)
	dial := func(server *httptest.Server) *websocket.Conn {
		t.Helper()
		socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses",
			http.Header{"Authorization": {"Bearer " + secret}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = socket.Close() })
		if err := socket.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
			t.Fatal(err)
		}
		return socket
	}
	type event struct {
		Type      string                `json:"type"`
		RequestID string                `json:"request_id"`
		Response  json.RawMessage       `json:"response"`
		Error     struct{ Code string } `json:"error"`
	}
	terminal := func(socket *websocket.Conn, input map[string]string) event {
		t.Helper()
		if err := socket.WriteJSON(input); err != nil {
			t.Fatal(err)
		}
		for range 8 {
			var result event
			if err := socket.ReadJSON(&result); err != nil {
				t.Fatal(err)
			}
			if result.Type == "response.completed" || result.Type == "error" {
				return result
			}
		}
		t.Fatal("no terminal response event")
		return event{}
	}
	input := map[string]string{"type": "response.create", "model": model, "input": "keep this turn", "event_id": "retained-turn"}
	original := dial(f.server)
	if err := original.WriteJSON(input); err != nil {
		t.Fatal(err)
	}
	select {
	case <-upstreamStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("turn did not reach the upstream")
	}
	var started usage.Request
	if err := f.a.DB.Where("customer_id=? AND business_key=?", customer, "responses-ws:retained-turn").Take(&started).Error; err != nil {
		t.Fatal(err)
	}
	_ = original.Close()
	close(completed)
	deadline := time.Now().Add(5 * time.Second)
	for {
		record, err := f.a.Usage.GetRequest(t.Context(), customer, started.ID)
		if err != nil {
			t.Fatal(err)
		}
		if record.Status == "completed" {
			if record.ActualATG.String() != "0.011" || record.ChargedATG.String() != "0.011" {
				t.Fatal("expected one verified usage debit")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("ended with %s after S3 failed", record.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cacheKey := "tokenhub:usage:responses-replay:" + started.ID
	sealed, err := f.a.Redis.Get(t.Context(), cacheKey).Result()
	if err != nil || strings.Contains(sealed, "original retained output") {
		t.Fatalf("missing encrypted fallback: %v", err)
	}
	if ttl, err := f.a.Redis.PTTL(t.Context(), cacheKey).Result(); err != nil || ttl <= 0 || ttl > 5*time.Minute {
		t.Fatalf("incorrect fallback expiry %v, %v", ttl, err)
	}
	detail := "/v1/me/requests/" + started.ID
	status, receipt, _ := f.call(t, "GET", detail, nil, "")
	mustOK(t, status, receipt)
	if receipt["result_url"] != "/api"+detail+"/result" || receipt["result_expires_at"] == nil {
		t.Fatalf("request detail did not offer the recovered result: %v", receipt["result_url"])
	}
	status, stored, _ := f.call(t, "GET", detail+"/result", nil, "")
	mustOK(t, status, stored)
	if output, _ := json.Marshal(stored["response"]); !strings.Contains(string(output), "original retained output") {
		t.Fatal("authenticated result endpoint did not recover the original event")
	}
	reader := dial(replayHost)
	for range 2 {
		recovered := terminal(reader, input)
		if recovered.Type != "response.completed" || recovered.RequestID != started.ID || !strings.Contains(string(recovered.Response), "original retained output") {
			t.Fatalf("different instance could not replay the exact original turn: %+v", recovered)
		}
	}
	input["input"] = "different content"
	if conflict := terminal(reader, input); conflict.Error.Code != "idempotency_conflict" {
		t.Fatalf("another payload could read the original turn: %+v", conflict)
	}
	input["input"] = "keep this turn"
	if calls.Load() != 1 || store.puts.Load() != 1 {
		t.Fatal("replaying the turn repeated its upstream call or upload")
	}
	if err := f.a.DB.Model(&usage.Request{}).Where("id=?", started.ID).
		Update("completed_at", time.Now().Add(-5*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if expired := terminal(reader, input); expired.Error.Code != "result_unavailable" || calls.Load() != 1 {
		t.Fatalf("replay extended the original retention: %+v", expired)
	}
	status, receipt, _ = f.call(t, "GET", detail, nil, "")
	mustOK(t, status, receipt)
	if receipt["result_url"] != nil {
		t.Fatal("request detail displayed an expired replay result")
	}
	input["event_id"] = "unstored-turn"
	noFallback := dial(noFallbackHost)
	unstored := terminal(noFallback, input)
	if unstored.Error.Code != "request_unavailable" {
		t.Fatalf("an unavailable replay store was not surfaced: %+v", unstored)
	}
	record, err := f.a.Usage.GetRequest(t.Context(), customer, unstored.RequestID)
	if err != nil || record.Status != "free" || record.Reason != "temporary_result_storage_unavailable" || !record.ChargedATG.IsZero() {
		t.Fatalf("a turn with no recoverable result was billed: %+v, error %v", record, err)
	}
	if failedReplay := terminal(noFallback, input); failedReplay.Error.Code != "result_unavailable" || calls.Load() != 2 {
		t.Fatalf("unavailable replay started a second generation: %+v", failedReplay)
	}
}
