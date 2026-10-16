package commerce

import (
	"context"
	"gorm.io/gorm"
	"time"
)

func (s *Service) DueRenewals(ctx context.Context, tx *gorm.DB, at time.Time, noticeBefore time.Duration, limit int) ([]Subscription, error) {
	if limit < 1 || limit > 200 || noticeBefore < 0 || noticeBefore > 30*24*time.Hour {
		return nil, ErrInvalidInput
	}
	var rows []subscriptionRow
	e := s.dbFor(ctx, tx).Where("status IN ('active','needs_funds') AND next_due_at<=?", at.Add(noticeBefore)).Order("next_due_at,id").Limit(limit).Find(&rows).Error
	out := make([]Subscription, len(rows))
	for i, r := range rows {
		out[i] = Subscription(r)
	}
	return out, e
}
func (s *Service) PendingDeliveries(ctx context.Context, tx *gorm.DB, limit int) ([]Order, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrInvalidInput
	}
	var rows []orderRow
	e := s.dbFor(ctx, tx).Where("status IN ('paid','exception')").Order("created_at,id").Limit(limit).Find(&rows).Error
	out := make([]Order, len(rows))
	for i, r := range rows {
		out[i] = Order(r)
	}
	return out, e
}
func (s *Service) DueRenewalsPage(ctx context.Context, tx *gorm.DB, at time.Time, noticeBefore time.Duration, cursor string, limit int) ([]Subscription, error) {
	if limit < 1 || limit > 200 || noticeBefore < 0 || noticeBefore > 30*24*time.Hour || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Where("status IN ('active','needs_funds') AND next_due_at<=?", at.Add(noticeBefore))
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []subscriptionRow
	e := q.Order("id").Limit(limit).Find(&rows).Error
	out := make([]Subscription, len(rows))
	for i, r := range rows {
		out[i] = Subscription(r)
	}
	return out, e
}
func (s *Service) PendingDeliveriesPage(ctx context.Context, tx *gorm.DB, cursor string, limit int) ([]Order, error) {
	if limit < 1 || limit > 200 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Where("status IN ('paid','exception')")
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []orderRow
	e := q.Order("id").Limit(limit).Find(&rows).Error
	out := make([]Order, len(rows))
	for i, r := range rows {
		out[i] = Order(r)
	}
	return out, e
}
