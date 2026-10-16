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
func (s *Service) RelayEvents(ctx context.Context, tx *gorm.DB, limit int, destination EventEnqueuer) (count int, err error) {
	if destination == nil || limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		var events []Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("published_at IS NULL").Order("created_at,id").Limit(limit).Find(&events).Error; err != nil {
			return err
		}
		for _, event := range events {
			if err := destination.EnqueueCommerceEvent(ctx, tx, event); err != nil {
				return err
			}
			if err := tx.Model(&Event{}).Where("id=? AND published_at IS NULL", event.ID).Update("published_at", s.now()).Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		count = 0
	}
	return
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
