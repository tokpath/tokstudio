package catalog

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
)

func codexAccountToken(workspace, tokenID string) string {
	claims := fmt.Sprintf(`{"exp":%d,"jti":%q,"https://api.openai.com/auth":{"chatgpt_account_id":%q}}`, time.Now().Add(time.Hour).Unix(), tokenID, workspace)
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".fixture"
}

func TestPostgresCodexAuthorizationMatchesTheRoutingAccount(t *testing.T) {
	s := newCommerceTestStore(t)
	ctx := t.Context()
	connection := subscriptionauth.Connection{ClientID: "codex-browser-test"}
	s.SubscriptionConnections = subscriptionauth.Connections{subscriptionauth.Codex: connection}
	browser := func(workspace, tokenID string) subscriptionauth.Credential {
		return subscriptionauth.Credential{
			AccountID: "browser-subject", AccessToken: codexAccountToken(workspace, tokenID),
			Connection: connection.Fingerprint(), ExpiresAt: time.Now().Add(time.Hour),
		}
	}
	native := func(workspace, tokenID string) string {
		data, _ := json.Marshal(subscriptionauth.Credential{AccessToken: codexAccountToken(workspace, tokenID)})
		return string(data)
	}
	for _, browserFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("browser_first_%t", browserFirst), func(t *testing.T) {
			pool, err := s.SaveSubscriptionPool(ctx, "platform", SubscriptionPoolInput{Name: "Codex", Kind: subscriptionauth.Codex, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			original := SubscriptionAccountInput{PoolID: pool, Name: "Original account", Enabled: true}
			var id string
			if browserFirst {
				id, err = s.SaveAuthorizedSubscriptionAccount(ctx, "platform", subscriptionauth.Codex, original, browser("workspace-a", "browser-first"))
				if err != nil {
					t.Fatal(err)
				}
				imported := SubscriptionAccountInput{PoolID: pool, Name: "Duplicate", Enabled: true, Credential: native("workspace-a", "native-second")}
				_, err = s.SaveSubscriptionAccount(ctx, "platform", imported)
			} else {
				imported := original
				imported.Credential = native("workspace-a", "native-first")
				id, err = s.SaveSubscriptionAccount(ctx, "platform", imported)
				if err != nil {
					t.Fatal(err)
				}
				_, err = s.SaveAuthorizedSubscriptionAccount(ctx, "platform", subscriptionauth.Codex, original, browser("workspace-a", "browser-second"))
			}
			if !errors.Is(err, ErrSubscriptionAccountExists) {
				t.Fatalf("duplicate routing account was not rejected: %v", err)
			}
			original.ID, original.ExpectedVersion = id, 1
			if _, err := s.SaveAuthorizedSubscriptionAccount(ctx, "platform", subscriptionauth.Codex, original, browser("workspace-b", "wrong-workspace")); !errors.Is(err, ErrSubscriptionAccountIdentity) {
				t.Fatalf("reauthorization accepted a different routing account: %v", err)
			}
			if _, err := s.SaveAuthorizedSubscriptionAccount(ctx, "platform", subscriptionauth.Codex, original, browser("workspace-a", "rotated")); err != nil {
				t.Fatalf("same-account reauthorization failed: %v", err)
			}
			original.ID, original.ExpectedVersion = "", 0
			if _, err := s.SaveAuthorizedSubscriptionAccount(ctx, "platform", subscriptionauth.Codex, original, browser("workspace-b", "different-workspace")); err != nil {
				t.Fatalf("distinct routing account with same OAuth subject was rejected: %v", err)
			}
		})
	}
}

