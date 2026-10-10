package billing

import "context"

// ReferralProgress uses the same confirmed-consumption and single-topup facts as
// eligibility. Gift credits and commission money never count as a topup.
type ReferralProgress struct {
	SpendMinor         int64 `json:"spend_minor"`
	LargestTopupMinor  int64 `json:"largest_topup_minor"`
	GiftGrantedMinor   int64 `json:"gift_granted_minor"`
	GiftRemainingMinor int64 `json:"gift_remaining_minor"`
}

func (s *Service) ReferralProgress(ctx context.Context, userID string) (*ReferralProgress, error) {
	out := &ReferralProgress{}
	if err := s.db.WithContext(ctx).Model(&usageRow{}).Where("user_id = ? AND state = ?", userID, UsageConfirmed).
		Select("COALESCE(SUM(customer_amount_minor), 0)").Scan(&out.SpendMinor).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Table("billing_ledger AS l").Joins("JOIN billing_wallets AS w ON w.id = l.wallet_id").
		Where("w.user_id = ? AND l.event_type = ? AND l.amount_minor > 0", userID, EventTopup).
		Select("COALESCE(MAX(l.amount_minor), 0)").Scan(&out.LargestTopupMinor).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Table("billing_ledger AS l").Joins("JOIN billing_wallets AS w ON w.id = l.wallet_id").
		Where("w.user_id = ? AND l.event_type = ? AND l.amount_minor > 0", userID, EventGiftCredit).
		Select("COALESCE(SUM(l.amount_minor), 0)").Scan(&out.GiftGrantedMinor).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&walletRow{}).Where("user_id = ?", userID).
		Select("COALESCE(SUM(gift_minor), 0)").Scan(&out.GiftRemainingMinor).Error; err != nil {
		return nil, err
	}
	return out, nil
}
