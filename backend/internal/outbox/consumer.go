package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Event struct {
	ID, Type, AggregateID string
	Data                  json.RawMessage
	CreatedAt             time.Time
}

// Consume is independent of Redis publication. The handler and acknowledgement
// commit in the same transaction; crashes and competing workers cannot lose or
// duplicate a notification. Only explicitly subscribed event types are exposed.
func (s *Service) Consume(ctx context.Context, consumer string, types []string, handler func(*gorm.DB, Event) error) error {
	for range 50 {
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var row eventRow
			err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
				Where(`event_type IN ? AND created_at >= (SELECT started_at FROM outbox_consumers WHERE name = ?)
                AND NOT EXISTS (SELECT 1 FROM outbox_consumptions WHERE consumer = ? AND event_id = outbox_events.id)`, types, consumer, consumer).
				Order("created_at, id").First(&row).Error
			if err != nil {
				return err
			}
			if err := handler(tx, Event{row.ID, row.EventType, row.AggregateID, json.RawMessage(row.PayloadJSON), row.CreatedAt}); err != nil {
				return err
			}
			return tx.Exec("INSERT INTO outbox_consumptions(consumer, event_id) VALUES (?, ?)", consumer, row.ID).Error
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
