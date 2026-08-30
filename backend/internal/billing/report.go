package billing

import "context"

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
