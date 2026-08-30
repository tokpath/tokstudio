package catalog

import (
	"context"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

const (
	AccountActive     = "active"
	AccountDisabled   = "disabled"
	AccountCooldown   = "cooldown"
	AccountInvalid    = "invalid"
	AccountExhausted  = "exhausted"
	AccountUnknown    = "unknown"
	AccountRotated    = "rotated"
	AccountKindAPIKey = "api_key"
	AccountKindSecret = "adapter_secret"
)

type accountRow struct {
	ID               string     `gorm:"column:id;primaryKey"`
	ProviderID       string     `gorm:"column:provider_id"`
	Ciphertext       string     `gorm:"column:ciphertext"`
	KeyHash          string     `gorm:"column:key_hash"`
	Status           string     `gorm:"column:status"`
	Kind             string     `gorm:"column:kind"`
	Label            string     `gorm:"column:label"`
	ModelTags        string     `gorm:"column:model_tags"`
	RPMLimit         int        `gorm:"column:rpm_limit"`
	ConcurrencyLimit int        `gorm:"column:concurrency_limit"`
	LastSuccessAt    *time.Time `gorm:"column:last_success_at"`
	LastErrorAt      *time.Time `gorm:"column:last_error_at"`
	LastErrorCode    string     `gorm:"column:last_error_code"`
	CooldownUntil    *time.Time `gorm:"column:cooldown_until"`
	RotatedAt        *time.Time `gorm:"column:rotated_at"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (accountRow) TableName() string { return "catalog_provider_credentials" }

// AccountView 是管理后台看到的账号池条目，只有指纹没有密文。
type AccountView struct {
	ID               string     `json:"id"`
	ProviderID       string     `json:"provider_id"`
	Kind             string     `json:"kind"`
	Label            string     `json:"label"`
	Status           string     `json:"status"`
	Fingerprint      string     `json:"fingerprint"`
	ModelTags        string     `json:"model_tags"`
	RPMLimit         int        `json:"rpm_limit"`
	ConcurrencyLimit int        `json:"concurrency_limit"`
	LastSuccessAt    *time.Time `json:"last_success_at,omitempty"`
	LastErrorAt      *time.Time `json:"last_error_at,omitempty"`
	LastErrorCode    string     `json:"last_error_code,omitempty"`
	CooldownUntil    *time.Time `json:"cooldown_until,omitempty"`
}

type AccountInput struct {
	Secret           string `json:"secret"`
	Kind             string `json:"kind"`
	Label            string `json:"label"`
	ModelTags        string `json:"model_tags"`
	Status           string `json:"status"`
	RPMLimit         int    `json:"rpm_limit"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
	CooldownSeconds  int    `json:"cooldown_seconds"`
}

func accountView(row accountRow) AccountView {
	return AccountView{
		ID: row.ID, ProviderID: row.ProviderID, Kind: row.Kind, Label: row.Label,
		Status: row.Status, Fingerprint: accountFingerprint(row.KeyHash), ModelTags: row.ModelTags,
		RPMLimit: row.RPMLimit, ConcurrencyLimit: row.ConcurrencyLimit,
		LastSuccessAt: row.LastSuccessAt, LastErrorAt: row.LastErrorAt,
		LastErrorCode: row.LastErrorCode, CooldownUntil: row.CooldownUntil,
	}
}

func accountFingerprint(hash string) string {
	if len(hash) < 8 {
		return hash
	}
	return hash[:8]
}

func normalizeAccountKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case AccountKindSecret:
		return AccountKindSecret
	default:
		return AccountKindAPIKey
	}
}

func matchesModelTags(tags, publicID string) bool {
	tags = strings.TrimSpace(tags)
	if tags == "" {
		return true
	}
	for _, part := range strings.Split(tags, ",") {
		if strings.TrimSpace(part) == publicID {
			return true
		}
	}
	return false
}

func accountUsable(row accountRow, now time.Time, publicID string) bool {
	switch row.Status {
	case AccountDisabled, AccountInvalid, AccountExhausted, AccountRotated:
		return false
	}
	if row.CooldownUntil != nil && now.Before(*row.CooldownUntil) {
		return false
	}
	if row.Status == AccountCooldown && row.CooldownUntil == nil {
		return false
	}
	return matchesModelTags(row.ModelTags, publicID)
}

