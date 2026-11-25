package usage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

type imageTestStore map[string][]byte

func (s imageTestStore) Put(_ context.Context, key string, data []byte, _ string, _ time.Time) error {
	s[key] = append([]byte(nil), data...)
	return nil
}

func (s imageTestStore) Get(_ context.Context, key string) ([]byte, error) {
	if body, ok := s[key]; ok {
		return append([]byte(nil), body...), nil
	}
	return nil, objectstore.ErrUnavailable
}

func (s imageTestStore) Delete(_ context.Context, key string) error {
	delete(s, key)
	return nil
}

func imageForTest(t *testing.T, stream bool) modelruntime.Request {
	t.Helper()
	data := `{"model":"public-model","prompt":"test image prompt","n":2}`
	if stream {
		data = `{"model":"public-model","prompt":"test image prompt","n":2,"stream":true}`
	}
	r, err := modelruntime.DecodeProtocol("incoming", modelruntime.Image, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func imageResultForTest() modelruntime.Result {
	return modelruntime.Result{
		State: "completed", Output: json.RawMessage(`{"data":[{"b64_json":"aW1hZ2U="},{"b64_json":"c2Vjb25k"}]}`),
		Usage: modelruntime.Usage{Source: "normalized_quantities", Reliable: true, Measurements: []modelruntime.Measurement{
			{Component: "output_image_images", Unit: "image", Quantity: "2"},
		}},
	}
}

func TestAsyncImageWorkerSettlesWithoutCustomerPolling(t *testing.T) {
	f := setup(t, "10", nil, modelruntime.DefaultBilling(modelruntime.Image).Capability(modelruntime.Image, false))
	store := imageTestStore{}
	f.s.ResultStore = store
	calls := 0
	f.s.Runtime = subscriptionTestRuntime(func(_ context.Context, attempt modelruntime.Attempt, input modelruntime.Request, _ modelruntime.Emit) (modelruntime.Result, error) {
		calls++
		if !input.Async || input.ID != attempt.RequestID || !strings.Contains(string(input.Input), "test image prompt") {
			t.Fatal("worker did not use the persisted request")
		}
		return imageResultForTest(), nil
	})
	input := imageForTest(t, false)
	accepted, err := f.s.SubmitImage(f.ctx, f.key.Key, "stable-image-key", input)
	if err != nil || accepted.Status != "accepted" || accepted.DeliveredAt == nil || accepted.AsyncInput == "" ||
		strings.Contains(accepted.AsyncInput, "test image prompt") || calls != 0 || !accepted.ChargedATG.IsZero() {
		t.Fatalf("image was not durably accepted before dispatch: %s %v", accepted.Status, err)
	}
	again, err := f.s.SubmitImage(f.ctx, f.key.Key, "stable-image-key", input)
	if err != nil || again.ID != accepted.ID {
		t.Fatal("idempotent submission changed the task", err)
	}
	if _, err := f.s.Execute(f.ctx, f.key.Key, "stable-image-key", input, nil); !errors.Is(err, ErrConflict) {
		t.Fatal("sync call reused an async task", err)
	}
	pending, err := f.s.PendingRequestsPage(f.ctx, "", 100)
	if err != nil || len(pending) != 1 || pending[0].ID != accepted.ID {
		t.Fatal("worker could not discover the accepted task", err)
	}
	work, err := f.s.GetRequest(f.ctx, pending[0].CustomerID, pending[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.RunImageTask(f.ctx, work); err != nil {
		t.Fatal(err)
	}
	if err := f.s.RunImageTask(f.ctx, work); err != nil {
		t.Fatal(err)
	}
	completed, err := f.s.GetRequest(f.ctx, f.customer, accepted.ID)
	if err != nil || completed.Status != "completed" || completed.ChargedATG.String() != "0.5" ||
		completed.DeliveredAt == nil || !completed.DeliveredAt.Equal(*accepted.DeliveredAt) || calls != 1 {
		t.Fatalf("worker failed to settle the delivered task exactly once: %s %s %v", completed.Status, completed.ChargedATG, err)
	}
	var retainedInput string
	if err := f.db.Model(&Request{}).Select("async_input").Where("id=?", accepted.ID).Row().Scan(&retainedInput); err != nil || retainedInput != "" {
		t.Fatal("worker left the encrypted request input after dispatch", err)
	}
	other := f.key.Key
	other.ID = "different-key"
	if _, err = f.s.RequestForKey(f.ctx, other, accepted.ID); !errors.Is(err, ErrKey) {
		t.Fatal("another key could read a private task", err)
	}
	result, err := f.s.ReadResult(f.ctx, f.customer, accepted.ID)
	if err != nil || string(result.Output) != string(imageResultForTest().Output) {
		t.Fatal("original task did not retain its result", err)
	}
	for _, ciphertext := range store {
		if strings.Contains(string(ciphertext), "aW1hZ2U=") {
			t.Fatal("private object stored plaintext media")
		}
	}
	f.s.now = func() time.Time { return time.Now().Add(f.s.ResultRetention + time.Hour) }
	if _, err = f.s.ReadResult(f.ctx, f.customer, accepted.ID); !errors.Is(err, ErrResultUnavailable) {
		t.Fatal("expired image was still retrievable", err)
	}
}

func TestImagePreviewDisconnectKeepsReceiptAndVerifiedSettlement(t *testing.T) {
	f := setup(t, "10", nil, modelruntime.DefaultBilling(modelruntime.Image).Capability(modelruntime.Image, false))
	f.s.ResultStore = imageTestStore{}
	client, disconnect := context.WithCancel(f.ctx)
	defer disconnect()
	var firstDelivery time.Time
	f.s.Runtime = subscriptionTestRuntime(func(upstream context.Context, attempt modelruntime.Attempt, _ modelruntime.Request, emit modelruntime.Emit) (modelruntime.Result, error) {
		for i := 0; i < 3; i++ {
			if err := emit(upstream, modelruntime.Event{Data: json.RawMessage(`{"type":"image_generation.partial_image"}`)}); err != nil {
				return modelruntime.Result{}, err
			}
			if i == 0 {
				row, err := f.s.GetRequest(f.ctx, f.customer, attempt.RequestID)
				if err != nil || row.DeliveredAt == nil || row.Status != "executing" || !row.ChargedATG.IsZero() {
					t.Fatal("first preview did not mark delivery during generation", err)
				}
				firstDelivery = *row.DeliveredAt
			}
		}
		if upstream.Err() != nil {
			t.Fatal("client disconnect canceled the custodian of the result")
		}
		return imageResultForTest(), nil
	})
	out, err := f.s.Execute(client, f.key.Key, "preview-disconnect", imageForTest(t, true), func(_ context.Context, _ modelruntime.Event) error {
		if client.Err() != nil {
			return client.Err()
		}
		disconnect()
		return nil
	})
	if err != nil || out.Request.Status != "completed" || out.Request.ChargedATG.String() != "0.5" ||
		out.Request.DeliveredAt == nil || !out.Request.DeliveredAt.Equal(firstDelivery) {
		t.Fatalf("stream lost its original delivery or settlement: %s %v", out.Request.Status, err)
	}
	if _, err = f.s.ReadResult(f.ctx, f.customer, out.Request.ID); err != nil {
		t.Fatal("missing final result after disconnect", err)
	}
}
