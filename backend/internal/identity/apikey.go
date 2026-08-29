package identity

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

type apiKeyRow struct {
	ID               string     `gorm:"column:id;primaryKey"`
	UserID           string     `gorm:"column:user_id"`
	Name             string     `gorm:"column:name"`
	Prefix           string     `gorm:"column:prefix"`
	SecretHash       string     `gorm:"column:secret_hash"`
	SecretCiphertext string     `gorm:"column:secret_ciphertext"`
	Status           string     `gorm:"column:status"`
	RPMLimit         int        `gorm:"column:rpm_limit"`
	ConcurrencyLimit int        `gorm:"column:concurrency_limit"`
	ExpiresAt        *time.Time `gorm:"column:expires_at"`
	LastUsedAt       *time.Time `gorm:"column:last_used_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (apiKeyRow) TableName() string { return "identity_api_keys" }

type apiKeyPolicyRow struct {
	APIKeyID      string `gorm:"column:api_key_id;primaryKey"`
	PublicModelID string `gorm:"column:public_model_id;primaryKey"`
	Allowed       bool   `gorm:"column:allowed"`
}

func (apiKeyPolicyRow) TableName() string { return "identity_api_key_model_policies" }

type APIKeyView struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id,omitempty"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Secret     string     `json:"key,omitempty"`
	Status     string     `json:"status"`
	RPMLimit   int        `json:"rpm_limit,omitempty"`
	Allowlist  []string   `json:"allowlist,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *Service) ListAPIKeySummaries(ctx context.Context) ([]APIKeyView, error) {
	var rows []apiKeyRow
	if err := s.db.WithContext(ctx).Order("created_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]APIKeyView, 0, len(rows))
	for _, row := range rows {
		out = append(out, viewFromRow(row, ""))
	}
	return out, nil
}

type APIKeyPrincipal struct {
	Principal
	APIKeyID         string
	Allowlist        []string
	RPMLimit         int
	ConcurrencyLimit int
}

func (s *Service) CreateAPIKey(ctx context.Context, user Principal, name, encKey string, allowlist []string, rpm int) (*APIKeyView, error) {
	raw, err := crypto.RandomToken("thk_")
	if err != nil {
		return nil, err
	}
	cipher, err := crypto.Seal(encKey, raw)
	if err != nil {
		return nil, err
	}
	if rpm <= 0 {
		rpm = 60
	}
	row := apiKeyRow{
		ID:               id.New("key"),
		UserID:           user.UserID,
		Name:             name,
		Prefix:           raw[:8],
		SecretHash:       crypto.HashToken(raw),
		SecretCiphertext: cipher,
		Status:           "active",
		RPMLimit:         rpm,
		ConcurrencyLimit: 5,
		CreatedAt:        time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	for _, model := range allowlist {
		_ = s.db.WithContext(ctx).Create(&apiKeyPolicyRow{APIKeyID: row.ID, PublicModelID: model, Allowed: true}).Error
	}
	return &APIKeyView{ID: row.ID, Name: row.Name, Prefix: row.Prefix, Secret: raw, Status: row.Status, Allowlist: allowlist, CreatedAt: row.CreatedAt}, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, user Principal, encKey string) ([]APIKeyView, error) {
	var rows []apiKeyRow
	if err := s.db.WithContext(ctx).Where("user_id = ?", user.UserID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]APIKeyView, 0, len(rows))
	for _, row := range rows {
		secret, _ := crypto.Open(encKey, row.SecretCiphertext)
		out = append(out, viewFromRow(row, secret))
	}
	return out, nil
}

func (s *Service) ConfirmOwnedKey(ctx context.Context, user Principal, keyID string) (string, error) {
	row, err := s.ownedKey(ctx, user, keyID)
	if err != nil {
		return "", err
	}
	return row.ID, nil
}

func (s *Service) ownedKey(ctx context.Context, user Principal, keyID string) (*apiKeyRow, error) {
	var row apiKeyRow
	q := s.db.WithContext(ctx).Where("id = ?", keyID)
	if !user.IsPlatformAdmin() && !user.HasRole("tech_admin") {
		q = q.Where("user_id = ?", user.UserID)
	}
	if err := q.First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Service) RotateAPIKey(ctx context.Context, user Principal, keyID, encKey string) (*APIKeyView, error) {
	row, err := s.ownedKey(ctx, user, keyID)
	if err != nil {
		return nil, err
	}
	raw, err := crypto.RandomToken("thk_")
	if err != nil {
		return nil, err
	}
	cipher, err := crypto.Seal(encKey, raw)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ?", row.ID).Updates(map[string]any{
		"prefix": raw[:8], "secret_hash": crypto.HashToken(raw), "secret_ciphertext": cipher, "status": "active",
	}).Error; err != nil {
		return nil, err
	}
	view := viewFromRow(*row, raw)
	view.Prefix = raw[:8]
	view.Status = "active"
	return &view, nil
}

func (s *Service) DisableAPIKey(ctx context.Context, user Principal, keyID string) (*APIKeyView, error) {
	row, err := s.ownedKey(ctx, user, keyID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ?", row.ID).Update("status", "disabled").Error; err != nil {
		return nil, err
	}
	view := viewFromRow(*row, "")
	view.Status = "disabled"
	return &view, nil
}

func (s *Service) ExpireAPIKey(ctx context.Context, user Principal, keyID string, when time.Time) (*APIKeyView, error) {
	row, err := s.ownedKey(ctx, user, keyID)
	if err != nil {
		return nil, err
	}
	if when.IsZero() {
		when = time.Now().UTC()
	}
	if err := s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ?", row.ID).Update("expires_at", when).Error; err != nil {
		return nil, err
	}
	view := viewFromRow(*row, "")
	view.ExpiresAt = &when
	return &view, nil
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, raw string) (*APIKeyPrincipal, error) {
	if raw == "" {
		return nil, nil
	}
	var row apiKeyRow
	if err := s.db.WithContext(ctx).Where("secret_hash = ? AND status = ?", crypto.HashToken(raw), "active").First(&row).Error; err != nil {
		return nil, nil
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, nil
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ?", row.ID).Update("last_used_at", now).Error
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ?", row.UserID).First(&user).Error; err != nil {
		return nil, err
	}
	principal, err := s.loadPrincipal(ctx, user)
	if err != nil {
		return nil, err
	}
	var policies []apiKeyPolicyRow
	_ = s.db.WithContext(ctx).Where("api_key_id = ? AND allowed = true", row.ID).Find(&policies).Error
	allow := make([]string, 0, len(policies))
	for _, policy := range policies {
		allow = append(allow, policy.PublicModelID)
	}
	return &APIKeyPrincipal{Principal: *principal, APIKeyID: row.ID, Allowlist: allow, RPMLimit: row.RPMLimit, ConcurrencyLimit: row.ConcurrencyLimit}, nil
}

func viewFromRow(row apiKeyRow, secret string) APIKeyView {
	return APIKeyView{
		ID: row.ID, UserID: row.UserID, Name: row.Name, Prefix: row.Prefix, Secret: secret,
		Status: row.Status, RPMLimit: row.RPMLimit, ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt, CreatedAt: row.CreatedAt,
	}
}
