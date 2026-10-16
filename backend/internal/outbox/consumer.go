package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Event struct {
	ID, Type, AggregateID string
	Data                  json.RawMessage
	CreatedAt             time.Time
}

// RegisterConsumer explicitly chooses the historical backfill policy. A nil
// cutoff replays all retained history. A non-nil cutoff must be a deliberate
// product decision; starting a process is never itself a cutoff. Existing
// registrations cannot silently change their policy.
func (s *Service) RegisterConsumer(ctx context.Context, consumer string, from *time.Time) error {
	if strings.TrimSpace(consumer) == "" {
		return errors.New("consumer name is required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO outbox_consumers(name,replay_from) VALUES (?,?) ON CONFLICT DO NOTHING`, consumer, from).Error; err != nil {
			return err
		}
		var row struct{ ReplayFrom *time.Time }
		if err := tx.Raw(`SELECT replay_from FROM outbox_consumers WHERE name=?`, consumer).Scan(&row).Error; err != nil {
			return err
		}
		if (row.ReplayFrom == nil) != (from == nil) || (from != nil && !row.ReplayFrom.Equal(*from)) {
			return errors.New("consumer already registered with a different replay policy")
		}
		return nil
	})
}

// Consume is independent of Redis publication. The default policy for an
// unregistered handler replays all retained history. A receipt keyed by handler
// and event ID commits atomically with handler database changes. Handlers must
// not perform network side effects inside this transaction: enqueue durable work.
func (s *Service) Consume(ctx context.Context, consumer string, types []string, handler func(*gorm.DB, Event) error) error {
	if strings.TrimSpace(consumer) == "" || handler == nil {
		return errors.New("consumer name and handler are required")
	}
	if len(types) == 0 {
		return nil
	}
	if err := s.db.WithContext(ctx).Exec(`INSERT INTO outbox_consumers(name,replay_from) VALUES (?,NULL) ON CONFLICT DO NOTHING`, consumer).Error; err != nil {
		return err
	}
	for range 50 {
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row eventRow
			err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where(`event_type IN ? AND EXISTS (SELECT 1 FROM outbox_consumers c WHERE c.name=? AND (c.replay_from IS NULL OR outbox_events.created_at >= c.replay_from))
				AND NOT EXISTS (SELECT 1 FROM outbox_consumptions WHERE consumer=? AND event_id=outbox_events.id)`, types, consumer, consumer).
				Order("created_at,id").First(&row).Error
			if err != nil {
				return err
			}
			// The unique receipt is the final arbiter even if a SELECT snapshot
			// began just before another transaction committed its receipt.
			receipt := tx.Exec(`INSERT INTO outbox_consumptions(consumer,event_id) VALUES (?,?) ON CONFLICT DO NOTHING`, consumer, row.ID)
			if receipt.Error != nil {
				return receipt.Error
			}
			if receipt.RowsAffected == 0 {
				return nil
			}
			return handler(tx, Event{ID: row.ID, Type: row.EventType, AggregateID: row.AggregateID, Data: json.RawMessage(row.PayloadJSON), CreatedAt: row.CreatedAt})
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
