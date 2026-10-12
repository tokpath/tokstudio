package identity

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

// DeliverEmails uses short leases so a crashed worker cannot lose queued mail.
// Payloads are encrypted at rest and erased on success, expiry or final failure.
func (s *Service) DeliverEmails(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&emailDeliveryRow{}).Where("expires_at <= ? AND status IN ?", now, []string{"pending", "sending"}).Updates(map[string]any{"status": "expired", "payload_ciphertext": "", "lease_id": nil}).Error; err != nil {
		return err
	}
	if !s.EmailAvailable() {
		return nil
	}
	for range 10 {
		var row emailDeliveryRow
		lease := id.New("lease")
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ? AND available_at <= ? AND expires_at > ?", []string{"pending", "sending"}, time.Now().UTC(), time.Now().UTC()).Order("created_at ASC").First(&row).Error; err != nil {
				return err
			}
			if row.ActionID != nil {
				row.LeaseID = nil
				var count int64
				if err := tx.Model(&emailActionRow{}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", *row.ActionID, time.Now().UTC()).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return tx.Model(&row).Updates(map[string]any{"status": "cancelled", "payload_ciphertext": "", "lease_id": nil}).Error
				}
			}
			row.LeaseID = &lease
			row.Attempts++
			return tx.Model(&row).Updates(map[string]any{"status": "sending", "attempts": row.Attempts, "lease_id": lease, "available_at": time.Now().UTC().Add(time.Minute)}).Error
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.LeaseID == nil {
			continue
		}
		plain, deliveryErr := crypto.Open(s.emailKey, row.PayloadCiphertext)
		var message mail.Message
		if deliveryErr == nil {
			deliveryErr = json.Unmarshal([]byte(plain), &message)
		}
		if deliveryErr == nil {
			deliveryErr = s.emailSender.Send(ctx, message)
		}
		updates := map[string]any{"lease_id": nil}
		if deliveryErr == nil {
			updates["status"] = "sent"
			updates["sent_at"] = time.Now().UTC()
			updates["payload_ciphertext"] = ""
			updates["last_error"] = nil
		} else if row.Attempts >= 5 {
			updates["status"] = "failed"
			updates["payload_ciphertext"] = ""
			updates["last_error"] = "delivery_failed"
		} else {
			updates["status"] = "pending"
			updates["last_error"] = "delivery_failed"
			updates["available_at"] = time.Now().UTC().Add(time.Duration(1<<(row.Attempts-1)) * 30 * time.Second)
		}
		if err := s.db.WithContext(ctx).Model(&emailDeliveryRow{}).Where("id = ? AND lease_id = ? AND status = ?", row.ID, lease, "sending").Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RunEmailDelivery(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.DeliverEmails(ctx); err != nil && ctx.Err() == nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
