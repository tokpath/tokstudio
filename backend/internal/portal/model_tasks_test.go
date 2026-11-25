package portal

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

type modelTaskTestStore struct{ items sync.Map }

func (s *modelTaskTestStore) Put(_ context.Context, key string, data []byte, _ string, _ time.Time) error {
	s.items.Store(key, bytes.Clone(data))
	return nil
}
func (s *modelTaskTestStore) Get(_ context.Context, key string) ([]byte, error) {
	if data, ok := s.items.Load(key); ok {
		return bytes.Clone(data.([]byte)), nil
	}
	return nil, objectstore.ErrUnavailable
}
func (s *modelTaskTestStore) Delete(_ context.Context, key string) error {
	s.items.Delete(key)
	return nil
}

func (f *testApp) modelTaskClient(t *testing.T, email string) (customer, secret string) {
	t.Helper()
	f.openRegistration(t)
	f.registered(t, email)
	installation := f.a.Installation
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET model_usage=true WHERE operator_id=?", installation.OperatorID).Error; err != nil {
		t.Fatal(err)
	}
	f.login(t, "customer", email, "customer-password-test")
	status, view, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, view)
	customer = view["customer"].(map[string]any)["id"].(string)
	if _, err := f.a.Ledger.IssueGrant(t.Context(), nil, ledger.GrantInput{
		OperatorID: installation.OperatorID, CustomerID: customer, OrderID: uuid.NewString(), BusinessKey: uuid.NewString(),
		ActorID: installation.OwnerStaffID, Reason: "Synthetic inference test", EffectiveAt: time.Now().Add(-time.Minute),
		Sources: []ledger.GrantSource{{Kind: "manual_gift", Amount: ledger.FromInt64(100)}},
	}); err != nil {
		t.Fatal(err)
	}
	status, key, _ := f.call(t, "POST", "/v1/me/keys", map[string]any{"name": "Fixture key", "budget": map[string]string{"period": "lifetime"}}, "")
	mustOK(t, status, key)
	if f.a.Usage.ResultStore == nil {
		f.a.Usage.ResultStore = &modelTaskTestStore{}
	}
	return customer, key["secret"].(string)
}

func TestPostgresResponsesWebSocketSettlesAndReplaysTurns(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body map[string]json.RawMessage
		if r.URL.Path != "/v1/responses" || json.NewDecoder(r.Body).Decode(&body) != nil ||
			string(body["stream"]) != "true" || string(body["store"]) != "false" || body["previous_response_id"] != nil {
			t.Error("wrong upstream Responses request")
		}
		input := string(body["input"])
		if !strings.Contains(input, "first prompt") || call == 2 && (!strings.Contains(input, "upstream reply") || !strings.Contains(input, "next prompt")) {
			t.Error("continuation lost the prior input or output", input)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: "+`{"type":"response.created","response":{"object":"response","id":"response-%d","created_at":123,"model":"test-chat","status":"in_progress","output":[]}}`+"\n\n", call)
		fmt.Fprint(w, "data: "+`{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"upstream reply"}`+"\n\n")
		fmt.Fprintf(w, "data: "+`{"type":"response.completed","response":{"object":"response","id":"response-%d","created_at":123,"model":"test-chat","status":"completed","output":[{"id":"msg_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"upstream reply","annotations":[]}]}],"usage":{"input_tokens":8,"output_tokens":3,"total_tokens":11,"input_tokens_details":{"cached_tokens":2}}}}`+"\n\n", call)
	}))
	t.Cleanup(upstream.Close)

	f := setupPortal(t)
	model := f.configureExternalCatalog(t, upstream.URL, modelruntime.Responses)
	customer, secret := f.modelTaskClient(t, "socket@model-task.test")
	dial := func() *websocket.Conn {
		t.Helper()
		socket, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/v1/responses",
			http.Header{"Authorization": {"Bearer " + secret}})
		if err != nil {
			t.Fatal("could not upgrade Responses connection", response, err)
		}
		t.Cleanup(func() { _ = socket.Close() })
		return socket
	}
	type event struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Response  struct {
			ID    string `json:"id"`
			Model string `json:"model"`
		} `json:"response"`
		Error struct{ Code string } `json:"error"`
	}
	run := func(socket *websocket.Conn, fields map[string]string) event {
		t.Helper()
		if err := socket.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := socket.WriteJSON(fields); err != nil {
			t.Fatal(err)
		}
		for range 16 {
			var got event
			if err := socket.ReadJSON(&got); err != nil {
				t.Fatal(err)
			}
			if got.Type == "response.completed" || got.Type == "error" {
				return got
			}
		}
		t.Fatal("missing terminal WebSocket event")
		return event{}
	}

	socket := dial()
	first := run(socket, map[string]string{"type": "response.create", "model": model, "input": "first prompt", "event_id": "turn-1"})
	secondInput := map[string]string{"type": "response.create", "model": model, "input": "next prompt",
		"previous_response_id": first.Response.ID, "event_id": "turn-2"}
	second := run(socket, secondInput)
	if first.Type != "response.completed" || second.Type != "response.completed" || first.RequestID == second.RequestID ||
		first.Response.Model != model || second.Response.Model != model {
		t.Fatalf("did not receive two public Responses events %+v %+v", first, second)
	}
	_ = socket.Close()
	reconnected := dial()
	replayed := run(reconnected, secondInput)
	if replayed != second || calls.Load() != 2 {
		t.Fatalf("reconnecting resubmitted or changed the previous request %+v, calls=%d", replayed, calls.Load())
	}
	secondInput["input"] = "different content"
	if conflict := run(reconnected, secondInput); conflict.Error.Code != "idempotency_conflict" || calls.Load() != 2 {
		t.Fatalf("the original key accepted a different turn %+v", conflict)
	}
	for _, id := range []string{first.RequestID, second.RequestID} {
		bill, err := f.a.Usage.GetRequest(t.Context(), customer, id)
		if err != nil || bill.Status != "completed" || bill.ChargedATG.String() != "0.011" {
			t.Fatalf("did not settle verified token usage %+v %v", bill, err)
		}
	}
}

