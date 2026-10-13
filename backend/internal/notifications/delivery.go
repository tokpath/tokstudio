package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func (s *Service) obsolete(ctx context.Context, row inboxRow) (bool, error) {
	if row.Kind == "alert_opened" && s.alerts != nil {
		open, err := s.alerts.NotificationAlertOpen(ctx, row.ReferenceID)
		return !open, err
	}
	if row.PeriodEnd != nil && row.ReferenceID != "" {
		sub, err := s.subscriptions.GetSubscription(ctx, row.ReferenceID, row.UserID)
		if errors.Is(err, plans.ErrNotFound) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if sub.PeriodEnd == nil || sub.PeriodEnd.After(*row.PeriodEnd) {
			return true, nil
		}
		if strings.HasPrefix(row.Kind, "expiring_") {
			return !row.PeriodEnd.After(time.Now().UTC()) || (sub.Status != plans.SubActive && sub.Status != plans.SubCancelAtPeriodEnd), nil
		}
		if row.Kind == "renewed" {
			return false, nil
		}
	}
	if row.Kind == "balance_low" || row.Kind == "balance_insufficient" {
		available, err := s.funds.NotificationFunds(ctx, row.UserID)
		return available >= row.RequiredMinor, err
	}
	if row.Kind == "quota_low" {
		available, err := s.funds.NotificationQuota(ctx, row.ReferenceID)
		return available >= row.RequiredMinor, err
	}
	if row.Kind == "quota_insufficient" {
		blocked, err := s.funds.NotificationQuotaBlocked(ctx, row.SubjectUserID, row.CustomerChannelID, row.RequiredMinor)
		return !blocked, err
	}
	if row.Kind == "quota_recovered" {
		available, err := s.funds.NotificationQuota(ctx, row.ReferenceID)
		return available < row.RequiredMinor, err
	}
	return false, nil
}

// Deliver uses expiring leases and safe error codes. SMTP acceptance is recorded
// as accepted, never as confirmed recipient delivery. A stable Message-ID makes
// retry copies traceable if the worker dies after DATA and before committing.
func (s *Service) Deliver(ctx context.Context) error {
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).Model(&deliveryRow{}).Where("expires_at<=? AND status IN ?", now, []string{"pending", "sending", "blocked_smtp", "blocked_unverified"}).
		Updates(map[string]any{"status": "expired", "payload_ciphertext": "", "lease_id": nil, "last_error": "notice_expired"}).Error; err != nil {
		return err
	}
	ready := mail.Available(s.sender) && s.key != ""
	for range 10 {
		var delivery deliveryRow
		lease := id.New("lease")
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("status IN ? AND available_at<=? AND expires_at>?",
				[]string{"pending", "sending", "blocked_smtp", "blocked_unverified"}, time.Now().UTC(), time.Now().UTC()).Order("available_at,created_at,id").First(&delivery).Error; err != nil {
				return err
			}
			return tx.Model(&delivery).Updates(map[string]any{"status": "sending", "lease_id": lease, "available_at": time.Now().UTC().Add(time.Minute)}).Error
		})
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var row inboxRow
		if err := s.db.WithContext(ctx).Where("id=?", delivery.NotificationID).First(&row).Error; err != nil {
			return err
		}
		recipient, err := s.recipients.NotificationUser(ctx, row.UserID)
		status, code := "", ""
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status, code = "cancelled", "recipient_unavailable"
		} else if err != nil {
			return err
		}
		if status == "" && recipient.BrandID != row.BrandID {
			status, code = "cancelled", "brand_mismatch"
		}
		if status == "" && (row.Portal == "admin" || row.Portal == "channel") {
			scopeType, scopeID, roles := "platform", "*", []string{"platform_admin", "ops_admin", "tech_admin"}
			if row.Portal == "channel" {
				scopeType, scopeID, roles = "channel", row.ReferenceID, []string{"channel_admin", "oem_ops", "oem_finance"}
			}
			eligible, err := s.recipients.NotificationStaff(ctx, scopeType, scopeID, roles)
			if err != nil {
				return err
			}
			found := false
			for _, staff := range eligible {
				if staff.UserID == row.UserID && staff.BrandID == row.BrandID {
					*recipient = staff
					found = true
					break
				}
			}
			if !found {
				status, code = "cancelled", "recipient_unavailable"
			}
		}
		if status == "" {
			obsolete, err := s.obsolete(ctx, row)
			if err != nil {
				return err
			}
			if obsolete {
				status, code = "cancelled", "condition_recovered"
			}
		}
		if status == "" {
			enabled, err := s.EmailEnabled(ctx, row.UserID, row.BrandID)
			if err != nil {
				return err
			}
			if !enabled && row.Kind != "smtp_test" {
				status, code = "cancelled", "email_disabled"
			}
		}
		if status == "" && !recipient.Verified {
			status, code = "blocked_unverified", "email_unverified"
		}
		if status == "" && !ready {
			status, code = "blocked_smtp", "smtp_unavailable"
		}
		if status == "" {
			recipient.Portal = row.Portal
		}
		if status == "" && s.origin(*recipient) == "" {
			status, code = "failed", "brand_origin_unavailable"
		}
		updates := map[string]any{"lease_id": nil, "last_error": code}
		if status != "" {
			updates["status"] = status
			updates["available_at"] = time.Now().UTC().Add(time.Minute)
			if status == "cancelled" || status == "failed" {
				updates["payload_ciphertext"] = ""
			}
		} else {
			message := s.message(row, *recipient, delivery.ID)
			body, err := json.Marshal(message)
			if err != nil {
				return err
			}
			encrypted, err := crypto.Seal(s.key, string(body))
			if err != nil {
				return err
			}
			// Rebuild using the currently verified address, avoiding stale emails.
			if err := s.db.WithContext(ctx).Model(&deliveryRow{}).Where("id=? AND lease_id=?", delivery.ID, lease).Update("payload_ciphertext", encrypted).Error; err != nil {
				return err
			}
			sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			sendErr := s.sender.Send(sendCtx, message)
			cancel()
			attempts := delivery.Attempts + 1
			updates["attempts"] = attempts
			if sendErr == nil {
				updates["status"] = "accepted"
				updates["accepted_at"] = time.Now().UTC()
				updates["payload_ciphertext"] = ""
			} else if errors.Is(sendErr, mail.ErrUnavailable) {
				updates["status"] = "blocked_smtp"
				updates["last_error"] = "smtp_unavailable"
				updates["attempts"] = delivery.Attempts
			} else if attempts >= 5 {
				updates["status"] = "failed"
				updates["last_error"] = "smtp_delivery_failed"
				updates["payload_ciphertext"] = ""
			} else {
				updates["status"] = "pending"
				updates["last_error"] = "smtp_delivery_failed"
				updates["available_at"] = time.Now().UTC().Add(time.Duration(30*(1<<uint(attempts-1))) * time.Second)
			}
		}
		if err := s.db.WithContext(ctx).Model(&deliveryRow{}).Where("id=? AND lease_id=?", delivery.ID, lease).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) Run(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Consume(ctx); err != nil && report != nil {
				report(errors.New("notification_consume_failed"))
			}
			if err := s.Deliver(ctx); err != nil && report != nil {
				report(errors.New("notification_queue_failed"))
			}
		}
	}
}

