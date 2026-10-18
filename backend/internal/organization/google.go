package organization

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGoogleConfig        = errors.New("organization: invalid google configuration")
	ErrGoogleClientTaken   = errors.New("organization: google client belongs to another brand")
	ErrGoogleNotVerified   = errors.New("organization: google callback has not been tested")
	ErrGoogleUnavailable   = errors.New("organization: google sign-in is not available")
	ErrGoogleState         = errors.New("organization: google sign-in state is missing or already used")
	ErrGoogleNeedsPassword = errors.New("organization: email already uses a password in this brand")
)

type GoogleProfile struct {
	Subject, Email, Name string
}

type GoogleCredential struct {
	ClientID, ClientSecret, RedirectURI string
	Version                             int64
}

type GoogleConfigView struct {
	ClientID          string   `json:"client_id"`
	SecretSet         bool     `json:"secret_set"`
	Enabled           bool     `json:"enabled"`
	Version           int64    `json:"version"`
	VerifiedRedirects []string `json:"verified_redirects"`
}

type GoogleConfigInput struct {
	ClientID, ClientSecret string
	Enabled                bool
	ExpectedVersion        int64
	RedirectURI            string
}

type GoogleIntent struct {
	BrandID, RedirectURI, InviteCode, NextPath string
	ConfigVersion                              int64
	TestOnly                                   bool
}

type googleConfigRow struct {
	BrandID           string `gorm:"primaryKey"`
	ClientID          string
	SecretCiphertext  string
	Enabled           bool
	Version           int64
	VerifiedRedirects []byte `gorm:"column:verified_redirects"`
	UpdatedAt         time.Time
}

func (googleConfigRow) TableName() string { return "organization_google_configs" }

type googleStateRow struct {
	ID, BrandID, StateHash, RedirectURI, InviteCode, NextPath string
	ConfigVersion                                             int64
	TestOnly                                                  bool
	ExpiresAt                                                 time.Time
	ConsumedAt                                                *time.Time
	CreatedAt                                                 time.Time
}

func (googleStateRow) TableName() string { return "organization_google_states" }

type googleTestRow struct {
	ID, TicketHash, BrandID, RedirectURI string
	ConfigVersion                        int64
	ExpiresAt                            time.Time
	ConsumedAt                           *time.Time
	CreatedAt                            time.Time
}

func (googleTestRow) TableName() string { return "organization_google_tests" }

func googleRedirects(raw []byte) []string {
	out := []string{}
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		return []string{}
	}
	return out
}
func validGoogleClientID(id string) bool {
	id = strings.TrimSpace(id)
	return strings.HasSuffix(id, ".apps.googleusercontent.com") && len(id) <= 255 && !strings.ContainsAny(id, " \t\r\n")
}

func (s *Service) GoogleConfig(ctx context.Context, tx *gorm.DB, brandID string) (GoogleConfigView, error) {
	if !validID(brandID) {
		return GoogleConfigView{}, ErrInvalidInput
	}
	var row googleConfigRow
	err := s.dbFor(ctx, tx).First(&row, "brand_id=?", brandID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GoogleConfigView{VerifiedRedirects: []string{}}, nil
	}
	if err != nil {
		return GoogleConfigView{}, err
	}
	return GoogleConfigView{ClientID: row.ClientID, SecretSet: row.SecretCiphertext != "", Enabled: row.Enabled, Version: row.Version, VerifiedRedirects: googleRedirects(row.VerifiedRedirects)}, nil
}

