package organization

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type intentRow struct {
	ID, BrandID, OperatorID, EmailNormalized string
	InviteCodeID, InviterID                  *string
	VerificationHash, PasswordHash, Purpose  string
	CredentialVersion                        int64
	CustomerID                               *string
	Status                                   string
	ExpiresAt                                time.Time
	VerifiedAt, CompletedAt                  *time.Time
	CreatedAt                                time.Time
}

func (intentRow) TableName() string { return "organization_registration_intents" }
func (r intentRow) DTO() RegistrationIntent {
	return RegistrationIntent{ID: r.ID, BrandID: r.BrandID, OperatorID: r.OperatorID, Email: r.EmailNormalized, InviteCodeID: r.InviteCodeID, InviterID: r.InviterID, Purpose: r.Purpose, Status: r.Status, ExpiresAt: r.ExpiresAt, VerifiedAt: r.VerifiedAt, CustomerID: r.CustomerID}
}

type codeRow struct {
	ID, Code, BrandID, Kind string
	CustomerID, OperatorID  *string
	Enabled                 bool
}

func (codeRow) TableName() string { return "organization_invite_codes" }

// ResolveRegistrationIntent is a preview of an explicit final selection. An
// invalid code is an error; no fallback to brand-direct registration occurs.
func (s *Service) ResolveRegistrationIntent(ctx context.Context, tx *gorm.DB, brandID, code string) (out RegistrationAttribution, err error) {
	b, err := s.GetBrand(ctx, tx, brandID)
	if err != nil {
		return out, err
	}
	if b.Status != "active" {
		return out, ErrBusinessClosed
	}
	out.Brand = b
	opID := b.OwnerOperatorID
	if code = strings.ToUpper(strings.TrimSpace(code)); code != "" {
		var c codeRow
		if err = s.dbFor(ctx, tx).First(&c, "code = ? AND brand_id = ? AND enabled = true", code, brandID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return out, ErrInvalidInviteCode
			}
			return out, err
		}
		out.InviteCodeID = &c.ID
		if c.Kind == "channel" {
			opID = *c.OperatorID
		} else {
			inviter, err := s.GetCustomer(ctx, tx, *c.CustomerID)
			if err != nil {
				return out, err
			}
			if inviter.Status != "active" {
				return out, ErrInvalidInviteCode
			}
			own, err := s.CurrentOwnership(ctx, tx, inviter.ID)
			if err != nil {
				return out, err
			}
			opID = own.OperatorID
			out.InviterID = c.CustomerID
		}
	}
	o, err := s.GetOperator(ctx, tx, opID)
	if err != nil {
		return out, err
	}
	if o.BrandID != brandID {
		return out, ErrInvalidInviteCode
	}
	if err = s.RequireBusiness(ctx, tx, opID, Registration); err != nil {
		return out, err
	}
	out.Operator = o
	return out, nil
}

func (s *Service) BeginRegistration(ctx context.Context, tx *gorm.DB, in RegistrationIntentRequest) (out VerificationChallenge, err error) {
	email, err := NormalizeEmail(in.Email)
	if err != nil {
		return out, err
	}
	passwordHash, err := HashPassword(in.Password)
	if err != nil {
		return out, err
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		a, err := s.ResolveRegistrationIntent(ctx, tx, in.BrandID, in.InviteCode)
		if err != nil {
			return err
		}
		var count int64
		if err = tx.Table("organization_customer_identities").Where("brand_id = ? AND email_normalized = ?", in.BrandID, email).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrEmailAlreadyRegistered
		}
		raw, err := token()
		if err != nil {
			return err
		}
		r := intentRow{ID: newID(), BrandID: in.BrandID, OperatorID: a.Operator.ID, EmailNormalized: email, InviteCodeID: a.InviteCodeID, InviterID: a.InviterID, VerificationHash: tokenHash(raw), PasswordHash: passwordHash, Purpose: "registration", Status: "pending", ExpiresAt: s.now().Add(30 * time.Minute), CreatedAt: s.now()}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = VerificationChallenge{Intent: r.DTO(), VerificationToken: raw}
		return nil
	})
	return
}