type DeliveryView struct {
	Kind           string     `json:"kind"`
	ID             string     `json:"id"`
	NotificationID string     `json:"notification_id"`
	UserID         string     `json:"user_id"`
	BrandID        string     `json:"brand_id"`
	Title          string     `json:"title"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	LastError      string     `json:"last_error"`
	CreatedAt      time.Time  `json:"created_at"`
	AcceptedAt     *time.Time `json:"accepted_at"`
}
type DeliveryPage struct {
	Items    []DeliveryView `json:"items" gorm:"-"`
	Pending  int64          `json:"pending"`
	Blocked  int64          `json:"blocked"`
	Failed   int64          `json:"failed"`
	Accepted int64          `json:"accepted"`
}

func (s *Service) Deliveries(ctx context.Context, status string) (*DeliveryPage, error) {
	page := &DeliveryPage{Items: []DeliveryView{}}
	q := s.db.WithContext(ctx).Table("notifications_email_deliveries d").Select("d.id,d.notification_id,n.user_id,n.brand_id,n.kind,n.title,d.status,d.attempts,d.last_error,d.created_at,d.accepted_at").Joins("JOIN notifications_inbox n ON n.id=d.notification_id")
	if status != "" {
		q = q.Where("d.status=?", status)
	}
	if err := q.Order("d.created_at DESC,d.id DESC").Limit(50).Scan(&page.Items).Error; err != nil {
		return nil, err
	}
	var counts struct {
		Pending, Blocked, Failed, Accepted int64
	}
	err := s.db.WithContext(ctx).Model(&deliveryRow{}).Select(`COUNT(*) FILTER (WHERE status IN ('pending','sending')) AS pending,
        COUNT(*) FILTER (WHERE status IN ('blocked_smtp','blocked_unverified')) AS blocked,
        COUNT(*) FILTER (WHERE status='failed') AS failed, COUNT(*) FILTER (WHERE status='accepted') AS accepted`).Scan(&counts).Error
	page.Pending, page.Blocked, page.Failed, page.Accepted = counts.Pending, counts.Blocked, counts.Failed, counts.Accepted
	return page, err
}
func (s *Service) Retry(ctx context.Context, deliveryID string) error {
	res := s.db.WithContext(ctx).Model(&deliveryRow{}).Where("id=? AND status IN ? AND expires_at>?", deliveryID, []string{"failed", "blocked_smtp", "blocked_unverified"}, time.Now().UTC()).
		Updates(map[string]any{"status": "pending", "attempts": 0, "last_error": "", "available_at": time.Now().UTC(), "lease_id": nil})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrIneligible
	}
	return nil
}
