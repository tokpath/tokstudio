package usage

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/platform/objectstore"
)

func TestExternalPrivateResultsExpiryDeletesBlobWithoutDeletingBilling(t *testing.T) {
	if os.Getenv("TOKENHUB_TEST_S3_ENDPOINT") == "" {
		t.Skip("real private S3/MinIO required")
	}
	f := setup(t, "100", nil)
	req, _, e := f.s.Admit(f.ctx, f.key.Key, "temporary-result", request(t, false))
	if e != nil {
		t.Fatal(e)
	}
	req, e = f.s.Settle(f.ctx, nil, req.ID, measured("12", "5"))
	if e != nil {
		t.Fatal(e)
	}
	result := modelruntime.Result{State: "completed", Output: json.RawMessage(`{"message":"private-model-result"}`), ContentType: "application/json"}
	if e = f.s.StoreResult(f.ctx, req, result); e != nil {
		t.Fatal(e)
	}
	var before resultRow
	if e = f.db.First(&before, "request_id=?", req.ID).Error; e != nil {
		t.Fatal(e)
	}
	if before.State != "ready" || before.ObjectKey == "" || before.Digest == "" {
		t.Fatal(before)
	}
	blob, e := f.s.ResultStore.Get(f.ctx, before.ObjectKey)
	if e != nil {
		t.Fatal(e)
	}
	if json.Valid(blob) {
		t.Fatal("model payload was not encrypted before S3 upload")
	}
	if _, e = f.s.ReadResult(f.ctx, uuid.NewString(), req.ID); e == nil {
		t.Fatal("different customer could read private result")
	}
	got, e := f.s.ReadResult(f.ctx, f.customer, req.ID)
	if e != nil || string(got.Output) != string(result.Output) {
		t.Fatal(got, e)
	}
	if e = f.s.StoreResult(f.ctx, req, result); e != nil {
		t.Fatal("retry", e)
	}
	var after resultRow
	if e = f.db.First(&after, "request_id=?", req.ID).Error; e != nil || !after.ExpiresAt.Equal(before.ExpiresAt) {
		t.Fatal("retry extended TTL", after, e)
	}
	changed := result
	changed.Output = json.RawMessage(`{"message":"different output"}`)
	if e = f.s.StoreResult(f.ctx, req, changed); !errors.Is(e, ErrConflict) {
		t.Fatal("result overwritten", e)
	}
	f.s.now = func() time.Time { return before.ExpiresAt.Add(time.Second) }
	if _, e = f.s.ReadResult(f.ctx, f.customer, req.ID); !errors.Is(e, ErrResultUnavailable) {
		t.Fatal(e)
	}
	// This is the interface consumed by the operations retention worker.
	count, e := f.s.PurgeExpiredResults(f.ctx, f.s.wallNow(), 100)
	if e != nil || count != 1 {
		t.Fatal(count, e)
	}
	if _, e = f.s.ResultStore.Get(f.ctx, before.ObjectKey); e == nil {
		t.Fatal("expired object remained readable")
	}
	preserved, e := f.s.GetRequest(f.ctx, f.customer, req.ID)
	if e != nil || preserved.ChargedATG.Cmp(d("0.017")) != 0 || preserved.Status != "completed" {
		t.Fatal("billing altered by retention", preserved, e)
	}
	if e = f.s.ResultStore.Put(f.ctx, "branding/../temporary-results/escape", []byte("x"), "text/plain", time.Time{}); !errors.Is(e, objectstore.ErrKey) {
		t.Fatal("prefix escaped", e)
	}
}

func TestExternalTemporaryLifecycleConfigurationPreservesPrivateAccess(t *testing.T) {
	if os.Getenv("TOKENHUB_TEST_S3_ENDPOINT") == "" {
		t.Skip("real S3/MinIO required")
	}
	f := setup(t, "1", nil)
	store, ok := f.s.ResultStore.(*objectstore.Store)
	if !ok {
		t.Fatal("real object store required")
	}
	if e := store.ConfigureTemporaryLifecycle(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e := store.ConfigureTemporaryLifecycle(f.ctx); e != nil {
		t.Fatal("idempotent lifecycle", e)
	}
}
