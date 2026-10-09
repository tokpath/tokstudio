package payment

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
)

type OfflineReceiptInput struct {
	UserID      string `json:"user_id"`
	AmountMinor int64  `json:"amount_minor"`
	CreditMinor int64  `json:"credit_minor"`
	Currency    string `json:"currency"`
	Reference   string `json:"reference"`
}

// RecordOfflineReceipt records money already received outside the application.
// A receipt, allocation, wallet and audit are committed together.
func (s *Service) RecordOfflineReceipt(ctx context.Context, ownerID string, in OfflineReceiptInput, recorder *audit.Service, record audit.RecordInput) (*OrderView, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.UserID == "" || in.AmountMinor <= 0 || in.CreditMinor <= 0 || in.Reference == "" || len(in.Reference) > 200 || (in.Currency != "CNY" && in.Currency != "USD") || record.ActorUserID == "" || recorder == nil || s.billing == nil {
		return nil, ErrInvalidAmount
	}
	if err := s.requireCollector(ctx, ownerID); err != nil {
		return nil, err
	}
	var result *OrderView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "offline-receipt:"+ownerID+":"+in.Reference).Error; err != nil {
			return err
		}
		var existing orderRow
		err := tx.Where("payee_channel_org_id = ? AND receipt_reference = ?", ownerID, in.Reference).First(&existing).Error
		if err == nil {
			if existing.Status != StatusPaid || existing.FulfilledAt == nil || existing.UserID != in.UserID || existing.AmountMinor != in.AmountMinor || existing.CreditMinor != in.CreditMinor || existing.Currency != in.Currency {
				return ErrReceiptConflict
			}
			result = orderView(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		channelID, err := identity.New(tx).BillingCustomerForOwner(ctx, in.UserID, ownerID)
		if err != nil {
			return err
		}
		if channelID == "" {
			channelID = identity.OfficialChannelID
		}
		topup, err := s.billing.CreateTopupTx(tx, in.UserID, channelID, in.CreditMinor, AdapterManual)
		if err != nil {
			return err
		}
		if _, err := s.billing.ConfirmTopupTx(tx, topup.ID, record.ActorUserID); err != nil {
			return err
		}
		now := time.Now().UTC()
		refType := "topup"
		row := orderRow{ID: id.New("pay"), UserID: in.UserID, ChannelOrgID: channelID, PayeeChannelOrgID: ownerID,
			Adapter: AdapterManual, Purpose: PurposeWallet, ReferenceType: &refType, ReferenceID: &topup.ID,
			AmountMinor: in.AmountMinor, CreditMinor: in.CreditMinor, Currency: in.Currency,
			ReceiptReference: in.Reference, RecordedBy: record.ActorUserID, Status: StatusPaid,
			CreatedAt: now, UpdatedAt: now, FulfilledAt: &now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		result = orderView(row)
		record.Action, record.ResourceType, record.ResourceID, record.After = "payment.offline.allocate", "payment_order", row.ID, result
		_, err = recorder.RecordTx(tx, record)
		return err
	})
	if err != nil {
		return nil, err
	}
	// Existing paid topups are safe to replay and trigger the eligibility check.
	if result.ReferenceID != "" {
		_, _ = s.billing.ConfirmTopup(ctx, result.ReferenceID, record.ActorUserID)
	}
	return result, nil
}
