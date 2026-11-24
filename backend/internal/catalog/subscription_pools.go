package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrSubscriptionAccountExists = errors.New("subscription account already exists in pool")
	ErrSubscriptionModelLimits   = errors.New("subscription model needs verified token limits")
	ErrSubscriptionUnavailable   = errors.New("subscription pool has no available account")
)

type SubscriptionPool struct {
	CredentialView
	Accounts []SubscriptionAccountView `json:"accounts"`
}

type SubscriptionAccountView struct {
	ID                 string     `json:"id"`
	PoolID             string     `json:"pool_id"`
	Name               string     `json:"name"`
	Enabled            bool       `json:"enabled"`
	Version            uint64     `json:"version"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	SubscriptionEndsAt *time.Time `json:"subscription_ends_at,omitempty"`
	CooldownUntil      *time.Time `json:"cooldown_until,omitempty"`
	Refreshable        bool       `json:"-"`
	ReauthRequired     bool       `json:"-"`
	Busy               bool       `json:"-"`
	Status             string     `json:"status" gorm:"-"`
}

type SubscriptionAccountInput struct {
	ID                 string     `json:"id"`
	PoolID             string     `json:"pool_id"`
	Name               string     `json:"name"`
	Credential         string     `json:"credential"`
	Enabled            bool       `json:"enabled"`
	SubscriptionEndsAt *time.Time `json:"subscription_ends_at"`
	ExpectedVersion    uint64     `json:"expected_version"`
}

type SubscriptionPoolInput struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	Kind            string                    `json:"subscription_kind"`
	Enabled         bool                      `json:"enabled"`
	ExpectedVersion uint64                    `json:"expected_version"`
	FirstAccount    *SubscriptionAccountInput `json:"first_account,omitempty"`
}

const subscriptionAccountColumns = "id,pool_id,name,enabled,version,expires_at,subscription_ends_at,cooldown_until,refreshable,reauth_required"

func (s CommerceCatalog) SubscriptionPools(ctx context.Context, actor string) ([]SubscriptionPool, error) {
	if !s.platform(actor) {
		return nil, ErrCatalogForbidden
	}
	var pools []CredentialView
	if err := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").
		Select("id,provider_id,name,enabled,version,subscription_kind").Where("subscription_kind<>''").
		Order("name,id").Find(&pools).Error; err != nil {
		return nil, err
	}
	var rows []SubscriptionAccountView
	if err := s.DB.WithContext(ctx).Table("tokenhub_subscription_accounts a").
		Select(subscriptionAccountColumns + `, EXISTS (SELECT 1 FROM tokenhub_route_leases l
		 WHERE l.subscription_account_id=a.id AND l.finished_at IS NULL AND l.expires_at>clock_timestamp()) AS busy`).
		Where("NOT removed").
		Order("name,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	byPool := make(map[string][]SubscriptionAccountView)
	for _, row := range rows {
		row.Status = row.status(now)
		byPool[row.PoolID] = append(byPool[row.PoolID], row)
	}
	out := make([]SubscriptionPool, 0, len(pools))
	for _, pool := range pools {
		accounts := byPool[pool.ID]
		if accounts == nil {
			accounts = []SubscriptionAccountView{}
		}
		out = append(out, SubscriptionPool{CredentialView: pool, Accounts: accounts})
	}
	return out, nil
}

func (a SubscriptionAccountView) status(now time.Time) string {
	switch {
	case !a.Enabled:
		return "paused"
	case a.SubscriptionEndsAt != nil && !a.SubscriptionEndsAt.After(now):
		return "subscription_expired"
	case a.ReauthRequired || !a.Refreshable && a.ExpiresAt != nil && !a.ExpiresAt.After(now):
		return "reauth_required"
	case a.CooldownUntil != nil && a.CooldownUntil.After(now):
		return "cooldown"
	case a.Busy:
		return "busy"
	default:
		return "available"
	}
}

func (s CommerceCatalog) SaveSubscriptionPool(ctx context.Context, actor string, in SubscriptionPoolInput) (string, error) {
	if !s.platform(actor) {
		return "", ErrCatalogForbidden
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 80 || subscriptionauth.Protocol(in.Kind) == "" {
		return "", ErrInvalidInput
	}
	if in.ExpectedVersion == 0 {
		in.ID = uuid.NewString()
	} else if in.ID == "" || in.FirstAccount != nil {
		return "", ErrInvalidInput
	}
	err := s.mutate(ctx, func(tx CommerceCatalog) error {
		pools := tx.DB.WithContext(ctx).Table("tokenhub_provider_credentials")
		if in.ExpectedVersion > 0 {
			result := pools.Where("id=? AND subscription_kind=? AND version=?", in.ID, in.Kind, in.ExpectedVersion).
				Updates(map[string]any{"name": in.Name, "enabled": in.Enabled, "version": in.ExpectedVersion + 1})
			return subscriptionVersion(result)
		}
		if err := pools.Create(map[string]any{
			"id": in.ID, "provider_id": in.ID, "name": in.Name, "enabled": in.Enabled,
			"subscription_kind": in.Kind, "ciphertext": "", "fingerprint": "", "version": 1,
		}).Error; err != nil || in.FirstAccount == nil {
			return err
		}
		first := *in.FirstAccount
		first.ID, first.PoolID, first.ExpectedVersion, first.Enabled = uuid.NewString(), in.ID, 0, true
		return tx.saveSubscriptionAccount(ctx, in.Kind, first)
	})
	return in.ID, err
}

func (s CommerceCatalog) SaveSubscriptionAccount(ctx context.Context, actor string, in SubscriptionAccountInput) (string, error) {
	if !s.platform(actor) {
		return "", ErrCatalogForbidden
	}
	if in.ExpectedVersion == 0 {
		in.ID = uuid.NewString()
	}
	err := s.mutate(ctx, func(tx CommerceCatalog) error {
		pool, err := tx.lockSubscriptionPool(ctx, in.PoolID)
		if err != nil {
			return err
		}
		return tx.saveSubscriptionAccount(ctx, pool.SubscriptionKind, in)
	})
	return in.ID, err
}

func (s CommerceCatalog) saveSubscriptionAccount(ctx context.Context, kind string, in SubscriptionAccountInput) error {
	in.Name = strings.TrimSpace(in.Name)
	if in.ID == "" || in.Name == "" || len([]rune(in.Name)) > 80 || (in.ExpectedVersion == 0 && in.Credential == "") {
		return ErrInvalidInput
	}
	fields := map[string]any{"name": in.Name, "enabled": in.Enabled, "version": in.ExpectedVersion + 1, "subscription_ends_at": in.SubscriptionEndsAt}
	if in.Credential != "" {
		credential, err := subscriptionauth.Parse(kind, in.Credential)
		if err != nil {
			return err
		}
		cipher, err := s.sealSubscriptionCredential(credential)
		if err != nil {
			return err
		}
		identity := credential.AccountID
		if identity == "" {
			identity = credential.AccessToken
		}
		fields["ciphertext"] = cipher
		fields["identity_hash"] = platformcrypto.HashToken(identity)
		fields["expires_at"] = optionalTime(credential.ExpiresAt)
		fields["refreshable"] = credential.RefreshToken != ""
		fields["reauth_required"] = false
		fields["cooldown_until"] = nil
	}
	accounts := s.DB.WithContext(ctx).Table("tokenhub_subscription_accounts")
	// Account edits and claims take the same pool lock, so conflict checks and
	// a replacement of rotating credentials are atomic across workers.
	if identity, ok := fields["identity_hash"]; ok {
		var duplicate bool
		if err := s.DB.WithContext(ctx).Raw(`SELECT EXISTS (SELECT 1 FROM tokenhub_subscription_accounts
		 WHERE pool_id=? AND identity_hash=? AND id<>?)`, in.PoolID, identity, in.ID).Scan(&duplicate).Error; err != nil {
			return err
		}
		if duplicate {
			return ErrSubscriptionAccountExists
		}
	}
	if in.ExpectedVersion == 0 {
		fields["id"], fields["pool_id"] = in.ID, in.PoolID
		return accounts.Create(fields).Error
	}
	return subscriptionVersion(accounts.Where("pool_id=? AND id=? AND version=? AND NOT removed", in.PoolID, in.ID, in.ExpectedVersion).Updates(fields))
}

func (s CommerceCatalog) RemoveSubscriptionAccount(ctx context.Context, actor, poolID, accountID string, version uint64) error {
	if !s.platform(actor) {
		return ErrCatalogForbidden
	}
	return s.mutate(ctx, func(tx CommerceCatalog) error {
		if _, err := tx.lockSubscriptionPool(ctx, poolID); err != nil {
			return err
		}
		r := tx.DB.WithContext(ctx).Table("tokenhub_subscription_accounts").Where("pool_id=? AND id=? AND version=? AND NOT removed", poolID, accountID, version).
			Updates(map[string]any{"removed": true, "enabled": false, "ciphertext": "", "identity_hash": accountID, "version": version + 1})
		return subscriptionVersion(r)
	})
}

func (s CommerceCatalog) lockSubscriptionPool(ctx context.Context, id string) (CredentialView, error) {
	var pool CredentialView
	err := s.DB.WithContext(ctx).Table("tokenhub_provider_credentials").
		Select("id,enabled,subscription_kind").Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id=? AND subscription_kind<>''", id).Take(&pool).Error
	return pool, err
}

func subscriptionVersion(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrCatalogVersion
	}
	return nil
}

func (s CommerceCatalog) sealSubscriptionCredential(c subscriptionauth.Credential) (string, error) {
	if s.EncryptionKey == "" {
		return "", ErrInvalidInput
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return platformcrypto.Seal(s.EncryptionKey, string(raw))
}

func optionalTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}
