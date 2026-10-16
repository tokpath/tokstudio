package ledger

import (
	"context"
	"gorm.io/gorm"
	"strings"
	"time"
)

type ExpiringCustomer struct{ OperatorID, CustomerID string }

// Only actual remaining grants need expiry work; immutable grant histories stay.
func (s *Service) ExpiringCustomerCredits(ctx context.Context, tx *gorm.DB, at time.Time, limit int) ([]ExpiringCustomer, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrScope
	}
	rows := []ExpiringCustomer{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Model(&Grant{}).Distinct("operator_id,customer_id").Where("expires_at<=? AND total-consumed-expired-cancelled>0", at).Order("operator_id,customer_id").Limit(limit).Find(&rows).Error
	})
	return rows, e
}

// Stable composite pagination prevents held/blocked early grants from starving
// later customers during repeated discovery scans.
func (s *Service) ExpiringCustomerCreditsPage(ctx context.Context, tx *gorm.DB, at time.Time, cursor string, limit int) ([]ExpiringCustomer, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrScope
	}
	parts := strings.SplitN(cursor, ":", 2)
	rows := []ExpiringCustomer{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Model(&Grant{}).Distinct("operator_id,customer_id").Where("expires_at<=? AND total-consumed-expired-cancelled>0", at)
		if cursor != "" {
			if len(parts) != 2 {
				return ErrScope
			}
			q = q.Where("(operator_id::text,customer_id::text)>(?,?)", parts[0], parts[1])
		}
		return q.Order("operator_id,customer_id").Limit(limit).Find(&rows).Error
	})
	return rows, e
}
