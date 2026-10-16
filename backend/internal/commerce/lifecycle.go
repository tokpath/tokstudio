package commerce

import (
	"context"
	"gorm.io/gorm"
)

// StopCustomerRenewals is a trusted lifecycle hook and must join the identity
// state transition transaction. Resuming usage never re-enables old consents.
func (s *Service) StopCustomerRenewals(ctx context.Context, tx *gorm.DB, operator, customer, evidence string) error {
	if tx == nil || !validID(operator) || !validID(customer) || evidence == "" {
		return ErrInvalidInput
	}
	own, err := s.org.CurrentOwnership(ctx, tx, customer)
	if err != nil {
		return err
	}
	if own.OperatorID != operator {
		return ErrScope
	}
	var rows []subscriptionRow
	if err = tx.Where("operator_id=? AND customer_id=? AND status<>'cancelled'", operator, customer).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		if err = tx.Model(&r).Updates(map[string]any{"status": "cancelled", "version": r.Version + 1}).Error; err != nil {
			return err
		}
		if err = s.event(tx, operator, r.ID, "renewal.cancelled", map[string]any{"customer_id": customer, "evidence": evidence}); err != nil {
			return err
		}
	}
	return nil
}
