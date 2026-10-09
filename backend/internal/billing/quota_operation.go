package billing

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"strings"
	"time"
)

type quotaOperationRow struct {
	ID           string
	OperationID  string
	ActorUserID  string
	ChannelOrgID string
	AmountMinor  int64
	ResultJSON   []byte
	CreatedAt    time.Time
}

func (quotaOperationRow) TableName() string { return "billing_quota_operations" }

type QuotaOperationView struct {
	ID           string     `json:"id"`
	OperationID  string     `json:"operation_id"`
	ChannelOrgID string     `json:"channel_org_id"`
	AmountMinor  int64      `json:"amount_minor"`
	CreatedAt    time.Time  `json:"created_at"`
	Quota        *QuotaView `json:"quota"`
}

func quotaOperationView(row quotaOperationRow) (*QuotaOperationView, error) {
	var quota QuotaView
	if err := json.Unmarshal(row.ResultJSON, &quota); err != nil {
		return nil, err
	}
	return &QuotaOperationView{row.ID, row.OperationID, row.ChannelOrgID, row.AmountMinor, row.CreatedAt, &quota}, nil
}
func (s *Service) QuotaOperation(ctx context.Context, actor, channelID, operationID string) (*QuotaOperationView, error) {
	var row quotaOperationRow
	err := s.db.WithContext(ctx).Where("actor_user_id=? AND channel_org_id=? AND operation_id=?", actor, channelID, operationID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return quotaOperationView(row)
}

// The operation and the quota ledger commit together. A retry always returns
// the original outcome, even after unrelated later adjustments.
func (s *Service) AdjustQuota(ctx context.Context, actor, channelID, operationID string, amount int64) (*QuotaOperationView, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" || actor == "" || amount == 0 {
		return nil, ErrInvalidAmount
	}
	var out *QuotaOperationView
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "quota-operation:"+operationID).Error; err != nil {
			return err
		}
		var old quotaOperationRow
		err := tx.Where("operation_id=?", operationID).First(&old).Error
		if err == nil {
			if old.ActorUserID != actor || old.ChannelOrgID != channelID || old.AmountMinor != amount {
				return ErrConflict
			}
			out, err = quotaOperationView(old)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "quota-owner:"+channelID).Error; err != nil {
			return err
		}
		service := *s
		service.db = tx
		recordID := id.New("qop")
		quota, err := service.grantChannelQuota(ctx, channelID, amount, actor, "quota_operation", recordID, "qop:"+operationID)
		if err != nil {
			return err
		}
		body, err := json.Marshal(quota)
		if err != nil {
			return err
		}
		row := quotaOperationRow{ID: recordID, OperationID: operationID, ActorUserID: actor, ChannelOrgID: channelID, AmountMinor: amount, ResultJSON: body, CreatedAt: time.Now().UTC()}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		out, err = quotaOperationView(row)
		return err
	})
	return out, err
}
