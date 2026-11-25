package subscriptionauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrOAuthNotConfigured = errors.New("subscription OAuth client is not configured")

// A connection is provisioned on the server from the supplier's authorization
// for this platform. Account imports and customer requests cannot set its URLs.
type Connection struct {
	AuthorizationURL string                   `json:"authorization_url"`
	TokenURL         string                   `json:"token_url"`
	InferenceURL     string                   `json:"inference_url"`
	Issuer           string                   `json:"issuer,omitempty"`
	UserInfoURL      string                   `json:"userinfo_url,omitempty"`
	ClientID         string                   `json:"client_id"`
	ClientSecret     string                   `json:"client_secret,omitempty"`
	TokenAuthMethod  string                   `json:"token_auth_method,omitempty"`
	Scope            string                   `json:"scope"`
	Resource         string                   `json:"resource,omitempty"`
	MediaPreset      string                   `json:"media_preset,omitempty"`
	Media            map[string]MediaEndpoint `json:"media,omitempty"`
}

type Connections map[string]Connection
type DestinationGuard func(context.Context, string) error

func LoadConnections(raw string, loopback bool) (Connections, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var out Connections
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid TOKENHUB_SUBSCRIPTION_CONNECTIONS JSON")
	}
	for kind, c := range out {
		if c.Validate(kind, loopback) != nil {
			return nil, fmt.Errorf("invalid subscription connection for %q", kind)
		}
	}
	return out, nil
}

func (c Connection) Validate(kind string, loopback bool) error {
	if Operation(kind) == "" || !safeToken(c.ClientID) || len(c.ClientID) > 512 ||
		c.ClientSecret != "" && !safeToken(c.ClientSecret) ||
		c.TokenAuthMethod != "" && c.TokenAuthMethod != "client_secret_basic" && c.TokenAuthMethod != "client_secret_post" ||
		c.TokenAuthMethod == "client_secret_basic" && c.ClientSecret == "" ||
		strings.TrimSpace(c.Scope) == "" || c.Issuer == "" && c.UserInfoURL == "" {
		return ErrOAuthNotConfigured
	}
	for _, endpoint := range []string{c.AuthorizationURL, c.TokenURL, c.InferenceURL} {
		if !validEndpoint(endpoint, loopback) {
			return ErrOAuthNotConfigured
		}
	}
	for _, endpoint := range []string{c.Issuer, c.UserInfoURL, c.Resource} {
		if endpoint != "" && !validEndpoint(endpoint, loopback) {
			return ErrOAuthNotConfigured
		}
	}
	if c.Issuer != "" && !hasScope(c.Scope, "openid") {
		return ErrOAuthNotConfigured
	}
	endpoint, _ := url.Parse(c.InferenceURL)
	want := "/" + Operation(kind)
	switch kind {
	case Claude:
		want = "/messages"
	case Gemini, Antigravity:
		want = "/chat/completions"
	}
	if !strings.HasSuffix(endpoint.Path, want) {
		return ErrOAuthNotConfigured
	}
	return c.validateMedia(kind, loopback)
}

func validEndpoint(raw string, loopback bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" ||
		strings.ContainsAny(raw, "\\\r\n") {
		return false
	}
	if u.Scheme == "https" {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return loopback && u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
}

func (c Connection) Fingerprint() string {
	// Rotating the server-side client secret does not change an existing grant.
	withoutSecret := c
	withoutSecret.ClientSecret = ""
	raw, _ := json.Marshal(withoutSecret)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (c Connection) Authorization(state, verifier, nonce, redirect string) string {
	u, _ := url.Parse(c.AuthorizationURL)
	query := u.Query()
	challenge := sha256.Sum256([]byte(verifier))
	query.Set("response_type", "code")
	query.Set("client_id", c.ClientID)
	query.Set("redirect_uri", redirect)
	query.Set("scope", c.Scope)
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	if c.Issuer != "" {
		query.Set("nonce", nonce)
	}
	if c.Resource != "" {
		query.Set("resource", c.Resource)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func RandomChallenge() (string, error) {
	var raw [32]byte
	_, err := rand.Read(raw[:])
	return base64.RawURLEncoding.EncodeToString(raw[:]), err
}

// Import keeps vendor-approved exported credentials available when operators
// already have them. Legacy Codex and Claude exports keep their native route.
func Import(kind, raw string, connections Connections) (Credential, error) {
	if c, err := Parse(kind, raw); err == nil {
		return c, nil
	}
	connection, ok := connections[kind]
	if !ok {
		if kind == Codex || kind == Claude {
			return Credential{}, ErrCredential
		}
		return Credential{}, ErrOAuthNotConfigured
	}
	var c Credential
	if len(raw) > 64<<10 || json.Unmarshal([]byte(raw), &c) != nil ||
		!validAccountID(c.AccountID) || !safeToken(c.AccessToken) ||
		c.RefreshToken != "" && !safeToken(c.RefreshToken) ||
		c.ExpiresAt.IsZero() || c.RefreshToken == "" && !c.ExpiresAt.After(time.Now()) {
		return Credential{}, ErrCredential
	}
	c.Connection = connection.Fingerprint()
	return c, nil
}

type oauthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (c Connection) Exchange(ctx context.Context, code, verifier, nonce, redirect string, guard DestinationGuard) (Credential, error) {
	if len(code) > 4096 || !safeToken(code) {
		return Credential{}, ErrCredential
	}
	tokens, err := c.token(ctx, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect}, "code_verifier": {verifier},
	}, guard)
	if err != nil {
		return Credential{}, err
	}
	var subject string
	if c.Issuer != "" {
		subject, err = c.idTokenSubject(tokens.IDToken, nonce)
	} else {
		subject, err = c.userInfoSubject(ctx, tokens.AccessToken, guard)
	}
	if err != nil {
		return Credential{}, err
	}
	return Credential{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken, AccountID: subject,
		ExpiresAt: time.Now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second), Connection: c.Fingerprint()}, nil
}

func (c Connection) Refresh(ctx context.Context, current Credential, guard DestinationGuard) (Credential, error) {
	if current.RefreshToken == "" || current.Connection != c.Fingerprint() {
		return Credential{}, ErrReauth
	}
	tokens, err := c.token(ctx, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {current.RefreshToken}}, guard)
	if err != nil {
		return Credential{}, err
	}
	current.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		current.RefreshToken = tokens.RefreshToken
	}
	current.ExpiresAt = time.Now().UTC().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	return current, nil
}

