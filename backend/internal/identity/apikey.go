package identity

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

var ErrChannelRequired = errors.New("channel org required")
var ErrAPIKeyNotInChannel = errors.New("api key not in channel")

type apiKeyRow struct {
	ID                  string     `gorm:"column:id;primaryKey"`
	UserID              string     `gorm:"column:user_id"`
	Name                string     `gorm:"column:name"`
	Prefix              string     `gorm:"column:prefix"`
	SecretHash          string     `gorm:"column:secret_hash"`
	SecretCiphertext    string     `gorm:"column:secret_ciphertext"`
	Status              string     `gorm:"column:status"`
	RPMLimit            int        `gorm:"column:rpm_limit"`
	ConcurrencyLimit    int        `gorm:"column:concurrency_limit"`
	ModelMode           string     `gorm:"column:model_mode"`
	BudgetLimitMinor    *int64     `gorm:"column:budget_limit_minor"`
	BudgetUsedMinor     int64      `gorm:"column:budget_used_minor"`
	BudgetReservedMinor int64      `gorm:"column:budget_reserved_minor"`
	ExpiresAt           *time.Time `gorm:"column:expires_at"`
	LastUsedAt          *time.Time `gorm:"column:last_used_at"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
}

func (apiKeyRow) TableName() string { return "identity_api_keys" }

type apiKeyPolicyRow struct {
	APIKeyID      string `gorm:"column:api_key_id;primaryKey"`
	PublicModelID string `gorm:"column:public_model_id;primaryKey"`
	Allowed       bool   `gorm:"column:allowed"`
}

func (apiKeyPolicyRow) TableName() string { return "identity_api_key_model_policies" }

type APIKeyView struct {
	ID                  string     `json:"id"`
	UserID              string     `json:"user_id,omitempty"`
	UserEmail           string     `json:"user_email,omitempty"`
	Name                string     `json:"name"`
	Prefix              string     `json:"prefix"`
	Secret              string     `json:"key,omitempty"`
	Status              string     `json:"status"`
	RPMLimit            int        `json:"rpm_limit,omitempty"`
	ConcurrencyLimit    int        `json:"concurrency_limit,omitempty"`
	Allowlist           []string   `json:"allowlist"`
	ModelMode           string     `json:"model_mode"`
	BudgetLimitMinor    *int64     `json:"budget_limit_minor"`
	BudgetUsedMinor     int64      `json:"budget_used_minor"`
	BudgetReservedMinor int64      `json:"budget_reserved_minor"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	LastUsedAt          *time.Time `json:"last_used_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
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
	if err := s.attachAllowlists(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListAPIKeySummariesForChannel 列出某渠道下属用户的 Key 摘要（无密文）。D38：平台不管用户 Key，渠道可看。
// 分两步查：先取渠道用户 ID，再按 user_id 查 Key。避免 GORM Joins+Scan/Find 扫嵌入列时字段为空。
func (s *Service) ListAPIKeySummariesForChannel(ctx context.Context, channelOrgID string) ([]APIKeyView, error) {
	channelOrgID = strings.TrimSpace(channelOrgID)
	if channelOrgID == "" {
		return nil, ErrChannelRequired
	}
	var users []userRow
	if err := s.db.WithContext(ctx).Select("id", "email").Where("channel_org_id = ?", channelOrgID).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return []APIKeyView{}, nil
	}
	emailByUser := make(map[string]string, len(users))
	userIDs := make([]string, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
		emailByUser[user.ID] = user.Email
	}
	var rows []apiKeyRow
	if err := s.db.WithContext(ctx).
		Where("user_id IN ?", userIDs).
		Order("created_at DESC").
		Limit(200).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]APIKeyView, 0, len(rows))
	for _, row := range rows {
		view := viewFromRow(row, "")
		view.UserEmail = emailByUser[row.UserID]
		out = append(out, view)
	}
	if err := s.attachAllowlists(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// DisableChannelAPIKey 渠道管理员禁本渠道用户的 Key（无密文回显）。
func (s *Service) DisableChannelAPIKey(ctx context.Context, channelOrgID, keyID string) (*APIKeyView, error) {
	channelOrgID = strings.TrimSpace(channelOrgID)
	keyID = strings.TrimSpace(keyID)
	if channelOrgID == "" || keyID == "" {
		return nil, ErrChannelRequired
	}
	var row apiKeyRow
	if err := s.db.WithContext(ctx).Where("id = ?", keyID).First(&row).Error; err != nil {
		return nil, ErrAPIKeyNotInChannel
	}
	var user userRow
	if err := s.db.WithContext(ctx).Select("id", "email", "channel_org_id").Where("id = ?", row.UserID).First(&user).Error; err != nil {
		return nil, ErrAPIKeyNotInChannel
	}
	if user.ChannelOrgID == nil || *user.ChannelOrgID != channelOrgID {
		return nil, ErrAPIKeyNotInChannel
	}
	if err := s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ?", row.ID).Update("status", "disabled").Error; err != nil {
		return nil, err
	}
	view := viewFromRow(row, "")
	view.Status = "disabled"
	view.UserEmail = user.Email
	return s.withAllowlist(ctx, view)
}

type APIKeyPrincipal struct {
	Principal
	APIKeyID         string
	ModelMode        string
	BudgetLimitMinor *int64
	Allowlist        []string
	RPMLimit         int
	ConcurrencyLimit int
}

type APIKeyLimits struct {
	OperationID      string     `json:"operation_id,omitempty"`
	Name             string     `json:"name"`
	ModelMode        string     `json:"model_mode"`
	Allowlist        []string   `json:"allowlist"`
	BudgetLimitMinor *int64     `json:"budget_limit_minor"`
	ExpiresAt        *time.Time `json:"expires_at"`
	RPMLimit         int        `json:"rpm_limit"`
	ConcurrencyLimit int        `json:"concurrency_limit"`
}

var ErrKeyCreationConflict = errors.New("api key creation operation conflict")

type apiKeyCreationRow struct {
	UserID      string `gorm:"primaryKey"`
	OperationID string `gorm:"primaryKey"`
	APIKeyID    string
	Fingerprint string
	CreatedAt   time.Time
}

func (apiKeyCreationRow) TableName() string { return "identity_api_key_creations" }

var ErrInvalidKeyLimits = errors.New("invalid key limits")
var ErrKeyBudgetExceeded = errors.New("api key budget exceeded")
var ErrKeyExpired = errors.New("api key expired")
var ErrKeyNotUsable = errors.New("api key disabled or expired")
var ErrKeyModelNotAllowed = errors.New("api key model not allowed")

func normalizeKeyLimits(in APIKeyLimits) (APIKeyLimits, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.OperationID = strings.TrimSpace(in.OperationID)
	if len(in.OperationID) > 128 {
		return in, ErrInvalidKeyLimits
	}
	in.Allowlist = normalizeAllowlist(in.Allowlist)
	// Old clients omitted the mode. Preserve their historical interpretation.
	if in.ModelMode == "" {
		in.ModelMode = "all"
		if len(in.Allowlist) > 0 {
			in.ModelMode = "selected"
		}
	}
	if in.Name == "" || (in.ModelMode != "all" && in.ModelMode != "selected") || (in.ModelMode == "selected" && len(in.Allowlist) == 0) || (in.BudgetLimitMinor != nil && *in.BudgetLimitMinor <= 0) || in.RPMLimit < 0 || in.ConcurrencyLimit < 0 {
		return in, ErrInvalidKeyLimits
	}
	if in.ModelMode == "all" {
		in.Allowlist = []string{}
	}
	if in.RPMLimit == 0 {
		in.RPMLimit = 60
	}
	if in.ConcurrencyLimit == 0 {
		in.ConcurrencyLimit = 5
	}
	if in.ExpiresAt != nil {
		utc := in.ExpiresAt.UTC()
		in.ExpiresAt = &utc
	}
	return in, nil
}

func (s *Service) CreateAPIKey(ctx context.Context, user Principal, name, encKey string, allowlist []string, rpm, concurrency int) (*APIKeyView, error) {
	return s.CreateAPIKeyWithLimits(ctx, user, encKey, APIKeyLimits{Name: name, Allowlist: allowlist, RPMLimit: rpm, ConcurrencyLimit: concurrency})
}

func (s *Service) CreateAPIKeyWithLimits(ctx context.Context, user Principal, encKey string, in APIKeyLimits) (*APIKeyView, error) {
	in, err := normalizeKeyLimits(in)
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
	row := apiKeyRow{ID: id.New("key"), UserID: user.UserID, Name: in.Name, Prefix: raw[:8], SecretHash: crypto.HashToken(raw), SecretCiphertext: cipher, Status: "active", ModelMode: in.ModelMode, BudgetLimitMinor: in.BudgetLimitMinor, ExpiresAt: in.ExpiresAt, RPMLimit: in.RPMLimit, ConcurrencyLimit: in.ConcurrencyLimit, CreatedAt: time.Now().UTC()}
	canonical := in
	canonical.OperationID = ""
	canonical.Allowlist = append([]string{}, in.Allowlist...)
	sort.Strings(canonical.Allowlist)
	fingerprintBytes, _ := json.Marshal(canonical)
	fingerprint := crypto.HashToken(string(fingerprintBytes))
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if in.OperationID != "" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "key.create:"+user.UserID+":"+in.OperationID).Error; err != nil {
				return err
			}
			var operation apiKeyCreationRow
			err := tx.Where("user_id=? AND operation_id=?", user.UserID, in.OperationID).First(&operation).Error
			if err == nil {
				if operation.Fingerprint != fingerprint {
					return ErrKeyCreationConflict
				}
				var existing apiKeyRow
				if err := tx.Where("id=? AND user_id=?", operation.APIKeyID, user.UserID).First(&existing).Error; err != nil {
					return err
				}
				row = existing
				raw, err = crypto.Open(encKey, row.SecretCiphertext)
				return err
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := replaceKeyPolicies(tx, row.ID, in.Allowlist); err != nil {
			return err
		}
		if in.OperationID != "" {
			return tx.Create(&apiKeyCreationRow{UserID: user.UserID, OperationID: in.OperationID, APIKeyID: row.ID, Fingerprint: fingerprint, CreatedAt: time.Now().UTC()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.withAllowlist(ctx, viewFromRow(row, raw))
}

func replaceKeyPolicies(tx *gorm.DB, keyID string, allowlist []string) error {
	if err := tx.Where("api_key_id = ?", keyID).Delete(&apiKeyPolicyRow{}).Error; err != nil {
		return err
	}
	for _, model := range allowlist {
		if err := tx.Create(&apiKeyPolicyRow{APIKeyID: keyID, PublicModelID: model, Allowed: true}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) UpdateAPIKeyLimits(ctx context.Context, user Principal, keyID string, in APIKeyLimits) (*APIKeyView, error) {
	in, err := normalizeKeyLimits(in)
	if err != nil {
		return nil, err
	}
	var row apiKeyRow
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", keyID, user.UserID).First(&row).Error; err != nil {
			return err
		}
		// A limit controls future admission. Lowering it never rewrites actual
		// usage or prevents an already admitted request from settling.
		if err := replaceKeyPolicies(tx, row.ID, in.Allowlist); err != nil {
			return err
		}
		row.Name = in.Name
		row.ModelMode = in.ModelMode
		row.BudgetLimitMinor = in.BudgetLimitMinor
		row.ExpiresAt = in.ExpiresAt
		row.RPMLimit = in.RPMLimit
		row.ConcurrencyLimit = in.ConcurrencyLimit
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	view := viewFromRow(row, "")
	view.Allowlist = in.Allowlist
	return &view, nil
}

func (s *Service) EnableAPIKey(ctx context.Context, user Principal, keyID string) (*APIKeyView, error) {
	row, err := s.ownedKey(ctx, user, keyID)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ? AND user_id = ?", keyID, user.UserID).Update("status", "active").Error; err != nil {
		return nil, err
	}
	row.Status = "active"
	return s.withAllowlist(ctx, viewFromRow(*row, ""))
}

// ReserveAPIKeyBudgetTx rechecks mutable policy while holding the same row lock
// used by edits. The caller's transaction also contains wallet authorization.
func ReserveAPIKeyBudgetTx(tx *gorm.DB, keyID, userID, modelID string, amount int64) error {
	if keyID == "" {
		return nil
	}
	var row apiKeyRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", keyID, userID).First(&row).Error; err != nil {
		return err
	}
	if row.Status != "active" || (row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now().UTC())) {
		return ErrKeyNotUsable
	}
	if row.ModelMode == "selected" {
		var n int64
		if err := tx.Model(&apiKeyPolicyRow{}).Where("api_key_id = ? AND public_model_id = ? AND allowed = true", keyID, modelID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrKeyModelNotAllowed
		}
	}
	if row.BudgetLimitMinor != nil {
		if amount > *row.BudgetLimitMinor-row.BudgetUsedMinor-row.BudgetReservedMinor {
			return ErrKeyBudgetExceeded
		}
	}
	return tx.Model(&apiKeyRow{}).Where("id = ?", keyID).Update("budget_reserved_minor", row.BudgetReservedMinor+amount).Error
}

// Every transition is called once under the authorization/charge lifecycle lock.
func AdjustAPIKeyBudgetTx(tx *gorm.DB, keyID string, usedDelta, reservedDelta int64) error {
	if keyID == "" {
		return nil
	}
	var row apiKeyRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", keyID).First(&row).Error; err != nil {
		return err
	}
	if row.BudgetUsedMinor+usedDelta < 0 || row.BudgetReservedMinor+reservedDelta < 0 {
		return ErrInvalidKeyLimits
	}
	return tx.Model(&apiKeyRow{}).Where("id = ?", keyID).Updates(map[string]any{"budget_used_minor": row.BudgetUsedMinor + usedDelta, "budget_reserved_minor": row.BudgetReservedMinor + reservedDelta}).Error
}

func (p APIKeyPrincipal) AllowsModel(model string) bool {
	if p.ModelMode != "selected" {
		return true
	}
	for _, id := range p.Allowlist {
		if id == model {
			return true
		}
	}
	return false
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
	if err := s.attachAllowlists(ctx, out); err != nil {
		return nil, err
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
	q := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", keyID, user.UserID)
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
		"prefix": raw[:8], "secret_hash": crypto.HashToken(raw), "secret_ciphertext": cipher,
	}).Error; err != nil {
		return nil, err
	}
	view := viewFromRow(*row, raw)
	view.Prefix = raw[:8]
	return s.withAllowlist(ctx, view)
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
	return s.withAllowlist(ctx, view)
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
	return s.withAllowlist(ctx, view)
}

func (s *Service) AuthenticateAPIKey(ctx context.Context, raw string) (*APIKeyPrincipal, error) {
	if raw == "" {
		return nil, nil
	}
	var row apiKeyRow
	if err := s.db.WithContext(ctx).Where("secret_hash = ?", crypto.HashToken(raw)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if row.Status != "active" {
		return nil, ErrKeyNotUsable
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(time.Now().UTC()) {
		return nil, ErrKeyExpired
	}
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&apiKeyRow{}).Where("id = ?", row.ID).Update("last_used_at", now).Error
	var user userRow
	if err := s.db.WithContext(ctx).Where("id = ? AND status = ?", row.UserID, UserStatusActive).First(&user).Error; err != nil {
		return nil, nil
	}
	disabled, err := s.staffDisabled(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if disabled {
		return nil, nil
	}
	principal, err := s.loadPrincipal(ctx, user)
	if err != nil {
		return nil, err
	}
	var policies []apiKeyPolicyRow
	if err := s.db.WithContext(ctx).Where("api_key_id = ? AND allowed = true", row.ID).Find(&policies).Error; err != nil {
		return nil, err
	}
	allow := make([]string, 0, len(policies))
	for _, policy := range policies {
		allow = append(allow, policy.PublicModelID)
	}
	return &APIKeyPrincipal{Principal: *principal, APIKeyID: row.ID, ModelMode: row.ModelMode, BudgetLimitMinor: row.BudgetLimitMinor, Allowlist: allow, RPMLimit: row.RPMLimit, ConcurrencyLimit: row.ConcurrencyLimit}, nil
}

func viewFromRow(row apiKeyRow, secret string) APIKeyView {
	return APIKeyView{
		ID: row.ID, UserID: row.UserID, Name: row.Name, Prefix: row.Prefix, Secret: secret,
		ModelMode: row.ModelMode, BudgetLimitMinor: row.BudgetLimitMinor, BudgetUsedMinor: row.BudgetUsedMinor, BudgetReservedMinor: row.BudgetReservedMinor, Status: row.Status, RPMLimit: row.RPMLimit, ConcurrencyLimit: row.ConcurrencyLimit,
		ExpiresAt: row.ExpiresAt, LastUsedAt: row.LastUsedAt, CreatedAt: row.CreatedAt,
	}
}

func normalizeAllowlist(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, model := range in {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func (s *Service) loadAllowlists(ctx context.Context, keyIDs []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(keyIDs) == 0 {
		return out, nil
	}
	var policies []apiKeyPolicyRow
	if err := s.db.WithContext(ctx).Where("api_key_id IN ? AND allowed = true", keyIDs).Order("public_model_id ASC").Find(&policies).Error; err != nil {
		return nil, err
	}
	for _, policy := range policies {
		out[policy.APIKeyID] = append(out[policy.APIKeyID], policy.PublicModelID)
	}
	return out, nil
}

func (s *Service) attachAllowlists(ctx context.Context, views []APIKeyView) error {
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ID)
	}
	lists, err := s.loadAllowlists(ctx, ids)
	if err != nil {
		return err
	}
	for i := range views {
		views[i].Allowlist = lists[views[i].ID]
		if views[i].Allowlist == nil {
			views[i].Allowlist = []string{}
		}
	}
	return nil
}

func (s *Service) withAllowlist(ctx context.Context, view APIKeyView) (*APIKeyView, error) {
	lists, err := s.loadAllowlists(ctx, []string{view.ID})
	if err != nil {
		return nil, err
	}
	view.Allowlist = lists[view.ID]
	if view.Allowlist == nil {
		view.Allowlist = []string{}
	}
	return &view, nil
}
