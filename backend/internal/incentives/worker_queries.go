package incentives

import (
	"context"
	"gorm.io/gorm"
)

func (s *Service) PendingRegistrationRewards(ctx context.Context, tx *gorm.DB, limit int) ([]string, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrInvalidInput
	}
	ids := []string{}
	e := s.dbFor(ctx, tx).Model(&promiseRow{}).Where("status='pending_delivery'").Order("created_at,intent_id").Limit(limit).Pluck("intent_id", &ids).Error
	return ids, e
}
func (s *Service) PendingRegistrationRewardsPage(ctx context.Context, tx *gorm.DB, cursor string, limit int) ([]string, error) {
	if limit < 1 || limit > 200 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Model(&promiseRow{}).Where("status='pending_delivery'")
	if cursor != "" {
		q = q.Where("intent_id>?", cursor)
	}
	ids := []string{}
	e := q.Order("intent_id").Limit(limit).Pluck("intent_id", &ids).Error
	return ids, e
}
