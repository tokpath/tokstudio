package portal

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (a *Application) queueStaffPasswordReset(c *gin.Context, tx *gorm.DB, email string) error {
	var staff Staff
	e := tx.Where("email=? AND status='active'", strings.ToLower(strings.TrimSpace(email))).Take(&staff).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	proof, e := pc.RandomToken("reset_")
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	record := challenge{ID: uuid.NewString(), IntentID: staff.ID, BrandID: brand(c).ID, Kind: "staff_password_reset", DisplayName: strconv.FormatInt(staff.Version, 10), ProofHash: pc.HashToken(proof), ExpiresAt: now.Add(30 * time.Minute), CreatedAt: now}
	if e = tx.Create(&record).Error; e != nil {
		return e
	}
	scheme := "https"
	if !a.Config.IsProduction() {
		scheme = "http"
	}
	link := scheme + "://" + a.requestHost(c) + "/reset-password?token=" + url.QueryEscape(record.ID+"."+proof+".staff")
	title := httpx.LocalizedText(httpx.RequestLocale(c.Request), "重置经营人员密码")
	message := mail.Message{ID: uuid.NewString(), To: staff.Email, BrandID: brand(c).ID, Brand: brand(c).Name, Subject: title, Text: title + ":\n" + link + httpx.LocalizedText(httpx.RequestLocale(c.Request), "\n链接过期后请重新发起。若非本人操作，请忽略。")}
	raw, _ := json.Marshal(message)
	cipher, e := pc.Seal(a.Config.AppKey, string(raw))
	if e != nil {
		return e
	}
	row := queuedMessage{ID: message.ID, SubjectID: record.ID, Ciphertext: cipher, Status: "pending", ExpiresAt: record.ExpiresAt, CreatedAt: now}
	if e = tx.Create(&row).Error; e != nil {
		return e
	}
	payload, _ := json.Marshal(map[string]string{"message_id": row.ID})
	_, e = a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-mail", Kind: "staff_password_reset", BusinessKey: row.ID, Payload: payload, MaxAttempts: 12})
	return e
}
func (a *Application) completeStaffPasswordReset(tx *gorm.DB, ch challenge, password string) error {
	var locked challenge
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", ch.ID).Error; e != nil {
		return e
	}
	if locked.CompletedAt != nil {
		return organization.ErrInvalidVerification
	}
	var staff Staff
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&staff, "id=? AND status='active'", locked.IntentID).Error; e != nil {
		return organization.ErrInvalidVerification
	}
	if strconv.FormatInt(staff.Version, 10) != locked.DisplayName {
		return organization.ErrInvalidVerification
	}
	hash, e := organization.HashPassword(password)
	if e != nil {
		return e
	}
	if e = tx.Model(&staff).Updates(map[string]any{"password_hash": hash, "version": staff.Version + 1, "must_change_password": false, "email_verified": true}).Error; e != nil {
		return e
	}
	if e = tx.Where("kind='staff' AND subject_id=?", staff.ID).Delete(&browserSession{}).Error; e != nil {
		return e
	}
	return tx.Model(&locked).Update("completed_at", time.Now().UTC()).Error
}
