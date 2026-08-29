package billing

import (
	"context"
	"errors"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) reserveChannelQuota(tx *gorm.DB, channelOrgID, requestID string, amount int64) error {
	if channelOrgID == "" || channelOrgID == identity.OfficialChannelID {
		return nil
	}
	var quota quotaRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
		First(&quota).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if quota.AvailableMinor < amount {
		return ErrInsufficientQuota
	}
	quota.AvailableMinor -= amount
	quota.ReservedMinor += amount
	quota.Version++
	if err := tx.Save(&quota).Error; err != nil {
		return err
	}
	return writeQuotaLedger(tx, quota.ID, "quota_reserve", -amount, "request", requestID, "qreserve:"+requestID)
}

func (s *Service) settleChannelQuota(tx *gorm.DB, channelOrgID, requestID string, reserved, customer int64) error {
	if channelOrgID == "" || channelOrgID == identity.OfficialChannelID {
		return nil
	}
	var quota quotaRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
		First(&quota).Error
	if err != nil {
		return nil
	}
	release := reserved - customer
	quota.ReservedMinor -= reserved
	quota.AvailableMinor += release
	quota.Version++
	if err := tx.Save(&quota).Error; err != nil {
		return err
	}
	if err := writeQuotaLedger(tx, quota.ID, "usage_debit", -customer, "request", requestID, "qdebit:"+requestID); err != nil {
		return err
	}
	if release > 0 {
		return writeQuotaLedger(tx, quota.ID, "quota_release", release, "request", requestID, "qrelease:"+requestID)
	}
	return nil
}

func (s *Service) releaseChannelQuota(tx *gorm.DB, channelOrgID, requestID string, amount int64) error {
	var quota quotaRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
		First(&quota).Error
	if err != nil {
		return nil
	}
	quota.ReservedMinor -= amount
	quota.AvailableMinor += amount
	quota.Version++
	if err := tx.Save(&quota).Error; err != nil {
		return err
	}
	return writeQuotaLedger(tx, quota.ID, "quota_release", amount, "request", requestID, "qrelease:"+requestID)
}

func (s *Service) ChannelQuota(ctx context.Context, channelOrgID string) (*QuotaView, error) {
	var quota quotaRow
	err := s.db.WithContext(ctx).
		Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
		First(&quota).Error
	if err != nil {
		return nil, ErrNotFound
	}
	return &QuotaView{OwnerID: quota.OwnerID, AvailableMinor: quota.AvailableMinor, ReservedMinor: quota.ReservedMinor, UnitType: quota.UnitType}, nil
}

func (s *Service) GrantChannelQuota(ctx context.Context, channelOrgID string, amount int64, actor string) (*QuotaView, error) {
	if amount == 0 || channelOrgID == "" {
		return nil, ErrInvalidAmount
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var quota quotaRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("owner_type = ? AND owner_id = ? AND unit_type = ?", "channel", channelOrgID, "usd_credit").
			First(&quota).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			quota = quotaRow{
				ID: id.New("qta"), OwnerType: "channel", OwnerID: channelOrgID, UnitType: "usd_credit",
			}
			if err := tx.Create(&quota).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		next := quota.AvailableMinor + amount
		if next < 0 {
			return ErrInsufficientQuota
		}
		quota.AvailableMinor = next
		quota.Version++
		if err := tx.Save(&quota).Error; err != nil {
			return err
		}
		kind := "platform_grant"
		if amount < 0 {
			kind = "quota_reclaim"
		}
		return writeQuotaLedger(tx, quota.ID, kind, amount, "admin", actor, "qgrant:"+channelOrgID+":"+id.New("gen"))
	})
	if err != nil {
		return nil, err
	}
	return s.ChannelQuota(ctx, channelOrgID)
}

func writeQuotaLedger(tx *gorm.DB, accountID, event string, amount int64, refType, refID, idem string) error {
	var existing quotaLedgerRow
	if err := tx.Where("idempotency_key = ?", idem).First(&existing).Error; err == nil {
		return nil
	}
	return tx.Create(&quotaLedgerRow{
		ID: id.New("qld"), AccountID: accountID, EventType: event, AmountMinor: amount,
		ReferenceType: refType, ReferenceID: refID, IdempotencyKey: idem,
	}).Error
}
