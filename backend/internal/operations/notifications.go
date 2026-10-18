package operations

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type Notification struct {
	ID          string     `json:"id"`
	OperatorID  string     `json:"operator_id"`
	RecipientID string     `json:"recipient_id"`
	BusinessKey string     `json:"business_key"`
	Category    string     `json:"category"`
	Severity    string     `json:"severity"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Reference   string     `json:"reference"`
	Fingerprint string     `json:"-"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
}

func (Notification) TableName() string { return "operations_notifications" }

type Preference struct {
	OperatorID  string    `gorm:"primaryKey" json:"operator_id"`
	RecipientID string    `gorm:"primaryKey" json:"recipient_id"`
	Marketing   bool      `json:"marketing"`
	UsageEmail  bool      `json:"usage_email"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Preference) TableName() string { return "operations_preferences" }

// Emit is a trusted domain hook. User-facing announcements must use Announce.
// Sensitive request content and credentials must never be passed into a notice.
func (s *Service) Emit(ctx context.Context, tx *gorm.DB, in Notification) (Notification, error) {
	var out Notification
	if !validID(in.OperatorID) || !validID(in.RecipientID) || in.BusinessKey == "" || len(in.BusinessKey) > 200 || strings.TrimSpace(in.Title) == "" || len(in.Title) > 300 || len(in.Body) > 16000 || (in.Category != "mandatory" && in.Category != "usage" && in.Category != "marketing") || (in.Severity != "info" && in.Severity != "warning" && in.Severity != "critical") {
		return out, ErrInvalid
	}
	fp := hash([]string{in.Category, in.Severity, in.Title, in.Body, in.Reference})
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := lock(t, in.OperatorID+in.RecipientID+in.BusinessKey); e != nil {
			return e
		}
		q := t.Where("operator_id=? AND recipient_id=? AND business_key=?", in.OperatorID, in.RecipientID, in.BusinessKey).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.Fingerprint != fp {
				return ErrConflict
			}
			return nil
		}
		recipient, e := s.self(ctx, t, in.RecipientID, in.OperatorID)
		if e != nil {
			return e
		}
		pref := Preference{Marketing: false, UsageEmail: true}
		if e = t.Where("operator_id=? AND recipient_id=?", in.OperatorID, in.RecipientID).Limit(1).Find(&pref).Error; e != nil {
			return e
		}
		in.ID = uuid.NewString()
		in.Fingerprint = fp
		in.CreatedAt = time.Now().UTC()
		in.ReadAt = nil
		out = in
		if e = t.Create(&out).Error; e != nil {
			return e
		}
		state := "pending"
		code := ""
		cipher := ""
		if (in.Category == "marketing" && !pref.Marketing) || (in.Category == "usage" && !pref.UsageEmail) {
			state = "suppressed"
			code = "preference"
		} else if !recipient.EmailVerified || !mail.ValidAddress(recipient.Email) {
			state = "needs_attention"
			code = "verified_email_required"
		} else {
			cipher, e = s.seal(mail.Message{ID: out.ID, To: recipient.Email, Brand: recipient.Brand, Subject: in.Title, Text: in.Body})
			if e != nil {
				return e
			}
		}
		return t.Table("operations_deliveries").Create(map[string]any{"notification_id": out.ID, "message_ciphertext": cipher, "state": state, "next_attempt_at": in.CreatedAt, "error_code": code}).Error
	})
	return out, e
}
func (s *Service) Announce(ctx context.Context, actor string, in Notification) (Notification, error) {
	var out Notification
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, in.OperatorID, "operations.manage"); e != nil {
			return e
		}
		var e error
		out, e = s.Emit(ctx, t, in)
		return e
	})
	return out, e
}
func (s *Service) Preferences(ctx context.Context, actor, operator string) (Preference, error) {
	out := Preference{OperatorID: operator, RecipientID: actor, UsageEmail: true}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		return t.Where("operator_id=? AND recipient_id=?", operator, actor).Limit(1).Find(&out).Error
	})
	return out, e
}
func (s *Service) SetPreferences(ctx context.Context, actor, operator string, marketing, usageEmail bool) (Preference, error) {
	out := Preference{OperatorID: operator, RecipientID: actor, Marketing: marketing, UsageEmail: usageEmail, UpdatedAt: time.Now().UTC()}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		return t.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "operator_id"}, {Name: "recipient_id"}}, DoUpdates: clause.AssignmentColumns([]string{"marketing", "usage_email", "updated_at"})}).Create(&out).Error
	})
	return out, e
}
func (s *Service) ListNotifications(ctx context.Context, actor, operator string, before *time.Time, limit int) ([]Notification, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	out := []Notification{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		q := t.Where("operator_id=? AND recipient_id=?", operator, actor)
		if before != nil {
			q = q.Where("created_at<?", before.UTC())
		}
		return q.Order("created_at DESC,id DESC").Limit(limit).Find(&out).Error
	})
	return out, e
}
func (s *Service) ReadNotification(ctx context.Context, actor, operator, id string) error {
	return s.within(ctx, nil, func(t *gorm.DB) error {
		if _, e := s.self(ctx, t, actor, operator); e != nil {
			return e
		}
		r := t.Model(&Notification{}).Where("id=? AND operator_id=? AND recipient_id=?", id, operator, actor).Update("read_at", time.Now().UTC())
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrForbidden
		}
		return nil
	})
}

type Delivery struct {
	NotificationID string     `json:"notification_id"`
	State          string     `json:"state"`
	Attempts       int        `json:"attempts"`
	ErrorCode      string     `json:"error_code,omitempty"`
	AcceptedAt     *time.Time `json:"accepted_at,omitempty"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
}

