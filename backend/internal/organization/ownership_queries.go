package organization

import (
	"context"

	"gorm.io/gorm"
)

// RequireAssumedCustomerResponsibility is a trusted cross-domain scope check.
// It grants no access to the former operator's merchant, staff or other clients.
func (s *Service) RequireAssumedCustomerResponsibility(ctx context.Context, tx *gorm.DB, currentOperator, sourceOperator, customer string) error {
	if tx == nil || !validID(currentOperator) || !validID(sourceOperator) || !validID(customer) || currentOperator == sourceOperator {
		return ErrInvalidInput
	}
	own, e := s.CurrentOwnership(ctx, tx, customer)
	if e != nil {
		return e
	}
	if own.OperatorID != currentOperator {
		return ErrForbidden
	}
	source, e := s.GetOperator(ctx, tx, sourceOperator)
	if e != nil {
		return e
	}
	if source.Kind != Channel || source.ParentOperatorID == nil || *source.ParentOperatorID != currentOperator {
		return ErrForbidden
	}
	brand, e := s.GetBrand(ctx, tx, source.BrandID)
	if e != nil {
		return e
	}
	if brand.OwnerOperatorID != currentOperator {
		return ErrForbidden
	}
	var count int64
	if e = tx.WithContext(ctx).Model(&ownershipRow{}).Where("customer_id=? AND operator_id=? AND ended_at IS NOT NULL", customer, sourceOperator).Count(&count).Error; e != nil {
		return e
	}
	if count == 0 {
		return ErrForbidden
	}
	return nil
}
