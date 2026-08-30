package billing

import (
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
