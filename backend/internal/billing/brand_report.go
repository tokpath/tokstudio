package billing

import "context"

// BrandReport describes the OEM's customer business. Its cost is the platform
// wholesale charge, never the platform's supplier invoice or attempt cost.
// Only an original committed/reversed charge proves settled consumption. Voided
// measurements that were superseded during reconciliation are not another cost.
type BrandReport struct {
	Requests     int64 `json:"requests"`
	RevenueMinor int64 `json:"revenue_minor"`
	CostMinor    int64 `json:"cost_minor"`
	MarginMinor  int64 `json:"margin_minor"`
	Pending      int64 `json:"pending"`
}

type BrandModelReport struct {
	BrandReport
	Model string `json:"model"`
}

func (s *Service) BrandReport(ctx context.Context, channelIDs []string) (*BrandReport, []BrandModelReport, error) {
	items := make([]BrandModelReport, 0)
	total := &BrandReport{}
	if len(channelIDs) == 0 {
		return total, items, nil
	}
	err := s.db.WithContext(ctx).Table("billing_usage_events AS u").
		Select(`public_model_id AS model,
		COUNT(*) FILTER (WHERE state = 'confirmed') AS requests,
		COALESCE(SUM(customer_amount_minor) FILTER (WHERE state = 'confirmed'), 0) AS revenue_minor,
		COALESCE(SUM(wholesale_amount_minor) FILTER (WHERE state IN ('confirmed','voided') AND EXISTS (SELECT 1 FROM billing_customer_charges c WHERE c.usage_event_id=u.id AND c.status IN ('committed','reversed'))), 0) AS cost_minor,
		COUNT(*) FILTER (WHERE state = 'pending_reconciliation') AS pending`).
		Where("channel_org_id IN ?", channelIDs).Group("public_model_id").Order("public_model_id").Scan(&items).Error
	if err != nil {
		return nil, nil, err
	}
	for i := range items {
		items[i].MarginMinor = items[i].RevenueMinor - items[i].CostMinor
		total.Requests += items[i].Requests
		total.RevenueMinor += items[i].RevenueMinor
		total.CostMinor += items[i].CostMinor
		total.Pending += items[i].Pending
	}
	total.MarginMinor = total.RevenueMinor - total.CostMinor
	return total, items, nil
}

func (s *Service) BrandPendingCounts(ctx context.Context, channelIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64)
	if len(channelIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		ChannelOrgID string
		Count        int64
	}
	err := s.db.WithContext(ctx).Table("billing_usage_events").Select("channel_org_id, COUNT(*) AS count").Where("channel_org_id IN ? AND state = ?", channelIDs, UsagePending).Group("channel_org_id").Scan(&rows).Error
	for _, row := range rows {
		counts[row.ChannelOrgID] = row.Count
	}
	return counts, err
}
