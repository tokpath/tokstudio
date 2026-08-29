package billing

import (
	"context"
	"time"
)

type DailyMoney struct {
	Day          string
	UsageMinor   int64
	RevenueMinor int64
	CostMinor    int64
}

func (s *Service) Report(ctx context.Context) (*ReportView, error) {
	type sumRow struct {
		Revenue    int64
		Upstream   int64
		Wholesale  int64
		Commission int64
		Refund     int64
		Pending    int64
	}
	var sums sumRow
	if err := s.db.WithContext(ctx).Raw(`
		SELECT
			COALESCE((SELECT SUM(amount_minor) FROM billing_customer_charges WHERE status = 'committed'), 0) AS revenue,
			COALESCE((SELECT SUM(amount_minor) FROM billing_cost_entries), 0) AS upstream,
			COALESCE((SELECT SUM(wholesale_amount_minor) FROM billing_usage_events WHERE state = 'confirmed'), 0) AS wholesale,
			COALESCE((SELECT SUM(amount_minor) FROM billing_commission_ledger WHERE status = 'frozen'), 0) AS commission,
			COALESCE((SELECT SUM(amount_minor) FROM billing_customer_charges WHERE status = 'reversed'), 0) AS refund,
			COALESCE((SELECT COUNT(*) FROM billing_authorizations WHERE status = 'pending_reconciliation'), 0) AS pending
	`).Scan(&sums).Error; err != nil {
		return nil, err
	}
	return &ReportView{
		RevenueMinor:     sums.Revenue,
		UpstreamMinor:    sums.Upstream,
		WholesaleMinor:   sums.Wholesale,
		CommissionMinor:  sums.Commission,
		RefundMinor:      sums.Refund,
		GrossProfitMinor: sums.Revenue - sums.Upstream - sums.Commission,
		PendingCount:     sums.Pending,
	}, nil
}

func (s *Service) DimMoney(ctx context.Context, dimension string) ([]DimMoneyView, error) {
	col := ""
	switch dimension {
	case "provider":
		col = "COALESCE(provider_id, '')"
	case "model":
		col = "public_model_id"
	case "channel":
		col = "COALESCE(channel_org_id, '')"
	case "user":
		col = "user_id"
	case "api_key":
		col = "COALESCE(api_key_id, '')"
	default:
		return nil, nil
	}
	type row struct {
		Key     string
		Usage   int64
		Revenue int64
		Cost    int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT ` + col + ` AS key,
			COALESCE(SUM(wholesale_amount_minor),0) AS usage,
			COALESCE(SUM(customer_amount_minor),0) AS revenue,
			COALESCE(SUM(upstream_cost_minor),0) AS cost
		FROM billing_usage_events
		WHERE state = 'confirmed'
		GROUP BY ` + col + `
	`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DimMoneyView, 0, len(rows))
	for _, item := range rows {
		out = append(out, DimMoneyView{Dimension: dimension, Key: item.Key, UsageMinor: item.Usage, RevenueMinor: item.Revenue, CostMinor: item.Cost})
	}
	return out, nil
}

func (s *Service) DailySeries(ctx context.Context, since time.Time) ([]DailyMoney, error) {
	type row struct {
		Day     string
		Usage   int64
		Revenue int64
		Cost    int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT to_char((occurred_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS day,
			COALESCE(SUM(wholesale_amount_minor),0) AS usage,
			COALESCE(SUM(customer_amount_minor),0) AS revenue,
			COALESCE(SUM(upstream_cost_minor),0) AS cost
		FROM billing_usage_events
		WHERE state = 'confirmed' AND occurred_at >= ?
		GROUP BY 1
		ORDER BY 1
	`, since.UTC()).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DailyMoney, 0, len(rows))
	for _, item := range rows {
		out = append(out, DailyMoney{Day: item.Day, UsageMinor: item.Usage, RevenueMinor: item.Revenue, CostMinor: item.Cost})
	}
	return out, nil
}

func (s *Service) Risk(ctx context.Context) (*RiskView, error) {
	view := &RiskView{}
	_ = s.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM billing_wallets WHERE available_minor < ?`, MinorPerUSD).Scan(&view.LowBalanceWallets).Error
	_ = s.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(reserved_minor),0) FROM billing_wallets`).Scan(&view.ReservedMinor).Error
	_ = s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(wholesale_amount_minor),0) FROM billing_usage_events
		WHERE state = 'confirmed' AND channel_org_id IS NOT NULL AND channel_org_id <> ''
	`).Scan(&view.ChannelSpendMinor).Error
	_ = s.db.WithContext(ctx).Raw(`SELECT COUNT(*) FROM billing_preauth_failures`).Scan(&view.PreauthFailed).Error
	return view, nil
}

func (s *Service) ChargeByRequest(ctx context.Context, requestID string) (*Settlement, error) {
	var charge chargeRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&charge).Error; err != nil {
		return nil, ErrNotFound
	}
	return &Settlement{
		ChargeID: charge.ID, UsageEventID: charge.UsageEventID,
		AmountMinor: charge.AmountMinor, State: charge.Status, Currency: CurrencyUSD,
	}, nil
}
