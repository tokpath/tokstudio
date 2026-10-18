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

var errStaffPasswordLength = errors.New("staff password length must be 10 to 72 bytes")

func (a *Application) inviteStaff(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Email, Role     string
		DeliveryMode    string `json:"delivery_mode"`
		DisplayName     string `json:"display_name"`
		InitialPassword string `json:"initial_password"`
	}
	if !body(c, &in) {
		return
	}
	if in.DeliveryMode == "" {
		in.DeliveryMode = "manual"
	}
	if in.DeliveryMode != "manual" && in.DeliveryMode != "email" {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	if in.DeliveryMode == "email" && !a.Mail.Available() {
		handleError(c, incentives.ErrNotReady)
		return
	}
	ctx := c.Request.Context()
	raw, e := a.command(c, "staff-invite:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		inv, e := a.Organization.InviteStaff(ctx, tx, actor, op.ID, in.Email, in.Role)
		if e != nil {
			return nil, e
		}
		// Serialize provisioning across all operators; an existing account is never reset.
		if e = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "staff-email:"+inv.Email).Error; e != nil {
			return nil, e
		}
		scheme := "https"
		if !a.Config.IsProduction() {
			scheme = "http"
		}
		origin := scheme + "://" + a.requestHost(c)
		if in.DeliveryMode == "manual" {
			var existing Staff
			e = tx.Where("email=?", inv.Email).Take(&existing).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				hash, err := organization.HashPassword(in.InitialPassword)
				if err != nil {
					if errors.Is(err, organization.ErrInvalidInput) {
						return nil, errStaffPasswordLength
					}
					return nil, err
				}
				staff := Staff{ID: uuid.NewString(), Email: inv.Email, DisplayName: strings.TrimSpace(in.DisplayName), PasswordHash: hash, Version: 1, Status: "active", MustChangePassword: true, CreatedAt: time.Now().UTC()}
				if staff.DisplayName == "" {
					staff.DisplayName = inv.Email
				}
				if e = tx.Create(&staff).Error; e != nil {
					return nil, e
				}
				if _, e = a.Organization.AcceptAuthenticatedStaffInvitation(ctx, tx, inv.Token, staff.ID, staff.Email); e != nil {
					return nil, e
				}
				return gin.H{"id": staff.ID, "status": "account_created", "email": staff.Email, "access_url": origin + "/login", "requires_password_change": true, "message": "账号已开通，请将登录入口、邮箱和初始密码交给负责人；首次登录需要修改密码"}, nil
			}
			if e != nil {
				return nil, e
			}
			if existing.Status != "active" {
				return nil, organization.ErrInvalidCredentials
			}
		}
		proof, e := pc.RandomToken("staff_")
		if e != nil {
			return nil, e
		}
		kind := "staff_invitation"
		if in.DeliveryMode == "manual" {
			kind = "staff_invitation_manual"
		}
		record := challenge{ID: uuid.NewString(), IntentID: inv.ID, BrandID: brand(c).ID, Kind: kind, DisplayName: inv.Email, ProofHash: pc.HashToken(proof), ExpiresAt: inv.ExpiresAt, CreatedAt: time.Now().UTC()}
		if e = tx.Create(&record).Error; e != nil {
			return nil, e
		}
		token := record.ID + "." + proof + "." + inv.Token
		link := origin + "/staff-invitation?token=" + url.QueryEscape(token)
		if in.DeliveryMode == "manual" {
			return gin.H{"id": inv.ID, "status": "invitation_created", "email": inv.Email, "access_url": link, "message": "此邮箱已有经营人员账号，原密码保持不变。请将邀请链接交给本人，72 小时内使用原密码接受邀请"}, nil
		}
		message := mail.Message{ID: uuid.NewString(), To: inv.Email, Brand: brand(c).Name, Subject: "经营人员邀请", Text: "您受邀加入 " + op.Name + "。请使用此邮箱接受邀请：\n" + link}
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
		ch, proof, e := a.verifiedChallenge(c, tx, in.Token, "")
		if e != nil {
			return nil, e
		}
		if ch.Kind != "staff_invitation" && ch.Kind != "staff_invitation_manual" {
			return nil, organization.ErrInvalidVerification
		}
		email := strings.ToLower(strings.TrimSpace(ch.DisplayName))
		if e = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "staff-email:"+email).Error; e != nil {
			return nil, e
		}
		var staff Staff
		e = tx.Where("email=?", email).Take(&staff).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			if ch.Kind == "staff_invitation_manual" {
				return nil, organization.ErrInvalidCredentials
			}
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
		if ch.Kind == "staff_invitation" && !staff.EmailVerified {
			if e = tx.Model(&staff).Update("email_verified", true).Error; e != nil {
				return nil, e
			}
		}
		if _, e = a.Organization.AcceptAuthenticatedStaffInvitation(ctx, tx, proof, staff.ID, staff.Email); e != nil {
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
