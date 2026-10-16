package portal

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

func (a *Application) inviteStaff(c *gin.Context, op organization.Operator, actor string) {
	var in struct{ Email, Role string }
	if !body(c, &in) {
		return
	}
	if !a.Mail.Available() {
		handleError(c, incentives.ErrNotReady)
		return
	}
	ctx := c.Request.Context()
	raw, e := a.command(c, "staff-invite:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		inv, e := a.Organization.InviteStaff(ctx, tx, actor, op.ID, in.Email, in.Role)
		if e != nil {
			return nil, e
		}
		proof, e := pc.RandomToken("staff_")
		if e != nil {
			return nil, e
		}
		record := challenge{ID: uuid.NewString(), IntentID: inv.ID, BrandID: brand(c).ID, Kind: "staff_invitation", DisplayName: inv.Email, ProofHash: pc.HashToken(proof), ExpiresAt: inv.ExpiresAt, CreatedAt: time.Now().UTC()}
		if e = tx.Create(&record).Error; e != nil {
			return nil, e
		}
		token := record.ID + "." + proof + "." + inv.Token
		scheme := "https"
		if !a.Config.IsProduction() {
			scheme = "http"
		}
		message := mail.Message{ID: uuid.NewString(), To: inv.Email, Brand: brand(c).Name, Subject: "经营人员邀请", Text: "您受邀加入 " + op.Name + "。请使用此邮箱与独立经营人员密码接受邀请：\n" + scheme + "://" + a.requestHost(c) + "/staff-invitation?token=" + url.QueryEscape(token)}
		encoded, _ := json.Marshal(message)
		cipher, e := pc.Seal(a.Config.AppKey, string(encoded))
		if e != nil {
			return nil, e
		}
		row := queuedMessage{ID: message.ID, SubjectID: record.ID, Ciphertext: cipher, Status: "pending", ExpiresAt: inv.ExpiresAt, CreatedAt: time.Now().UTC()}
		if e = tx.Create(&row).Error; e != nil {
			return nil, e
		}
		payload, _ := json.Marshal(map[string]string{"message_id": message.ID})
		_, e = a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-mail", Kind: "staff_invitation", BusinessKey: message.ID, Payload: payload, MaxAttempts: 12})
		return commandResult(inv.ID, "invitation_queued"), e
	})
	jsonResponse(c, raw, e)
}
func (a *Application) acceptStaff(c *gin.Context) {
	var in struct {
		Token, Password string
		DisplayName     string `json:"display_name"`
	}
	if !body(c, &in) {
		return
	}
	if a.throttle(c.Request.Context(), "staff-accept:"+c.ClientIP()) != nil {
		fail(c, 429, "auth_rate", "请求过于频繁", false)
		return
	}
	ctx := c.Request.Context()
	raw, e := a.command(c, "staff-accept:"+brand(c).ID, in, func(tx *gorm.DB) (any, error) {
		ch, proof, e := a.verifiedChallenge(c, tx, in.Token, "staff_invitation")
		if e != nil {
			return nil, e
		}
		email := strings.ToLower(strings.TrimSpace(ch.DisplayName))
		var staff Staff
		e = tx.Where("email=?", email).Take(&staff).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			hash, err := organization.HashPassword(in.Password)
			if err != nil {
				return nil, err
			}
			staff = Staff{ID: uuid.NewString(), Email: email, DisplayName: in.DisplayName, PasswordHash: hash, Version: 1, Status: "active", EmailVerified: true, CreatedAt: time.Now()}
			if staff.DisplayName == "" {
				staff.DisplayName = email
			}
			if e = tx.Create(&staff).Error; e != nil {
				return nil, e
			}
		} else if e != nil {
			return nil, e
		} else if staff.Status != "active" || !organization.CheckPassword(staff.PasswordHash, in.Password) {
			return nil, organization.ErrInvalidCredentials
		}
		if _, e = a.Organization.AcceptStaffInvitation(ctx, tx, proof, organization.VerifiedStaff{ID: staff.ID, Email: staff.Email, EmailVerified: true}); e != nil {
			return nil, e
		}
		if e = tx.Model(&challenge{}).Where("id=?", ch.ID).Update("completed_at", time.Now()).Error; e != nil {
			return nil, e
		}
		return commandResult(staff.ID, "staff_invitation_accepted"), nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) publishKOL(c *gin.Context) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	id := subject(c)
	out, e := a.command(c, "kol:"+id.Customer.Customer.ID, raw, func(tx *gorm.DB) (any, error) {
		var in incentives.CampaignInput
		if json.Unmarshal(raw, &in) != nil {
			return nil, incentives.ErrInvalidInput
		}
		in.OperatorID = id.Customer.Ownership.OperatorID
		in.ActorStaffID = ""
		in.OwnerCustomerID = id.Customer.Customer.ID
		in.Kind = "kol"
		out, e := a.Incentives.PublishKOLCampaign(c.Request.Context(), tx, id.Customer.Customer.ID, in)
		return gin.H{"id": out.CampaignID, "status": "published", "version": out.Version}, e
	})
	jsonResponse(c, out, e)
}
func (a *Application) emailChange(c *gin.Context) {
	var in struct {
		CurrentPassword string `json:"current_password"`
		NewEmail        string `json:"new_email"`
	}
	if !body(c, &in) {
		return
	}
	if !a.Mail.Available() {
		handleError(c, incentives.ErrNotReady)
		return
	}
	id := subject(c)
	raw, e := a.command(c, "email-change:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		ch, e := a.Organization.BeginEmailChange(c.Request.Context(), tx, id.Customer.Customer.ID, in.CurrentPassword, in.NewEmail)
		if e != nil {
			return nil, e
		}
		e = a.queueChallenge(c.Request.Context(), tx, c, ch, "email_change", "")
		return commandResult(ch.Intent.ID, "verification_queued"), e
	})
	jsonResponse(c, raw, e)
}
