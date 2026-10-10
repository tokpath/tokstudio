package billing

import (
	"context"
	"errors"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

type ChannelPnLView struct {
	ChannelOrgID     string `json:"channel_org_id"`
	PoolStatus       string `json:"pool_status"`
	ModelCostMinor   int64  `json:"model_cost_minor"`
	RechargeMinor    int64  `json:"recharge_minor"`
	UnconsumedMinor  int64  `json:"unconsumed_minor"`
	ConsumedMinor    int64  `json:"consumed_minor"`
	MarketingMinor   int64  `json:"marketing_minor"`
	MarketingFrozen  int64  `json:"marketing_frozen_minor"`
	MarketingIssued  int64  `json:"marketing_issued_minor"`
	SupplierMinor    int64  `json:"supplier_minor"`
	AttemptCost      *int64 `json:"attempt_cost_minor,omitempty"`
	SellMinor        int64  `json:"sell_minor"`
	MarginMinor      int64  `json:"margin_minor"`
	PnLMinor         int64  `json:"pnl_minor"`
	BusinessType     string `json:"business_type"`
	OEMSalesMinor    int64  `json:"oem_sales_minor"`
	SelfRevenueMinor int64  `json:"self_revenue_minor"`
	PurchaseMinor    int64  `json:"purchase_minor"`
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
	revenue, cost, margin := report.RevenueMinor, report.CostMinor, report.MarginMinor
	businessType := "oem"
	var oemSales, selfRevenue, purchases int64
	if channelOrgID == identity.OfficialChannelID {
		platform, err := s.PlatformReport(ctx)
		if err != nil {
			return nil, err
		}
		revenue, cost, margin = platform.RevenueMinor, platform.UpstreamMinor, platform.GrossProfitMinor
		oemSales, selfRevenue = platform.OEMSalesMinor, platform.SelfRevenueMinor
		businessType = "platform"
	} else if err := s.db.WithContext(ctx).Model(&oemPurchaseRow{}).Select("COALESCE(SUM(sale_amount_minor),0)").Where("oem_channel_org_id=? AND status='completed'", channelOrgID).Scan(&purchases).Error; err != nil {
		return nil, err
	}
	marketing := marketingFrozen + marketingIssued
	// Attempt costs have their own request-level projection. They are not a
	// second supplier cash expense and must not be fabricated from wholesale.
	return &ChannelPnLView{
		ChannelOrgID: channelOrgID, PoolStatus: poolStatus, ModelCostMinor: cost,
		RechargeMinor:   recharge,
		UnconsumedMinor: unconsumed,
		ConsumedMinor:   consumed,
		MarketingMinor:  marketing,
		MarketingFrozen: marketingFrozen,
		MarketingIssued: marketingIssued,
		SupplierMinor:   supplier,
		SellMinor:       revenue,
		MarginMinor:     margin,
		PnLMinor:        margin + marketing,
		BusinessType:    businessType, OEMSalesMinor: oemSales, SelfRevenueMinor: selfRevenue, PurchaseMinor: purchases,
	}, nil
}
