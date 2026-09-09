package billing

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ListPendingReconciliation 列出待对账工作队列。默认只看 pending；status=all 含已解（voided）。
func (s *Service) ListPendingReconciliation(ctx context.Context, in QueryUsageInput) ([]UsageGapView, error) {
	if in.Limit <= 0 {
		in.Limit = 50
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	q := s.db.WithContext(ctx).Model(&usageRow{}).Order("occurred_at DESC").Limit(in.Limit)
	q = applyUsageFilters(q, in)
	switch in.State {
	case "", UsagePending:
		q = q.Where("state = ?", UsagePending)
	case "all":
		q = q.Where("state IN ?", []string{UsagePending, UsageVoided})
	default:
		q = q.Where("state = ?", in.State)
	}
	var rows []usageRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.attachAuthGaps(ctx, rows)
}

// GetUsageGap 按 usage id 或 request_id 打开缺口详情。
func (s *Service) GetUsageGap(ctx context.Context, key string) (*UsageGapView, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	var row usageRow
	if err := s.db.WithContext(ctx).Where("id = ? OR request_id = ?", key, key).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	gaps, err := s.attachAuthGaps(ctx, []usageRow{row})
	if err != nil {
		return nil, err
	}
	if len(gaps) == 0 {
		return nil, ErrNotFound
	}
	return &gaps[0], nil
}

// ResolvePending 标记已解：作废 pending usage、释放预授权。已结算的账单拒绝，禁止估算扣款。
func (s *Service) ResolvePending(ctx context.Context, in ResolvePendingInput) (*ResolvePendingResult, error) {
	keys := uniqueNonEmpty(append(append([]string{}, in.IDs...), in.RequestIDs...))
	if len(keys) == 0 {
		return nil, ErrInvalidAmount
	}
	out := &ResolvePendingResult{Items: make([]UsageGapView, 0, len(keys))}
	for _, key := range keys {
		item, err := s.resolveOne(ctx, key)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, *item)
	}
	return out, nil
}

func (s *Service) resolveOne(ctx context.Context, key string) (*UsageGapView, error) {
	var out *UsageGapView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row usageRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? OR request_id = ?", key, key).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.State == UsageConfirmed {
			return ErrAlreadyCharged
		}
		if row.State == UsagePending {
			row.State = UsageVoided
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			if _, err := s.outbox.EnqueueTx(tx, "usage.reconciliation.resolved", "usage_event", row.ID, map[string]any{
				"request_id": row.RequestID,
			}); err != nil {
				return err
			}
		}
		var auth authRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", row.RequestID).First(&auth).Error; err == nil {
			if err := s.releaseAuthTx(ctx, tx, &auth); err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		out = gapFrom(row, auth)
		return nil
	})
	return out, err
}

func (s *Service) attachAuthGaps(ctx context.Context, rows []usageRow) ([]UsageGapView, error) {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.RequestID)
	}
	auths := map[string]authRow{}
	if len(ids) > 0 {
		var found []authRow
		if err := s.db.WithContext(ctx).Where("request_id IN ?", ids).Find(&found).Error; err != nil {
			return nil, err
		}
		for _, auth := range found {
			auths[auth.RequestID] = auth
		}
	}
	out := make([]UsageGapView, 0, len(rows))
	for _, row := range rows {
		out = append(out, *gapFrom(row, auths[row.RequestID]))
	}
	return out, nil
}

func applyUsageFilters(q *gorm.DB, in QueryUsageInput) *gorm.DB {
	if in.UserID != "" {
		q = q.Where("user_id = ?", in.UserID)
	}
	if in.APIKeyID != "" {
		q = q.Where("api_key_id = ?", in.APIKeyID)
	}
	if in.ChannelOrgID != "" {
		q = q.Where("channel_org_id = ?", in.ChannelOrgID)
	}
	if in.PublicModelID != "" {
		q = q.Where("public_model_id = ?", in.PublicModelID)
	}
	if in.RequestID != "" {
		q = q.Where("request_id = ?", in.RequestID)
	}
	if !in.Since.IsZero() {
		q = q.Where("occurred_at >= ?", in.Since.UTC())
	}
	if !in.Until.IsZero() {
		q = q.Where("occurred_at <= ?", in.Until.UTC())
	}
	return q
}

func gapFrom(row usageRow, auth authRow) *UsageGapView {
	views := usageViews([]usageRow{row})
	gap := UsageGapView{UsageView: views[0], MissingUsage: usageMissing(row.UnitUsage) || row.State == UsagePending}
	if auth.ID != "" {
		gap.AuthStatus = auth.Status
		gap.ReservedMinor = auth.AmountMinor
		if auth.Status == AuthReleased || auth.Status == AuthSettled || auth.Status == AuthReversed {
			gap.ReservedMinor = 0
		}
		gap.SettledMinor = auth.SettledMinor
	}
	return &gap
}

func usageMissing(raw []byte) bool {
	if len(raw) == 0 {
		return true
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	if missing, ok := payload["missing"].(bool); ok && missing {
		return true
	}
	return len(payload) == 0
}

func uniqueNonEmpty(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
