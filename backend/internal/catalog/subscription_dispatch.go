package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const subscriptionSchedulable = `a.enabled AND NOT a.removed AND NOT a.reauth_required
 AND (a.subscription_ends_at IS NULL OR a.subscription_ends_at > clock_timestamp())
 AND (a.cooldown_until IS NULL OR a.cooldown_until <= clock_timestamp())
 AND (a.refreshable OR a.expires_at IS NULL OR a.expires_at > clock_timestamp())`

// ClaimSubscriptionAccount runs in the same transaction as the route lease
// and the attempt record. The pool lock serializes its short slot claims.
func (s CommerceCatalog) ClaimSubscriptionAccount(ctx context.Context, attempt *modelruntime.Attempt, exclude []string) error {
	if attempt.SubscriptionKind == "" {
		return nil
	}
	pool, err := s.lockSubscriptionPool(ctx, attempt.CredentialID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSubscriptionUnavailable
	}
	if err != nil {
		return err
	}
	if !pool.Enabled || pool.SubscriptionKind != attempt.SubscriptionKind || subscriptionauth.Protocol(pool.SubscriptionKind) != attempt.Provider {
		return ErrSubscriptionUnavailable
	}
	db := s.DB.WithContext(ctx)
	var account struct {
		ID           string
		Version      uint64
		IdentityHash string
	}
	q := db.Table("tokenhub_subscription_accounts a").Select("a.id,a.version,a.identity_hash").
		Where("a.pool_id=? AND "+subscriptionSchedulable, pool.ID).
		Where(`NOT EXISTS (SELECT 1 FROM tokenhub_route_leases l WHERE l.subscription_account_id=a.id
		 AND l.finished_at IS NULL AND l.expires_at>clock_timestamp())`).
		Order("a.last_selected_at ASC NULLS FIRST,a.id")
	if len(exclude) > 0 {
		q = q.Where("a.id NOT IN ?", exclude)
	}
	if err = q.Take(&account).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrSubscriptionUnavailable
	} else if err != nil {
		return err
	}
	r := db.Model(&routeLease{}).Where("attempt_id=? AND upstream_id=? AND request_id=? AND subscription_account_id IS NULL", attempt.ID, attempt.UpstreamID, attempt.RequestID).
		Updates(map[string]any{"subscription_account_id": account.ID, "subscription_account_version": account.Version})
	if err = subscriptionVersion(r); err != nil {
		return err
	}
	if err = db.Exec("UPDATE tokenhub_subscription_accounts SET last_selected_at=clock_timestamp() WHERE id=?", account.ID).Error; err != nil {
		return err
	}
	attempt.SubscriptionAccountID = account.ID
	attempt.SubscriptionIdentity = account.IdentityHash
	return nil
}

