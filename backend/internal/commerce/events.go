package commerce

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Event struct {
	ID          string          `json:"id"`
	OperatorID  string          `json:"operator_id"`
	AggregateID string          `json:"aggregate_id"`
	Kind        string          `json:"kind"`
	Payload     json.RawMessage `json:"payload" gorm:"type:jsonb"`
	CreatedAt   time.Time       `json:"created_at"`
	PublishedAt *time.Time      `json:"published_at,omitempty"`
}

func (Event) TableName() string { return "commerce_events" }

// EventEnqueuer must persist its destination job/outbox record using tx. The
// event ID is the durable consumer deduplication identity.
type EventEnqueuer interface {
	EnqueueCommerceEvent(context.Context, *gorm.DB, Event) error
}
type EventEnqueuerFunc func(context.Context, *gorm.DB, Event) error

func (f EventEnqueuerFunc) EnqueueCommerceEvent(ctx context.Context, tx *gorm.DB, e Event) error {
	return f(ctx, tx, e)
}

// PublishEvent keeps emission and the acknowledgement in one transaction.
func (s *Service) PublishEvent(ctx context.Context, tx *gorm.DB, id string, destination EventEnqueuer) error {
	return s.within(ctx, tx, func(t *gorm.DB) error {
		var event Event
		result := t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND published_at IS NULL", id).Limit(1).Find(&event)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if err := destination.EnqueueCommerceEvent(ctx, t, event); err != nil {
			return err
		}
		return t.Model(&event).Update("published_at", s.now()).Error
	})
}

func (s *Service) ListCustomerOrders(ctx context.Context, tx *gorm.DB, customerID string, limit int) ([]Order, error) {
	if !validID(customerID) || limit < 1 || limit > 200 {
		return nil, ErrInvalidInput
	}
	var rows []orderRow
	if err := s.dbFor(ctx, tx).Where("customer_id=?", customerID).Order("created_at DESC,id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Order, len(rows))
	for i, r := range rows {
		out[i] = Order(r)
	}
	return out, nil
}
func (s *Service) ListPackages(ctx context.Context, tx *gorm.DB, operatorID string) ([]Package, error) {
	if !validID(operatorID) {
		return nil, ErrInvalidInput
	}
	var rows []packageRow
	if err := s.dbFor(ctx, tx).Where("operator_id=? AND (new_purchase_enabled OR renewal_enabled)", operatorID).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Package, len(rows))
	for i, r := range rows {
		out[i] = Package(r)
	}
	return out, nil
}

func (s *Service) CountCompletedOrders(ctx context.Context, tx *gorm.DB, customerID string) (int64, error) {
	if !validID(customerID) {
		return 0, ErrInvalidInput
	}
	var count int64
	err := s.dbFor(ctx, tx).Model(&orderRow{}).Where("customer_id=? AND kind IN ('package','renewal') AND status='completed'", customerID).Count(&count).Error
	return count, err
}
