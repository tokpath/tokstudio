package plans

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// A refunded purchase must never remain eligible for a later renewal.
func (s *Service) RefundSubscriptionTx(tx *gorm.DB, subscriptionID string) error {
	var row subRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", subscriptionID).First(&row).Error; err != nil {
		return err
	}
	if err := s.ReverseSourceTx(tx, subscriptionID); err != nil {
		return err
	}
	row.Status = SubCancelled
	row.RenewalPolicy = RenewManual
	row.NextRetryAt = nil
	row.GraceUntil = nil
	row.UpdatedAt = time.Now().UTC()
	return tx.Save(&row).Error
}