func (s CommerceCatalog) ResolveSubscriptionCredential(ctx context.Context, attempt modelruntime.Attempt, operation modelruntime.Operation) (modelruntime.SubscriptionCredential, error) {
	unavailable := modelruntime.ErrSubscriptionAuthUnavailable
	var row struct {
		Ciphertext string
		Version    uint64
	}
	db := s.DB.WithContext(ctx)
	accounts := db.Table("tokenhub_subscription_accounts a").Select("a.ciphertext,a.version").
		Joins("JOIN tokenhub_provider_credentials c ON c.id=a.pool_id AND c.subscription_kind=?", attempt.SubscriptionKind).
		Where("a.id=? AND c.id=?", attempt.SubscriptionAccountID, attempt.CredentialID)
	lifecycle := operation == modelruntime.VideoRetrieve || operation == modelruntime.VideoDownload
	if lifecycle {
		accounts = accounts.Where("NOT a.removed AND a.identity_hash=? AND EXISTS ("+
			"SELECT 1 FROM usage_attempts run WHERE run.id=? AND run.request_id=? AND run.upstream_id=? AND run.task_id=? "+
			"AND run.definition->>'subscription_account_id'=a.id AND run.definition->>'subscription_identity'=a.identity_hash)",
			attempt.SubscriptionIdentity, attempt.ID, attempt.RequestID, attempt.UpstreamID, attempt.TaskID)
	} else {
		accounts = accounts.Joins("JOIN tokenhub_upstreams u ON u.provider_id=c.provider_id AND u.definition->>'credential_id'=c.id AND u.definition->>'enabled'='true'").
			Joins("JOIN tokenhub_route_leases l ON l.upstream_id=u.id AND l.subscription_account_id=a.id AND l.subscription_account_version=a.version").
			Where("c.enabled AND u.id=? AND l.attempt_id=? AND l.request_id=? AND l.finished_at IS NULL AND l.expires_at>clock_timestamp()",
				attempt.UpstreamID, attempt.ID, attempt.RequestID).Where(subscriptionSchedulable)
	}
	err := accounts.Take(&row).Error
	if err != nil || subscriptionauth.Protocol(attempt.SubscriptionKind) != attempt.Provider {
		return modelruntime.SubscriptionCredential{}, unavailable
	}
	plain, err := platformcrypto.Open(s.EncryptionKey, row.Ciphertext)
	var credential subscriptionauth.Credential
	if err != nil || json.Unmarshal([]byte(plain), &credential) != nil || credential.AccessToken == "" {
		return modelruntime.SubscriptionCredential{}, modelruntime.ErrSubscriptionReauth
	}
	var connection subscriptionauth.Connection
	if credential.Connection != "" {
		profile, ok := s.SubscriptionConnections[attempt.SubscriptionKind]
		if !ok || profile.Fingerprint() != credential.Connection {
			return modelruntime.SubscriptionCredential{}, modelruntime.ErrSubscriptionReauth
		}
		connection = profile
	} else if subscriptionauth.MediaAllowed(attempt.SubscriptionKind, string(operation)) ||
		attempt.SubscriptionKind != subscriptionauth.Codex && attempt.SubscriptionKind != subscriptionauth.Claude {
		return modelruntime.SubscriptionCredential{}, modelruntime.ErrSubscriptionReauth
	}
	endpoint := connection.InferenceURL
	var protocol string
	if subscriptionauth.MediaAllowed(attempt.SubscriptionKind, string(operation)) {
		media, ok := connection.MediaEndpoint(string(operation))
		if operation == modelruntime.VideoDownload && !ok {
			media, ok = connection.MediaEndpoint(subscriptionauth.VideoRetrieve)
			ok = ok && media.Protocol == "xai"
		}
		if !ok || attempt.Capability.Streaming && !media.Streaming {
			return modelruntime.SubscriptionCredential{}, unavailable
		}
		endpoint, err = media.Resolve(attempt.UpstreamModel, attempt.TaskID)
		if err != nil {
			return modelruntime.SubscriptionCredential{}, unavailable
		}
		protocol = media.Protocol
	}
	if refreshSubscriptionGrant(credential, time.Now()) {
		// Lifecycle polls and the worker can need the same rotating refresh
		// token. Re-read it under the account lock before contacting the issuer.
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		err := s.DB.WithContext(persist).Transaction(func(tx *gorm.DB) error {
			account := tx.Table("tokenhub_subscription_accounts").
				Where("id=? AND version=? AND NOT removed AND (enabled OR ?)", attempt.SubscriptionAccountID, row.Version, lifecycle)
			var current struct{ Ciphertext string }
			if err := account.Select("ciphertext").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error; err != nil {
				return unavailable
			}
			if current.Ciphertext != row.Ciphertext {
				plain, err := platformcrypto.Open(s.EncryptionKey, current.Ciphertext)
				var refreshed subscriptionauth.Credential
				if err != nil || json.Unmarshal([]byte(plain), &refreshed) != nil || refreshed.AccessToken == "" {
					return modelruntime.ErrSubscriptionReauth
				}
				credential = refreshed
				if !refreshSubscriptionGrant(credential, time.Now()) {
					return nil
				}
			}
			var refreshErr error
			if credential.Connection != "" {
				credential, refreshErr = connection.Refresh(persist, credential, s.SubscriptionGuard)
			} else {
				credential, refreshErr = subscriptionauth.Refresh(persist, attempt.SubscriptionKind, credential)
			}
			if errors.Is(refreshErr, subscriptionauth.ErrReauth) {
				return modelruntime.ErrSubscriptionReauth
			}
			if refreshErr != nil || !credential.ExpiresAt.After(time.Now()) {
				return unavailable
			}
			cipher, err := s.sealSubscriptionCredential(credential)
			if err != nil {
				return unavailable
			}
			return subscriptionVersion(tx.Table("tokenhub_subscription_accounts").Where("id=?", attempt.SubscriptionAccountID).
				Updates(map[string]any{"ciphertext": cipher, "expires_at": credential.ExpiresAt, "refreshable": credential.RefreshToken != ""}))
		})
		if errors.Is(err, modelruntime.ErrSubscriptionReauth) {
			return modelruntime.SubscriptionCredential{}, err
		}
		if err != nil {
			return modelruntime.SubscriptionCredential{}, unavailable
		}
	}
	return modelruntime.SubscriptionCredential{AccessToken: credential.AccessToken, AccountID: credential.AccountID, Endpoint: endpoint, Protocol: protocol}, nil
}

func refreshSubscriptionGrant(credential subscriptionauth.Credential, now time.Time) bool {
	return !credential.ExpiresAt.IsZero() && (credential.ExpiresAt.Before(now) ||
		credential.RefreshToken != "" && credential.ExpiresAt.Before(now.Add(time.Minute)))
}

func observeSubscriptionAccount(db *gorm.DB, lease routeLease, result modelruntime.Result, now time.Time) error {
	account := db.Table("tokenhub_subscription_accounts").
		Where("id=? AND version=? AND NOT removed", lease.SubscriptionAccountID, lease.SubscriptionAccountVersion)
	if result.ProviderStatus == 401 || result.ErrorCode == "subscription_reauth_required" {
		return account.Update("reauth_required", true).Error
	}
	until := now.Add(time.Minute)
	if result.SubscriptionRetryAt != nil && result.SubscriptionRetryAt.After(now) {
		until = *result.SubscriptionRetryAt
	}
	return account.Update("cooldown_until", until).Error
}
