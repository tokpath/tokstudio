package billing

import (
	"context"
	"errors"
)

type CustomerUsageSummary struct {
	ConfirmedCount int64 `json:"confirmed_count"`
	PendingCount   int64 `json:"pending_count"`
	ConfirmedMinor int64 `json:"confirmed_minor"`
	RefundedMinor  int64 `json:"refunded_minor"`
}

// Aggregate all facts; a recent page is never used as the customer's total.
func (s *Service) CustomerUsage(ctx context.Context, userID string, channelIDs []string) (*CustomerUsageSummary, error) {
	out := &CustomerUsageSummary{}
	err := s.db.WithContext(ctx).Table("billing_usage_events").Where("user_id=? AND COALESCE(channel_org_id, '') IN ?", userID, channelIDs).Select("COUNT(*) FILTER (WHERE state='confirmed') AS confirmed_count,COUNT(*) FILTER (WHERE state='pending') AS pending_count,COALESCE(SUM(customer_amount_minor) FILTER (WHERE state='confirmed'),0) AS confirmed_minor").Scan(out).Error
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Table("billing_customer_charges c").Joins("JOIN billing_usage_events u ON u.id=c.usage_event_id").Where("u.user_id=? AND c.status='reversed' AND COALESCE(u.channel_org_id, '') IN ?", userID, channelIDs).Select("COALESCE(SUM(c.amount_minor),0)").Scan(&out.RefundedMinor).Error
	return out, err
}

// CustomerCredit reads existing balances without creating a wallet or exposing
// merchant/pool balances or the customer's commission recovery information.
type CustomerCredit struct {
	AvailableMinor int64 `json:"available_minor"`
	ReservedMinor  int64 `json:"reserved_minor"`
	GiftMinor      int64 `json:"gift_minor"`
}

func (s *Service) CustomerCredit(ctx context.Context, userID string) (*CustomerCredit, error) {
	out := &CustomerCredit{}
	err := s.db.WithContext(ctx).Model(&walletRow{}).Where("user_id=?", userID).Select("COALESCE(SUM(available_minor),0) AS available_minor,COALESCE(SUM(reserved_minor),0) AS reserved_minor,COALESCE(SUM(gift_minor),0) AS gift_minor").Scan(out).Error
	return out, err
}

// The wallet is a single global account. It cannot be split into brand balances
// from an attribution snapshot; deny a restricted projection with foreign facts.
var ErrCustomerCreditScope = errors.New("customer credit includes facts outside the authorized brand")

func (s *Service) CustomerCreditScope(ctx context.Context, userID string, channelIDs []string) error {
	var outside bool
	err := s.db.WithContext(ctx).Raw(`SELECT EXISTS(
 SELECT 1 FROM billing_topups WHERE user_id=? AND COALESCE(channel_org_id,'') NOT IN ?
 UNION ALL SELECT 1 FROM billing_usage_events WHERE user_id=? AND COALESCE(channel_org_id,'') NOT IN ?
 )`, userID, channelIDs, userID, channelIDs).Scan(&outside).Error
	if err != nil {
		return err
	}
	if outside {
		return ErrCustomerCreditScope
	}
	return nil
}