func (s *Service) SaveGoogleConfig(ctx context.Context, tx *gorm.DB, actor, brandID, encKey string, in GoogleConfigInput) (GoogleConfigView, error) {
	in.ClientID, in.ClientSecret = strings.TrimSpace(in.ClientID), strings.TrimSpace(in.ClientSecret)
	if !validID(actor) || !validID(brandID) || encKey == "" || !validGoogleClientID(in.ClientID) || len(in.ClientSecret) > 2048 {
		return GoogleConfigView{}, ErrGoogleConfig
	}
	var view GoogleConfigView
	err := s.withTx(ctx, tx, func(tx *gorm.DB) error {
		if err := s.RequireBrandBranding(ctx, tx, actor, brandID); err != nil {
			return err
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "google-config:"+brandID).Error; err != nil {
			return err
		}
		var row googleConfigRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "brand_id=?", brandID).Error
		fresh := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !fresh {
			return err
		}
		if row.Version != in.ExpectedVersion {
			return ErrConflict
		}
		var taken int64
		if err = tx.Model(&googleConfigRow{}).Where("client_id=? AND brand_id<>?", in.ClientID, brandID).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return ErrGoogleClientTaken
		}
		changed := row.ClientID != in.ClientID || in.ClientSecret != ""
		if row.ClientID != in.ClientID && in.ClientSecret == "" {
			return ErrGoogleConfig
		}
		redirects := googleRedirects(row.VerifiedRedirects)
		if changed {
			redirects = []string{}
		}
		if in.Enabled && len(redirects) == 0 {
			return ErrGoogleNotVerified
		}
		if in.ClientSecret != "" {
			row.SecretCiphertext, err = crypto.Seal(encKey, in.ClientSecret)
			if err != nil {
				return err
			}
		}
		if row.SecretCiphertext == "" {
			return ErrGoogleConfig
		}
		row.BrandID, row.ClientID, row.Enabled = brandID, in.ClientID, in.Enabled
		row.Version++
		row.VerifiedRedirects, _ = json.Marshal(redirects)
		row.UpdatedAt = s.now()
		if fresh {
			err = tx.Create(&row).Error
		} else {
			err = tx.Save(&row).Error
		}
		if err != nil {
			return err
		}
		view = GoogleConfigView{ClientID: row.ClientID, SecretSet: true, Enabled: row.Enabled, Version: row.Version, VerifiedRedirects: redirects}
		return nil
	})
	return view, err
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func (s *Service) OpenGoogleCredential(ctx context.Context, tx *gorm.DB, brandID, redirectURI, encKey string, testing bool) (GoogleCredential, error) {
	var row googleConfigRow
	err := s.dbFor(ctx, tx).First(&row, "brand_id=?", brandID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || row.SecretCiphertext == "" {
		return GoogleCredential{}, ErrGoogleUnavailable
	}
	if err != nil {
		return GoogleCredential{}, err
	}
	if !testing && (!row.Enabled || !containsString(googleRedirects(row.VerifiedRedirects), redirectURI)) {
		return GoogleCredential{}, ErrGoogleUnavailable
	}
	secret, err := crypto.Open(encKey, row.SecretCiphertext)
	if err != nil || secret == "" {
		return GoogleCredential{}, ErrGoogleUnavailable
	}
	return GoogleCredential{ClientID: row.ClientID, ClientSecret: secret, RedirectURI: redirectURI, Version: row.Version}, nil
}

func (s *Service) StartGoogle(ctx context.Context, tx *gorm.DB, intent GoogleIntent) (string, error) {
	if !validID(intent.BrandID) || intent.RedirectURI == "" || intent.ConfigVersion < 1 {
		return "", ErrGoogleUnavailable
	}
	state, err := token()
	if err != nil {
		return "", err
	}
	row := googleStateRow{ID: newID(), BrandID: intent.BrandID, StateHash: tokenHash(state), RedirectURI: intent.RedirectURI, InviteCode: strings.ToUpper(strings.TrimSpace(intent.InviteCode)), NextPath: intent.NextPath, ConfigVersion: intent.ConfigVersion, TestOnly: intent.TestOnly, ExpiresAt: s.now().Add(15 * time.Minute), CreatedAt: s.now()}
	if err = s.dbFor(ctx, tx).Create(&row).Error; err != nil {
		return "", err
	}
	return state, nil
}

func (s *Service) ConsumeGoogleState(ctx context.Context, tx *gorm.DB, state string) (GoogleIntent, error) {
	var intent GoogleIntent
	err := s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var row googleStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "state_hash=? AND expires_at>? AND consumed_at IS NULL", tokenHash(state), s.now()).Error; err != nil {
			return ErrGoogleState
		}
		now := s.now()
		if err := tx.Model(&row).Update("consumed_at", now).Error; err != nil {
			return err
		}
		intent = GoogleIntent{BrandID: row.BrandID, RedirectURI: row.RedirectURI, InviteCode: row.InviteCode, NextPath: row.NextPath, ConfigVersion: row.ConfigVersion, TestOnly: row.TestOnly}
		return nil
	})
	return intent, err
}

