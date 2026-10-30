package usage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Key struct {
	ID               string          `json:"id"`
	CustomerID       string          `json:"customer_id"`
	BrandID          string          `json:"brand_id"`
	Name             string          `json:"name"`
	Kind             string          `json:"kind"`
	Prefix           string          `json:"prefix"`
	TokenHash        string          `json:"-"`
	Status           string          `json:"status"`
	ModelIDs         json.RawMessage `json:"model_ids" gorm:"type:jsonb"`
	BudgetLimit      *ledger.Decimal `json:"budget_limit"`
	Period           ledger.Period   `json:"period"`
	Timezone         string          `json:"timezone"`
	RateLimitRPM     *int64          `json:"rate_limit_rpm"`
	ConcurrencyLimit *int64          `json:"concurrency_limit"`
	Version          int64           `json:"version"`
	SecretCiphertext string          `json:"-" gorm:"column:secret_ciphertext"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (Key) TableName() string { return "usage_keys" }

type KeyInput struct {
	Name                           string          `json:"name"`
	ModelIDs                       []string        `json:"model_ids"`
	BudgetLimit                    *ledger.Decimal `json:"budget_limit"`
	Period                         ledger.Period   `json:"period"`
	Timezone                       string          `json:"timezone"`
	RateLimitRPM, ConcurrencyLimit *int64
}
type KeySecret struct {
	Key    Key    `json:"key"`
	Secret string `json:"secret"`
}
type keyCommand struct {
	CustomerID, BusinessKey, KeyID, Action, Fingerprint string
	CreatedAt                                           time.Time
}

func (keyCommand) TableName() string { return "usage_key_commands" }
func validateKey(in KeyInput) error {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 100 || (in.BudgetLimit != nil && (!in.BudgetLimit.Valid() || in.BudgetLimit.Sign() < 0)) || (in.RateLimitRPM != nil && *in.RateLimitRPM < 1) || (in.ConcurrencyLimit != nil && *in.ConcurrencyLimit < 1) {
		return ErrKey
	}
	if _, e := ledger.BudgetWindow(time.Now(), time.Now().Add(-time.Second), in.Period, in.Timezone); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, id := range in.ModelIDs {
		if strings.TrimSpace(id) == "" || seen[id] {
			return ErrKey
		}
		seen[id] = true
	}
	return nil
}
func (s *Service) CreateKey(ctx context.Context, customer, businessKey string, in KeyInput) (KeySecret, error) {
	var out KeySecret
	if businessKey == "" || s.EncryptionKey == "" {
		return out, ErrKey
	}
	if e := validateKey(in); e != nil {
		return out, e
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "key-command:"+customer+":"+businessKey).Error; e != nil {
			return e
		}
		var command keyCommand
		e := t.First(&command, "customer_id=? AND business_key=?", customer, businessKey).Error
		if e == nil {
			if command.Fingerprint != s.fingerprint(in) || command.Action != "create" {
				return ErrConflict
			}
			return ErrSecretDisplayed
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		c, e := s.Organization.GetCustomer(ctx, t, customer)
		if e != nil {
			return e
		}
		if c.Status != "active" {
			return ErrKey
		}
		secret, e := platformcrypto.RandomToken("th_")
		if e != nil {
			return e
		}
		models, _ := json.Marshal(in.ModelIDs)
		// Creation is the start of the lifetime budget window. Use the same
		// database clock as admission, even when API hosts have clock skew.
		now, e := s.businessTime(t)
		if e != nil {
			return e
		}
		sealed, e := s.sealSecret(secret)
		if e != nil {
			return e
		}
		k := Key{ID: uuid.NewString(), CustomerID: customer, BrandID: c.BrandID, Name: in.Name, Prefix: secret[:11], TokenHash: platformcrypto.HashToken(secret), SecretCiphertext: sealed, Status: "active", ModelIDs: models, BudgetLimit: in.BudgetLimit, Period: in.Period, Timezone: in.Timezone, RateLimitRPM: in.RateLimitRPM, ConcurrencyLimit: in.ConcurrencyLimit, Version: 1, CreatedAt: now, UpdatedAt: now}
		k.Kind = "api"
		if e = t.Create(&k).Error; e != nil {
			return e
		}
		command = keyCommand{CustomerID: customer, BusinessKey: businessKey, KeyID: k.ID, Action: "create", Fingerprint: s.fingerprint(in), CreatedAt: now}
		if e = t.Create(&command).Error; e != nil {
			return e
		}
		out = KeySecret{k, secret}
		return nil
	})
	return out, e
}
func (s *Service) UpdateKey(ctx context.Context, customer, id string, expected int64, in KeyInput) (Key, error) {
	var out Key
	if e := validateKey(in); e != nil {
		return out, e
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=? AND customer_id=? AND kind='api'", id, customer).Error; e != nil {
			return e
		}
		if out.Status == "deleted" || out.Version != expected {
			return ErrConflict
		}
		models, _ := json.Marshal(in.ModelIDs)
		out.Name = in.Name
		out.ModelIDs = models
		out.BudgetLimit = in.BudgetLimit
		out.Period = in.Period
		out.Timezone = in.Timezone
		out.RateLimitRPM = in.RateLimitRPM
		out.ConcurrencyLimit = in.ConcurrencyLimit
		out.Version++
		out.UpdatedAt = s.wallNow()
		return t.Save(&out).Error
	})
	return out, e
}
func (s *Service) SetKeyState(ctx context.Context, customer, id, state string, expected int64) (Key, error) {
	var out Key
	if state != "active" && state != "disabled" && state != "deleted" {
		return out, ErrKey
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=? AND customer_id=? AND kind='api'", id, customer).Error; e != nil {
			return e
		}
		if out.Status == state {
			return nil
		}
		if out.Status == "deleted" || out.Version != expected {
			return ErrConflict
		}
		out.Status = state
		out.Version++
		out.UpdatedAt = s.wallNow()
		return t.Save(&out).Error
	})
	return out, e
}
func (s *Service) ResetKey(ctx context.Context, customer, id, businessKey string, expected int64) (KeySecret, error) {
	var out KeySecret
	if businessKey == "" {
		return out, ErrKey
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		var k Key
		if e := t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&k, "id=? AND customer_id=? AND kind='api'", id, customer).Error; e != nil {
			return e
		}
		var old keyCommand
		e := t.First(&old, "customer_id=? AND business_key=?", customer, businessKey).Error
		if e == nil {
			if old.KeyID != id || old.Action != "reset" {
				return ErrConflict
			}
			return ErrSecretDisplayed
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if k.Version != expected || k.Status == "deleted" {
			return ErrConflict
		}
		secret, e := platformcrypto.RandomToken("th_")
		if e != nil {
			return e
		}
		sealed, e := s.sealSecret(secret)
		if e != nil {
			return e
		}
		k.TokenHash = platformcrypto.HashToken(secret)
		k.Prefix = secret[:11]
		k.SecretCiphertext = sealed
		k.Version++
		k.UpdatedAt = s.wallNow()
		if e = t.Save(&k).Error; e != nil {
			return e
		}
		command := keyCommand{CustomerID: customer, BusinessKey: businessKey, KeyID: id, Action: "reset", Fingerprint: s.fingerprint([]string{id, "reset"}), CreatedAt: s.wallNow()}
		if e = t.Create(&command).Error; e != nil {
			return e
		}
		out = KeySecret{k, secret}
		return nil
	})
	return out, e
}
func (s *Service) AuthenticateKey(ctx context.Context, brand, secret string) (Key, error) {
	var k Key
	if len(secret) < 24 || len(secret) > 256 {
		return k, ErrKey
	}
	e := s.DB.WithContext(ctx).First(&k, "token_hash=? AND brand_id=? AND kind='api' AND status='active'", platformcrypto.HashToken(secret), brand).Error
	if e != nil {
		return Key{}, ErrKey
	}
	return k, nil
}
func (s *Service) sealSecret(secret string) (string, error) {
	if s.EncryptionKey == "" {
		return "", ErrKey
	}
	return platformcrypto.Seal(s.EncryptionKey, secret)
}

// OpenKeySecret returns the stored API key for its owner. Keys created before
// the ciphertext was kept have nothing to reveal.
func (s *Service) OpenKeySecret(k Key) (string, error) {
	if k.SecretCiphertext == "" || k.Kind != "api" || k.Status == "deleted" {
		return "", nil
	}
	return platformcrypto.Open(s.EncryptionKey, k.SecretCiphertext)
}
func (s *Service) ListKeys(ctx context.Context, customer string) ([]Key, error) {
	var rows []Key
	e := s.DB.WithContext(ctx).Where("customer_id=? AND kind='api' AND status<>'deleted'", customer).Order("created_at,id").Find(&rows).Error
	return rows, e
}

type Budget struct {
	LimitATG    *ledger.Decimal `json:"limit_atg"`
	Period      ledger.Period   `json:"period"`
	Timezone    string          `json:"timezone"`
	SpentATG    ledger.Decimal  `json:"spent_atg"`
	HeldATG     ledger.Decimal  `json:"held_atg"`
	OverageATG  ledger.Decimal  `json:"overage_atg"`
	WindowStart time.Time       `json:"window_start"`
	WindowEnd   *time.Time      `json:"window_end"`
}

func (s *Service) KeyBudget(ctx context.Context, tx *gorm.DB, k Key, at time.Time) (Budget, error) {
	if at.IsZero() {
		t := tx
		if t == nil {
			t = s.DB.WithContext(ctx)
		}
		var e error
		at, e = s.businessTime(t)
		if e != nil {
			return Budget{}, e
		}
	}
	w, e := ledger.BudgetWindow(at, k.CreatedAt, k.Period, k.Timezone)
	if e != nil {
		return Budget{}, e
	}
	out := Budget{LimitATG: k.BudgetLimit, Period: k.Period, Timezone: k.Timezone, WindowStart: w.Start, WindowEnd: w.End}
	e = s.within(ctx, tx, func(t *gorm.DB) error {
		q := t.Model(&Request{}).Where("key_id=? AND accepted_at>=?", k.ID, w.Start)
		if w.End != nil {
			q = q.Where("accepted_at<?", *w.End)
		}
		var sums struct{ Spent, Held ledger.Decimal }
		if e := q.Select("COALESCE(sum(actual_atg-corrected_atg),0)::text AS spent, COALESCE(sum(CASE WHEN status IN ('accepted','executing','submitted','awaiting_usage') THEN estimate_atg ELSE 0 END),0)::text AS held").Scan(&sums).Error; e != nil {
			return e
		}
		out.SpentATG = sums.Spent
		out.HeldATG = sums.Held
		if k.BudgetLimit != nil && sums.Spent.Cmp(*k.BudgetLimit) > 0 {
			out.OverageATG = sums.Spent.Sub(*k.BudgetLimit)
		}
		return nil
	})
	return out, e
}
