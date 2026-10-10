package billing

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
)

type PlatformBusinessReport struct {
	ReportView
	SelfRevenueMinor     int64  `json:"self_revenue_minor"`
	OEMSalesMinor        int64  `json:"oem_sales_minor"`
	MarketingMinor       int64  `json:"marketing_minor"`
	OperatingProfitMinor int64  `json:"operating_profit_minor"`
	Scope                string `json:"scope"`
}
type businessCommissionSource interface {
	BusinessTotals(context.Context, []string) (int64, int64, int64, error)
}

func (s *Service) PlatformReport(ctx context.Context) (*PlatformBusinessReport, error) {
	ids, err := identity.New(s.db).BrandChannelIDs(ctx, identity.OfficialChannelID)
	if err != nil {
		return nil, err
	}
	self, _, err := s.BrandReport(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := &PlatformBusinessReport{SelfRevenueMinor: self.RevenueMinor, Scope: "platform_business"}
	if err := s.db.WithContext(ctx).Model(&oemPurchaseRow{}).Where("status='completed'").Select("COALESCE(SUM(sale_amount_minor),0)").Scan(&out.OEMSalesMinor).Error; err != nil {
		return nil, err
	}
	// Customer reversals do not imply that the supplier refunded its actual
	// service cost. Retain all original cost entries, including failed attempts.
	if err := s.db.WithContext(ctx).Model(&costRow{}).Select("COALESCE(SUM(amount_minor),0)").Scan(&out.UpstreamMinor).Error; err != nil {
		return nil, err
	}
	if source, ok := s.commissioner.(businessCommissionSource); ok {
		out.CommissionMinor, out.CommissionExpenseMinor, out.MarketingMinor, err = source.BusinessTotals(ctx, ids)
		if err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Model(&authRow{}).Where("status=?", UsagePending).Count(&out.PendingCount).Error; err != nil {
		return nil, err
	}
	out.RevenueMinor = out.SelfRevenueMinor + out.OEMSalesMinor
	out.GrossProfitMinor = out.RevenueMinor - out.UpstreamMinor
	out.OperatingProfitMinor = out.GrossProfitMinor + out.MarketingMinor
	return out, nil
}

func (s *Service) PlatformDailySeries(ctx context.Context, since time.Time) ([]DailyMoney, error) {
	ids, err := identity.New(s.db).BrandChannelIDs(ctx, identity.OfficialChannelID)
	if err != nil {
		return nil, err
	}
	var rows []DailyMoney
	err = s.db.WithContext(ctx).Raw(`SELECT day,SUM(revenue) AS revenue_minor,SUM(cost) AS cost_minor FROM (
  SELECT to_char((occurred_at AT TIME ZONE 'UTC')::date,'YYYY-MM-DD') AS day,customer_amount_minor AS revenue,0 AS cost FROM billing_usage_events WHERE state='confirmed' AND channel_org_id IN ? AND occurred_at>=?
  UNION ALL SELECT to_char((completed_at AT TIME ZONE 'UTC')::date,'YYYY-MM-DD'),sale_amount_minor,0 FROM billing_oem_purchases WHERE completed_at>=?
  UNION ALL SELECT to_char((reversed_at AT TIME ZONE 'UTC')::date,'YYYY-MM-DD'),-sale_amount_minor,0 FROM billing_oem_purchases WHERE reversed_at>=?
  UNION ALL SELECT to_char((created_at AT TIME ZONE 'UTC')::date,'YYYY-MM-DD'),0,amount_minor FROM billing_cost_entries WHERE created_at>=?
 ) facts GROUP BY day ORDER BY day`, ids, since.UTC(), since.UTC(), since.UTC(), since.UTC()).Scan(&rows).Error
	return rows, err
}

// Platform dimensions never sum OEM terminal selling prices. Unallocated OEM
// sales are presented as their own category rather than fabricated model sales.
func (s *Service) PlatformDimMoney(ctx context.Context, dimension string) ([]DimMoneyView, error) {
	column := map[string]string{"provider": "COALESCE(u.provider_id,'')", "model": "u.public_model_id", "channel": "COALESCE(u.channel_org_id,'')", "user": "u.user_id", "api_key": "COALESCE(u.api_key_id,'')"}[dimension]
	if column == "" {
		return []DimMoneyView{}, nil
	}
	ids, err := identity.New(s.db).BrandChannelIDs(ctx, identity.OfficialChannelID)
	if err != nil {
		return nil, err
	}
	costColumn := column
	if dimension == "provider" {
		costColumn = "c.provider_id"
	}
	var out []DimMoneyView
	err = s.db.WithContext(ctx).Raw(`SELECT key,SUM(revenue) AS revenue_minor,SUM(cost) AS cost_minor,SUM(requests) AS requests FROM (
  SELECT `+column+` AS key,CASE WHEN u.state='confirmed' AND u.channel_org_id IN ? THEN u.customer_amount_minor ELSE 0 END AS revenue,0 AS cost,CASE WHEN u.state='confirmed' THEN 1 ELSE 0 END AS requests FROM billing_usage_events u
  UNION ALL SELECT `+costColumn+`,0,c.amount_minor,0 FROM billing_cost_entries c LEFT JOIN (SELECT DISTINCT ON(request_id) * FROM billing_usage_events ORDER BY request_id,occurred_at,id) u ON u.request_id=c.request_id
  UNION ALL SELECT `+map[bool]string{true: "oem_channel_org_id", false: "'oem_service_sales'"}[dimension == "channel"]+`,sale_amount_minor,0,0 FROM billing_oem_purchases WHERE status='completed'
 ) facts GROUP BY key ORDER BY key`, ids).Scan(&out).Error
	for i := range out {
		out[i].Dimension = dimension
	}
	return out, err
}