func (s *Service) ListDeliveries(ctx context.Context, actor, operator string, limit int) ([]Delivery, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	out := []Delivery{}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		return t.Table("operations_deliveries d").Select("d.notification_id,d.state,d.attempts,d.error_code,d.accepted_at,d.next_attempt_at,d.lease_until").Joins("JOIN operations_notifications n ON n.id=d.notification_id").Where("n.operator_id=?", operator).Order("n.created_at DESC").Limit(limit).Scan(&out).Error
	})
	return out, e
}

// RunNotifications uses at-least-once SMTP submission. A server can accept DATA
// before the worker crashes; stable Message-ID helps clients deduplicate, but
// accepted is deliberately never reported as delivered or read.
func (s *Service) RunNotifications(ctx context.Context, limit int) ([]Delivery, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var jobs []struct {
		NotificationID, MessageCiphertext string
		Attempts                          int
	}
	e := s.DB.WithContext(ctx).Transaction(func(t *gorm.DB) error {
		now := time.Now().UTC()
		if e := t.Table("operations_deliveries").Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).Where("(state='pending' AND next_attempt_at<=?) OR (state='processing' AND lease_until<?)", now, now).Order("next_attempt_at").Limit(limit).Find(&jobs).Error; e != nil {
			return e
		}
		for _, j := range jobs {
			if e := t.Table("operations_deliveries").Where("notification_id=?", j.NotificationID).Updates(map[string]any{"state": "processing", "lease_until": now.Add(45 * time.Second)}).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	out := []Delivery{}
	for _, j := range jobs {
		var msg mail.Message
		callErr := s.open(j.MessageCiphertext, &msg)
		suppressed := false
		suppressionCode := "preference"
		if callErr == nil {
			// Preferences and the verified address can change while queued.
			var n Notification
			callErr = s.DB.WithContext(ctx).First(&n, "id=?", j.NotificationID).Error
			if callErr == nil && strings.HasPrefix(n.BusinessKey, restrictionReviewKey) {
				var current bool
				current, callErr = s.restrictionReviewCurrent(ctx, n)
				if callErr == nil && !current {
					suppressed = true
					suppressionCode = "review_resolved_or_access_changed"
				}
			}
			if callErr == nil && !suppressed {
				recipient, resolveErr := s.self(ctx, s.DB, n.RecipientID, n.OperatorID)
				callErr = resolveErr
				pref := Preference{UsageEmail: true}
				if callErr == nil {
					callErr = s.DB.WithContext(ctx).Where("operator_id=? AND recipient_id=?", n.OperatorID, n.RecipientID).Limit(1).Find(&pref).Error
				}
				suppressed = (n.Category == "marketing" && !pref.Marketing) || (n.Category == "usage" && !pref.UsageEmail)
				if callErr == nil && (!recipient.EmailVerified || !mail.ValidAddress(recipient.Email)) {
					callErr = ErrNotReady
				}
				msg.To = recipient.Email
				msg.Brand = recipient.Brand
			}
		}
		if callErr == nil && !suppressed {
			if !mail.Available(s.Mail) {
				callErr = ErrNotReady
			} else {
				work, cancel := context.WithTimeout(ctx, 25*time.Second)
				callErr = s.Mail.Send(work, msg)
				cancel()
			}
		}
		d := Delivery{NotificationID: j.NotificationID, State: "accepted", Attempts: j.Attempts + 1}
		updates := map[string]any{"attempts": d.Attempts, "lease_until": nil, "error_code": ""}
		if suppressed {
			d.State = "suppressed"
			updates["error_code"] = suppressionCode
		} else if callErr == nil {
			at := time.Now().UTC()
			d.AcceptedAt = &at
			updates["accepted_at"] = at
		} else {
			d.State = "pending"
			d.ErrorCode = "submission_failed"
			if callErr == ErrNotReady {
				d.ErrorCode = "smtp_unavailable"
			}
			if d.Attempts >= 12 {
				d.State = "needs_attention"
			}
			power := j.Attempts
			if power > 10 {
				power = 10
			}
			updates["next_attempt_at"] = time.Now().UTC().Add(time.Minute * time.Duration(1<<power))
			updates["error_code"] = d.ErrorCode
		}
		updates["state"] = d.State
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		dbErr := s.DB.WithContext(persist).Table("operations_deliveries").Where("notification_id=? AND state='processing'", j.NotificationID).Updates(updates).Error
		cancel()
		if dbErr != nil {
			return out, dbErr
		}
		out = append(out, d)
	}
	return out, nil
}
func (s *Service) RetryDelivery(ctx context.Context, actor, operator, id string) error {
	return s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.require(ctx, t, actor, operator, "operations.manage"); e != nil {
			return e
		}
		var n Notification
		if e := t.First(&n, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		recipient, e := s.self(ctx, t, n.RecipientID, n.OperatorID)
		if e != nil {
			return e
		}
		if !recipient.EmailVerified || !mail.ValidAddress(recipient.Email) {
			return ErrNotReady
		}
		ciphertext, e := s.seal(mail.Message{ID: n.ID, To: recipient.Email, Brand: recipient.Brand, Subject: n.Title, Text: n.Body})
		if e != nil {
			return e
		}
		result := t.Table("operations_deliveries").Where("notification_id=? AND state='needs_attention'", id).Updates(map[string]any{"state": "pending", "attempts": 0, "next_attempt_at": time.Now().UTC(), "error_code": "", "message_ciphertext": ciphertext})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		return nil
	})
}

// GetDelivery is a trusted worker read for durable acknowledgement workflows.
// SMTP acceptance is returned exactly as recorded; it is not mailbox delivery.
func (s *Service) GetDelivery(ctx context.Context, tx *gorm.DB, notificationID string) (Delivery, error) {
	var out Delivery
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Table("operations_deliveries").Select("notification_id,state,attempts,error_code,accepted_at,next_attempt_at,lease_until").Where("notification_id=?", notificationID).Take(&out).Error
	})
	return out, e
}
func (s *Service) GetNotificationRead(ctx context.Context, tx *gorm.DB, notificationID string) (*time.Time, error) {
	var out struct{ ReadAt *time.Time }
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Model(&Notification{}).Select("read_at").Where("id=?", notificationID).Take(&out).Error
	})
	return out.ReadAt, e
}
