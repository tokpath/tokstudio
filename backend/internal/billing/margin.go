package billing

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type marginCorrectionRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	Kind           string    `gorm:"column:kind"`
	RequestID      string    `gorm:"column:request_id"`
	AttemptID      *string   `gorm:"column:attempt_id"`
	Reason         *string   `gorm:"column:reason"`
	Status         string    `gorm:"column:status"`
	ActorUserID    string    `gorm:"column:actor_user_id"`
	IdempotencyKey string    `gorm:"column:idempotency_key"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (marginCorrectionRow) TableName() string { return "billing_margin_corrections" }

// AllowedSupplierSource 线下付款来源。禁止把 attempt 成本伪装成供应商流水。
func AllowedSupplierSource(sourceType string) bool {
	switch strings.TrimSpace(sourceType) {
	case SupplierInvoice, SupplierRecharge, SupplierOther:
		return true
	default:
		return false
	}
}

func InventedSupplierSource(sourceType string) bool {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case "estimate", "estimated", "attempt_cost", "usage_cost", "guessed", "invented",
		"fill_cost", "adjust_margin", "estimated_cost", "blind_cost":
		return true
	default:
		return false
	}
}

func priceTiersFromRaw(raw []byte) PriceTierSnapshot {
	dims := map[string]string{}
	if len(raw) > 0 {
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			str := func(keys ...string) string {
				for _, key := range keys {
					if v, ok := m[key]; ok {
						s := strings.TrimSpace(strings.Trim(strings.ReplaceAll(stringifyAny(v), `"`, ""), " "))
						if s != "" && s != "<nil>" {
							return s
						}
					}
				}
				return ""
			}
			dims["upstream_in"] = str("upstream_cost_input")
			dims["upstream_out"] = str("upstream_cost_output")
			dims["wholesale_in"] = str("wholesale_input")
			dims["wholesale_out"] = str("wholesale_output")
			dims["sell_in"] = str("customer_sell_input", "input")
			dims["sell_out"] = str("customer_sell_output", "output")
			dims["channel_in"] = str("channel_customer_input")
			dims["channel_out"] = str("channel_customer_output")
		}
	}
	return PriceTierSnapshot{
		Upstream:  formatTierIO(dims["upstream_in"], dims["upstream_out"]),
		Wholesale: formatTierIO(dims["wholesale_in"], dims["wholesale_out"]),
		Sell:      formatTierIO(dims["sell_in"], dims["sell_out"]),
		Channel:   formatTierIO(dims["channel_in"], dims["channel_out"]),
	}
}

