package commission

import "context"

// BusinessTotals is scoped to the original brand business, including its
// channels. A payout changes liability, not the already recognized expense.
func (s *Service) BusinessTotals(ctx context.Context, channels []string) (liability, expense, marketing int64, err error) {
	if len(channels) == 0 {
		return
	}
	var totals struct{ Liability, Expense int64 }
	err = s.db.WithContext(ctx).Model(&entryRow{}).Select(`COALESCE(SUM(CASE WHEN status IN ('frozen','available','held','settled') THEN amount_minor ELSE 0 END),0) AS liability,COALESCE(SUM(CASE WHEN status <> 'reversed' THEN amount_minor ELSE 0 END),0) AS expense`).Where("channel_org_id IN ?", channels).Scan(&totals).Error
	if err != nil {
		return
	}
	liability, expense = totals.Liability, totals.Expense
	err = s.db.WithContext(ctx).Model(&marketingRow{}).Select("COALESCE(SUM(amount_minor),0)").Where("channel_org_id IN ? AND status IN ?", channels, []string{MarketingFrozen, MarketingIssued}).Scan(&marketing).Error
	return
}
