package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

const redisPublishedStream = "tokenhub:outbox:published"
const deliveryLease = 30 * time.Second

var ErrDeliveryLeaseLost = errors.New("outbox delivery lease no longer current")

// CloudEvent is an at-least-once envelope. Its stable ID is the idempotency key
// for every receiving handler; Redis stream entry IDs are not business IDs.
type CloudEvent struct {
	SpecVersion     string          `json:"specversion"`
	ID              string          `json:"id"`
	Source          string          `json:"source"`
	Type            string          `json:"type"`
	Time            time.Time       `json:"time"`
	DataContentType string          `json:"datacontenttype"`
	Data            json.RawMessage `json:"data"`
}

// Worker claims a short database lease, releases the transaction, then publishes.
// A crash after publication and before acknowledgment can redeliver the same ID.
// Business consumers must acknowledge in the same transaction as their changes.
type Worker struct {
	db      *gorm.DB
	redis   *redis.Client
	logger  zerolog.Logger
	publish func(context.Context, []byte) error
}

func NewWorker(gdb *gorm.DB, redisClient *redis.Client, logger zerolog.Logger) *Worker {
	return &Worker{db: gdb, redis: redisClient, logger: logger, publish: func(ctx context.Context, body []byte) error {
		return redisClient.XAdd(ctx, &redis.XAddArgs{Stream: redisPublishedStream, Values: map[string]any{"cloudevent": string(body)}}).Err()
	}}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = redisx.TouchWorkerHeartbeat(ctx, w.redis)
			if err := w.drain(ctx); err != nil {
				w.logger.Error().Err(err).Msg("outbox_drain_failed")
			}
		}
	}
}

func (w *Worker) claimOne(ctx context.Context) (eventRow, error) {
	var row eventRow
	err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`WITH exhausted AS (
			SELECT id FROM outbox_events WHERE status='publishing' AND lease_until <= clock_timestamp()
			AND attempts >= max_attempts FOR UPDATE SKIP LOCKED)
			UPDATE outbox_events e SET status='failed',lease_until=NULL,last_error='lease expired on final attempt'
			FROM exhausted x WHERE e.id=x.id`).Error; err != nil {
			return err
		}
		var rows []eventRow
		if err := tx.Raw(`WITH candidate AS (
			SELECT id FROM outbox_events WHERE attempts < max_attempts
			AND ((status='pending' AND available_at <= clock_timestamp())
			OR (status='publishing' AND lease_until <= clock_timestamp()))
			ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1)
			UPDATE outbox_events e SET status='publishing',attempts=e.attempts+1,fence=e.fence+1,
			lease_until=clock_timestamp() + (? * interval '1 microsecond')
			FROM candidate c WHERE e.id=c.id RETURNING e.*`, deliveryLease.Microseconds()).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return gorm.ErrRecordNotFound
		}
		row = rows[0]
		return nil
	})
	return row, err
}

func (w *Worker) drain(ctx context.Context) error {
	// Claim just before each send so a slow earlier send cannot consume the lease
	// of a whole preclaimed batch. No database transaction spans network I/O.
	for range 20 {
		row, err := w.claimOne(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := w.publishOne(ctx, row); err != nil {
			w.logger.Error().Err(err).Str("event_id", row.ID).Msg("outbox_publish_failed")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) publishOne(ctx context.Context, row eventRow) error {
	event := CloudEvent{SpecVersion: "1.0", ID: row.ID, Source: "tokenhub/outbox", Type: row.EventType, Time: row.CreatedAt.UTC(), DataContentType: "application/json", Data: json.RawMessage(row.PayloadJSON)}
	body, err := json.Marshal(event)
	if err != nil {
		return w.finish(ctx, row, err)
	}
	publishCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = w.publish(publishCtx, body)
	cancel()
	return w.finish(ctx, row, err)
}

func (w *Worker) finish(ctx context.Context, row eventRow, publishErr error) error {
	updates := map[string]any{"lease_until": nil}
	if publishErr == nil {
		updates["status"] = "published"
		updates["published_at"] = gorm.Expr("clock_timestamp()")
		updates["last_error"] = nil
	} else {
		status := "pending"
		if row.Attempts >= row.MaxAttempts {
			status = "failed"
		}
		updates["status"] = status
		updates["available_at"] = gorm.Expr("clock_timestamp() + (? * interval '1 second')", deliveryBackoff(row.Attempts))
		message := publishErr.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
		updates["last_error"] = message
	}
	result := w.db.WithContext(ctx).Model(&eventRow{}).
		Where("id=? AND status='publishing' AND fence=? AND lease_until > clock_timestamp()", row.ID, row.Fence).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrDeliveryLeaseLost
	}
	if publishErr != nil {
		return fmt.Errorf("publish event %s: %w", row.ID, publishErr)
	}
	return nil
}

func deliveryBackoff(attempt int) int {
	if attempt < 1 {
		return 1
	}
	if attempt > 10 {
		return 900
	}
	delay := 1 << uint(attempt-1)
	if delay > 900 {
		return 900
	}
	return delay
}
