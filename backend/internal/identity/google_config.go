package identity

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGoogleConfigInput   = errors.New("invalid google configuration")
	ErrGoogleClientTaken   = errors.New("google client belongs to another brand")
	ErrGoogleNotVerified   = errors.New("test the primary domain before enabling google")
	ErrGoogleEncryptionKey = errors.New("google credential encryption key is not configured")
)

type googleConfigRow struct {
	BrandID          string `gorm:"primaryKey"`
	ClientID         string
	SecretCiphertext string
	Enabled          bool
	Version          int64
	VerificationJSON []byte `gorm:"column:verification_json"`
	UpdatedAt        time.Time
}

func (googleConfigRow) TableName() string { return "identity_google_configs" }

type GoogleVerification struct {
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	Error      string     `json:"error,omitempty"`
}
type GoogleConfigView struct {
	ClientID     string                        `json:"client_id"`
	SecretSet    bool                          `json:"secret_set"`
	Enabled      bool                          `json:"enabled"`
	Version      int64                         `json:"version"`
	RedirectURIs []string                      `json:"redirect_uris"`
	Verification map[string]GoogleVerification `json:"verification"`
}
type GoogleConfigInput struct {
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	Enabled         bool   `json:"enabled"`
	ExpectedVersion int64  `json:"expected_version"`
}

// Google callbacks are fixed paths on registered browser entry points.
func GoogleRedirectURIs(brand *BrandView) []string {
	out := []string{}
	for _, host := range []string{brand.PrimaryDomain, brand.AdminDomain} {
		uri := "https://" + host + "/login/oauth/google"
		if !containsString(out, uri) {
			out = append(out, uri)
		}
	}
	return out
}
func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func googleVerification(row googleConfigRow) map[string]GoogleVerification {
	out := map[string]GoogleVerification{}
	_ = json.Unmarshal(row.VerificationJSON, &out)
	return out
}
func googleConfigView(row googleConfigRow, brand *BrandView) *GoogleConfigView {
	view := &GoogleConfigView{ClientID: row.ClientID, SecretSet: row.SecretCiphertext != "", Enabled: row.Enabled, Version: row.Version, RedirectURIs: GoogleRedirectURIs(brand), Verification: map[string]GoogleVerification{}}
	verification := googleVerification(row)
	for _, uri := range view.RedirectURIs {
		if item, ok := verification[uri]; ok {
			view.Verification[uri] = item
		}
	}
	return view
}
func (s *Service) OEMGoogleBrand(ctx context.Context, channelID string) (*BrandView, error) {
	var channel channelRow
	if err := s.db.WithContext(ctx).Where("id = ? AND type = ?", channelID, ChannelTypeC).First(&channel).Error; err != nil {
		return nil, mapNotFound(err)
	}
	return s.BrandByID(ctx, channel.BrandID)
}
func (s *Service) GoogleConfig(ctx context.Context, brandID string) (*GoogleConfigView, error) {
	brand, err := s.BrandByID(ctx, brandID)
	if err != nil {
		return nil, err
	}
	var row googleConfigRow
	err = s.db.WithContext(ctx).Where("brand_id = ?", brandID).First(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return googleConfigView(row, brand), nil
}
func (s *Service) SaveGoogleConfig(ctx context.Context, actor Principal, brandID, encKey string, in GoogleConfigInput) (*GoogleConfigView, error) {
	if !actor.HasRole("platform_admin", "tech_admin") {
		return nil, ErrChannelImmutable
	}
	in.ClientID, in.ClientSecret = strings.TrimSpace(in.ClientID), strings.TrimSpace(in.ClientSecret)
	if encKey == "" {
		return nil, ErrGoogleEncryptionKey
	}
	if !strings.HasSuffix(in.ClientID, ".apps.googleusercontent.com") || len(in.ClientID) > 255 || strings.ContainsAny(in.ClientID, " \t\r\n") || len(in.ClientSecret) > 2048 {
		return nil, ErrGoogleConfigInput
	}
	brand, err := s.BrandByID(ctx, brandID)
	if err != nil {
		return nil, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serializes creation and prevents the same client being assigned twice.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "google-configs").Error; err != nil {
			return err
		}
		var row googleConfigRow
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id = ?", brandID).First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if row.Version != in.ExpectedVersion {
			return ErrDeliveryConflict
		}
		var n int64
		if err := tx.Model(&googleConfigRow{}).Where("client_id = ? AND brand_id <> ?", in.ClientID, brandID).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrGoogleClientTaken
		}
		changed := row.ClientID != in.ClientID || in.ClientSecret != ""
		if row.ClientID != in.ClientID && in.ClientSecret == "" {
			return ErrGoogleConfigInput
		}
		verification := googleVerification(row)
		if changed {
			verification = map[string]GoogleVerification{}
		}
		primary := GoogleRedirectURIs(brand)[0]
		if in.Enabled && verification[primary].VerifiedAt == nil {
			return ErrGoogleNotVerified
		}
		if in.ClientSecret != "" {
			row.SecretCiphertext, err = crypto.Seal(encKey, in.ClientSecret)
			if err != nil {
				return err
			}
		}
		if row.SecretCiphertext == "" {
			return ErrGoogleConfigInput
		}
		row.BrandID, row.ClientID, row.Enabled = brandID, in.ClientID, in.Enabled
		row.Version++
		row.VerificationJSON, _ = json.Marshal(verification)
		row.UpdatedAt = time.Now().UTC()
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GoogleConfig(ctx, brandID)
}

