package plans

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// The published plan remains locked through subscription and payment-order creation.
func (s *Service) CreateSubscriptionTx(tx *gorm.DB, userID, channelID, planID, adapter, methodRef, brandOwnerID string, autoRenew bool) (*SubscriptionView, error) {
	var row planRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", planID).First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	if autoRenew && (!row.AutoRenewAllowed || row.BillingPeriod == PeriodOnce || adapter != "stripe") {
		return nil, ErrInvalidPlan
	}
	scoped := *s
	scoped.db = tx
	return scoped.createSubscription(tx.Statement.Context, userID, channelID, planID, adapter, methodRef, brandOwnerID, autoRenew)
}
