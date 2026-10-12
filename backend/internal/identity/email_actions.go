package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

const (
	EmailVerify = "verify_email"
	EmailReset  = "reset_password"
)

var ErrEmailUnavailable = errors.New("email delivery is not configured")
var ErrEmailLink = errors.New("email link is invalid or expired")

// Origin is chosen by the HTTP adapter from configured origins, never user input.
type EmailActionContext struct{ Origin, Locale string }
type EmailActionInput struct {
	BrandID, Email, UserID, Purpose string
	Context                         EmailActionContext
}

type emailActionRow struct {
	ID, UserID, BrandID, Email, Purpose, TokenHash string
	ExpiresAt, CreatedAt                           time.Time
	ConsumedAt                                     *time.Time
}

func (emailActionRow) TableName() string { return "identity_email_actions" }

type emailDeliveryRow struct {
	ID, UserID                        string
	ActionID                          *string
	PayloadCiphertext, Status         string
	Attempts                          int
	AvailableAt, ExpiresAt, CreatedAt time.Time
	LeaseID, LastError                *string
	SentAt                            *time.Time
}

func (emailDeliveryRow) TableName() string { return "identity_email_deliveries" }

func (s *Service) SetEmailSender(sender mail.Sender, encryptionKey string) {
	s.emailSender, s.emailKey = sender, encryptionKey
}
func (s *Service) EmailAvailable() bool {
	return s.emailSender != nil && strings.TrimSpace(s.emailKey) != ""
}

// RequestEmailAction deliberately returns the same result for missing, disabled,
// verified and cooling-down accounts. Sending happens later in the durable queue.
func (s *Service) RequestEmailAction(ctx context.Context, in EmailActionInput) error {
	if !s.EmailAvailable() {
		return ErrEmailUnavailable
	}
	if in.Purpose != EmailVerify && in.Purpose != EmailReset {
		return ErrEmailLink
	}
	email := normalizeEmail(in.Email)
	if !mail.ValidAddress(email) {
		return ErrInvalidCredentials
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userRow
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id = ? AND lower(email) = ? AND status = ?", in.BrandID, email, "active")
		if in.UserID != "" {
			query = query.Where("id = ?", in.UserID)
		}
		if err := query.First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		scoped := *s
		scoped.db = tx
		disabled, err := scoped.staffDisabled(ctx, user.ID)
		if err != nil {
			return err
		}
		if disabled {
			return nil
		}
		if in.Purpose == EmailVerify && user.EmailVerifiedAt != nil {
			return nil
		}
		var recent int64
		if err := tx.Model(&emailActionRow{}).Where("user_id = ? AND purpose = ? AND created_at > ?", user.ID, in.Purpose, time.Now().UTC().Add(-time.Minute)).Count(&recent).Error; err != nil {
			return err
		}
		if recent > 0 {
			return nil
		}
		return s.queueEmailAction(tx, user, in.Purpose, in.Context)
	})
}

func (s *Service) queueEmailAction(tx *gorm.DB, user userRow, purpose string, delivery EmailActionContext) error {
	if !s.EmailAvailable() {
		return ErrEmailUnavailable
	}
	origin, err := url.Parse(delivery.Origin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") || (origin.Scheme != "https" && origin.Scheme != "http") {
		return ErrEmailUnavailable
	}
	var brand brandRow
	if err := tx.Where("id = ?", deref(user.BrandID)).First(&brand).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := s.invalidateEmailActions(tx, user.ID, purpose, now); err != nil {
		return err
	}
	raw, err := crypto.RandomToken("themail_")
	if err != nil {
		return err
	}
	expires := now.Add(24 * time.Hour)
	path := "/verify-email"
	if purpose == EmailReset {
		expires = now.Add(30 * time.Minute)
		path = "/reset-password"
	}
	row := emailActionRow{ID: id.New("ema"), UserID: user.ID, BrandID: deref(user.BrandID), Email: user.Email, Purpose: purpose, TokenHash: crypto.HashToken(raw), ExpiresAt: expires, CreatedAt: now}
	if err := tx.Create(&row).Error; err != nil {
		return err
	}
	origin.Path = path
	origin.Fragment = "token=" + raw
	locale := delivery.Locale
	if locale == "" {
		locale = user.Locale
	}
	message := emailActionMessage(user.Email, brand.Name, purpose, localeOrDefault(locale), origin.String())
	return s.enqueueEmail(tx, user.ID, &row.ID, message, expires)
}

func (s *Service) enqueueEmail(tx *gorm.DB, userID string, actionID *string, message mail.Message, expires time.Time) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	ciphertext, err := crypto.Seal(s.emailKey, string(body))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return tx.Create(&emailDeliveryRow{ID: id.New("eml"), UserID: userID, ActionID: actionID, PayloadCiphertext: ciphertext, Status: "pending", AvailableAt: now, ExpiresAt: expires, CreatedAt: now}).Error
}