func (c Connection) token(ctx context.Context, form url.Values, guard DestinationGuard) (oauthTokens, error) {
	if guard == nil || guard(ctx, c.TokenURL) != nil {
		return oauthTokens{}, ErrRefresh
	}
	if c.Resource != "" {
		form.Set("resource", c.Resource)
	}
	if c.TokenAuthMethod != "client_secret_basic" {
		form.Set("client_id", c.ClientID)
		if c.ClientSecret != "" {
			form.Set("client_secret", c.ClientSecret)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokens{}, ErrRefresh
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.TokenAuthMethod == "client_secret_basic" {
		req.SetBasicAuth(url.QueryEscape(c.ClientID), url.QueryEscape(c.ClientSecret))
	}
	resp, err := refreshClient.Do(req)
	if err != nil {
		return oauthTokens{}, ErrRefresh
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return oauthTokens{}, ErrReauth
	case http.StatusOK:
	default:
		return oauthTokens{}, ErrRefresh
	}
	var token oauthTokens
	if json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&token) != nil ||
		!safeToken(token.AccessToken) || token.RefreshToken != "" && !safeToken(token.RefreshToken) ||
		!strings.EqualFold(token.TokenType, "Bearer") || token.ExpiresIn <= 0 || token.ExpiresIn > 365*24*60*60 {
		return oauthTokens{}, ErrRefresh
	}
	if token.Scope != "" {
		for _, scope := range strings.Fields(c.Scope) {
			if !hasScope(token.Scope, scope) {
				return oauthTokens{}, ErrReauth
			}
		}
	}
	return token, nil
}

func (c Connection) userInfoSubject(ctx context.Context, access string, guard DestinationGuard) (string, error) {
	if guard(ctx, c.UserInfoURL) != nil {
		return "", ErrRefresh
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.UserInfoURL, nil)
	if err != nil {
		return "", ErrRefresh
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Accept", "application/json")
	resp, err := refreshClient.Do(req)
	if err != nil {
		return "", ErrRefresh
	}
	defer resp.Body.Close()
	var profile struct {
		Subject string `json:"sub"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&profile) != nil || !validAccountID(profile.Subject) {
		return "", ErrCredential
	}
	return profile.Subject, nil
}

func (c Connection) idTokenSubject(token, nonce string) (string, error) {
	// The ID token comes directly from the configured token server over
	// verified TLS. The code exchange never accepts an ID token from the browser.
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[2] == "" {
		return "", ErrCredential
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Issuer   string          `json:"iss"`
		Subject  string          `json:"sub"`
		Nonce    string          `json:"nonce"`
		Expires  int64           `json:"exp"`
		IssuedAt int64           `json:"iat"`
		Audience json.RawMessage `json:"aud"`
		AZP      string          `json:"azp"`
	}
	now := time.Now()
	if err != nil || json.Unmarshal(raw, &claims) != nil || claims.Issuer != c.Issuer || !validAccountID(claims.Subject) ||
		claims.Expires <= now.Unix() || claims.IssuedAt <= 0 || claims.IssuedAt > now.Add(time.Minute).Unix() ||
		subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return "", ErrCredential
	}
	var audience string
	if json.Unmarshal(claims.Audience, &audience) != nil {
		var audiences []string
		if json.Unmarshal(claims.Audience, &audiences) != nil || len(audiences) != 1 {
			return "", ErrCredential
		}
		audience = audiences[0]
	}
	if audience != c.ClientID || claims.AZP != "" && claims.AZP != c.ClientID {
		return "", ErrCredential
	}
	return claims.Subject, nil
}

func hasScope(scopes, value string) bool {
	for _, scope := range strings.Fields(scopes) {
		if scope == value {
			return true
		}
	}
	return false
}

func validAccountID(id string) bool {
	return len(id) <= 255 && safeToken(id)
}
