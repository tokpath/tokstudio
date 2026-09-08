package billing

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

func (s *Service) CreateTopup(ctx context.Context, userID, channelOrgID string, amountMinor int64, method string) (*TopupView, error) {
	if amountMinor <= 0 {
		return nil, ErrInvalidAmount
	}
	if method == "" {
		method = "manual"
	}
	now := time.Now().UTC()
	row := topupRow{
		ID: id.New("top"), UserID: userID, AmountMinor: amountMinor, Currency: CurrencyUSD,
		PaymentMethod: method, Status: TopupPending, CreatedAt: now, UpdatedAt: now,
	}
	if channelOrgID != "" {
		row.ChannelOrgID = &channelOrgID
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	return topupView(row), nil
}

func (s *Service) GetTopup(ctx context.Context, id, userID string) (*TopupView, error) {
	var row topupRow
	q := s.db.WithContext(ctx).Where("id = ?", id)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, ErrNotFound
	}
	return topupView(row), nil
}

func (s *Service) ConfirmTopup(ctx context.Context, topupID, actorUserID string) (*TopupView, error) {
	var view *TopupView
	var channelID string
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row topupRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", topupID).First(&row).Error; err != nil {
			return ErrNotFound
		}
		if row.ChannelOrgID != nil {
			channelID = *row.ChannelOrgID
		}
		if row.Status == TopupPaid {
			view = topupView(row)
			return nil
		}
		if row.Status != TopupPending {
			return ErrTopupNotPending
		}
		if err := creditWallet(tx, row.UserID, row.AmountMinor, EventTopup, "topup", row.ID, "topup:"+row.ID); err != nil {
			return err
		}
		if err := s.issueAllocation(tx, row.UserID, channelID, "topup", row.ID, row.AmountMinor); err != nil {
			return err
		}
		row.Status = TopupPaid
		row.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.topup.paid", "topup", row.ID, map[string]any{
			"user_id": row.UserID, "amount_minor": row.AmountMinor, "actor": actorUserID,
		}); err != nil {
			return err
		}
		view = topupView(row)
		return nil
	})
	if err == nil && view != nil {
		s.considerEligibility(ctx, view.UserID, channelID, view.AmountMinor)
	}
	return view, err
}

func (s *Service) Redeem(ctx context.Context, userID, channelOrgID, code string) (*TopupView, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, ErrRedeemUnavailable
	}
	var view *TopupView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var redeem redeemRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", code).First(&redeem).Error; err != nil {
			return ErrRedeemUnavailable
		}
		if redeem.Status != "active" || redeem.RedeemedCount >= redeem.MaxRedemptions {
			return ErrRedeemUnavailable
		}
		now := time.Now().UTC()
		row := topupRow{
			ID: id.New("top"), UserID: userID, AmountMinor: redeem.AmountMinor, Currency: redeem.Currency,
			PaymentMethod: "redeem_code", Status: TopupPaid, RedeemCode: &code, CreatedAt: now, UpdatedAt: now,
		}
		if channelOrgID != "" {
			row.ChannelOrgID = &channelOrgID
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := creditWallet(tx, userID, redeem.AmountMinor, EventTopup, "topup", row.ID, "redeem:"+row.ID); err != nil {
			return err
		}
		if err := s.issueAllocation(tx, userID, channelOrgID, "topup", row.ID, redeem.AmountMinor); err != nil {
			return err
		}
		redeem.RedeemedCount++
		if err := tx.Save(&redeem).Error; err != nil {
			return err
		}
		if _, err := s.outbox.EnqueueTx(tx, "billing.topup.paid", "topup", row.ID, map[string]any{
			"user_id": userID, "amount_minor": redeem.AmountMinor, "code": code,
		}); err != nil {
			return err
		}
		view = topupView(row)
		return nil
	})
	if err == nil && view != nil {
		s.considerEligibility(ctx, userID, channelOrgID, view.AmountMinor)
	}
	return view, err
}

func (s *Service) RefundTopup(ctx context.Context, topupID string) (*TopupView, error) {
	var view *TopupView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row topupRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", topupID).First(&row).Error; err != nil {
			return ErrNotFound
		}
		if row.Status == TopupRefunded {
			view = topupView(row)
			return nil
		}
		if row.Status != TopupPaid {
			return ErrTopupNotPending
		}
		if err := debitAvailable(tx, row.UserID, row.AmountMinor, EventRefund, "topup", row.ID, "refund-topup:"+row.ID); err != nil {
			return err
		}
		if err := reclaimAllocation(tx, "topup", row.ID); err != nil {
			return err
		}
		row.Status = TopupRefunded
		row.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		view = topupView(row)
		return nil
	})
	return view, err
}

func creditWallet(tx *gorm.DB, userID string, amount int64, event, refType, refID, idem string) error {
	wallet, err := lockWallet(tx, userID)
	if err != nil {
		return err
	}
	wallet.AvailableMinor += amount
	wallet.Version++
	wallet.UpdatedAt = time.Now().UTC()
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	return writeLedger(tx, wallet, event, amount, refType, refID, idem)
}

func debitAvailable(tx *gorm.DB, userID string, amount int64, event, refType, refID, idem string) error {
	wallet, err := lockWallet(tx, userID)
	if err != nil {
		return err
	}
	if wallet.AvailableMinor < amount {
		return ErrInsufficientBalance
	}
	wallet.AvailableMinor -= amount
	wallet.Version++
	wallet.UpdatedAt = time.Now().UTC()
	if err := tx.Save(wallet).Error; err != nil {
		return err
	}
	return writeLedger(tx, wallet, event, -amount, refType, refID, idem)
}

func topupView(row topupRow) *TopupView {
	view := &TopupView{
		ID: row.ID, UserID: row.UserID, AmountMinor: row.AmountMinor, Currency: row.Currency,
		PaymentMethod: row.PaymentMethod, Status: row.Status, CreatedAt: row.CreatedAt,
	}
	if row.RedeemCode != nil {
		view.RedeemCode = *row.RedeemCode
	}
	return view
}
