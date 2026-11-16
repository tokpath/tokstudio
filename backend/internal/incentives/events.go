package incentives

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) ObserveGrantEvent(ctx context.Context, tx *gorm.DB, grant ledger.Grant, event ledger.GrantEvent) error {
	if event.PaidMoney.IsZero() {
		return nil
	}
	if event.Kind != "usage" && event.Kind != "expiry" {
		return ErrInvalidInput
	}
	if tx == nil {
		return ErrInvalidInput
	}
	instant, err := s.BusinessInstant(ctx, tx)
	if err != nil {
		return err
	}
	return s.AccrueGrantEvent(ctx, tx, GrantEventInput{EventID: event.ID, OperatorID: grant.OperatorID, CustomerID: grant.CustomerID, OrderID: grant.OrderID, Kind: event.Kind, PaidMoney: event.PaidMoney, OccurredAt: event.CreatedAt, BusinessSequence: instant.Sequence})
}

type EventEnqueuer interface {
	EnqueueIncentiveEvent(context.Context, *gorm.DB, Event) error
}
type EventEnqueuerFunc func(context.Context, *gorm.DB, Event) error

func (f EventEnqueuerFunc) EnqueueIncentiveEvent(ctx context.Context, tx *gorm.DB, e Event) error {
	return f(ctx, tx, e)
}

// PublishEvent commits one notification and its acknowledgement together.
// The durable worker retries each event independently using its original ID.
func (s *Service) PublishEvent(ctx context.Context, tx *gorm.DB, id string, sink EventEnqueuer) error {
	if sink == nil || !validID(id) {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		var event Event
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND published_at IS NULL", id).Limit(1).Find(&event)
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		if err := sink.EnqueueIncentiveEvent(ctx, tx, event); err != nil {
			return err
		}
		return tx.Model(&event).Update("published_at", gorm.Expr("clock_timestamp()")).Error
	})
}
