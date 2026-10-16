package organization

import (
	"context"

	"gorm.io/gorm"
)

// ReferralCounts exposes relationship counts only, without another customer's
// identity, wallet, Key or usage data. Callers use an authenticated customer ID.
func (s *Service) ReferralCounts(ctx context.Context, tx *gorm.DB, customerID string) (direct, indirect int64, err error) {
	if !validID(customerID) {
		return 0, 0, ErrInvalidInput
	}
	db := s.dbFor(ctx, tx)
	if err = db.Table("organization_referral_edges").Where("inviter_id=?", customerID).Count(&direct).Error; err != nil {
		return
	}
	err = db.Table("organization_referral_edges child").Joins("JOIN organization_referral_edges parent ON parent.invitee_id=child.inviter_id").Where("parent.inviter_id=?", customerID).Count(&indirect).Error
	return
}
