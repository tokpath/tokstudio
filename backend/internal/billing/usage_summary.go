package billing

import (
	"context"
	"time"
)

type UsageTotals struct {
	Requests         int64 `json:"requests"`
	Confirmed        int64 `json:"confirmed"`
	Pending          int64 `json:"pending"`
	Voided           int64 `json:"voided"`
	AmountMinor      int64 `json:"amount_minor"`
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	ReasoningTokens  int64 `json:"reasoning_tokens"`
}
type UsageGroup struct {
	Key string `json:"key"`
	UsageTotals
}
type UsageSummary struct {
	Totals      UsageTotals  `json:"totals"`
	Daily       []UsageGroup `json:"daily"`
	Keys        []UsageGroup `json:"keys"`
	Models      []UsageGroup `json:"models"`
	TimeZone    string       `json:"time_zone"`
	Since       time.Time    `json:"since"`
	Until       time.Time    `json:"until"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// The report and every dimension share the full authorized SQL scope. List
// limits and cursors never change a financial aggregate.
func (s *Service) UsageSummary(ctx context.Context, in QueryUsageInput, zone string) (*UsageSummary, error) {
	if _, err := time.LoadLocation(zone); err != nil {
		return nil, err
	}
	totals := `COUNT(DISTINCT request_id) AS requests,
 COUNT(DISTINCT request_id) FILTER (WHERE state = 'confirmed') AS confirmed,
 COUNT(DISTINCT request_id) FILTER (WHERE state = 'pending_reconciliation') AS pending,
 COUNT(DISTINCT request_id) FILTER (WHERE state = 'voided') AS voided,
 COALESCE(SUM(customer_amount_minor) FILTER (WHERE state = 'confirmed'),0) AS amount_minor,
 COALESCE(SUM(COALESCE((unit_usage_json->>'prompt_tokens')::bigint,0)) FILTER (WHERE state = 'confirmed'),0) AS prompt_tokens,
 COALESCE(SUM(COALESCE((unit_usage_json->>'completion_tokens')::bigint,0)) FILTER (WHERE state = 'confirmed'),0) AS completion_tokens,
 COALESCE(SUM(COALESCE((unit_usage_json->>'reasoning_tokens')::bigint,0)) FILTER (WHERE state = 'confirmed'),0) AS reasoning_tokens`
	in.Cursor = ""
	in.Limit = 0
	base := applyUsageFilters(s.db.WithContext(ctx).Model(&usageRow{}), in)
	if in.State != "" {
		base = base.Where("state = ?", in.State)
	}
	out := &UsageSummary{Daily: []UsageGroup{}, Keys: []UsageGroup{}, Models: []UsageGroup{}, TimeZone: zone, Since: in.Since, Until: in.Until, GeneratedAt: time.Now().UTC()}
	if err := base.Select(totals).Scan(&out.Totals).Error; err != nil {
		return nil, err
	}
	// New sessions are intentional: GORM query chains are mutable.
	for _, dim := range []struct {
		expr   string
		target *[]UsageGroup
		args   []any
	}{
		{"to_char(occurred_at AT TIME ZONE ?, 'YYYY-MM-DD')", &out.Daily, []any{zone}},
		{"COALESCE(api_key_id,'')", &out.Keys, nil}, {"public_model_id", &out.Models, nil},
	} {
		q := applyUsageFilters(s.db.WithContext(ctx).Model(&usageRow{}), in)
		if in.State != "" {
			q = q.Where("state = ?", in.State)
		}
		// The daily expression parameter is bound twice, once for SELECT and GROUP.
		if len(dim.args) > 0 {
			if err := q.Select(dim.expr+" AS key,"+totals, dim.args...).Group("key").Order("key").Scan(dim.target).Error; err != nil {
				return nil, err
			}
		} else if err := q.Select(dim.expr + " AS key," + totals).Group("key").Order("key").Scan(dim.target).Error; err != nil {
			return nil, err
		}
	}
	return out, nil
}

type RequestFacts struct {
	Authorization *Reservation `json:"authorization,omitempty"`
	Usages        []UsageView  `json:"usage"`
	Charges       []Settlement `json:"charges"`
}

func (s *Service) RequestFacts(ctx context.Context, requestID string) (*RequestFacts, error) {
	usages, err := s.QueryUsage(ctx, QueryUsageInput{RequestID: requestID, Unlimited: true})
	if err != nil {
		return nil, err
	}
	charges, err := s.ListChargesByRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := &RequestFacts{Usages: usages, Charges: charges}
	var auth authRow
	result := s.db.WithContext(ctx).Where("request_id = ?", requestID).Find(&auth)
	if result.Error != nil {
		return nil, result.Error
	}
	if auth.ID != "" {
		out.Authorization = &Reservation{ID: auth.ID, RequestID: auth.RequestID, AmountMinor: auth.AmountMinor, Status: auth.Status, Currency: CurrencyUSD}
	}
	return out, nil
}
