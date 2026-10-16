package organization

import (
	"context"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HashPassword enforces bcrypt's byte limit explicitly (bcrypt must never
// silently truncate). Digests are never returned in customer-facing DTOs.
func HashPassword(password string) (string, error) {
	if len(password) < 10 || len(password) > 72 {
		return "", ErrInvalidInput
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(b), err
}
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

type credentialRow struct {
	CustomerID, PasswordHash string
	Version                  int64
	UpdatedAt                time.Time
}

func (credentialRow) TableName() string { return "organization_customer_credentials" }

type accessTokenRow struct {
	ID, CustomerID, TokenHash string
	CredentialVersion         int64
	ExpiresAt                 time.Time
	RevokedAt                 *time.Time
	CreatedAt                 time.Time
}

func (accessTokenRow) TableName() string { return "organization_access_tokens" }

func (s *Service) Login(ctx context.Context, tx *gorm.DB, brandID, email, password string) (out LoginResult, err error) {
	email, err = NormalizeEmail(email)
	if err != nil || !validID(brandID) {
		return out, ErrInvalidCredentials
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var auth credentialRow
		q := tx.Table("organization_customer_credentials AS c").Select("c.*").Joins("JOIN organization_customer_identities i ON i.customer_id=c.customer_id").Joins("JOIN organization_customers u ON u.id=c.customer_id").Where("i.brand_id=? AND i.email_normalized=? AND i.is_current=true AND u.status IN ('active','stopped','closing')", brandID, email).Limit(1).Find(&auth)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 || !CheckPassword(auth.PasswordHash, password) {
			return ErrInvalidCredentials
		}
		customer, err := s.GetCustomer(ctx, tx, auth.CustomerID)
		if err != nil {
			return err
		}
		ownership, err := s.CurrentOwnership(ctx, tx, auth.CustomerID)
		if err != nil {
			return err
		}
		brand, err := s.GetBrand(ctx, tx, customer.BrandID)
		if err != nil {
			return err
		}
		if brand.Status != "active" {
			return ErrBusinessClosed
		}
		if err = s.RequireUnfrozen(ctx, tx, customer.BrandID, ownership.OperatorID, customer.ID, "customer_login"); err != nil {
			return err
		}
		raw, err := token()
		if err != nil {
			return err
		}
		now := s.now()
		t := accessTokenRow{ID: newID(), CustomerID: auth.CustomerID, TokenHash: tokenHash(raw), CredentialVersion: auth.Version, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}
		if err = tx.Create(&t).Error; err != nil {
			return err
		}
		out = LoginResult{Principal: Principal{Customer: customer, Ownership: ownership, CredentialVersion: auth.Version}, AccessToken: raw, ExpiresAt: t.ExpiresAt}
		return nil
	})
	return
}

// AuthenticateToken resolves current ownership for every request. Tokens hold
// no fixed operating entity, so a takeover cannot leave stale authorization.
func (s *Service) AuthenticateToken(ctx context.Context, tx *gorm.DB, raw string) (out Principal, err error) {
	if raw == "" {
		return out, ErrInvalidCredentials
	}
	var row accessTokenRow
	q := s.dbFor(ctx, tx).Where("token_hash=? AND revoked_at IS NULL AND expires_at>?", tokenHash(raw), s.now()).Limit(1).Find(&row)
	if q.Error != nil {
		return out, q.Error
	}
	if q.RowsAffected == 0 {
		return out, ErrInvalidCredentials
	}
	var auth credentialRow
	if err = s.dbFor(ctx, tx).First(&auth, "customer_id=?", row.CustomerID).Error; err != nil {
		return out, classify(err)
	}
	if auth.Version != row.CredentialVersion {
		return out, ErrInvalidCredentials
	}
	out.Customer, err = s.GetCustomer(ctx, tx, row.CustomerID)
	if err != nil {
		return
	}
	if !customerCanAuthenticate(out.Customer.Status) {
		return out, ErrInvalidCredentials
	}
	b, err := s.GetBrand(ctx, tx, out.Customer.BrandID)
	if err != nil {
		return out, err
	}
	if b.Status != "active" {
		return out, ErrBusinessClosed
	}
	out.Ownership, err = s.CurrentOwnership(ctx, tx, row.CustomerID)
	if err != nil {
		return out, err
	}
	if err = s.RequireUnfrozen(ctx, tx, out.Customer.BrandID, out.Ownership.OperatorID, out.Customer.ID, "customer_login"); err != nil {
		return out, err
	}
	out.CredentialVersion = auth.Version
	return
}
func (s *Service) AuthenticateBrandToken(ctx context.Context, tx *gorm.DB, brandID, raw string) (Principal, error) {
	p, err := s.AuthenticateToken(ctx, tx, raw)
	if err != nil {
		return p, err
	}
	if p.Customer.BrandID != brandID {
		return Principal{}, ErrForbidden
	}
	return p, nil
}
func (s *Service) Logout(ctx context.Context, tx *gorm.DB, raw string) error {
	return s.withTx(ctx, tx, func(tx *gorm.DB) error {
		return tx.Model(&accessTokenRow{}).Where("token_hash=? AND revoked_at IS NULL", tokenHash(raw)).Update("revoked_at", s.now()).Error
	})
}

// BeginPasswordReset returns a secret intended solely for email delivery. An
// HTTP adapter should use the same outward response for unknown addresses.
func (s *Service) BeginPasswordReset(ctx context.Context, tx *gorm.DB, brandID, email string) (out VerificationChallenge, err error) {
	email, err = NormalizeEmail(email)
	if err != nil {
		return out, ErrInvalidInput
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var identity struct{ CustomerID string }
		q := tx.Table("organization_customer_identities").Select("customer_id").Where("brand_id=? AND email_normalized=? AND is_current=true", brandID, email).Limit(1).Find(&identity)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			return ErrInvalidCredentials
		}
		customer, err := s.GetCustomer(ctx, tx, identity.CustomerID)
		if err != nil {
			return err
		}
		if !customerCanAuthenticate(customer.Status) {
			return ErrInvalidCredentials
		}
		own, err := s.CurrentOwnership(ctx, tx, customer.ID)
		if err != nil {
			return err
		}
		raw, err := token()
		if err != nil {
			return err
		}
		var credential credentialRow
		if err = tx.First(&credential, "customer_id=?", customer.ID).Error; err != nil {
			return err
		}
		r := intentRow{ID: newID(), BrandID: brandID, OperatorID: own.OperatorID, EmailNormalized: email, VerificationHash: tokenHash(raw), CredentialVersion: credential.Version, Purpose: "password_reset", CustomerID: &customer.ID, Status: "pending", ExpiresAt: s.now().Add(30 * time.Minute), CreatedAt: s.now()}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = VerificationChallenge{Intent: r.DTO(), VerificationToken: raw}
		return nil
	})
	return
}
func (s *Service) CompletePasswordReset(ctx context.Context, tx *gorm.DB, intentID, raw, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.withTx(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.lockedProof(tx, intentID, raw)
		if err != nil {
			return err
		}
		if r.Purpose != "password_reset" || r.CustomerID == nil || r.Status != "verified" {
			return ErrInvalidVerification
		}
		var current int64
		if err = tx.Table("organization_customer_identities").Where("customer_id=? AND email_normalized=? AND is_current=true", *r.CustomerID, r.EmailNormalized).Count(&current).Error; err != nil {
			return err
		}
		if current != 1 {
			return ErrInvalidVerification
		}
		var credential credentialRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&credential, "customer_id=?", *r.CustomerID).Error; err != nil {
			return err
		}
		if credential.Version != r.CredentialVersion {
			return ErrInvalidVerification
		}
		if err = tx.Model(&credentialRow{}).Where("customer_id=?", *r.CustomerID).Updates(map[string]any{"password_hash": hash, "version": gorm.Expr("version+1"), "updated_at": s.now()}).Error; err != nil {
			return err
		}
		if err = tx.Model(&r).Updates(map[string]any{"status": "completed", "completed_at": s.now()}).Error; err != nil {
			return err
		}
		return s.event(tx, r.OperatorID, "", "customer.password_reset", r.ID, map[string]any{"customer_id": r.CustomerID})
	})
}

