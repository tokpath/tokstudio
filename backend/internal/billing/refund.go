package billing

import (
	"context"
	"errors"
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
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if charge.Status == ChargeReversed {
			out = &Settlement{ChargeID: charge.ID, UsageEventID: charge.UsageEventID, AmountMinor: charge.AmountMinor, State: ChargeReversed, Currency: CurrencyUSD}
			return nil
		}
		var auth authRow
		if err := tx.Where("request_id = ?", requestID).First(&auth).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		userID := auth.UserID
		if userID == "" {
			var usage usageRow
			if err := tx.Where("id = ?", charge.UsageEventID).First(&usage).Error; err != nil {
				return err
			}
			userID = usage.UserID
		}
		if userID == "" {
			return ErrNotFound
		}
		walletCredit, _, _ := chargeRefundAmounts(charge.AmountMinor, auth)

		if err := s.reverseLegacyCommission(tx, charge.UsageEventID); err != nil {
			return err
		}
		if s.commissioner != nil {
			if err := s.commissioner.ReverseUsageTx(tx, charge.UsageEventID); err != nil {
				return err
			}
		}
		if walletCredit > 0 {
			if err := creditWallet(tx, userID, walletCredit, EventRefund, "customer_charge", charge.ID, "refund:"+requestID); err != nil {
				return err
			}
		}
		if s.coverer != nil {
			if err := s.coverer.ReverseByRequestTx(tx, requestID); err != nil {
				return err
			}
		}
		if err := reverseAllocationConsumes(tx, requestID); err != nil {
			return err
		}
		var usage usageRow
		if err := tx.Where("id = ?", charge.UsageEventID).First(&usage).Error; err != nil {
			return err
		}
		usage.State = UsageVoided
		if err := tx.Save(&usage).Error; err != nil {
			return err
		}
		charge.Status = ChargeReversed
		if err := tx.Save(&charge).Error; err != nil {
			return err
		}
		if auth.ID != "" {
			auth.Status = AuthReversed
			auth.UpdatedAt = time.Now().UTC()
			if err := tx.Save(&auth).Error; err != nil {
				return err
			}
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.charge.reversed", "customer_charge", charge.ID, map[string]any{
			"request_id": requestID, "amount_minor": charge.AmountMinor,
		}); err != nil {
			return err
		}
		out = &Settlement{ChargeID: charge.ID, UsageEventID: charge.UsageEventID, AmountMinor: charge.AmountMinor, State: ChargeReversed, Currency: CurrencyUSD}
		return nil
	})
	return out, err
}

func (s *Service) accrueCommission(tx *gorm.DB, usage usageRow) error {
	if s.commissioner != nil {
		return s.commissioner.AccrueUsage(context.Background(), usage.ID, usage.RequestID, usage.UserID, stringPtr(usage.ChannelOrgID), usage.WholesaleAmountMinor)
	}
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
	return tx.Create(&row).Error
}

func (s *Service) reverseCommission(tx *gorm.DB, usageEventID string) error {
	if err := s.reverseLegacyCommission(tx, usageEventID); err != nil {
		return err
	}
	if s.commissioner != nil {
		return s.commissioner.ReverseUsage(tx.Statement.Context, usageEventID)
	}
	return nil
}

func (s *Service) reverseLegacyCommission(tx *gorm.DB, usageEventID string) error {
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
		if s.commissioner != nil {
			view = &CommissionView{UsageEventID: usage.ID, Status: CommissionFrozen, PolicyVersion: CommissionPolicyM3}
			return nil
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

// ChargeRefundPreview exposes the original consumption split, never external cash movement.
type ChargeRefundPreview struct {
	RequestID        string `json:"request_id"`
	UserID           string `json:"user_id"`
	ModelID          string `json:"model_id"`
	State            string `json:"state"`
	AmountMinor      int64  `json:"amount_minor"`
	WalletMinor      int64  `json:"wallet_minor"`
	EntitlementMinor int64  `json:"entitlement_minor"`
	GiftMinor        int64  `json:"gift_minor"`
}

func chargeRefundAmounts(amount int64, auth authRow) (wallet, entitlement, gift int64) {
	wallet = amount
	if auth.ID == "" {
		return
	}
	entitlement = entitlementKeep(auth, amount)
	if entitlement > wallet {
		entitlement = wallet
	}
	wallet -= entitlement
	gift = auth.GiftSettledMinor
	if gift > wallet {
		gift = wallet
	}
	wallet -= gift
	return
}

func (s *Service) PreviewChargeRefund(ctx context.Context, requestID string) (*ChargeRefundPreview, error) {
	var charge chargeRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&charge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var auth authRow
	if err := s.db.WithContext(ctx).Where("request_id = ?", requestID).First(&auth).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var usage usageRow
	if err := s.db.WithContext(ctx).Where("id = ?", charge.UsageEventID).First(&usage).Error; err != nil {
		return nil, err
	}
	wallet, entitlement, gift := chargeRefundAmounts(charge.AmountMinor, auth)
	return &ChargeRefundPreview{RequestID: requestID, UserID: usage.UserID, ModelID: usage.PublicModelID, State: charge.Status, AmountMinor: charge.AmountMinor, WalletMinor: wallet, EntitlementMinor: entitlement, GiftMinor: gift}, nil
}
