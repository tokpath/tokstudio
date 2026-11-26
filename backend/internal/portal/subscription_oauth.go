package portal

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	platformcrypto "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
	"github.com/tokpath/tokstudio/backend/internal/subscriptionauth"
	"gorm.io/gorm"
)

type subscriptionOAuthDraft struct {
	Account  catalog.SubscriptionAccountInput
	Verifier string
	Nonce    string
}

func (a *Application) subscriptionCallbackURI(c *gin.Context, kind string) string {
	origin, err := url.Parse(a.Config.WebOrigin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" ||
		!strings.EqualFold(origin.Host, a.requestHost(c)) || !a.officialHostname(origin.Hostname()) ||
		origin.Scheme != "https" && !(a.Config.TestLoopback && origin.Scheme == "http") {
		return ""
	}
	return (&url.URL{Scheme: origin.Scheme, Host: origin.Host, Path: "/api/v1/subscription-oauth/" + kind + "/callback"}).String()
}

func (a *Application) subscriptionOAuthStart(c *gin.Context) {
	op, _, ok := a.staffScope(c, "subscription-pools")
	if !ok {
		return
	}
	var in catalog.SubscriptionAccountInput
	if !body(c, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.PoolID == "" || in.ExpectedVersion > 0 && in.Name == "" || len([]rune(in.Name)) > 80 || in.Credential != "" ||
		(in.ExpectedVersion == 0) != (in.ID == "") {
		handleError(c, catalog.ErrInvalidInput)
		return
	}
	ctx := c.Request.Context()
	kind, err := a.Catalog.SubscriptionPoolKind(ctx, op.ID, in.PoolID)
	if err != nil {
		handleError(c, err)
		return
	}
	connection, configured := a.Config.SubscriptionConnections[kind]
	redirect := a.subscriptionCallbackURI(c, kind)
	if !configured || redirect == "" {
		handleError(c, subscriptionauth.ErrOAuthNotConfigured)
		return
	}
	sessionHash := subject(c).Session.TokenHash
	raw, err := a.command(c, "subscription-oauth:"+sessionHash, in, func(tx *gorm.DB) (any, error) {
		state, err := subscriptionauth.RandomChallenge()
		if err != nil {
			return nil, err
		}
		verifier, err := subscriptionauth.RandomChallenge()
		if err != nil {
			return nil, err
		}
		nonce, err := subscriptionauth.RandomChallenge()
		if err != nil {
			return nil, err
		}
		draft, err := json.Marshal(subscriptionOAuthDraft{Account: in, Verifier: verifier, Nonce: nonce})
		if err != nil {
			return nil, err
		}
		sealed, err := platformcrypto.Seal(a.Config.AppKey, string(draft))
		if err != nil {
			return nil, err
		}
		if err = tx.Exec("DELETE FROM portal_subscription_oauth WHERE expires_at<=clock_timestamp()").Error; err != nil {
			return nil, err
		}
		err = tx.Exec(`INSERT INTO portal_subscription_oauth
			(state_hash,session_hash,supplier_kind,redirect_uri,connection_hash,ciphertext,expires_at)
			VALUES(?,?,?,?,?,?,?)`, platformcrypto.HashToken(state), sessionHash, kind, redirect, connection.Fingerprint(), sealed,
			time.Now().UTC().Add(10*time.Minute)).Error
		return gin.H{"url": connection.Authorization(state, verifier, nonce, redirect)}, err
	})
	jsonResponse(c, raw, err)
}

func (a *Application) subscriptionOAuthCallback(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	kind, state := c.Param("kind"), c.Query("state")
	connection, configured := a.Config.SubscriptionConnections[kind]
	identity := subject(c)
	opID := a.Installation.OperatorID
	ctx := c.Request.Context()
	redirect := a.subscriptionCallbackURI(c, kind)
	resultURL := &url.URL{Path: "/manage/" + opID + "/subscription-pools"}
	finish := func(status, poolID string) {
		query := url.Values{"oauth": {status}}
		if poolID != "" {
			query.Set("pool", poolID)
		}
		resultURL.RawQuery = query.Encode()
		c.Redirect(http.StatusSeeOther, resultURL.String())
	}
	if !configured || redirect == "" || len(state) != 43 || identity == nil || identity.Staff.ID == "" ||
		a.Organization.RequirePermission(ctx, nil, identity.Staff.ID, opID, "catalog.manage") != nil {
		finish("failed", "")
		return
	}
	var sealed string
	result := a.DB.WithContext(ctx).Raw(`DELETE FROM portal_subscription_oauth
		WHERE state_hash=? AND session_hash=? AND supplier_kind=? AND redirect_uri=? AND connection_hash=?
		AND expires_at>clock_timestamp() RETURNING ciphertext`, platformcrypto.HashToken(state), identity.Session.TokenHash,
		kind, redirect, connection.Fingerprint()).Scan(&sealed)
	if result.Error != nil || result.RowsAffected != 1 {
		finish("failed", "")
		return
	}
	var draft subscriptionOAuthDraft
	plain, err := platformcrypto.Open(a.Config.AppKey, sealed)
	if err != nil || json.Unmarshal([]byte(plain), &draft) != nil {
		finish("failed", "")
		return
	}
	if c.Query("error") == "access_denied" {
		finish("cancelled", draft.Account.PoolID)
		return
	}
	if c.Query("error") != "" {
		finish("failed", draft.Account.PoolID)
		return
	}
	grant, err := connection.Exchange(ctx, c.Query("code"), draft.Verifier, draft.Nonce, redirect, a.guardEndpoint)
	if err != nil {
		finish("failed", draft.Account.PoolID)
		return
	}
	models := connection.DiscoverModels(ctx, grant, a.guardEndpoint)
	_, err = a.Catalog.SaveAuthorizedSubscriptionAccount(ctx, opID, kind, draft.Account, grant, models...)
	switch {
	case err == nil:
		finish("saved", draft.Account.PoolID)
	case errors.Is(err, catalog.ErrSubscriptionAccountExists):
		finish("duplicate", draft.Account.PoolID)
	case errors.Is(err, catalog.ErrSubscriptionAccountIdentity):
		finish("identity_mismatch", draft.Account.PoolID)
	case errors.Is(err, catalog.ErrCatalogVersion):
		finish("conflict", draft.Account.PoolID)
	default:
		finish("failed", draft.Account.PoolID)
	}
}
