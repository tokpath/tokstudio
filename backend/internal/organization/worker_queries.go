package organization

import (
	"context"
	"gorm.io/gorm"
)

// OperatorPage is a trusted worker enumeration, never a supplier directory API.
func (s *Service) OperatorPage(ctx context.Context, tx *gorm.DB, cursor string, limit int) ([]Operator, error) {
	if limit < 1 || limit > 200 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Order("id").Limit(limit)
	if cursor != "" {
		q = q.Where("id>?", cursor)
	}
	var rows []operatorRow
	if e := q.Find(&rows).Error; e != nil {
		return nil, e
	}
	out := make([]Operator, len(rows))
	for i, r := range rows {
		out[i] = Operator(r)
	}
	return out, nil
}

type CurrentCustomer struct{ CustomerID, OperatorID, BrandID string }

func (s *Service) CurrentCustomerPage(ctx context.Context, tx *gorm.DB, cursor string, limit int) ([]CurrentCustomer, error) {
	if limit < 1 || limit > 200 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Table("organization_service_ownerships").Where("ended_at IS NULL").Order("customer_id").Limit(limit)
	if cursor != "" {
		q = q.Where("customer_id>?", cursor)
	}
	rows := []CurrentCustomer{}
	e := q.Find(&rows).Error
	return rows, e
}
