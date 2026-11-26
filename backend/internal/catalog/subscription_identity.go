package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const codexIdentityPrefix = "codex:"
const duplicateIdentityPrefix = "duplicate:"

var errNoCodexIdentityChanges = errors.New("no Codex account identities changed")

type subscriptionAccountIdentityRow struct {
	ID           string
	IdentityHash string
	Ciphertext   string
	Version      uint64
}

func stableSubscriptionIdentity(kind string, credential subscriptionauth.Credential) string {
	if kind == subscriptionauth.Codex {
		if account := subscriptionauth.CodexRoutingAccount(credential.AccessToken); account != "" {
			return "routing:" + account
		}
	}
	if credential.AccountID != "" {
		return "subject:" + credential.AccountID
	}
	return ""
}

func subscriptionIdentityHash(kind string, credential subscriptionauth.Credential) string {
	if kind == subscriptionauth.Codex {
		identity := stableSubscriptionIdentity(kind, credential)
		if identity == "" {
			identity = "token:" + credential.AccessToken
		}
		return codexIdentityPrefix + platformcrypto.HashToken(identity)
	}
	if credential.AccountID != "" {
		return platformcrypto.HashToken(credential.AccountID)
	}
	return platformcrypto.HashToken(credential.AccessToken)
}

func (s CommerceCatalog) openSubscriptionCredential(ciphertext string) (subscriptionauth.Credential, error) {
	plain, err := platformcrypto.Open(s.EncryptionKey, ciphertext)
	var credential subscriptionauth.Credential
	if err != nil || json.Unmarshal([]byte(plain), &credential) != nil || credential.AccessToken == "" {
		return subscriptionauth.Credential{}, subscriptionauth.ErrCredential
	}
	return credential, nil
}

// The caller holds the pool lock. The account lock also serializes credential
// comparison with a background token refresh.
func (s CommerceCatalog) checkSubscriptionAccountIdentity(ctx context.Context, kind string, in SubscriptionAccountInput, grant *subscriptionauth.Credential) (string, error) {
	db := s.DB.WithContext(ctx)
	if in.ExpectedVersion > 0 {
		var prior subscriptionAccountIdentityRow
		err := db.Table("tokenhub_subscription_accounts").Where("pool_id=? AND id=? AND version=? AND NOT removed", in.PoolID, in.ID, in.ExpectedVersion).
			Clauses(clause.Locking{Strength: "UPDATE"}).Take(&prior).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrCatalogVersion
		}
		if err != nil {
			return "", err
		}
		quarantined := strings.HasPrefix(prior.IdentityHash, duplicateIdentityPrefix)
		if grant == nil && strings.HasPrefix(prior.IdentityHash, codexIdentityPrefix) {
			return "", nil
		}
		credential, err := s.openSubscriptionCredential(prior.Ciphertext)
		if err != nil {
			if grant == nil && !quarantined {
				return "", nil
			}
			return "", ErrSubscriptionAccountIdentity
		}
		if grant == nil {
			grant = &credential
		} else {
			previous, replacement := stableSubscriptionIdentity(kind, credential), stableSubscriptionIdentity(kind, *grant)
			if previous != replacement || previous == "" && (credential.AccessToken != grant.AccessToken || credential.RefreshToken != grant.RefreshToken) {
				return "", ErrSubscriptionAccountIdentity
			}
		}
	}
	identity := subscriptionIdentityHash(kind, *grant)
	var matches []subscriptionAccountIdentityRow
	query := db.Table("tokenhub_subscription_accounts").Where("pool_id=? AND id<>? AND NOT removed", in.PoolID, in.ID)
	if kind == subscriptionauth.Codex {
		query = query.Where("identity_hash=? OR (identity_hash NOT LIKE ? AND identity_hash NOT LIKE ?)",
			identity, codexIdentityPrefix+"%", duplicateIdentityPrefix+"%")
	} else {
		query = query.Where("identity_hash=?", identity)
	}
	if err := query.Find(&matches).Error; err != nil {
		return "", err
	}
	for _, row := range matches {
		if row.IdentityHash == identity {
			return "", ErrSubscriptionAccountExists
		}
		// An older Codex browser grant was indexed by its OAuth subject.
		credential, err := s.openSubscriptionCredential(row.Ciphertext)
		if err == nil && subscriptionIdentityHash(kind, credential) == identity {
			return "", ErrSubscriptionAccountExists
		}
	}
	return identity, nil
}

// ReconcileCodexAccountIdentities indexes older grants by their routing
// account. Keep duplicate rows for historical usage and disable dispatch.
func (s CommerceCatalog) ReconcileCodexAccountIdentities(ctx context.Context) error {
	var pools []string
	if err := s.DB.WithContext(ctx).Table("tokenhub_subscription_accounts a").Distinct("a.pool_id").
		Joins("JOIN tokenhub_provider_credentials c ON c.id=a.pool_id AND c.subscription_kind=?", subscriptionauth.Codex).
		Where("NOT a.removed AND a.identity_hash NOT LIKE ? AND a.identity_hash NOT LIKE ?", codexIdentityPrefix+"%", duplicateIdentityPrefix+"%").
		Pluck("a.pool_id", &pools).Error; err != nil {
		return err
	}
	for _, poolID := range pools {
		if err := s.mutate(ctx, func(tx CommerceCatalog) error {
			if _, err := tx.lockSubscriptionPool(ctx, poolID); err != nil {
				return err
			}
			return tx.reconcileCodexPool(ctx, poolID)
		}); err != nil && !errors.Is(err, errNoCodexIdentityChanges) {
			return err
		}
	}
	return nil
}

func (s CommerceCatalog) reconcileCodexPool(ctx context.Context, poolID string) error {
	db := s.DB.WithContext(ctx)
	var accounts []subscriptionAccountIdentityRow
	if err := db.Table("tokenhub_subscription_accounts a").
		Where("a.pool_id=? AND NOT a.removed AND a.identity_hash NOT LIKE ?", poolID, duplicateIdentityPrefix+"%").
		Order("(" + subscriptionSchedulable + ") DESC").Order("a.enabled DESC").Order("a.id").
		Clauses(clause.Locking{Strength: "UPDATE"}).Find(&accounts).Error; err != nil {
		return err
	}
	winners := make(map[string]subscriptionAccountIdentityRow, len(accounts))
	changed := false
	for _, account := range accounts {
		identity := account.IdentityHash
		if !strings.HasPrefix(identity, codexIdentityPrefix) {
			credential, err := s.openSubscriptionCredential(account.Ciphertext)
			if err != nil {
				continue
			}
			identity = subscriptionIdentityHash(subscriptionauth.Codex, credential)
		}
		if _, exists := winners[identity]; !exists {
			winners[identity] = account
			continue
		}
		if err := db.Table("tokenhub_subscription_accounts").Where("id=?", account.ID).
			Updates(map[string]any{"identity_hash": duplicateIdentityPrefix + account.ID, "enabled": false, "version": account.Version + 1}).Error; err != nil {
			return err
		}
		changed = true
	}
	for identity, account := range winners {
		if account.IdentityHash != identity {
			if err := db.Table("tokenhub_subscription_accounts").Where("id=?", account.ID).Update("identity_hash", identity).Error; err != nil {
				return err
			}
			changed = true
		}
	}
	if !changed {
		return errNoCodexIdentityChanges
	}
	return nil
}
