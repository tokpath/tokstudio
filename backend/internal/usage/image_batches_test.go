package usage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"gorm.io/gorm"
)

func TestPostgresImageBatchSettlesOriginalItemsAndReplaysOnce(t *testing.T) {
	f := setup(t, "10", nil, modelruntime.DefaultBilling(modelruntime.Image).Capability(modelruntime.Image, false))
	f.s.ResultStore = imageTestStore{}
	initial := imageForTest(t, false)
	initial.ID = ""
	bad, err := modelruntime.DecodeProtocol("", modelruntime.Image, []byte(`{"model":"public-model","prompt":"fixture rejected","n":2}`))
	if err != nil {
		t.Fatal(err)
	}
	input := ImageBatchInput{ModelID: "public-model", Items: []ImageBatchEntry{
		{CustomID: "first", Request: initial}, {CustomID: "unavailable", Request: bad}, {CustomID: "cancel", Request: initial},
	}}
	calls := 0
	f.s.Runtime = subscriptionTestRuntime(func(_ context.Context, attempt modelruntime.Attempt, r modelruntime.Request, _ modelruntime.Emit) (modelruntime.Result, error) {
		calls++
		if !r.Async || attempt.RequestID != r.ID {
			t.Fatal("batch bypassed the original async task")
		}
		if strings.Contains(string(r.Input), "fixture rejected") {
			return modelruntime.Result{State: "not_dispatched"}, nil
		}
		return imageResultForTest(), nil
	})
	accepted, err := f.s.SubmitImageBatch(f.ctx, f.key.Key, "image-batch-test", input)
	if err != nil || len(accepted.Items) != 3 || accepted.Items[0].EstimateATG.String() != "0.5" {
		t.Fatalf("bad batch admission %+v %v", accepted, err)
	}
	for _, item := range accepted.Items[:2] {
		row, err := f.s.GetRequest(f.ctx, f.customer, item.RequestID)
		if err != nil || row.DeliveredAt == nil || !row.AcceptedAt.Equal(accepted.CreatedAt) {
			t.Fatalf("item did not share the batch acceptance and price time %+v %v", row, err)
		}
		if err = f.s.RunImageTask(f.ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.s.CancelImageBatch(f.ctx, f.key.Key, accepted.ID); err != nil {
		t.Fatal(err)
	}
	row, err := f.s.GetRequest(f.ctx, f.customer, accepted.Items[2].RequestID)
	if err != nil || !row.CancelRequested {
		t.Fatalf("batch cancellation missed a queued item %+v %v", row, err)
	}
	if err := f.s.RunImageTask(f.ctx, row); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.SubmitImageBatch(f.ctx, f.key.Key, "image-batch-test", input)
	if err != nil || replay.ID != accepted.ID || len(replay.Items) != 3 || calls != 2 {
		t.Fatalf("batch replay created new work %+v %v calls=%d", replay, err, calls)
	}
	if replay.Items[0].Status != "completed" || replay.Items[0].ChargedATG.String() != "0.5" ||
		!replay.Items[1].ChargedATG.IsZero() || !replay.Items[2].ChargedATG.IsZero() || replay.Items[2].Status != "cancelled" {
		t.Fatalf("wrong batch settlement %+v", replay.Items)
	}
	budget, err := f.s.KeyBudget(f.ctx, nil, f.key.Key, f.s.wallNow())
	if err != nil || budget.SpentATG.String() != "0.5" || !budget.HeldATG.IsZero() {
		t.Fatalf("batch left an incorrect hold %+v %v", budget, err)
	}
	input.TaskName = "something different"
	if _, err := f.s.SubmitImageBatch(f.ctx, f.key.Key, "image-batch-test", input); !errors.Is(err, ErrConflict) {
		t.Fatal("batch idempotency did not bind the original content", err)
	}
	other, err := f.s.CreateKey(f.ctx, f.customer, "other-image-key", KeyInput{Name: "Other key", Period: ledger.Lifetime, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReadImageBatch(f.ctx, other.Key, accepted.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal("batch was visible with another key", err)
	}
}

func TestPostgresImageBatchRollsBackOnAnUnaffordableItem(t *testing.T) {
	f := setup(t, "0.75", nil, modelruntime.DefaultBilling(modelruntime.Image).Capability(modelruntime.Image, false))
	f.s.ResultStore = imageTestStore{}
	f.s.Runtime = subscriptionTestRuntime(func(context.Context, modelruntime.Attempt, modelruntime.Request, modelruntime.Emit) (modelruntime.Result, error) {
		t.Fatal("failed admission dispatched an image")
		return modelruntime.Result{}, nil
	})
	image := imageForTest(t, false)
	image.ID = ""
	input := ImageBatchInput{ModelID: "public-model", Items: []ImageBatchEntry{
		{CustomID: "one", Request: image}, {CustomID: "two", Request: image},
	}}
	if _, err := f.s.SubmitImageBatch(f.ctx, f.key.Key, "image-batch-unaffordable", input); !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatal("batch should have rejected the whole reservation", err)
	}
	var count int64
	if err := f.db.Model(&Request{}).Where("key_id=?", f.key.Key.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("partial image requests were admitted: %d, %v", count, err)
	}
	batches, _, err := f.s.ListImageBatches(f.ctx, f.key.Key, "", 20)
	if err != nil || len(batches) != 0 {
		t.Fatalf("a batch was left visible after rollback: %+v, %v", batches, err)
	}
	budget, err := f.s.KeyBudget(f.ctx, nil, f.key.Key, f.s.wallNow())
	if err != nil || !budget.HeldATG.IsZero() || !budget.SpentATG.IsZero() {
		t.Fatalf("batch rollback left a charge or hold %+v %v", budget, err)
	}
}
