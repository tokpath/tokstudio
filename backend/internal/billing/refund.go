package billing

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

// RefundCharge 对已确认客户账单做反向流水，并冲正挂接的佣金。不改历史金额。
func (s *Service) RefundCharge(ctx context.Context, requestID string) (*Settlement, error) {
	var out *Settlement
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var charge chargeRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("request_id = ?", requestID).First(&charge).Error; err != nil {
			return ErrNotFound
		}
		if charge.Status == ChargeReversed {
			out = &Settlement{ChargeID: charge.ID, UsageEventID: charge.UsageEventID, AmountMinor: charge.AmountMinor, State: ChargeReversed, Currency: CurrencyUSD}
			return nil
		}
		var auth authRow
		_ = tx.Where("request_id = ?", requestID).First(&auth)
		userID := auth.UserID
		if userID == "" {
			var usage usageRow
			if err := tx.Where("id = ?", charge.UsageEventID).First(&usage).Error; err == nil {
				userID = usage.UserID
			}
		}
		if userID == "" {
			return ErrNotFound
		}
		walletCredit := charge.AmountMinor
		if auth.ID != "" {
			keep := entitlementKeep(auth, charge.AmountMinor)
			if walletCredit > keep {
				walletCredit = charge.AmountMinor - keep
			} else {
				walletCredit = 0
			}
		}
		if walletCredit > 0 {
			if err := creditWallet(tx, userID, walletCredit, EventRefund, "customer_charge", charge.ID, "refund:"+requestID); err != nil {
				return err
			}
		}
		if err := s.reverseCommission(tx, charge.UsageEventID); err != nil {
			return err
		}
		if err := reverseAllocationConsumes(tx, requestID); err != nil {
			return err
		}
		var usage usageRow
		if err := tx.Where("id = ?", charge.UsageEventID).First(&usage).Error; err == nil {
			usage.State = UsageVoided
			_ = tx.Save(&usage).Error
		}
		charge.Status = ChargeReversed
		if err := tx.Save(&charge).Error; err != nil {
			return err
		}
		if auth.ID != "" {
			auth.Status = AuthReversed
			auth.UpdatedAt = time.Now().UTC()
			_ = tx.Save(&auth).Error
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.charge.reversed", "customer_charge", charge.ID, map[string]any{
			"request_id": requestID, "amount_minor": charge.AmountMinor,
		}); err != nil {
			return err
		}
		out = &Settlement{ChargeID: charge.ID, UsageEventID: charge.UsageEventID, AmountMinor: charge.AmountMinor, State: ChargeReversed, Currency: CurrencyUSD}
		return nil
	})
	if err == nil && s.coverer != nil {
		_ = s.coverer.ReverseByRequest(ctx, requestID)
	}
	return out, err
}

func (s *Service) accrueCommission(tx *gorm.DB, usage usageRow) error {
	var frozen commissionRow
	if err := tx.Where("usage_event_id = ? AND status = ?", usage.ID, CommissionFrozen).First(&frozen).Error; err == nil {
		return nil
	}
	amount := usage.WholesaleAmountMinor * int64(CommissionRateBPS) / 10000
	key := "commission:" + usage.ID
	var existing commissionRow
	if err := tx.Where("idempotency_key = ?", key).First(&existing).Error; err == nil {
		key = "commission:" + usage.ID + ":" + id.New("gen")
	}
	row := commissionRow{
		ID: id.New("com"), UsageEventID: usage.ID, PolicyVersion: CommissionPolicyM3,
		BaseAmountMinor: usage.WholesaleAmountMinor, AmountMinor: amount,
		Status: CommissionFrozen, IdempotencyKey: key,
		CreatedAt: time.Now().UTC(), ChannelOrgID: usage.ChannelOrgID,
	}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	if s.commissioner != nil {
		_ = s.commissioner.AccrueUsage(context.Background(), usage.ID, usage.RequestID, usage.UserID, stringPtr(usage.ChannelOrgID), usage.WholesaleAmountMinor)
	}
	return nil
}

func (s *Service) reverseCommission(tx *gorm.DB, usageEventID string) error {
	var rows []commissionRow
	if err := tx.Where("usage_event_id = ? AND status <> ?", usageEventID, CommissionReversed).Find(&rows).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for i := range rows {
		rev := commissionRow{
			ID: id.New("com"), UsageEventID: usageEventID, PolicyVersion: rows[i].PolicyVersion,
			BaseAmountMinor: rows[i].BaseAmountMinor, AmountMinor: -rows[i].AmountMinor,
			Status: CommissionReversed, ReversalOf: &rows[i].ID,
			IdempotencyKey: "commission-rev:" + rows[i].ID, CreatedAt: now,
			ChannelOrgID: rows[i].ChannelOrgID,
		}
		if err := tx.Where("idempotency_key = ?", rev.IdempotencyKey).FirstOrCreate(&rev).Error; err != nil {
			return err
		}
		rows[i].Status = CommissionReversed
		if err := tx.Save(&rows[i]).Error; err != nil {
			return err
		}
	}
	if s.commissioner != nil {
		_ = s.commissioner.ReverseUsage(context.Background(), usageEventID)
	}
	return nil
}

// RecalcCommission 用 usage 上的价格快照重算佣金：先冲正旧流水，再按同一批发价基数挂新冻结额。
func (s *Service) RecalcCommission(ctx context.Context, usageEventID string) (*CommissionView, error) {
	var view *CommissionView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var usage usageRow
		if err := tx.Where("id = ?", usageEventID).First(&usage).Error; err != nil {
			return ErrNotFound
		}
		if err := s.reverseCommission(tx, usage.ID); err != nil {
			return err
		}
		if usage.State != UsageConfirmed {
			view = &CommissionView{UsageEventID: usage.ID, Status: CommissionReversed, PolicyVersion: CommissionPolicyM3}
			return nil
		}
		if err := s.accrueCommission(tx, usage); err != nil {
			return err
		}
		var row commissionRow
		if err := tx.Where("usage_event_id = ? AND status = ?", usage.ID, CommissionFrozen).Order("created_at DESC").First(&row).Error; err != nil {
			return err
		}
		view = &CommissionView{ID: row.ID, UsageEventID: row.UsageEventID, AmountMinor: row.AmountMinor, Status: row.Status, PolicyVersion: row.PolicyVersion}
		return nil
	})
	return view, err
}

func (s *Service) ListCommissions(ctx context.Context, usageEventID string) ([]CommissionView, error) {
	var rows []commissionRow
	q := s.db.WithContext(ctx).Order("created_at")
	if usageEventID != "" {
		q = q.Where("usage_event_id = ?", usageEventID)
	}
	if err := q.Limit(100).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]CommissionView, 0, len(rows))
	for _, row := range rows {
		out = append(out, CommissionView{ID: row.ID, UsageEventID: row.UsageEventID, AmountMinor: row.AmountMinor, Status: row.Status, PolicyVersion: row.PolicyVersion})
	}
	return out, nil
}