func (s *Service) lockedProof(tx *gorm.DB, intentID, raw string) (intentRow, error) {
	if !validID(intentID) || raw == "" {
		return intentRow{}, ErrInvalidVerification
	}
	var r intentRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id = ?", intentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return r, ErrInvalidVerification
		}
		return r, err
	}
	if !tokenMatches(raw, r.VerificationHash) || (!r.ExpiresAt.After(s.now()) && r.Status != "completed") {
		return r, ErrInvalidVerification
	}
	return r, nil
}
func (s *Service) VerifyRegistration(ctx context.Context, tx *gorm.DB, intentID, raw string) (out RegistrationIntent, err error) {
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.lockedProof(tx, intentID, raw)
		if err != nil {
			return err
		}
		if r.Status == "pending" {
			now := s.now()
			if err = tx.Model(&r).Updates(map[string]any{"status": "verified", "verified_at": now}).Error; err != nil {
				return err
			}
			r.Status = "verified"
			r.VerifiedAt = &now
		}
		out = r.DTO()
		return nil
	})
	return
}

// VerifyEmail is also used for email-change and password-reset challenges.
func (s *Service) VerifyEmail(ctx context.Context, tx *gorm.DB, intentID, raw string) (RegistrationIntent, error) {
	return s.VerifyRegistration(ctx, tx, intentID, raw)
}