// BeginEmailChange requires an already authenticated account and its current
// password; the new address additionally needs its own email proof.
func (s *Service) BeginEmailChange(ctx context.Context, tx *gorm.DB, customerID, currentPassword, newEmail string) (out VerificationChallenge, err error) {
	email, err := NormalizeEmail(newEmail)
	if err != nil {
		return out, err
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		c, err := s.GetCustomer(ctx, tx, customerID)
		if err != nil {
			return err
		}
		if !customerCanAuthenticate(c.Status) {
			return ErrInvalidCredentials
		}
		var auth credentialRow
		if err = tx.First(&auth, "customer_id=?", customerID).Error; err != nil {
			return err
		}
		if !CheckPassword(auth.PasswordHash, currentPassword) {
			return ErrInvalidCredentials
		}
		var existing []struct {
			CustomerID string
			IsCurrent  bool
		}
		if err = tx.Table("organization_customer_identities").Select("customer_id,is_current").Where("brand_id=? AND email_normalized=?", c.BrandID, email).Find(&existing).Error; err != nil {
			return err
		}
		if len(existing) > 0 {
			if existing[0].CustomerID != customerID {
				return ErrEmailAlreadyRegistered
			}
			if existing[0].IsCurrent {
				return ErrConflict
			}
		}
		own, err := s.CurrentOwnership(ctx, tx, customerID)
		if err != nil {
			return err
		}
		raw, err := token()
		if err != nil {
			return err
		}
		r := intentRow{ID: newID(), BrandID: c.BrandID, OperatorID: own.OperatorID, EmailNormalized: email, VerificationHash: tokenHash(raw), CredentialVersion: auth.Version, Purpose: "email_change", CustomerID: &customerID, Status: "pending", ExpiresAt: s.now().Add(30 * time.Minute), CreatedAt: s.now()}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = VerificationChallenge{Intent: r.DTO(), VerificationToken: raw}
		return nil
	})
	return
}
func (s *Service) CompleteEmailChange(ctx context.Context, tx *gorm.DB, intentID, raw string) error {
	return s.withTx(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.lockedProof(tx, intentID, raw)
		if err != nil {
			return err
		}
		if r.Purpose != "email_change" || r.CustomerID == nil {
			return ErrInvalidVerification
		}
		if r.Status == "completed" {
			return nil
		}
		if r.Status != "verified" || r.VerifiedAt == nil {
			return ErrInvalidVerification
		}
		var c customerRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&c, "id=?", *r.CustomerID).Error; err != nil {
			return err
		}
		if !customerCanAuthenticate(c.Status) {
			return ErrInvalidCredentials
		}
		var credential credentialRow
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&credential, "customer_id=?", c.ID).Error; err != nil {
			return err
		}
		if credential.Version != r.CredentialVersion {
			return ErrInvalidVerification
		}
		var owners []struct{ CustomerID string }
		if err = tx.Table("organization_customer_identities").Select("customer_id").Where("brand_id=? AND email_normalized=?", r.BrandID, r.EmailNormalized).Find(&owners).Error; err != nil {
			return err
		}
		if len(owners) > 0 && owners[0].CustomerID != c.ID {
			return ErrEmailAlreadyRegistered
		}
		if err = tx.Table("organization_customer_identities").Where("customer_id=? AND is_current=true", c.ID).Update("is_current", false).Error; err != nil {
			return err
		}
		if len(owners) > 0 {
			err = tx.Table("organization_customer_identities").Where("brand_id=? AND email_normalized=? AND customer_id=?", r.BrandID, r.EmailNormalized, c.ID).Updates(map[string]any{"is_current": true, "verified_at": *r.VerifiedAt}).Error
		} else {
			err = tx.Exec(`INSERT INTO organization_customer_identities(id,brand_id,customer_id,email_normalized,is_current,verified_at,created_at) VALUES(?,?,?,?,true,?,?)`, newID(), r.BrandID, c.ID, r.EmailNormalized, *r.VerifiedAt, s.now()).Error
		}
		if err != nil {
			return err
		}
		// Revoke old sessions and old-address recovery capabilities on a change.
		if err = tx.Model(&credentialRow{}).Where("customer_id=?", c.ID).Updates(map[string]any{"version": gorm.Expr("version+1"), "updated_at": s.now()}).Error; err != nil {
			return err
		}
		if err = tx.Model(&r).Updates(map[string]any{"status": "completed", "completed_at": s.now()}).Error; err != nil {
			return err
		}
		return s.event(tx, r.OperatorID, "", "customer.email_changed", r.ID, map[string]any{"customer_id": c.ID})
	})
}
