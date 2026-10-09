package billing

import (
	"context"
	"errors"
)

type ChannelPnLView struct {
	ChannelOrgID    string `json:"channel_org_id"`
	PoolStatus      string `json:"pool_status"`
	ModelCostMinor  int64  `json:"model_cost_minor"`
	RechargeMinor   int64  `json:"recharge_minor"`
	UnconsumedMinor int64  `json:"unconsumed_minor"`
	ConsumedMinor   int64  `json:"consumed_minor"`
	MarketingMinor  int64  `json:"marketing_minor"`
	MarketingFrozen int64  `json:"marketing_frozen_minor"`
	MarketingIssued int64  `json:"marketing_issued_minor"`
	SupplierMinor   int64  `json:"supplier_minor"`
	AttemptCost     *int64 `json:"attempt_cost_minor,omitempty"`
	SellMinor       int64  `json:"sell_minor"`
	MarginMinor     int64  `json:"margin_minor"`
	PnLMinor        int64  `json:"pnl_minor"`
}

func (s *Service) ChannelPnL(ctx context.Context, channelOrgID string, marketingFrozen, marketingIssued, supplier int64, brandChannels ...[]string) (*ChannelPnLView, error) {
	if channelOrgID == "" {
		return nil, ErrNotFound
	}
	quota, err := s.ChannelQuota(ctx, channelOrgID)
	poolStatus := "available"
	if errors.Is(err, ErrNotFound) {
		poolStatus = "not_established"
		quota = &QuotaView{OwnerID: channelOrgID}
	} else if err != nil {
		return nil, err
	}
	unconsumed := quota.AvailableMinor
	issued := quota.IssuedMinor
	recharge := unconsumed + issued
	channels := []string{channelOrgID}
	if len(brandChannels) > 0 {
		channels = brandChannels[0]
	}
	report, _, err := s.BrandReport(ctx, channels)
	if err != nil {
		return nil, err
	}
	consumed := report.RevenueMinor
	marketing := marketingFrozen + marketingIssued
	// Attempt costs have their own request-level projection. They are not a
	// second supplier cash expense and must not be fabricated from wholesale.
	return &ChannelPnLView{
		ChannelOrgID: channelOrgID, PoolStatus: poolStatus, ModelCostMinor: report.CostMinor,
		RechargeMinor:   recharge,
		UnconsumedMinor: unconsumed,
		ConsumedMinor:   consumed,
		MarketingMinor:  marketing,
		MarketingFrozen: marketingFrozen,
		MarketingIssued: marketingIssued,
		SupplierMinor:   supplier,
		SellMinor:       report.RevenueMinor,
		MarginMinor:     report.MarginMinor,
		PnLMinor:        consumed + marketing + supplier,
	}, nil
}