func (s *Service) RecordGoogleVerification(ctx context.Context, tx *gorm.DB, brandID, redirectURI string, version int64, ok bool) error {
	return s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var row googleConfigRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "brand_id=? AND version=?", brandID, version).Error; err != nil {
			return ErrConflict
		}
		redirects := googleRedirects(row.VerifiedRedirects)
		if ok && !containsString(redirects, redirectURI) {
			redirects = append(redirects, redirectURI)
		}
		if !ok {
			kept := []string{}
			for _, item := range redirects {
				if item != redirectURI {
					kept = append(kept, item)
				}
			}
			redirects = kept
		}
		raw, _ := json.Marshal(redirects)
		return tx.Model(&row).Update("verified_redirects", raw).Error
	})
}

func (s *Service) CreateGoogleTest(ctx context.Context, tx *gorm.DB, brandID, redirectURI string, version int64) (string, error) {
	ticket, err := token()
	if err != nil {
		return "", err
	}
	row := googleTestRow{ID: newID(), TicketHash: tokenHash(ticket), BrandID: brandID, RedirectURI: redirectURI, ConfigVersion: version, ExpiresAt: s.now().Add(10 * time.Minute), CreatedAt: s.now()}
	if err = s.dbFor(ctx, tx).Create(&row).Error; err != nil {
		return "", err
	}
	return ticket, nil
}

func (s *Service) ConsumeGoogleTest(ctx context.Context, tx *gorm.DB, ticket, redirectURI string) (GoogleIntent, error) {
	var intent GoogleIntent
	err := s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var row googleTestRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "ticket_hash=? AND redirect_uri=? AND expires_at>? AND consumed_at IS NULL", tokenHash(ticket), redirectURI, s.now()).Error; err != nil {
			return ErrGoogleState
		}
		if err := tx.Model(&row).Update("consumed_at", s.now()).Error; err != nil {
			return err
		}
		intent = GoogleIntent{BrandID: row.BrandID, RedirectURI: row.RedirectURI, ConfigVersion: row.ConfigVersion, TestOnly: true}
		return nil
	})
	return intent, err
}

// FinishGoogleSignIn logs in the customer already bound to this Google subject
// inside the current brand, or registers a new customer. An email that already
// uses a password is not attached automatically.
func (s *Service) FinishGoogleSignIn(ctx context.Context, tx *gorm.DB, intent GoogleIntent, profile GoogleProfile, hooks ...RegistrationHook) (out LoginResult, err error) {
	email, err := NormalizeEmail(profile.Email)
	if err != nil || strings.TrimSpace(profile.Subject) == "" || len(profile.Subject) > 255 || strings.ContainsAny(profile.Subject, " \t\r\n") {
		return out, ErrGoogleUnavailable
	}
	err = s.withTx(ctx, tx, func(tx *gorm.DB) error {
		var cfg googleConfigRow
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&cfg, "brand_id=? AND version=? AND enabled=true", intent.BrandID, intent.ConfigVersion).Error; err != nil {
			return ErrGoogleUnavailable
		}
		if !containsString(googleRedirects(cfg.VerifiedRedirects), intent.RedirectURI) {
			return ErrGoogleUnavailable
		}
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "google-user:"+intent.BrandID+":"+profile.Subject).Error; err != nil {
			return err
		}
		var existing struct {
			CustomerID    string
			GoogleSubject string
		}
		q := tx.Table("organization_customer_identities").Select("customer_id, google_subject").Where("brand_id=? AND google_subject=? AND is_current=true", intent.BrandID, profile.Subject).Limit(1).Find(&existing)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			return s.issueCustomerLogin(ctx, tx, existing.CustomerID, &out)
		}
		emailRow := existing
		q = tx.Table("organization_customer_identities").Select("customer_id, google_subject").Where("brand_id=? AND email_normalized=? AND is_current=true", intent.BrandID, email).Limit(1).Find(&emailRow)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 1 {
			return ErrGoogleNeedsPassword
		}
		return s.registerGoogleCustomer(ctx, tx, intent, email, profile, hooks, &out)
	})
	return out, err
}

