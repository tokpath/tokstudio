package billing

import "context"

type ChannelPnLView struct {
	ChannelOrgID    string `json:"channel_org_id"`
	RechargeMinor   int64  `json:"recharge_minor"`
	UnconsumedMinor int64  `json:"unconsumed_minor"`
	ConsumedMinor   int64  `json:"consumed_minor"`
	MarketingMinor  int64  `json:"marketing_minor"`
	MarketingFrozen int64  `json:"marketing_frozen_minor"`
	MarketingIssued int64  `json:"marketing_issued_minor"`
	SupplierMinor   int64  `json:"supplier_minor"`
	AttemptCost     int64  `json:"attempt_cost_minor"`
	SellMinor       int64  `json:"sell_minor"`
	MarginMinor     int64  `json:"margin_minor"`
	PnLMinor        int64  `json:"pnl_minor"`
}

func (s *Service) ChannelPnL(ctx context.Context, channelOrgID string, marketingFrozen, marketingIssued, supplier int64) (*ChannelPnLView, error) {
	if channelOrgID == "" {
		return nil, ErrNotFound
	}
	quota, err := s.ChannelQuota(ctx, channelOrgID)
	if err != nil {
		quota = &QuotaView{OwnerID: channelOrgID}
	}
	unconsumed := quota.AvailableMinor
	issued := quota.IssuedMinor
	recharge := unconsumed + issued
	consumed := recharge - unconsumed
	if consumed < 0 {
		consumed = 0
	}
	marketing := marketingFrozen + marketingIssued
	margin, err := s.AssembleMargin(ctx, QueryUsageInput{ChannelOrgID: channelOrgID, Limit: 1})
	if err != nil {
		margin = &MarginView{}
	}
	return &ChannelPnLView{
		ChannelOrgID:    channelOrgID,
		RechargeMinor:   recharge,
		UnconsumedMinor: unconsumed,
		ConsumedMinor:   consumed,
		MarketingMinor:  marketing,
		MarketingFrozen: marketingFrozen,
		MarketingIssued: marketingIssued,
		SupplierMinor:   supplier,
		AttemptCost:     margin.CostMinor,
		SellMinor:       margin.SellMinor,
		MarginMinor:     margin.MarginMinor,
		PnLMinor:        consumed + marketing + supplier,
	}, nil
}