func TestPostgresImageBatchAPIProvidesPrivateItemsAndArchive(t *testing.T) {
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Error("image batch used the wrong inference path")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"created":123,"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(pngBytes.Bytes()))
	}))
	t.Cleanup(upstream.Close)
	f := setupPortal(t)
	model := f.configureExternalCatalog(t, upstream.URL, modelruntime.Image)
	customer, secret := f.modelTaskClient(t, "batch@model-task.test")
	call := func(method, path string, input any) (int, http.Header, []byte) {
		t.Helper()
		body, _ := json.Marshal(input)
		req, err := http.NewRequestWithContext(t.Context(), method, f.server.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+secret)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "fixture-image-batch")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, res.Header, b
	}
	batchBody := map[string]any{"model": model, "items": []map[string]string{{"custom_id": "first", "prompt": "draw a square"}}}
	status, headers, raw := call("POST", "/v1/images/batches", batchBody)
	path := headers.Get("Location")
	if status != http.StatusAccepted || !strings.HasPrefix(path, "/v1/images/batches/") || !bytes.Contains(raw, []byte(`"delivered_at"`)) {
		t.Fatalf("batch failed to accept %d %s", status, raw)
	}
	_, _, raw = call("GET", path+"/items", nil)
	var items struct {
		Data []struct {
			RequestID string `json:"request_id"`
		}
	}
	if json.Unmarshal(raw, &items) != nil || len(items.Data) != 1 || items.Data[0].RequestID == "" {
		t.Fatalf("batch lacks an original usage request %s", raw)
	}
	request, err := f.a.Usage.GetRequest(t.Context(), customer, items.Data[0].RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.Usage.RunImageTask(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	status, _, raw = call("GET", path, nil)
	if status != http.StatusOK || !bytes.Contains(raw, []byte(`"charged_atg":"0.25"`)) {
		t.Fatalf("batch did not show actual settlement %d %s", status, raw)
	}
	status, headers, raw = call("GET", path+"/items/first/content", nil)
	if status != http.StatusOK || headers.Get("Content-Type") != "image/png" || !bytes.Equal(raw, pngBytes.Bytes()) {
		t.Fatal("batch did not return the original image")
	}
	status, _, raw = call("GET", path+"/download", nil)
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if status != http.StatusOK || err != nil || len(archive.File) != 2 ||
		archive.File[0].Name != "01-first/1.png" || archive.File[1].Name != "manifest.json" {
		t.Fatalf("wrong image batch archive %d %v", status, err)
	}
}
