package operations

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

// Language follows the recipient's existing UI selector. It never changes
// subscription consent or the language of operator-authored announcements.
func (s *Service) SetMailLanguage(ctx context.Context, actor, operator, language string) error {
	if language != "zh" && language != "en" && language != "ja" && language != "ru" {
		return ErrInvalid
	}
	return s.within(ctx, nil, func(tx *gorm.DB) error {
		if _, err := s.self(ctx, tx, actor, operator); err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO operations_preferences(operator_id,recipient_id,marketing,usage_email,language,updated_at) VALUES(?,?,false,true,?,clock_timestamp()) ON CONFLICT(operator_id,recipient_id) DO UPDATE SET language=excluded.language,updated_at=excluded.updated_at WHERE operations_preferences.language<>excluded.language`, operator, actor, language).Error
	})
}

func (s *Service) notificationMail(tx *gorm.DB, n Notification, recipient Recipient) (mail.Message, error) {
	message := mail.Message{ID: n.ID, To: recipient.Email, Brand: recipient.Brand, BrandID: recipient.BrandID, Subject: n.Title, Text: n.Body}
	if !n.SystemGenerated() {
		return message, nil
	}
	var pref Preference
	if err := tx.Where("operator_id=? AND recipient_id=?", n.OperatorID, n.RecipientID).Limit(1).Find(&pref).Error; err != nil {
		return message, err
	}
	message.Subject = httpx.LocalizedSystemMessage(pref.Language, n.Title)
	message.Text = httpx.LocalizedSystemMessage(pref.Language, n.Body)
	return message, nil
}
