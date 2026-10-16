package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
)

func setup(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	gdb := dbtest.New(t)
	name, files := Migrations()
	if err := db.Apply(gdb, []db.ModuleMigrations{{Module: name, FS: files}}); err != nil {
		t.Fatal(err)
	}
	return gdb, New(gdb)
}

func TestPreparePreservesNumbersAndBounds(t *testing.T) {
	job, err := prepare(Input{Kind: "delivery", BusinessKey: "order", Payload: json.RawMessage(`{"large":123456789012345678901234567890.123456789}`)})
	if err != nil || string(job.Payload) != `{"large":123456789012345678901234567890.123456789}` {
		t.Fatalf("precision lost: %s %v", job.Payload, err)
	}
	for _, input := range []Input{{Kind: "x"}, {Kind: "x", BusinessKey: "y", MaxAttempts: 101}, {Kind: "x", BusinessKey: "y", Payload: json.RawMessage(`{} {}`)}} {
		if _, err := prepare(input); err == nil {
			t.Fatal("invalid job accepted")
		}
	}
	for attempt := 1; attempt <= 100; attempt++ {
		if delay := retryDelay(attempt); delay < time.Second || delay > 15*time.Minute {
			t.Fatalf("unbounded delay: %s", delay)
		}
	}
}

func TestEnqueueIdempotencyAndRollback(t *testing.T) {
	gdb, s := setup(t)
	ctx := context.Background()
	input := Input{Kind: "delivery", BusinessKey: "order-1", Payload: json.RawMessage(`{"b":2,"a":1}`)}
	job, err := s.Enqueue(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	input.Payload = json.RawMessage(` { "a":1, "b":2 } `)
	again, err := s.Enqueue(ctx, input)
	if err != nil || again.ID != job.ID {
		t.Fatalf("duplicate not stable: %+v %v", again, err)
	}
	input.Payload = json.RawMessage(`{"a":3}`)
	if _, err := s.Enqueue(ctx, input); !errors.Is(err, ErrKeyConflict) {
		t.Fatalf("different input accepted: %v", err)
	}
	rollback := errors.New("abort business")
	err = gdb.Transaction(func(tx *gorm.DB) error {
		_, err := s.EnqueueTx(tx, Input{Kind: "delivery", BusinessKey: "rolled-back"})
		if err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int64
	gdb.Model(&Job{}).Count(&count)
	if count != 1 {
		t.Fatalf("transactional enqueue leaked: %d", count)
	}
	claimed, err := s.Claim(ctx, "default", "worker", time.Minute, 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("default schedule not due: %v %v", claimed, err)
	}
}

func TestConcurrentWorkersClaimAndCompleteExactlyOneDatabaseEffect(t *testing.T) {
	gdb, s := setup(t)
	ctx := context.Background()
	if err := gdb.Exec(`CREATE TABLE effects(job_id TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	for n := range 40 {
		if _, err := s.Enqueue(ctx, Input{Kind: "delivery", BusinessKey: fmt.Sprint(n)}); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	var handled atomic.Int64
	for n := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			owner := fmt.Sprintf("worker-%d", n)
			for {
				jobs, err := s.Claim(ctx, "default", owner, time.Minute, 3)
				if err != nil {
					errs <- err
					return
				}
				if len(jobs) == 0 {
					return
				}
				for _, job := range jobs {
					err = gdb.Transaction(func(tx *gorm.DB) error {
						if err := tx.Exec(`INSERT INTO effects(job_id) VALUES (?)`, job.ID).Error; err != nil {
							return err
						}
						return s.CompleteTx(tx, job.ID, owner, job.Fence)
					})
					if err != nil {
						errs <- err
						return
					}
					handled.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if handled.Load() != 40 {
		t.Fatalf("handled %d, want 40", handled.Load())
	}
	var count int64
	gdb.Table("effects").Count(&count)
	if count != 40 {
		t.Fatalf("effects=%d", count)
	}
}

func TestLeaseReclaimFencesStaleResultAndDeadLetters(t *testing.T) {
	gdb, s := setup(t)
	ctx := context.Background()
	job, err := s.Enqueue(ctx, Input{Kind: "delivery", BusinessKey: "crash", MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Claim(ctx, "default", "old", time.Minute, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("claim: %v %v", first, err)
	}
	if err := gdb.Exec(`UPDATE durable_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=?`, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	second, err := s.Claim(ctx, "default", "new", time.Minute, 1)
	if err != nil || len(second) != 1 || second[0].Fence <= first[0].Fence {
		t.Fatalf("reclaim: %v %v", second, err)
	}
	if err := s.Complete(ctx, job.ID, "old", first[0].Fence); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale complete: %v", err)
	}
	if err := s.Fail(ctx, job.ID, "old", first[0].Fence, "old error"); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale fail: %v", err)
	}
	if err := s.ExtendLease(ctx, job.ID, "old", first[0].Fence, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale renewal: %v", err)
	}
	if err := s.Fail(ctx, job.ID, "new", second[0].Fence, "permanent failure"); err != nil {
		t.Fatal(err)
	}
	dead, err := s.DeadLetters(ctx, "default", 10)
	if err != nil || len(dead) != 1 || dead[0].Attempts != 2 || dead[0].LastError != "permanent failure" {
		t.Fatalf("dead letters: %v %v", dead, err)
	}
	if claims, err := s.Claim(ctx, "default", "other", time.Minute, 10); err != nil || len(claims) != 0 {
		t.Fatalf("dead job reclaimed: %v %v", claims, err)
	}
}

func TestRetrySchedulingAndFinalLeaseExpiry(t *testing.T) {
	gdb, s := setup(t)
	ctx := context.Background()
	job, err := s.Enqueue(ctx, Input{Kind: "delivery", BusinessKey: "retry", MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Claim(ctx, "default", "w", time.Minute, 1)
	if err != nil || len(first) != 1 {
		t.Fatal(err)
	}
	if err := s.Fail(ctx, job.ID, "w", first[0].Fence, "temporary"); err != nil {
		t.Fatal(err)
	}
	if claims, err := s.Claim(ctx, "default", "w", time.Minute, 1); err != nil || len(claims) != 0 {
		t.Fatalf("retry before schedule: %v %v", claims, err)
	}
	if err := gdb.Exec(`UPDATE durable_jobs SET available_at=clock_timestamp()-interval '1 second' WHERE id=?`, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	last, err := s.Claim(ctx, "default", "w", time.Minute, 1)
	if err != nil || len(last) != 1 {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE durable_jobs SET lease_until=clock_timestamp()-interval '1 second' WHERE id=?`, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if claims, err := s.Claim(ctx, "default", "w2", time.Minute, 1); err != nil || len(claims) != 0 {
		t.Fatalf("exhausted lease reclaimed: %v %v", claims, err)
	}
	dead, err := s.DeadLetters(ctx, "default", 10)
	if err != nil || len(dead) != 1 {
		t.Fatalf("lost final attempt: %v %v", dead, err)
	}
}

func TestClaimSkipsLockedRows(t *testing.T) {
	gdb, s := setup(t)
	ctx := context.Background()
	first, err := s.Enqueue(ctx, Input{Kind: "delivery", BusinessKey: "locked"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Enqueue(ctx, Input{Kind: "delivery", BusinessKey: "available"})
	if err != nil {
		t.Fatal(err)
	}
	tx := gdb.Begin()
	defer tx.Rollback()
	if err := tx.Exec(`SELECT id FROM durable_jobs WHERE id=? FOR UPDATE`, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	claims, err := s.Claim(ctx, "default", "worker", time.Minute, 2)
	if err != nil || len(claims) != 1 || claims[0].ID != second.ID {
		t.Fatalf("SKIP LOCKED failed: %v %v", claims, err)
	}
}
