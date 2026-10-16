package incentives

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) ObserveGrantEvent(ctx context.Context, tx *gorm.DB, grant ledger.Grant, event ledger.GrantEvent) error {
	if event.PaidUSD.IsZero() {
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
	return s.AccrueGrantEvent(ctx, tx, GrantEventInput{EventID: event.ID, OperatorID: grant.OperatorID, CustomerID: grant.CustomerID, OrderID: grant.OrderID, Kind: event.Kind, PaidUSD: event.PaidUSD, OccurredAt: event.CreatedAt, BusinessSequence: instant.Sequence})
}

type EventEnqueuer interface {
	EnqueueIncentiveEvent(context.Context, *gorm.DB, Event) error
}
type EventEnqueuerFunc func(context.Context, *gorm.DB, Event) error

func (f EventEnqueuerFunc) EnqueueIncentiveEvent(ctx context.Context, tx *gorm.DB, e Event) error {
	return f(ctx, tx, e)
}
func (s *Service) RelayEvents(ctx context.Context, tx *gorm.DB, limit int, sink EventEnqueuer) (count int, err error) {
	if sink == nil || limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		var events []Event
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("published_at IS NULL").Order("created_at,id").Limit(limit).Find(&events).Error; err != nil {
			return err
		}
		for _, e := range events {
			if err := sink.EnqueueIncentiveEvent(ctx, tx, e); err != nil {
				return err
			}
			if err := tx.Model(&Event{}).Where("id=?", e.ID).Update("published_at", gorm.Expr("clock_timestamp()")).Error; err != nil {
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