// SaveGoogleConfigTx lets the application commit sanitized audit evidence in
// the same transaction as a credential change.
func (s *Service) SaveGoogleConfigTx(tx *gorm.DB, actor Principal, brandID, encKey string, in GoogleConfigInput) (*GoogleConfigView, error) {
	scoped := *s
	scoped.db = tx
	return scoped.SaveGoogleConfig(tx.Statement.Context, actor, brandID, encKey, in)
}

// A nil config means no database override exists. Disabled overrides never fall
// back to the instance's legacy official-brand credentials.
func (s *Service) ResolveGoogleConfig(ctx context.Context, brandID, redirectURI, encKey string, testing bool) (*GoogleOAuthConfig, int64, error) {
	var row googleConfigRow
	err := s.db.WithContext(ctx).Where("brand_id = ?", brandID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	brand, err := s.BrandByID(ctx, brandID)
	if err != nil {
		return nil, 0, err
	}
	if !containsString(GoogleRedirectURIs(brand), redirectURI) || (!testing && (!row.Enabled || googleVerification(row)[redirectURI].VerifiedAt == nil)) {
		return nil, row.Version, ErrGoogleUnavailable
	}
	if encKey == "" {
		return nil, row.Version, ErrGoogleUnavailable
	}
	secret, err := crypto.Open(encKey, row.SecretCiphertext)
	if err != nil {
		return nil, row.Version, ErrGoogleUnavailable
	}
	return &GoogleOAuthConfig{ClientID: row.ClientID, ClientSecret: secret, RedirectURL: redirectURI}, row.Version, nil
}

type googleTestTicketRow struct {
	TicketHash    string `gorm:"primaryKey"`
	BrandID       string
	ConfigVersion int64
	RedirectURI   string
	CreatedBy     string
	ExpiresAt     time.Time
	ConsumedAt    *time.Time
}

func (googleTestTicketRow) TableName() string { return "identity_google_test_tickets" }
func (s *Service) CreateGoogleTestTicket(ctx context.Context, actor Principal, brandID, redirectURI, encKey string) (string, error) {
	if !actor.HasRole("platform_admin", "tech_admin") {
		return "", ErrChannelImmutable
	}
	cfg, version, err := s.ResolveGoogleConfig(ctx, brandID, redirectURI, encKey, true)
	if err != nil {
		return "", err
	}
	if cfg == nil {
		return "", ErrGoogleUnavailable
	}
	ticket, err := crypto.RandomToken("gtest_")
	if err != nil {
		return "", err
	}
	row := googleTestTicketRow{TicketHash: crypto.HashToken(ticket), BrandID: brandID, ConfigVersion: version, RedirectURI: redirectURI, CreatedBy: actor.UserID, ExpiresAt: time.Now().UTC().Add(5 * time.Minute)}
	if err := s.db.WithContext(ctx).Where("expires_at <= ?", time.Now().UTC()).Delete(&googleTestTicketRow{}).Error; err != nil {
		return "", err
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return "", err
	}
	target, _ := url.Parse(redirectURI)
	target.Path += "/test"
	target.RawQuery = url.Values{"ticket": {ticket}}.Encode()
	return target.String(), nil
}
func (s *Service) ConsumeGoogleTestTicket(ctx context.Context, ticket, redirectURI string) (OAuthIntent, error) {
	var row googleTestTicketRow
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("ticket_hash = ? AND expires_at > ? AND consumed_at IS NULL AND redirect_uri = ?", crypto.HashToken(ticket), time.Now().UTC(), redirectURI).First(&row).Error; err != nil {
			return ErrInvalidCredentials
		}
		return tx.Model(&row).Update("consumed_at", time.Now().UTC()).Error
	})
	return OAuthIntent{BrandID: row.BrandID, ConfigVersion: row.ConfigVersion, RedirectURI: row.RedirectURI, TestOnly: true}, err
}
func (s *Service) RecordGoogleVerification(ctx context.Context, intent OAuthIntent, reason string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row googleConfigRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("brand_id = ? AND version = ?", intent.BrandID, intent.ConfigVersion).First(&row).Error; err != nil {
			return ErrDeliveryConflict
		}
		verification := googleVerification(row)
		item := GoogleVerification{Error: reason}
		if reason == "" {
			now := time.Now().UTC()
			item.VerifiedAt = &now
		}
		verification[intent.RedirectURI] = item
		raw, _ := json.Marshal(verification)
		return tx.Model(&row).Update("verification_json", raw).Error
	})
}
