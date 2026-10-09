package payment

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
)

type OfflineReceiptInput struct {
	OperationID           string    `json:"operation_id"`
	UserID                string    `json:"user_id"`
	AmountMinor           int64     `json:"amount_minor"`
	CreditMinor           int64     `json:"credit_minor"`
	Currency              string    `json:"currency"`
	Reference             string    `json:"reference"`
	Note                  string    `json:"note"`
	OccurredAt            time.Time `json:"occurred_at"`
	ExpectedIssueRatioBPS int64     `json:"expected_issue_ratio_bps"`
}

type offlineOperationRow struct {
	OperationID       string    `gorm:"column:operation_id;primaryKey"`
	ActorUserID       string    `gorm:"column:actor_user_id"`
	PayeeChannelOrgID string    `gorm:"column:payee_channel_org_id"`
	PayloadJSON       []byte    `gorm:"column:payload_json"`
	OrderID           string    `gorm:"column:order_id"`
	CreatedAt         time.Time `gorm:"column:created_at"`
}

func (offlineOperationRow) TableName() string { return "payment_offline_operations" }

type ExternalReceiptConflict struct{ OrderID string }

func (e *ExternalReceiptConflict) Error() string { return "external transaction already recorded" }

func (s *Service) OfflineOperation(ctx context.Context, operationID, actor, ownerID string) (*OrderView, error) {
	var row offlineOperationRow
	err := s.db.WithContext(ctx).Where("operation_id = ? AND actor_user_id = ? AND payee_channel_org_id = ?", operationID, actor, ownerID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, row.OrderID, "")
}

type OfflinePreview struct {
	UserID            string `json:"user_id"`
	ChannelOrgID      string `json:"channel_org_id"`
	PayeeChannelOrgID string `json:"payee_channel_org_id"`
	CreditMinor       int64  `json:"credit_minor"`
	IssueRatioBPS     int64  `json:"issue_ratio_bps"`
	PoolBeforeMinor   *int64 `json:"pool_before_minor,omitempty"`
	PoolDebitMinor    int64  `json:"pool_debit_minor"`
	PoolAfterMinor    *int64 `json:"pool_after_minor,omitempty"`
}

func (s *Service) PreviewOfflineReceipt(ctx context.Context, ownerID, userID string, credit int64) (*OfflinePreview, error) {
	if credit <= 0 || s.billing == nil {
		return nil, ErrInvalidAmount
	}
	if err := s.requireCollector(ctx, ownerID); err != nil {
		return nil, err
	}
	channelID, err := identity.New(s.db).BillingCustomerForOwner(ctx, userID, ownerID)
	if err != nil {
		return nil, err
	}
	view := &OfflinePreview{UserID: userID, ChannelOrgID: channelID, PayeeChannelOrgID: ownerID, CreditMinor: credit, IssueRatioBPS: billing.DefaultIssueRatioBPS}
	if ownerID == identity.OfficialChannelID {
		return view, nil
	}
	rule, err := s.billing.IssueRule(ctx, ownerID)
	if err != nil {
		return nil, err
	}
	view.IssueRatioBPS = rule.IssueRatioBPS
	debit, err := billing.ConvertQuota(credit, rule.IssueRatioBPS)
	if err != nil {
		return nil, err
	}
	quota, err := s.billing.ChannelQuota(ctx, ownerID)
	if errors.Is(err, billing.ErrNotFound) {
		return nil, billing.ErrInsufficientQuota
	}
	if err != nil {
		return nil, err
	}
	if quota.AvailableMinor < debit {
		return nil, billing.ErrInsufficientQuota
	}
	after := quota.AvailableMinor - debit
	view.PoolBeforeMinor = &quota.AvailableMinor
	view.PoolDebitMinor = debit
	view.PoolAfterMinor = &after
	return view, nil
}

// RecordOfflineReceipt records money already received outside the application.
// A receipt, allocation, wallet and audit are committed together.
func (s *Service) RecordOfflineReceipt(ctx context.Context, ownerID string, in OfflineReceiptInput, recorder *audit.Service, record audit.RecordInput) (*OrderView, error) {
	in.Reference = strings.TrimSpace(in.Reference)
	in.OperationID = strings.TrimSpace(in.OperationID)
	in.Note = strings.TrimSpace(in.Note)
	in.OccurredAt = in.OccurredAt.UTC()
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.OperationID == "" || len(in.OperationID) > 100 || in.OccurredAt.IsZero() || in.OccurredAt.After(time.Now().Add(5*time.Minute)) || len(in.Note) > 2000 || in.UserID == "" || in.AmountMinor <= 0 || in.CreditMinor <= 0 || len(in.Reference) > 200 || (in.Currency != "CNY" && in.Currency != "USD") || record.ActorUserID == "" || recorder == nil || s.billing == nil {
		return nil, ErrInvalidAmount
	}
	if err := s.requireCollector(ctx, ownerID); err != nil {
		return nil, err
	}
	var result *OrderView
	payload, _ := json.Marshal(in)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "offline-operation:"+in.OperationID).Error; err != nil {
			return err
		}
		var operation offlineOperationRow
		err := tx.Where("operation_id = ?", in.OperationID).First(&operation).Error
		if err == nil {
			var original OfflineReceiptInput
			if operation.ActorUserID != record.ActorUserID || operation.PayeeChannelOrgID != ownerID || json.Unmarshal(operation.PayloadJSON, &original) != nil || original != in {
				return ErrReceiptConflict
			}
			var existing orderRow
			if err := tx.Where("id = ?", operation.OrderID).First(&existing).Error; err != nil {
				return err
			}
			result = orderView(existing)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if in.Reference != "" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "external-receipt:"+ownerID+":"+in.Reference).Error; err != nil {
				return err
			}
			var existing orderRow
			if err := tx.Where("payee_channel_org_id = ? AND receipt_reference = ?", ownerID, in.Reference).First(&existing).Error; err == nil {
				return &ExternalReceiptConflict{OrderID: existing.ID}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		channelID, err := identity.New(tx).BillingCustomerForOwner(ctx, in.UserID, ownerID)
		if err != nil {
			return err
		}
		if channelID == "" {
			channelID = identity.OfficialChannelID
		}
		if ownerID != identity.OfficialChannelID {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "quota-issue-rule:"+ownerID).Error; err != nil {
				return err
			}
			rule, err := billing.New(tx, s.outbox).IssueRule(ctx, ownerID)
			if err != nil {
				return err
			}
			if in.ExpectedIssueRatioBPS != rule.IssueRatioBPS {
				return ErrPreviewChanged
			}
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
			ReceiptReference: in.Reference, ReceiptNote: in.Note, ReceivedAt: &in.OccurredAt, RecordedBy: record.ActorUserID, Status: StatusPaid,
			CreatedAt: now, UpdatedAt: now, FulfilledAt: &now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		operation = offlineOperationRow{OperationID: in.OperationID, ActorUserID: record.ActorUserID, PayeeChannelOrgID: ownerID, PayloadJSON: payload, OrderID: row.ID, CreatedAt: now}
		if err := tx.Create(&operation).Error; err != nil {
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
