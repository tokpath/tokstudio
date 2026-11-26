package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const subscriptionEligible = `a.enabled AND NOT a.removed AND NOT a.reauth_required AND a.identity_hash NOT LIKE 'duplicate:%'
 AND (a.subscription_ends_at IS NULL OR a.subscription_ends_at > clock_timestamp())
 AND (a.refreshable OR a.expires_at IS NULL OR a.expires_at > clock_timestamp())`

const subscriptionSchedulable = subscriptionEligible + ` AND (a.cooldown_until IS NULL OR a.cooldown_until <= clock_timestamp())`

// ClaimSubscriptionAccount runs in the same transaction as the route lease
// and the attempt record. The pool lock serializes its short slot claims.
func (s CommerceCatalog) ClaimSubscriptionAccount(ctx context.Context, attempt *modelruntime.Attempt, exclude []string) error {
	if attempt.SubscriptionKind == "" {
		return nil
	}
	if string(attempt.Capability.Operation) != subscriptionauth.Operation(attempt.SubscriptionKind) {
		return ErrSubscriptionUnavailable
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
		ID      string
		Version uint64
	}
	q := db.Table("tokenhub_subscription_accounts a").Select("a.id,a.version").
		Where("a.pool_id=? AND "+subscriptionSchedulable, pool.ID).
		Where(`NOT EXISTS (SELECT 1 FROM tokenhub_route_leases l WHERE l.subscription_account_id=a.id
		 AND l.finished_at IS NULL AND l.expires_at>clock_timestamp())`).
		Order(clause.OrderBy{Expression: clause.Expr{
			SQL:  "jsonb_exists(a.discovered_models, ?) DESC, a.last_selected_at ASC NULLS FIRST, a.id",
			Vars: []any{attempt.UpstreamModel},
		}})
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
	return nil
}

func (s CommerceCatalog) ResolveSubscriptionCredential(ctx context.Context, attempt modelruntime.Attempt, operation modelruntime.Operation) (modelruntime.SubscriptionCredential, error) {
	unavailable := modelruntime.ErrSubscriptionAuthUnavailable
	if attempt.SubscriptionKind == "" || string(operation) != subscriptionauth.Operation(attempt.SubscriptionKind) ||
		subscriptionauth.Protocol(attempt.SubscriptionKind) != attempt.Provider {
		return modelruntime.SubscriptionCredential{}, unavailable
	}
	var row struct {
		Ciphertext string
		Version    uint64
	}
	db := s.DB.WithContext(ctx)
	err := db.Table("tokenhub_subscription_accounts a").Select("a.ciphertext,a.version").
		Joins("JOIN tokenhub_provider_credentials c ON c.id=a.pool_id AND c.subscription_kind=?", attempt.SubscriptionKind).
		Joins("JOIN tokenhub_upstreams u ON u.provider_id=c.provider_id AND u.definition->>'credential_id'=c.id AND u.definition->>'enabled'='true'").
		Joins("JOIN tokenhub_route_leases l ON l.upstream_id=u.id AND l.subscription_account_id=a.id AND l.subscription_account_version=a.version").
		Where("a.id=? AND c.id=? AND c.enabled AND u.id=? AND l.attempt_id=? AND l.request_id=? AND l.finished_at IS NULL AND l.expires_at>clock_timestamp()",
			attempt.SubscriptionAccountID, attempt.CredentialID, attempt.UpstreamID, attempt.ID, attempt.RequestID).
		Where(subscriptionSchedulable).Take(&row).Error
	if err != nil {
		return modelruntime.SubscriptionCredential{}, unavailable
	}
	credential, err := s.openSubscriptionCredential(row.Ciphertext)
	if err != nil {
		return modelruntime.SubscriptionCredential{}, modelruntime.ErrSubscriptionReauth
	}
	var connection subscriptionauth.Connection
	if credential.Connection != "" {
		profile, ok := s.SubscriptionConnections[attempt.SubscriptionKind]
		if !ok || profile.Fingerprint() != credential.Connection {
			return modelruntime.SubscriptionCredential{}, modelruntime.ErrSubscriptionReauth
		}
		connection = profile
	} else if attempt.SubscriptionKind != subscriptionauth.Codex && attempt.SubscriptionKind != subscriptionauth.Claude {
		return modelruntime.SubscriptionCredential{}, modelruntime.ErrSubscriptionReauth
	}
	if refreshSubscriptionGrant(credential, time.Now()) {
		// Re-read the rotating refresh token under the account lock.
		persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		err := s.DB.WithContext(persist).Transaction(func(tx *gorm.DB) error {
			account := tx.Table("tokenhub_subscription_accounts").
				Where("id=? AND version=? AND NOT removed AND enabled", attempt.SubscriptionAccountID, row.Version)
			var current struct{ Ciphertext string }
			if err := account.Select("ciphertext").Clauses(clause.Locking{Strength: "UPDATE"}).Take(&current).Error; err != nil {
				return unavailable
			}
			if current.Ciphertext != row.Ciphertext {
				refreshed, err := s.openSubscriptionCredential(current.Ciphertext)
				if err != nil {
					return modelruntime.ErrSubscriptionReauth
				}
				credential = refreshed
				if !refreshSubscriptionGrant(credential, time.Now()) {
					return nil
				}
			}
			var refreshErr error
			previousIdentity := stableSubscriptionIdentity(attempt.SubscriptionKind, credential)
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
			if stableSubscriptionIdentity(attempt.SubscriptionKind, credential) != previousIdentity {
				return modelruntime.ErrSubscriptionReauth
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
	return modelruntime.SubscriptionCredential{AccessToken: credential.AccessToken, AccountID: credential.AccountID, Endpoint: connection.InferenceURL}, nil
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