func TestPostgresLegacyCodexDuplicatesAreRetainedAndCannotBeScheduledTogether(t *testing.T) {
	s := newCommerceTestStore(t)
	ctx := t.Context()
	pool, err := s.SaveSubscriptionPool(ctx, "platform", SubscriptionPoolInput{Name: "Legacy Codex", Kind: subscriptionauth.Codex, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	insert := func(id, legacyIdentity, ciphertext string, enabled, reauth bool) {
		t.Helper()
		if err := s.DB.Table("tokenhub_subscription_accounts").Create(map[string]any{
			"id": id, "pool_id": pool, "name": id, "ciphertext": ciphertext, "identity_hash": platformcrypto.HashToken(legacyIdentity),
			"enabled": enabled, "reauth_required": reauth, "version": 1, "refreshable": false,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	seal := func(identity, tokenID string) string {
		t.Helper()
		ciphertext, err := s.sealSubscriptionCredential(subscriptionauth.Credential{AccountID: identity, AccessToken: codexAccountToken("shared-workspace", tokenID)})
		if err != nil {
			t.Fatal(err)
		}
		return ciphertext
	}
	nativeCipher := seal("shared-workspace", "legacy-native")
	insert("native", "shared-workspace", nativeCipher, true, true)
	insert("browser", "browser-subject", seal("browser-subject", "legacy-browser"), true, false)
	insert("unreadable", "unreadable", "invalid-ciphertext", false, true)

	account := func(id string) (row struct {
		Ciphertext string
		Version    uint64
		Enabled    bool
	}) {
		t.Helper()
		if err := s.DB.Table("tokenhub_subscription_accounts").Where("id=? AND NOT removed", id).Take(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	if err := s.ReconcileCodexAccountIdentities(ctx); err != nil {
		t.Fatal(err)
	}
	if duplicate := account("native"); duplicate.Enabled || duplicate.Ciphertext != nativeCipher || duplicate.Version != 2 {
		t.Fatal("legacy duplicate was not disabled with its identity and credentials retained")
	}
	if !account("browser").Enabled || account("unreadable").Version != 1 {
		t.Fatal("available account or unreadable legacy record was changed unexpectedly")
	}
	revision := func() uint64 {
		t.Helper()
		version, err := s.LockRevision(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return version
	}
	before := revision()
	if err := s.ReconcileCodexAccountIdentities(ctx); err != nil || revision() != before || account("native").Version != 2 {
		t.Fatalf("reconciliation was not idempotent: %v", err)
	}
	edit := SubscriptionAccountInput{PoolID: pool, ID: "native", Name: "native", ExpectedVersion: 2, Enabled: true}
	if _, err := s.SaveSubscriptionAccount(ctx, "platform", edit); !errors.Is(err, ErrSubscriptionAccountExists) || account("native").Enabled {
		t.Fatalf("an edit reactivated a duplicate account: %v", err)
	}
	if err := s.RemoveSubscriptionAccount(ctx, "platform", pool, "browser", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSubscriptionAccount(ctx, "platform", edit); err != nil || !account("native").Enabled {
		t.Fatalf("operator could not keep the other account after removing the duplicate: %v", err)
	}
}

func TestPostgresOpaqueSubscriptionImportCannotReplaceAnUnverifiedAccount(t *testing.T) {
	s := newCommerceTestStore(t)
	ctx := t.Context()
	pool, err := s.SaveSubscriptionPool(ctx, "platform", SubscriptionPoolInput{Name: "Claude", Kind: subscriptionauth.Claude, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	in := SubscriptionAccountInput{PoolID: pool, Name: "Claude Code", Enabled: true, Credential: "sk-ant-oat-test-original"}
	id, err := s.SaveSubscriptionAccount(ctx, "platform", in)
	if err != nil {
		t.Fatal(err)
	}
	in.ID, in.ExpectedVersion = id, 1
	if _, err := s.SaveSubscriptionAccount(ctx, "platform", in); err != nil {
		t.Fatalf("reimporting the same credentials failed: %v", err)
	}
	in.ExpectedVersion, in.Credential = 2, "sk-ant-oat-test-different-account"
	if _, err := s.SaveSubscriptionAccount(ctx, "platform", in); !errors.Is(err, ErrSubscriptionAccountIdentity) {
		t.Fatalf("manually imported opaque credentials could overwrite the previous account: %v", err)
	}
	in.ID, in.ExpectedVersion = "", 0
	if _, err := s.SaveSubscriptionAccount(ctx, "platform", in); err != nil {
		t.Fatalf("unverified credentials cannot be registered separately: %v", err)
	}
}