func (s *Service) invalidateEmailActions(tx *gorm.DB, userID, purpose string, now time.Time) error {
	actions := tx.Model(&emailActionRow{}).Select("id").Where("user_id = ? AND purpose = ?", userID, purpose)
	if err := tx.Model(&emailDeliveryRow{}).Where("action_id IN (?) AND status IN ?", actions, []string{"pending", "sending"}).Updates(map[string]any{"status": "cancelled", "payload_ciphertext": "", "lease_id": nil}).Error; err != nil {
		return err
	}
	return tx.Model(&emailActionRow{}).Where("user_id = ? AND purpose = ? AND consumed_at IS NULL", userID, purpose).Update("consumed_at", now).Error
}

// CompleteEmailAction is one-use and locks the user before the action, just like
// resend and password changes. Email proof alone never creates a login session.
func (s *Service) CompleteEmailAction(ctx context.Context, brandID, purpose, token, password string) (string, error) {
	if (purpose != EmailVerify && purpose != EmailReset) || !strings.HasPrefix(token, "themail_") || len(token) != 56 {
		return "", ErrEmailLink
	}
	var action emailActionRow
	if err := s.db.WithContext(ctx).Where("brand_id = ? AND purpose = ? AND token_hash = ? AND consumed_at IS NULL AND expires_at > ?", brandID, purpose, crypto.HashToken(token), time.Now().UTC()).First(&action).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrEmailLink
		}
		return "", err
	}
	var hash string
	if purpose == EmailReset {
		var err error
		hash, err = HashPassword(password)
		if err != nil {
			return "", err
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user userRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND brand_id = ? AND email = ? AND status = ?", action.UserID, brandID, action.Email, "active").First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEmailLink
			}
			return err
		}
		scoped := *s
		scoped.db = tx
		disabled, err := scoped.staffDisabled(ctx, user.ID)
		if err != nil {
			return err
		}
		if disabled {
			return ErrEmailLink
		}
		now := time.Now().UTC()
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND consumed_at IS NULL AND expires_at > ?", action.ID, now).First(&action).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEmailLink
			}
			return err
		}
		updates := map[string]any{"email_verified_at": now, "updated_at": now}
		if purpose == EmailReset {
			updates["password_hash"] = hash
			if err := tx.Model(&tokenRow{}).Where("user_id = ? AND status = ?", user.ID, "active").Update("status", "revoked").Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&userRow{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := s.invalidateEmailActions(tx, user.ID, purpose, now); err != nil {
			return err
		}
		if purpose == EmailReset {
			if err := s.invalidateEmailActions(tx, user.ID, EmailVerify, now); err != nil {
				return err
			}
			if s.EmailAvailable() {
				var brand brandRow
				if err := tx.Where("id = ?", brandID).First(&brand).Error; err != nil {
					return err
				}
				message := emailActionMessage(user.Email, brand.Name, "password_changed", localeOrDefault(user.Locale), "")
				return s.enqueueEmail(tx, user.ID, nil, message, now.Add(24*time.Hour))
			}
		}
		return nil
	})
	return action.UserID, err
}

func emailActionMessage(email, brand, purpose, locale, link string) mail.Message {
	var subject, text string
	switch locale {
	case "en":
		switch purpose {
		case EmailVerify:
			subject = "Verify your email"
			text = "Confirm your email for %s using this link (valid for 24 hours):\n\n%s\n\nIf you did not request this email, you can ignore it."
		case EmailReset:
			subject = "Reset your password"
			text = "Reset your %s account password using this link (valid for 30 minutes):\n\n%s\n\nIf you did not request a password reset, ignore this email. Your password has not changed."
		default:
			subject = "Your password was changed"
			text = "Your password for %s was changed and previous login sessions were revoked. If this was not you, reset your password immediately."
		}
	case "ja":
		switch purpose {
		case EmailVerify:
			subject = "メールアドレスの確認"
			text = "%s のメールアドレスを次のリンクで確認してください（24時間有効）：\n\n%s\n\n心当たりがない場合は、このメールを無視してください。"
		case EmailReset:
			subject = "パスワードの再設定"
			text = "%s のパスワードを次のリンクで再設定してください（30分間有効）：\n\n%s\n\n心当たりがない場合は無視してください。パスワードは変更されていません。"
		default:
			subject = "パスワードが変更されました"
			text = "%s のパスワードが変更され、以前のログインセッションは無効になりました。ご本人でない場合は、直ちにパスワードを再設定してください。"
		}
	default:
		switch purpose {
		case EmailVerify:
			subject = "验证邮箱"
			text = "请确认你在 %s 使用的邮箱，点击下方链接完成验证（24 小时内有效）：\n\n%s\n\n如果不是你本人申请，请忽略此邮件。"
		case EmailReset:
			subject = "重置密码"
			text = "请通过下方链接重置你在 %s 的账号密码（30 分钟内有效）：\n\n%s\n\n如果不是你本人申请，请忽略此邮件，你的密码尚未更改。"
		default:
			subject = "密码已更新"
			text = "你在 %s 的密码已更新，旧登录会话已撤销。如果不是本人操作，请立即重新申请密码找回。"
		}
	}
	if purpose == "password_changed" {
		text = fmt.Sprintf(text, brand)
	} else {
		text = fmt.Sprintf(text, brand, link)
	}
	return mail.Message{To: email, Brand: brand, Subject: "[" + brand + "] " + subject, Text: text}
}