func (s *Service) ListAccounts(ctx context.Context, providerID, q string) ([]AccountView, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	var rows []accountRow
	if err := s.db.WithContext(ctx).Where("provider_id = ?", providerID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	out := make([]AccountView, 0, len(rows))
	for _, row := range rows {
		view := accountView(row)
		if q != "" && !strings.Contains(strings.ToLower(view.Label+view.Status+view.Kind+view.Fingerprint), q) {
			continue
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) AddAccount(ctx context.Context, providerID, encKey string, in AccountInput) (*AccountView, error) {
	var provider providerRow
	if err := s.db.WithContext(ctx).Where("id = ?", providerID).First(&provider).Error; err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(in.Secret)
	if secret == "" {
		return nil, ErrInvalidInput
	}
	sealed, err := crypto.Seal(encKey, secret)
	if err != nil {
		return nil, err
	}
	row := accountRow{
		ID: id.New("crd"), ProviderID: providerID, Ciphertext: sealed,
		KeyHash: crypto.HashToken(secret), Status: AccountActive,
		Kind: normalizeAccountKind(in.Kind), Label: strings.TrimSpace(in.Label),
		ModelTags: strings.TrimSpace(in.ModelTags), RPMLimit: in.RPMLimit,
		ConcurrencyLimit: in.ConcurrencyLimit, CreatedAt: time.Now().UTC(),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(provider.CredentialRef) == "" {
		_ = s.db.WithContext(ctx).Model(&providerRow{}).Where("id = ?", providerID).
			Update("credential_ref", row.ID).Error
	}
	view := accountView(row)
	return &view, nil
}

func (s *Service) PatchAccount(ctx context.Context, providerID, accountID string, in AccountInput) (*AccountView, error) {
	var row accountRow
	if err := s.db.WithContext(ctx).Where("id = ? AND provider_id = ?", accountID, providerID).First(&row).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if in.Kind != "" {
		updates["kind"] = normalizeAccountKind(in.Kind)
	}
	if in.Label != "" {
		updates["label"] = strings.TrimSpace(in.Label)
	}
	if in.ModelTags != "" {
		updates["model_tags"] = strings.TrimSpace(in.ModelTags)
	}
	if in.Status != "" {
		updates["status"] = strings.ToLower(strings.TrimSpace(in.Status))
	}
	if in.RPMLimit != 0 {
		updates["rpm_limit"] = in.RPMLimit
	}
	if in.ConcurrencyLimit != 0 {
		updates["concurrency_limit"] = in.ConcurrencyLimit
	}
	if in.CooldownSeconds > 0 {
		until := time.Now().UTC().Add(time.Duration(in.CooldownSeconds) * time.Second)
		updates["status"] = AccountCooldown
		updates["cooldown_until"] = until
	}
	if len(updates) > 0 {
		if err := s.db.WithContext(ctx).Model(&accountRow{}).Where("id = ?", accountID).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	if err := s.db.WithContext(ctx).Where("id = ?", accountID).First(&row).Error; err != nil {
		return nil, err
	}
	view := accountView(row)
	return &view, nil
}

// pickAccount 返回应使用的账号。skip=true 表示池子里没有可用账号，调用方应跳过该 Provider。
func (s *Service) pickAccount(ctx context.Context, providerID, publicID string) (accountID string, skip bool) {
	var rows []accountRow
	if err := s.db.WithContext(ctx).Where("provider_id = ?", providerID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return "", false
	}
	if len(rows) == 0 {
		return "", false
	}
	now := time.Now().UTC()
	for _, row := range rows {
		if accountUsable(row, now, publicID) {
			return row.ID, false
		}
	}
	return "", true
}

func (s *Service) RecordAccountOutcome(ctx context.Context, accountID string, httpStatus int) error {
	if strings.TrimSpace(accountID) == "" {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]any{}
	switch {
	case httpStatus >= 200 && httpStatus < 300:
		updates["last_success_at"] = now
		updates["status"] = AccountActive
		updates["cooldown_until"] = nil
	case httpStatus == 401:
		updates["status"] = AccountInvalid
		updates["last_error_at"] = now
		updates["last_error_code"] = "401"
	case httpStatus == 403:
		updates["last_error_at"] = now
		updates["last_error_code"] = "403"
	case httpStatus == 429:
		until := now.Add(30 * time.Second)
		updates["status"] = AccountCooldown
		updates["cooldown_until"] = until
		updates["last_error_at"] = now
		updates["last_error_code"] = "429"
	case httpStatus >= 500:
		updates["last_error_at"] = now
		updates["last_error_code"] = "5xx"
	default:
		return nil
	}
	return s.db.WithContext(ctx).Model(&accountRow{}).Where("id = ?", accountID).Updates(updates).Error
}
