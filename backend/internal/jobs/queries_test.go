package jobs

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestDeadRedrivePreservesBusinessAndFencesStaleWorker(t *testing.T) {
	db, s := setup(t)
	ctx := context.Background()
	original, e := s.Enqueue(ctx, Input{Queue: "redrive", Kind: "refund-query", BusinessKey: "stable-external-fact", Payload: json.RawMessage(`{"provider_id":"original"}`), MaxAttempts: 1})
	if e != nil {
		t.Fatal(e)
	}
	rows, e := s.Claim(ctx, "redrive", "old", time.Minute, 1)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	old := rows[0]
	if e = s.Fail(ctx, old.ID, "old", old.Fence, "verified dependency outage"); e != nil {
		t.Fatal(e)
	}
	if e = s.Redrive(ctx, old.ID, 2); e != nil {
		t.Fatal(e)
	}
	if e = s.Complete(ctx, old.ID, "old", old.Fence); e == nil {
		t.Fatal("stale worker completed redriven job")
	}
	rows, e = s.Claim(ctx, "redrive", "new", time.Minute, 1)
	if e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	current := rows[0]
	if current.ID != original.ID || current.BusinessKey != original.BusinessKey || string(current.Payload) != string(original.Payload) || current.Attempts != 2 || current.MaxAttempts != 3 || current.Fence <= old.Fence {
		t.Fatal("redrive changed immutable identity or reset history", current)
	}
	if e = s.Complete(ctx, current.ID, "new", current.Fence); e != nil {
		t.Fatal(e)
	}
	if e = s.Redrive(ctx, current.ID, 1); e == nil {
		t.Fatal("completed command was restarted")
	}
	views, e := s.List(ctx, "", "redrive", "", 1)
	if e != nil || len(views) != 1 {
		t.Fatal(views, e)
	}
	if _, e = s.List(ctx, views[0].ID, "redrive", "", 1); e != nil {
		t.Fatal("public queue cursor rejected real job ID", e)
	}
	var count int64
	db.Model(&Job{}).Where("queue='redrive'").Count(&count)
	if count != 1 {
		t.Fatal("new job created", count)
	}
}