func stringifyAny(v any) string {
	switch t := v.(type) {
	case string:
		return t
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func formatTierIO(in, out string) string {
	in = strings.TrimSpace(in)
	out = strings.TrimSpace(out)
	if in == "" && out == "" {
		return ""
	}
	if out == "" {
		return in
	}
	if in == "" {
		return out
	}
	return in + "/" + out
}

func marginOf(sell, cost int64) int64 { return sell - cost }

// AssembleMargin 把 TokenHub usage + cost_entries 装配成毛利。
// 合计走 SQL（不受列表 limit 影响）；明细只含已确认且有 attempt 事实的行。
// 缺 attempt 成本不估算，只计入 pending。供应商现金流水不进入成本合计。
func (s *Service) AssembleMargin(ctx context.Context, in QueryUsageInput) (*MarginView, error) {
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	totals, err := s.marginTotals(ctx, in)
	if err != nil {
		return nil, err
	}
	items, err := s.listAttemptCosts(ctx, in)
	if err != nil {
		return nil, err
	}
	return &MarginView{
		CostMinor:    totals.CostMinor,
		SellMinor:    totals.SellMinor,
		MarginMinor:  totals.MarginMinor,
		PendingCount: totals.PendingCount,
		Items:        items,
	}, nil
}

type marginTotals struct {
	CostMinor    int64
	SellMinor    int64
	MarginMinor  int64
	PendingCount int64
}

func usageScopeSQL(in QueryUsageInput, alias string) (string, []any) {
	col := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	parts := []string{}
	args := []any{}
	if in.ChannelOrgID != "" {
		parts = append(parts, col("channel_org_id")+" = ?")
		args = append(args, in.ChannelOrgID)
	}
	if in.RequestID != "" {
		parts = append(parts, col("request_id")+" = ?")
		args = append(args, in.RequestID)
	}
	if in.PublicModelID != "" {
		parts = append(parts, col("public_model_id")+" = ?")
		args = append(args, in.PublicModelID)
	}
	if in.UserID != "" {
		parts = append(parts, col("user_id")+" = ?")
		args = append(args, in.UserID)
	}
	if in.APIKeyID != "" {
		parts = append(parts, col("api_key_id")+" = ?")
		args = append(args, in.APIKeyID)
	}
	if !in.Since.IsZero() {
		parts = append(parts, col("occurred_at")+" >= ?")
		args = append(args, in.Since.UTC())
	}
	if !in.Until.IsZero() {
		parts = append(parts, col("occurred_at")+" <= ?")
		args = append(args, in.Until.UTC())
	}
	if len(parts) == 0 {
		return "", args
	}
	return " AND " + strings.Join(parts, " AND "), args
}

func (s *Service) marginTotals(ctx context.Context, in QueryUsageInput) (marginTotals, error) {
	var out marginTotals
	scope, scopeArgs := usageScopeSQL(in, "")
	uScope, uScopeArgs := usageScopeSQL(in, "u")
	sellQ := `SELECT COALESCE(SUM(customer_amount_minor), 0) FROM billing_usage_events WHERE state = ?` + scope
	costQ := `
		SELECT COALESCE(SUM(c.amount_minor), 0)
		FROM billing_cost_entries c
		WHERE EXISTS (
			SELECT 1 FROM billing_usage_events u
			WHERE u.request_id = c.request_id AND u.state = ?` + uScope + `
		)`
	orphanCostQ := `
		SELECT COALESCE(SUM(u.upstream_cost_minor), 0)
		FROM billing_usage_events u
		WHERE u.state = ?
		  AND u.attempt_id IS NOT NULL AND u.attempt_id <> ''
		  AND NOT EXISTS (
			SELECT 1 FROM billing_cost_entries c WHERE c.attempt_id = u.attempt_id
		  )` + uScope
	pendingQ := `SELECT COUNT(*) FROM billing_usage_events WHERE state = ?` + scope
	confirmed := append([]any{UsageConfirmed}, scopeArgs...)
	confirmedU := append([]any{UsageConfirmed}, uScopeArgs...)
	if err := s.db.WithContext(ctx).Raw(sellQ, confirmed...).Scan(&out.SellMinor).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Raw(costQ, confirmedU...).Scan(&out.CostMinor).Error; err != nil {
		return out, err
	}
	var orphan int64
	if err := s.db.WithContext(ctx).Raw(orphanCostQ, confirmedU...).Scan(&orphan).Error; err != nil {
		return out, err
	}
	out.CostMinor += orphan
	pendingArgs := append([]any{UsagePending}, scopeArgs...)
	if err := s.db.WithContext(ctx).Raw(pendingQ, pendingArgs...).Scan(&out.PendingCount).Error; err != nil {
		return out, err
	}
	out.MarginMinor = marginOf(out.SellMinor, out.CostMinor)
	return out, nil
}

func (s *Service) listAttemptCosts(ctx context.Context, in QueryUsageInput) ([]AttemptCostView, error) {
	q := s.db.WithContext(ctx).Model(&usageRow{}).
		Where("state = ?", UsageConfirmed).
		Where("attempt_id IS NOT NULL AND attempt_id <> ''").
		Order("occurred_at DESC").Limit(in.Limit)
	q = applyUsageFilters(q, QueryUsageInput{
		UserID: in.UserID, APIKeyID: in.APIKeyID, ChannelOrgID: in.ChannelOrgID,
		PublicModelID: in.PublicModelID, RequestID: in.RequestID, Since: in.Since, Until: in.Until,
	})
	var usages []usageRow
	if err := q.Find(&usages).Error; err != nil {
		return nil, err
	}
	if len(usages) == 0 {
		return []AttemptCostView{}, nil
	}
	attemptIDs := make([]string, 0, len(usages))
	seen := map[string]struct{}{}
	for _, row := range usages {
		if row.AttemptID == nil || *row.AttemptID == "" {
			continue
		}
		if _, ok := seen[*row.AttemptID]; ok {
			continue
		}
		seen[*row.AttemptID] = struct{}{}
		attemptIDs = append(attemptIDs, *row.AttemptID)
	}
	costByAttempt := map[string]costRow{}
	if len(attemptIDs) > 0 {
		var costs []costRow
		if err := s.db.WithContext(ctx).Where("attempt_id IN ?", attemptIDs).Find(&costs).Error; err != nil {
			return nil, err
		}
		for _, row := range costs {
			costByAttempt[row.AttemptID] = row
		}
	}
	out := make([]AttemptCostView, 0, len(usages))
	listed := map[string]struct{}{}
	for _, row := range usages {
		if row.AttemptID == nil || *row.AttemptID == "" {
			continue
		}
		if _, ok := listed[*row.AttemptID]; ok {
			continue
		}
		listed[*row.AttemptID] = struct{}{}
		costMinor := row.UpstreamCostMinor
		if cost, ok := costByAttempt[*row.AttemptID]; ok {
			costMinor = cost.AmountMinor
		}
		view := usageViews([]usageRow{row})[0]
		out = append(out, AttemptCostView{
			AttemptID: *row.AttemptID, RequestID: row.RequestID, UsageEventID: row.ID,
			CostSource: CostSourceTokenHub, CostMinor: costMinor, SellMinor: row.CustomerAmountMinor,
			MarginMinor: marginOf(row.CustomerAmountMinor, costMinor), WholesaleMinor: row.WholesaleAmountMinor,
			Prices: priceTiersFromRaw(row.UnitPrices), UnitPrices: row.UnitPrices,
			State: row.State, PublicModelID: row.PublicModelID, ProviderID: view.ProviderID,
			UpstreamModelID: view.UpstreamModelID, FactSource: view.FactSource,
			ChannelOrgID: view.ChannelOrgID, UserID: row.UserID, OccurredAt: row.OccurredAt,
		})
	}
	return out, nil
}

// CostFact 是 TokenHub 已入账的 attempt 成本，只读。
type CostFact struct {
	ID          string
	RequestID   string
	AttemptID   string
	ProviderID  string
	AmountMinor int64
}

func costFacts(rows []costRow) []CostFact {
	out := make([]CostFact, 0, len(rows))
	for _, row := range rows {
		out = append(out, CostFact{
			ID: row.ID, RequestID: row.RequestID, AttemptID: row.AttemptID,
			ProviderID: row.ProviderID, AmountMinor: row.AmountMinor,
		})
	}
	return out
}

// ListCostFactsByAttempt 只读 attempt 成本事实，供 Sentinel 断言不双写。
func (s *Service) ListCostFactsByAttempt(ctx context.Context, attemptID string) ([]CostFact, error) {
	if attemptID == "" {
		return nil, nil
	}
	var rows []costRow
	if err := s.db.WithContext(ctx).Where("attempt_id = ?", attemptID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return costFacts(rows), nil
}

func (s *Service) ListCostFactsByRequest(ctx context.Context, requestID string) ([]CostFact, error) {
	if requestID == "" {
		return nil, nil
	}
	var rows []costRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).Find(&rows).Error; err != nil {
		return nil, err
	}
	return costFacts(rows), nil
}

// FileMarginCorrection 记补成本 / 调毛利工单。禁止带金额估算；缺确认由 HTTP 层 409。
func (s *Service) FileMarginCorrection(ctx context.Context, actorUserID string, in MarginCorrectionInput) (*MarginCorrectionView, error) {
	in.Kind = strings.TrimSpace(in.Kind)
	in.RequestID = strings.TrimSpace(in.RequestID)
	in.AttemptID = strings.TrimSpace(in.AttemptID)
	in.Reason = strings.TrimSpace(in.Reason)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if actorUserID == "" || in.RequestID == "" || in.IdempotencyKey == "" {
		return nil, ErrInvalidAmount
	}
	if in.Kind != CorrectionFillCost && in.Kind != CorrectionAdjustMargin {
		return nil, ErrInvalidAmount
	}
	if in.AmountMinor != 0 {
		return nil, ErrInventedCost
	}
	var out *MarginCorrectionView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing marginCorrectionRow
		if err := tx.Where("idempotency_key = ?", in.IdempotencyKey).First(&existing).Error; err == nil {
			out = correctionView(existing)
			return nil
		}
		if in.Kind == CorrectionFillCost {
			if err := s.ensureMissingCostPending(tx, in.RequestID); err != nil {
				return err
			}
			var costs []costRow
			if err := tx.Where("request_id = ?", in.RequestID).Find(&costs).Error; err != nil {
				return err
			}
			if len(costs) > 0 && in.AmountMinor != 0 {
				return ErrInventedCost
			}
		}
		row := marginCorrectionRow{
			ID: id.New("mcr"), Kind: in.Kind, RequestID: in.RequestID,
			Status: CorrectionOpen, ActorUserID: actorUserID,
			IdempotencyKey: in.IdempotencyKey, CreatedAt: time.Now().UTC(),
		}
		row.AttemptID = optStr(in.AttemptID)
		row.Reason = optStr(in.Reason)
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		out = correctionView(row)
		return nil
	})
	return out, err
}

func (s *Service) ensureMissingCostPending(tx *gorm.DB, requestID string) error {
	var usage usageRow
	if err := tx.Where("request_id = ?", requestID).Order("occurred_at DESC").First(&usage).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}
	if usage.State == UsagePending {
		return nil
	}
	if usage.State == UsageConfirmed && usage.AttemptID != nil && *usage.AttemptID != "" {
		var cost costRow
		if err := tx.Where("attempt_id = ?", *usage.AttemptID).First(&cost).Error; err == nil {
			return nil
		}
	}
	if usage.State == UsageConfirmed {
		return nil
	}
	usage.State = UsagePending
	return tx.Save(&usage).Error
}

func correctionView(row marginCorrectionRow) *MarginCorrectionView {
	v := &MarginCorrectionView{
		ID: row.ID, Kind: row.Kind, RequestID: row.RequestID, Status: row.Status,
		ActorUserID: row.ActorUserID, IdempotencyKey: row.IdempotencyKey, CreatedAt: row.CreatedAt,
	}
	if row.AttemptID != nil {
		v.AttemptID = *row.AttemptID
	}
	if row.Reason != nil {
		v.Reason = *row.Reason
	}
	return v
}
