package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/db/dbtest"
)

func setupOutbox(t *testing.T) (*gorm.DB, *Service) {
	t.Helper()
	gdb := dbtest.New(t)
	name, files := Migrations()
	if err := db.Apply(gdb, []db.ModuleMigrations{{Module: name, FS: files}}); err != nil {
		t.Fatal(err)
	}
	return gdb, New(gdb)
}

func TestConsumerReplaysHistoryAndRollsBackFailedReceipt(t *testing.T) {
	gdb, s := setupOutbox(t)
	ctx := context.Background()
	eventID, err := s.Enqueue(ctx, "test.created", "order", "one", map[string]any{"ok": true})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE outbox_events SET created_at='2000-01-01T00:00:00Z' WHERE id=?`, eventID).Error; err != nil {
		t.Fatal(err)
	}
	// Registration time is intentionally newer than the event. Full replay is
	// the default, so a worker introduced after a crash still handles it.
	failure := errors.New("handler failed")
	if err := s.Consume(ctx, "handler-a", []string{"test.created"}, func(tx *gorm.DB, event Event) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var receipts int64
	gdb.Table("outbox_consumptions").Count(&receipts)
	if receipts != 0 {
		t.Fatal("failed handler acknowledged event")
	}
	count := 0
	handler := func(tx *gorm.DB, event Event) error {
		count++
		if event.ID != eventID {
			t.Fatal("event changed")
		}
		return nil
	}
	for range 2 {
		if err := s.Consume(ctx, "handler-a", []string{"test.created"}, handler); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Consume(ctx, "handler-b", []string{"test.created"}, handler); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("want one invocation per handler, got %d", count)
	}
	cutoff := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.RegisterConsumer(ctx, "deliberate-cutoff", &cutoff); err != nil {
		t.Fatal(err)
	}
	if err := s.Consume(ctx, "deliberate-cutoff", []string{"test.created"}, handler); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("explicit replay cutoff ignored")
	}
	if err := s.RegisterConsumer(ctx, "deliberate-cutoff", nil); err == nil {
		t.Fatal("policy silently changed")
	}
}

func TestConcurrentConsumerReceiptsAreIdempotent(t *testing.T) {
	gdb, s := setupOutbox(t)
	ctx := context.Background()
	if err := gdb.Exec(`CREATE TABLE consumed_effects(event_id TEXT PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	for range 32 {
		if _, err := s.Enqueue(ctx, "test.created", "order", "one", nil); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- s.Consume(ctx, "same-handler", []string{"test.created"}, func(tx *gorm.DB, event Event) error {
				calls.Add(1)
				return tx.Exec(`INSERT INTO consumed_effects(event_id) VALUES (?)`, event.ID).Error
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	gdb.Table("consumed_effects").Count(&count)
	if count != 32 || calls.Load() != 32 {
		t.Fatalf("effects=%d calls=%d", count, calls.Load())
	}
}

func TestPublisherDoesNotHoldDatabaseLockAcrossNetwork(t *testing.T) {
	gdb, s := setupOutbox(t)
	ctx := context.Background()
	eventID, err := s.Enqueue(ctx, "test.created", "order", "one", nil)
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	w := &Worker{db: gdb, logger: zerolog.Nop(), publish: func(_ context.Context, body []byte) error {
		calls++
		// A separate connection must be able to lock the claimed row while
		// the external publisher is running; NOWAIT would fail with old code.
		return gdb.Transaction(func(tx *gorm.DB) error {
			return tx.Exec(`SELECT id FROM outbox_events WHERE id=? FOR UPDATE NOWAIT`, eventID).Error
		})
	}}
	if err := w.drain(ctx); err != nil {
		t.Fatal(err)
	}
	var row eventRow
	if err := gdb.First(&row, "id=?", eventID).Error; err != nil {
		t.Fatal(err)
	}
	if calls != 1 || row.Status != "published" || row.Attempts != 1 {
		t.Fatalf("publication: calls=%d row=%+v", calls, row)
	}
}

func TestLostAcknowledgmentRedeliversSameEventIDAndConsumerDeduplicates(t *testing.T) {
	gdb, s := setupOutbox(t)
	ctx := context.Background()
	eventID, err := s.Enqueue(ctx, "test.created", "order", "one", nil)
	if err != nil {
		t.Fatal(err)
	}
	var published []string
	w := &Worker{db: gdb, logger: zerolog.Nop(), publish: func(_ context.Context, body []byte) error {
		var event CloudEvent
		if err := json.Unmarshal(body, &event); err != nil {
			return err
		}
		published = append(published, event.ID)
		if len(published) == 1 {
			return gdb.Exec(`UPDATE outbox_events SET lease_until=clock_timestamp()-interval '1 second' WHERE id=?`, eventID).Error
		}
		return nil
	}}
	first, err := w.claimOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.publishOne(ctx, first); !errors.Is(err, ErrDeliveryLeaseLost) {
		t.Fatalf("stale ack: %v", err)
	}
	second, err := w.claimOne(ctx)
	if err != nil || second.Fence <= first.Fence {
		t.Fatalf("reclaim: %+v %v", second, err)
	}
	if err := w.publishOne(ctx, second); err != nil {
		t.Fatal(err)
	}
	if len(published) != 2 || published[0] != eventID || published[1] != eventID {
		t.Fatalf("event identity changed: %v", published)
	}
	if err := w.finish(ctx, first, nil); !errors.Is(err, ErrDeliveryLeaseLost) {
		t.Fatalf("stale worker changed terminal status: %v", err)
	}
	calls := 0
	for range 2 {
		if err := s.Consume(ctx, "business-handler", []string{"test.created"}, func(_ *gorm.DB, _ Event) error { calls++; return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("duplicate business effect: %d", calls)
	}
}

func TestPublishFailuresHaveBoundedRetries(t *testing.T) {
	gdb, s := setupOutbox(t)
	ctx := context.Background()
	eventID, err := s.Enqueue(ctx, "test.created", "order", "one", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE outbox_events SET max_attempts=2 WHERE id=?`, eventID).Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("redis unavailable")
	w := &Worker{db: gdb, logger: zerolog.Nop(), publish: func(context.Context, []byte) error { return failure }}
	for range 2 {
		row, err := w.claimOne(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.publishOne(ctx, row); !errors.Is(err, failure) {
			t.Fatal(err)
		}
		if err := gdb.Exec(`UPDATE outbox_events SET available_at=clock_timestamp()-interval '1 second' WHERE id=?`, eventID).Error; err != nil {
			t.Fatal(err)
		}
	}
	var row eventRow
	if err := gdb.First(&row, "id=?", eventID).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "failed" || row.Attempts != 2 {
		t.Fatalf("unbounded retry: %+v", row)
	}
	if _, err := w.claimOne(ctx); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("dead letter reclaimed: %v", err)
	}
}
