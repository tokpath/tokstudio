package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
	"gorm.io/gorm"
)

func (a *Application) brandDTO(ctx context.Context, b organization.Brand) gin.H {
	theme, e := a.Branding.ResolveTheme(ctx, nil, b.ID, "")
	if e != nil {
		return gin.H{"id": b.ID, "name": b.Name, "api_base_url": a.brandAPIEndpoint(ctx, b.ID), "theme": gin.H{}}
	}
	return gin.H{"id": b.ID, "name": b.Name, "api_base_url": a.brandAPIEndpoint(ctx, b.ID), "theme": theme.Theme}
}
func (a *Application) operatorDTO(ctx context.Context, op organization.Operator) gin.H {
	result := gin.H{"id": op.ID, "name": op.Name, "kind": op.Kind, "brand_id": op.BrandID, "legal_name": op.LegalName}
	theme, e := a.Branding.ResolveTheme(ctx, nil, op.BrandID, channelThemeID(op))
	if e == nil {
		result["theme"] = theme.Theme
	}
	return result
}
func (a *Application) sessionResponse(c *gin.Context) {
	id := subject(c)
	if id == nil {
		c.JSON(200, gin.H{"kind": "anonymous"})
		return
	}
	ctx := c.Request.Context()
	if id.Session.Kind == "customer" {
		b, e := a.Organization.GetBrand(ctx, nil, id.Customer.Customer.BrandID)
		if e != nil {
			handleError(c, e)
			return
		}
		op, e := a.Organization.GetOperator(ctx, nil, id.Customer.Ownership.OperatorID)
		if e != nil {
			handleError(c, e)
			return
		}
		email, e := a.Organization.CustomerEmail(ctx, nil, id.Customer.Customer.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		c.JSON(200, gin.H{"kind": "customer", "customer": gin.H{"id": id.Customer.Customer.ID, "email": email, "display_name": id.Customer.Customer.DisplayName}, "brand": a.brandDTO(ctx, b), "operator": a.operatorDTO(ctx, op), "csrf_token": a.csrf(id.Token, id.Session.BrandID)})
		return
	}
	workspaces, e := a.Organization.MembershipsForStaff(ctx, nil, id.Staff.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, ws := range workspaces {
		items = append(items, gin.H{"operator": a.operatorDTO(ctx, ws.Operator), "permissions": navigationPermissions(ws.Permissions, ws.Operator.Kind)})
	}
	c.JSON(200, gin.H{"kind": "staff", "staff": gin.H{"id": id.Staff.ID, "email": id.Staff.Email, "display_name": id.Staff.DisplayName}, "workspaces": items, "requires_password_change": id.Staff.MustChangePassword, "csrf_token": a.csrf(id.Token, id.Session.BrandID)})
}
func navigationPermissions(domain []string, kind organization.OperatorKind) []string {
	out := append([]string{}, domain...)
	allowed := map[string]bool{}
	for _, permission := range domain {
		allowed[permission] = true
		if permission == "*" {
			return []string{"*"}
		}
	}
	resources := []string{"overview", "customers", "plans", "orders", "payments", "payment-methods", "evidence", "valuations", "receipt-valuations", "pricing", "gifts", "marketing", "commissions", "assets", "ledger", "refunds", "withdrawals", "disputes", "invoices", "reconciliation", "reports", "exports", "operators", "contracts", "procurement", "procurement-invoices", "coverage", "alerts", "takeovers", "takeover-settlements", "catalog-changes", "exits", "branding", "domains", "channel-themes", "staff", "approvals", "risk", "restrictions", "notifications", "business-status", "audit", "retention", "payment-assets", "providers", "catalog", "routes", "routing", "requests", "jobs", "mail"}
	for _, resource := range resources {
		if allowed[permissionFor(resource)] || resource == "orders" && allowed["payments.manage"] || resource == "refunds" && allowed["payments.manage"] || resource == "exits" && (allowed["payments.manage"] || allowed["refunds.manage"]) {
			out = append(out, resource+".read")
		}
	}
	return out
}
func (a *Application) throttle(ctx context.Context, scope string) error {
	var count int
	e := a.DB.WithContext(ctx).Raw(`INSERT INTO portal_security_attempts(key,window_start,attempts) VALUES (?,now(),1)
 ON CONFLICT(key) DO UPDATE SET attempts=CASE WHEN portal_security_attempts.window_start<now()-interval '10 minutes' THEN 1 ELSE portal_security_attempts.attempts+1 END,
 window_start=CASE WHEN portal_security_attempts.window_start<now()-interval '10 minutes' THEN now() ELSE portal_security_attempts.window_start END RETURNING attempts`, pc.HashToken(scope)).Scan(&count).Error
	if e != nil {
		return e
	}
	if count > 20 {
		return errors.New("authentication rate exceeded")
	}
	return nil
}
func (a *Application) login(c *gin.Context) {
	var in struct{ Kind, Email, Password string }
	if !body(c, &in) {
		return
	}
	if _, ok := key(c); !ok {
		return
	}
	ctx := c.Request.Context()
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if a.throttle(ctx, "login:"+c.ClientIP()+":"+email) != nil {
		fail(c, 429, "auth_rate", "登录尝试过多，请稍后重试", false)
		return
	}
	if in.Kind == "" {
		var candidate Staff
		staffErr := a.DB.WithContext(ctx).First(&candidate, "email=? AND status='active'", email).Error
		if staffErr != nil && !errors.Is(staffErr, gorm.ErrRecordNotFound) {
			handleError(c, staffErr)
			return
		}
		staffMatch := staffErr == nil && organization.CheckPassword(candidate.PasswordHash, in.Password)
		_, customerErr := a.Organization.AuthenticateCredentials(ctx, nil, brand(c).ID, email, in.Password)
		if customerErr != nil && !errors.Is(customerErr, organization.ErrInvalidCredentials) && !errors.Is(customerErr, organization.ErrBusinessClosed) && !errors.Is(customerErr, organization.ErrFrozen) {
			handleError(c, customerErr)
			return
		}
		customerMatch := customerErr == nil
		if staffMatch && customerMatch {
			c.JSON(200, gin.H{"kind": "workspace_choice", "choices": []string{"customer", "staff"}})
			return
		}
		if staffMatch {
			in.Kind = "staff"
		} else if customerMatch {
			in.Kind = "customer"
		}
	}
	var id identity
	now := time.Now().UTC()
	var e error
	if in.Kind == "customer" {
		var result organization.LoginResult
		result, e = a.Organization.Login(ctx, nil, brand(c).ID, email, in.Password)
		if e == nil {
			id.Customer = result.Principal
			id.Token = result.AccessToken
			id.Session = browserSession{TokenHash: pc.HashToken(id.Token), Kind: "customer", BrandID: brand(c).ID, SubjectID: result.Principal.Customer.ID, CredentialVersion: result.Principal.CredentialVersion, ExpiresAt: result.ExpiresAt, CreatedAt: now}
		}
	} else if in.Kind == "staff" {
		e = a.DB.WithContext(ctx).First(&id.Staff, "email=? AND status='active'", email).Error
		if e == nil && !organization.CheckPassword(id.Staff.PasswordHash, in.Password) {
			e = organization.ErrInvalidCredentials
		}
		if e == nil {
			id.Token, e = pc.RandomToken("ths_")
			id.Session = browserSession{TokenHash: pc.HashToken(id.Token), Kind: "staff", BrandID: brand(c).ID, SubjectID: id.Staff.ID, CredentialVersion: id.Staff.Version, ExpiresAt: now.Add(12 * time.Hour), CreatedAt: now}
		}
	} else {
		e = organization.ErrInvalidCredentials
	}
	if e != nil {
		fail(c, 401, "invalid_credentials", "邮箱或密码错误，或此身份不可用", false)
		return
	}
	if e = a.DB.WithContext(ctx).Create(&id.Session).Error; e != nil {
		handleError(c, e)
		return
	}
	a.setCookie(c, id.Token, id.Session.ExpiresAt)
	c.Set("identity", &id)
	a.sessionResponse(c)
}
func (a *Application) logout(c *gin.Context) {
	id := subject(c)
	e := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if id.Session.Kind == "customer" {
			if err := a.Organization.Logout(c.Request.Context(), tx, id.Token); err != nil {
				return err
			}
		}
		return tx.Delete(&browserSession{}, "token_hash=?", id.Session.TokenHash).Error
	})
	if e != nil {
		handleError(c, e)
		return
	}
	a.setCookie(c, "", time.Now().Add(-time.Hour))
	c.JSON(200, commandResult("session", "logged_out"))
}

