package billing

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NotificationFunds projects current eligible credit through the same coverer
// as admission; an empty cash wallet with a valid plan is not insufficient.
func (s *Service) NotificationFunds(ctx context.Context, userID string) (int64, error) {
	var available int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var wallet walletRow
		err := tx.Where("user_id=? AND currency=?", userID, CurrencyUSD).First(&wallet).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		available = wallet.AvailableMinor
		if s.coverer != nil {
			credit, err := s.coverer.AvailableUSDTx(tx, userID)
			if err != nil {
				return err
			}
			available += credit
		}
		return nil
	})
	return available, err
}

func (s *Service) RunQuotaNotifications(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.ProcessQuotaNotifications(ctx); err != nil && report != nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) NotificationQuota(ctx context.Context, channelID string) (int64, error) {
	var row quotaRow
	err := s.db.WithContext(ctx).Where("owner_type=? AND owner_id=? AND unit_type=?", "channel", channelID, "usd_credit").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return row.AvailableMinor, err
}

// Reuse admission's quota check including allocated user stock. A replenished
// pool alone does not repair an exhausted allocation, and an empty pool does not
// invalidate stock already allocated to this customer.
func (s *Service) NotificationQuotaBlocked(ctx context.Context, userID, channelID string, required int64) (bool, error) {
	var blocked bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		need := required
		if s.coverer != nil {
			credit, err := s.coverer.AvailableUSDTx(tx, userID)
			if err != nil {
				return err
			}
			need -= credit
			if need < 0 {
				need = 0
			}
		}
		err := s.reserveChannelQuota(tx, userID, channelID, "", required, need)
		if errors.Is(err, ErrInsufficientQuota) {
			blocked = true
			return nil
		}
		return err
	})
	return blocked, err
}

// ProcessQuotaNotifications records transitions once rather than sending on
// every polling cycle. Already allocated user credit is not revoked by this.
func (s *Service) ProcessQuotaNotifications(ctx context.Context) error {
	type state struct {
		OwnerID string `gorm:"primaryKey"`
		Low     bool
	}
	var quotas []quotaRow
	if err := s.db.WithContext(ctx).Where("owner_type=? AND unit_type=?", "channel", "usd_credit").Find(&quotas).Error; err != nil {
		return err
	}
	for _, snapshot := range quotas {
		if skipChannelQuota(snapshot.OwnerID) {
			continue
		}
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Table("billing_quota_notification_state").Clauses(clause.OnConflict{DoNothing: true}).Create(&state{OwnerID: snapshot.OwnerID}).Error; err != nil {
				return err
			}
			var old state
			if err := tx.Table("billing_quota_notification_state").Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_id=?", snapshot.OwnerID).First(&old).Error; err != nil {
				return err
			}
			var quota quotaRow
			if err := tx.Where("id=?", snapshot.ID).First(&quota).Error; err != nil {
				return err
			}
			low := quota.AvailableMinor < MinorPerUSD
			if low == old.Low {
				return nil
			}
			kind := "billing.quota.low"
			if !low {
				kind = "billing.quota.recovered"
			}
			if err := tx.Table("billing_quota_notification_state").Where("owner_id=?", quota.OwnerID).Update("low", low).Error; err != nil {
				return err
			}
			_, err := s.outbox.EnqueueTx(tx, kind, "channel_quota", quota.ID, map[string]any{"channel_org_id": quota.OwnerID, "required_minor": MinorPerUSD})
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