func (s *Service) issueCustomerLogin(ctx context.Context, tx *gorm.DB, customerID string, out *LoginResult) error {
	customer, err := s.GetCustomer(ctx, tx, customerID)
	if err != nil {
		return err
	}
	if customer.Status != "active" && customer.Status != "stopped" && customer.Status != "closing" {
		return ErrInvalidCredentials
	}
	ownership, err := s.CurrentOwnership(ctx, tx, customerID)
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
	var auth credentialRow
	if err = tx.First(&auth, "customer_id=?", customerID).Error; err != nil {
		return err
	}
	raw, err := token()
	if err != nil {
		return err
	}
	now := s.now()
	access := accessTokenRow{ID: newID(), CustomerID: customerID, TokenHash: tokenHash(raw), CredentialVersion: auth.Version, ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}
	if err = tx.Create(&access).Error; err != nil {
		return err
	}
	*out = LoginResult{Principal: Principal{Customer: customer, Ownership: ownership, CredentialVersion: auth.Version}, AccessToken: raw, ExpiresAt: access.ExpiresAt}
	return nil
}

func (s *Service) registerGoogleCustomer(ctx context.Context, tx *gorm.DB, intent GoogleIntent, email string, profile GoogleProfile, hooks []RegistrationHook, out *LoginResult) error {
	attribution, err := s.ResolveRegistrationIntent(ctx, tx, intent.BrandID, intent.InviteCode)
	if err != nil {
		return err
	}
	var op operatorRow
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&op, "id=?", attribution.Operator.ID).Error; err != nil {
		return err
	}
	if err = s.RequireBusiness(ctx, tx, attribution.Operator.ID, Registration); err != nil {
		return err
	}
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	now := s.now()
	customerID := newID()
	customer := customerRow{ID: customerID, BrandID: intent.BrandID, DisplayName: name, Status: "active", Version: 1, RegisteredAt: now, NewCustomerRecordedAt: now}
	if err = tx.Create(&customer).Error; err != nil {
		return err
	}
	if err = tx.Exec(`INSERT INTO organization_customer_identities(id,brand_id,customer_id,email_normalized,google_subject,is_current,verified_at,created_at) VALUES(?,?,?,?,?,true,?,?)`, newID(), intent.BrandID, customerID, email, profile.Subject, now, now).Error; err != nil {
		return err
	}
	secret, err := token()
	if err != nil {
		return err
	}
	hash, err := HashPassword(secret + "Aa1")
	if err != nil {
		return err
	}
	if err = tx.Exec(`INSERT INTO organization_customer_credentials(customer_id,password_hash,version,updated_at) VALUES(?,?,1,?)`, customerID, hash, now).Error; err != nil {
		return err
	}
	registrationID := newID()
	ownership := ownershipRow{ID: newID(), CustomerID: customerID, BrandID: intent.BrandID, OperatorID: attribution.Operator.ID, Version: 1, EffectiveAt: now, EvidenceReference: registrationID}
	if err = tx.Create(&ownership).Error; err != nil {
		return err
	}
	if attribution.InviterID != nil {
		if err = tx.Exec(`INSERT INTO organization_referral_edges(invitee_id,inviter_id,brand_id,registration_operator_id,created_at) VALUES(?,?,?,?,?)`, customerID, *attribution.InviterID, intent.BrandID, attribution.Operator.ID, now).Error; err != nil {
			return err
		}
	}
	code, err := inviteCode()
	if err != nil {
		return err
	}
	if err = tx.Exec(`INSERT INTO organization_invite_codes(id,code,brand_id,kind,customer_id,enabled,created_at) VALUES(?,?,?,'user',?,true,?)`, newID(), code, intent.BrandID, customerID, now).Error; err != nil {
		return err
	}
	result := RegistrationResult{Customer: Customer(customer), Ownership: Ownership(ownership), InviteCode: code, InviterID: attribution.InviterID, NewlyRegistered: true}
	for _, hook := range hooks {
		if hook != nil {
			if err = hook.OnRegistered(ctx, tx, result); err != nil {
				return err
			}
		}
	}
	if err = s.event(tx, ownership.OperatorID, "", "customer.registered", registrationID, map[string]any{"customer_id": customerID, "google_subject": profile.Subject, "ownership_version": 1, "inviter_id": attribution.InviterID}); err != nil {
		return err
	}
	return s.issueCustomerLogin(ctx, tx, customerID, out)
}