type challenge struct {
	ID, IntentID, BrandID, Kind, DisplayName, ProofHash string
	ExpiresAt                                           time.Time
	CompletedAt                                         *time.Time
	CreatedAt                                           time.Time
}

func (challenge) TableName() string { return "portal_challenges" }

type queuedMessage struct {
	ID, SubjectID, Ciphertext, Status string
	DeliveredAt                       *time.Time
	ExpiresAt, CreatedAt              time.Time
}

func (queuedMessage) TableName() string { return "portal_messages" }
func (a *Application) queueChallenge(ctx context.Context, tx *gorm.DB, c *gin.Context, ch organization.VerificationChallenge, kind, name string, continuations ...string) error {
	proof, e := pc.RandomToken("verify_")
	if e != nil {
		return e
	}
	now := time.Now().UTC()
	record := challenge{ID: uuid.NewString(), IntentID: ch.Intent.ID, BrandID: ch.Intent.BrandID, Kind: kind, DisplayName: name, ProofHash: pc.HashToken(proof), ExpiresAt: ch.Intent.ExpiresAt, CreatedAt: now}
	if e = tx.Create(&record).Error; e != nil {
		return e
	}
	// The opaque browser token carries only this portal challenge and its proof;
	// organization verification material is encrypted exclusively in the mailbox.
	token := record.ID + "." + proof + "." + ch.VerificationToken
	path := "/verify-email"
	title := "验证邮箱并完成注册"
	if kind == "password_reset" {
		path = "/reset-password"
		title = "重置客户登录密码"
	}
	if kind == "email_change" {
		title = "验证新的邮箱地址"
	}
	scheme := "https"
	if !a.Config.IsProduction() && c.Request.TLS == nil {
		scheme = "http"
	}
	link := scheme + "://" + a.requestHost(c) + path + "?token=" + url.QueryEscape(token)
	if kind == "registration" && len(continuations) > 0 {
		if next := safeCustomerNext(continuations[0]); next != "" {
			link += "&next=" + url.QueryEscape(next)
		}
	}
	locale := httpx.RequestLocale(c.Request)
	title = httpx.LocalizedText(locale, title)
	message := mail.Message{ID: uuid.NewString(), To: ch.Intent.Email, Brand: brand(c).Name, Subject: title, Text: title + ":\n" + link + httpx.LocalizedText(locale, "\n链接过期后请重新发起。若非本人操作，请忽略。")}
	raw, _ := json.Marshal(message)
	cipher, e := pc.Seal(a.Config.AppKey, string(raw))
	if e != nil {
		return e
	}
	msg := queuedMessage{ID: message.ID, SubjectID: record.ID, Ciphertext: cipher, Status: "pending", ExpiresAt: ch.Intent.ExpiresAt, CreatedAt: now}
	if e = tx.Create(&msg).Error; e != nil {
		return e
	}
	payload, _ := json.Marshal(map[string]string{"message_id": msg.ID})
	_, e = a.Jobs.EnqueueTx(tx, jobs.Input{Queue: "portal-mail", Kind: "verification", BusinessKey: msg.ID, Payload: payload, MaxAttempts: 12})
	return e
}
func (a *Application) register(c *gin.Context) {
	var in struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		InviteCode  string `json:"invite_code"`
		Next        string `json:"next"`
	}
	if !body(c, &in) {
		return
	}
	in.Next = safeCustomerNext(in.Next)
	if !a.Mail.Available() {
		fail(c, 409, "pending_configuration", "当前经营方尚未配置验证邮件，请联系经营方", false)
		return
	}
	if a.throttle(c.Request.Context(), "register:"+c.ClientIP()) != nil {
		fail(c, 429, "auth_rate", "请求过于频繁，请稍后重试", false)
		return
	}
	raw, e := a.command(c, "registration:"+brand(c).ID, in, func(tx *gorm.DB) (any, error) {
		ch, e := a.Organization.BeginRegistration(c.Request.Context(), tx, organization.RegistrationIntentRequest{BrandID: brand(c).ID, Email: in.Email, Password: in.Password, InviteCode: in.InviteCode})
		if e != nil {
			return nil, e
		}
		if _, e = a.Incentives.PrepareRegistrationPromise(c.Request.Context(), tx, ch.Intent); e != nil {
			return nil, e
		}
		if e = a.queueChallenge(c.Request.Context(), tx, c, ch, "registration", in.DisplayName, in.Next); e != nil {
			return nil, e
		}
		op, e := a.Organization.GetOperator(c.Request.Context(), tx, ch.Intent.OperatorID)
		if e != nil {
			return nil, e
		}
		return gin.H{"registration_id": ch.Intent.ID, "verification_required": true, "message": "验证邮件已进入发送队列，请通过邮件完成注册", "operator": a.operatorDTO(c.Request.Context(), op)}, nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) verifiedChallenge(c *gin.Context, tx *gorm.DB, token, kind string) (challenge, string, error) {
	var ch challenge
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ch, "", organization.ErrInvalidVerification
	}
	q := tx.Where("id=? AND brand_id=? AND expires_at>?", parts[0], brand(c).ID, time.Now())
	if kind != "" {
		q = q.Where("kind=?", kind)
	}
	e := q.Take(&ch).Error
	if e != nil || ch.ProofHash != pc.HashToken(parts[1]) {
		return ch, "", organization.ErrInvalidVerification
	}
	return ch, parts[2], nil
}
func (a *Application) verify(c *gin.Context) {
	var in struct{ Token string }
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "verify:"+brand(c).ID, in, func(tx *gorm.DB) (any, error) {
		ch, proof, e := a.verifiedChallenge(c, tx, in.Token, "")
		if e != nil {
			return nil, e
		}
		if ch.Kind == "email_change" {
			if _, e = a.Organization.VerifyEmail(c.Request.Context(), tx, ch.IntentID, proof); e != nil {
				return nil, e
			}
			if e = a.Organization.CompleteEmailChange(c.Request.Context(), tx, ch.IntentID, proof); e != nil {
				return nil, e
			}
			return commandResult(ch.ID, "email_changed"), nil
		}
		if ch.Kind != "registration" {
			return nil, organization.ErrInvalidVerification
		}
		if _, e = a.Organization.VerifyRegistration(c.Request.Context(), tx, ch.IntentID, proof); e != nil {
			return nil, e
		}
		result, e := a.Organization.CompleteRegistration(c.Request.Context(), tx, organization.CompleteRegistrationInput{IntentID: ch.IntentID, VerificationToken: proof, DisplayName: ch.DisplayName}, a.Incentives)
		if e != nil {
			return nil, e
		}
		if e = tx.Model(&challenge{}).Where("id=?", ch.ID).Update("completed_at", time.Now()).Error; e != nil {
			return nil, e
		}
		return commandResult(result.Customer.ID, "verified"), nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) passwordReset(c *gin.Context) {
	var in struct{ Email string }
	if !body(c, &in) {
		return
	}
	if _, ok := key(c); !ok {
		return
	}
	if !a.Mail.Available() {
		fail(c, 409, "pending_configuration", "验证邮件尚未配置", false)
		return
	}
	if a.throttle(c.Request.Context(), "reset:"+c.ClientIP()) != nil {
		fail(c, 429, "auth_rate", "请求过于频繁", false)
		return
	}
	raw, e := a.command(c, "password-reset:"+brand(c).ID, in, func(tx *gorm.DB) (any, error) {
		if e := a.queueStaffPasswordReset(c, tx, in.Email); e != nil {
			return nil, e
		}
		ch, e := a.Organization.BeginPasswordReset(c.Request.Context(), tx, brand(c).ID, in.Email)
		if errors.Is(e, organization.ErrNotFound) || errors.Is(e, organization.ErrInvalidCredentials) {
			return commandResult(uuid.NewString(), "accepted"), nil
		}
		if e != nil {
			return nil, e
		}
		if e = a.queueChallenge(c.Request.Context(), tx, c, ch, "password_reset", ""); e != nil {
			return nil, e
		}
		return commandResult(ch.Intent.ID, "accepted"), nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) passwordResetComplete(c *gin.Context) {
	var in struct{ Token, Password string }
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "password-complete:"+brand(c).ID, in, func(tx *gorm.DB) (any, error) {
		ch, proof, e := a.verifiedChallenge(c, tx, in.Token, "")
		if e != nil {
			return nil, e
		}
		if ch.Kind == "staff_password_reset" {
			if e = a.completeStaffPasswordReset(tx, ch, in.Password); e != nil {
				return nil, e
			}
			return commandResult(ch.ID, "password_reset"), nil
		}
		if ch.Kind != "password_reset" {
			return nil, organization.ErrInvalidVerification
		}
		if _, e = a.Organization.VerifyEmail(c.Request.Context(), tx, ch.IntentID, proof); e != nil {
			return nil, e
		}
		if e = a.Organization.CompletePasswordReset(c.Request.Context(), tx, ch.IntentID, proof, in.Password); e != nil {
			return nil, e
		}
		return commandResult(ch.ID, "password_changed"), nil
	})
	jsonResponse(c, raw, e)
}

// RunMailJobs performs network delivery outside the claim/completion transaction.
// A crash after SMTP acceptance can resend the same stable Message-ID.
func (a *Application) RunMailJobs(ctx context.Context, owner string, limit int) error {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		rows, e := a.Jobs.Claim(ctx, "portal-mail", owner, 30*time.Second, 1)
		if e != nil {
			return e
		}
		if len(rows) == 0 {
			return nil
		}
		for _, job := range rows {
			var payload struct {
				MessageID string `json:"message_id"`
			}
			var msg queuedMessage
			var message mail.Message
			e = json.Unmarshal(job.Payload, &payload)
			if e == nil {
				e = a.DB.WithContext(ctx).First(&msg, "id=?", payload.MessageID).Error
			}
			if e == nil && time.Now().After(msg.ExpiresAt) {
				e = a.Jobs.Complete(ctx, job.ID, owner, job.Fence)
				continue
			}
			if e == nil {
				var plain string
				plain, e = pc.Open(a.Config.AppKey, msg.Ciphertext)
				if e == nil {
					e = json.Unmarshal([]byte(plain), &message)
				}
			}
			if e == nil {
				e = a.Mail.Send(ctx, message)
			}
			if e != nil {
				_ = a.Jobs.Fail(ctx, job.ID, owner, job.Fence, "mail delivery unavailable")
				continue
			}
			e = a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if e := a.Jobs.CompleteTx(tx, job.ID, owner, job.Fence); e != nil {
					return e
				}
				return tx.Model(&queuedMessage{}).Where("id=?", msg.ID).Updates(map[string]any{"status": "accepted", "delivered_at": time.Now(), "ciphertext": ""}).Error
			})
			if e != nil {
				return e
			}
		}
	}
	return nil
}
