package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

const redisPublishedStream = "tokenhub:outbox:published"

// CloudEvent 与 Dapr 兼容的 envelope。替换投递器时不改业务事件契约。
type CloudEvent struct {
	SpecVersion     string          `json:"specversion"`
	ID              string          `json:"id"`
	Source          string          `json:"source"`
	Type            string          `json:"type"`
	Time            time.Time       `json:"time"`
	DataContentType string          `json:"datacontenttype"`
	Data            json.RawMessage `json:"data"`
}

// Worker 轮询 pending 事件，发布到 Redis Stream，并标记 published。
type Worker struct {
	db     *gorm.DB
	redis  *redis.Client
	logger zerolog.Logger
}

func NewWorker(db *gorm.DB, redisClient *redis.Client, logger zerolog.Logger) *Worker {
	return &Worker{db: db, redis: redisClient, logger: logger}
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

func (w *Worker) drain(ctx context.Context) error {
	var rows []eventRow
	if err := w.db.WithContext(ctx).
		Where("status = ? AND available_at <= ?", "pending", time.Now().UTC()).
		Order("created_at ASC").
		Limit(20).
		Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.publishOne(ctx, row); err != nil {
			w.logger.Error().Err(err).Str("event_id", row.ID).Msg("outbox_publish_failed")
		}
	}
	return nil
}

func (w *Worker) publishOne(ctx context.Context, row eventRow) error {
	event := CloudEvent{
		SpecVersion:     "1.0",
		ID:              row.ID,
		Source:          "tokenhub/outbox",
		Type:            row.EventType,
		Time:            row.CreatedAt.UTC(),
		DataContentType: "application/json",
		Data:            json.RawMessage(row.PayloadJSON),
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked eventRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = ?", row.ID, "pending").
			First(&locked).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		if err := w.redis.XAdd(ctx, &redis.XAddArgs{
			Stream: redisPublishedStream,
			Values: map[string]any{"cloudevent": string(body)},
		}).Err(); err != nil {
			next := time.Now().UTC().Add(time.Duration(locked.Attempts+1) * time.Second)
			msg := err.Error()
			return tx.Model(&eventRow{}).Where("id = ?", locked.ID).Updates(map[string]any{
				"attempts":     locked.Attempts + 1,
				"available_at": next,
				"last_error":   msg,
			}).Error
		}
		now := time.Now().UTC()
		return tx.Model(&eventRow{}).Where("id = ?", locked.ID).Updates(map[string]any{
			"status":       "published",
			"published_at": now,
			"last_error":   nil,
			"attempts":     locked.Attempts + 1,
		}).Error
	})
}

// RecentPublished 供验收脚本确认 Worker 已投递。
func RecentPublished(ctx context.Context, client *redis.Client, count int64) ([]string, error) {
	items, err := client.XRevRangeN(ctx, redisPublishedStream, "+", "-", count).Result()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value, ok := item.Values["cloudevent"].(string); ok {
			out = append(out, value)
		}
	}
	return out, nil
}