func (s *Service) CompleteRegistration(ctx context.Context, tx *gorm.DB, in CompleteRegistrationInput, hooks ...RegistrationHook) (out RegistrationResult, err error) {
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.lockedProof(tx, in.IntentID, in.VerificationToken)
		if err != nil {
			return err
		}
		if r.Purpose != "registration" {
			return ErrInvalidVerification
		}
		if r.Status == "completed" && r.CustomerID != nil {
			out, err = s.registrationResult(ctx, tx, *r.CustomerID)
			return err
		}
		if r.Status != "verified" || r.VerifiedAt == nil || r.PasswordHash == "" {
			return ErrInvalidVerification
		}
		// Recheck the exact displayed attribution. A takeover, disabled code or
		// changed business status invalidates the old intent rather than reassigning it.
		code := ""
		if r.InviteCodeID != nil {
			var c codeRow
			if err = tx.First(&c, "id = ?", *r.InviteCodeID).Error; err != nil {
				return err
			}
			code = c.Code
		}
		a, err := s.ResolveRegistrationIntent(ctx, tx, r.BrandID, code)
		if err != nil {
			return err
		}
		if a.Operator.ID != r.OperatorID || !sameOptionalID(a.InviterID, r.InviterID) {
			return ErrStaleOwnership
		}
		// Lock the responsible operator against business closure / takeover.
		var op operatorRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&op, "id = ?", r.OperatorID).Error; err != nil {
			return err
		}
		if err = s.RequireBusiness(ctx, tx, r.OperatorID, Registration); err != nil {
			return err
		}
		a, err = s.ResolveRegistrationIntent(ctx, tx, r.BrandID, code)
		if err != nil {
			return err
		}
		if a.Operator.ID != r.OperatorID || !sameOptionalID(a.InviterID, r.InviterID) {
			return ErrStaleOwnership
		}
		var now time.Time
		if err = tx.Raw(`SELECT clock_timestamp()`).Scan(&now).Error; err != nil {
			return err
		}
		now = now.UTC()
		c := customerRow{ID: newID(), BrandID: r.BrandID, DisplayName: strings.TrimSpace(in.DisplayName), Status: "active", RegisteredAt: now, NewCustomerRecordedAt: now}
		if err = tx.Create(&c).Error; err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO organization_customer_identities(id,brand_id,customer_id,email_normalized,is_current,verified_at,created_at) VALUES(?,?,?,?,true,?,?)`, newID(), r.BrandID, c.ID, r.EmailNormalized, *r.VerifiedAt, now).Error; err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO organization_customer_credentials(customer_id,password_hash,version,updated_at) VALUES(?,?,1,?)`, c.ID, r.PasswordHash, now).Error; err != nil {
			return err
		}
		o := ownershipRow{ID: newID(), CustomerID: c.ID, BrandID: r.BrandID, OperatorID: r.OperatorID, Version: 1, EffectiveAt: now, EvidenceReference: r.ID}
		if err = tx.Create(&o).Error; err != nil {
			return err
		}
		if r.InviterID != nil {
			if err = tx.Exec(`INSERT INTO organization_referral_edges(invitee_id,inviter_id,brand_id,registration_operator_id,created_at) VALUES(?,?,?,?,?)`, c.ID, *r.InviterID, r.BrandID, r.OperatorID, now).Error; err != nil {
				return err
			}
		}
		userCode, err := inviteCode()
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO organization_invite_codes(id,code,brand_id,kind,customer_id,enabled,created_at) VALUES(?,?,?,'user',?,true,?)`, newID(), userCode, r.BrandID, c.ID, now).Error; err != nil {
			return err
		}
		if err = tx.Model(&r).Updates(map[string]any{"status": "completed", "customer_id": c.ID, "completed_at": now, "password_hash": ""}).Error; err != nil {
			return err
		}
		out = RegistrationResult{Customer: Customer(c), Ownership: Ownership(o), InviteCode: userCode, InviterID: r.InviterID, NewlyRegistered: true}
		for _, hook := range hooks {
			if hook != nil {
				if err = hook.OnRegistered(ctx, tx, out); err != nil {
					return err
				}
			}
		}
		return s.event(tx, o.OperatorID, "", "customer.registered", r.ID, map[string]any{"customer_id": c.ID, "ownership_version": 1, "inviter_id": r.InviterID})
	})
	return
}
func sameOptionalID(a, b *string) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func (s *Service) registrationResult(ctx context.Context, tx *gorm.DB, customerID string) (out RegistrationResult, err error) {
	out.Customer, err = s.GetCustomer(ctx, tx, customerID)
	if err != nil {
		return
	}
	out.Ownership, err = s.CurrentOwnership(ctx, tx, customerID)
	if err != nil {
		return
	}
	var c codeRow
	if err = s.dbFor(ctx, tx).First(&c, "customer_id = ? AND kind = 'user'", customerID).Error; err != nil {
		return
	}
	out.InviteCode = c.Code
	var rows []struct{ InviterID string }
	err = s.dbFor(ctx, tx).Table("organization_referral_edges").Select("inviter_id").Where("invitee_id = ?", customerID).Scan(&rows).Error
	if len(rows) > 0 {
		out.InviterID = &rows[0].InviterID
	}
	return
}

func (s *Service) CurrentOwnership(ctx context.Context, tx *gorm.DB, customerID string) (Ownership, error) {
	if !validID(customerID) {
		return Ownership{}, ErrInvalidInput
	}
	var r ownershipRow
	err := s.dbFor(ctx, tx).First(&r, "customer_id = ? AND ended_at IS NULL", customerID).Error
	return Ownership(r), classify(err)
}
func (s *Service) AssertOwnershipVersion(ctx context.Context, tx *gorm.DB, customerID, operatorID string, version int64) error {
	o, err := s.CurrentOwnership(ctx, tx, customerID)
	if err != nil {
		return err
	}
	if o.OperatorID != operatorID || o.Version != version {
		return ErrStaleOwnership
	}
	return nil
}

// LockOwnership joins the shared operator -> customer lock order used by
// registration, business closure and takeover. Asset commands should call it
// before locking their own accounts. The outer transaction is mandatory.
func (s *Service) LockOwnership(ctx context.Context, tx *gorm.DB, customerID, operatorID string, version int64) (Ownership, error) {
	if tx == nil || !validID(customerID) || !validID(operatorID) {
		return Ownership{}, ErrInvalidInput
	}
	tx = tx.WithContext(ctx)
	var operator operatorRow
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&operator, "id=?", operatorID).Error; err != nil {
		return Ownership{}, classify(err)
	}
	var row ownershipRow
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&row, "customer_id=? AND ended_at IS NULL", customerID).Error; err != nil {
		return Ownership{}, classify(err)
	}
	if row.OperatorID != operatorID || row.Version != version {
		return Ownership{}, ErrStaleOwnership
	}
	return Ownership(row), nil
}
func (s *Service) ReferralChain(ctx context.Context, tx *gorm.DB, customerID string) ([]Referral, error) {
	if !validID(customerID) {
		return nil, ErrInvalidInput
	}
	out := []Referral{}
	id := customerID
	for i := 0; i < 2; i++ {
		var row Referral
		r := s.dbFor(ctx, tx).Table("organization_referral_edges").Where("invitee_id = ?", id).Limit(1).Find(&row)
		if r.Error != nil {
			return nil, r.Error
		}
		if r.RowsAffected == 0 {
			break
		}
		out = append(out, row)
		id = row.InviterID
	}
	return out, nil
}

// CustomerInScope is the reusable query boundary for staff APIs. Callers get
// one customer's public data only after exact-space permission and ownership.
func (s *Service) CustomerInScope(ctx context.Context, tx *gorm.DB, staffID, operatorID, customerID string) (Customer, error) {
	if err := s.RequirePermission(ctx, tx, staffID, operatorID, "customers.read"); err != nil {
		return Customer{}, err
	}
	o, err := s.CurrentOwnership(ctx, tx, customerID)
	if err != nil {
		return Customer{}, err
	}
	if o.OperatorID != operatorID {
		return Customer{}, ErrForbidden
	}
	return s.GetCustomer(ctx, tx, customerID)
}
