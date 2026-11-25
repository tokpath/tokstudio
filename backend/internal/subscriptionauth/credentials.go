package subscriptionauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	Codex       = "codex"
	Claude      = "claude"
	Grok        = "grok"
	Gemini      = "gemini"
	Antigravity = "antigravity"
)

var (
	ErrCredential = errors.New("invalid subscription credential")
	ErrReauth     = errors.New("subscription authorization must be replaced")
	ErrRefresh    = errors.New("subscription authorization refresh unavailable")
)

type Credential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Connection   string    `json:"connection,omitempty"`
}

func Protocol(kind string) string {
	switch kind {
	case Codex, Grok, Gemini, Antigravity:
		return "openai"
	case Claude:
		return "anthropic"
	default:
		return ""
	}
}

func Operation(kind string) string {
	switch kind {
	case Codex, Grok:
		return "responses"
	case Claude, Gemini, Antigravity:
		return "chat"
	default:
		return ""
	}
}

func PricingProvider(kind string) string {
	switch kind {
	case Grok:
		return "xai"
	case Gemini, Antigravity:
		return "gemini"
	default:
		return Protocol(kind)
	}
}

// Parse accepts credentials supplied by the operator from an authorized client.
// Claims only supply routing hints and expiry. The provider verifies the token.
func Parse(kind, raw string) (Credential, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 64<<10 || Protocol(kind) == "" {
		return Credential{}, ErrCredential
	}
	var c Credential
	switch kind {
	case Codex:
		var exported struct {
			Tokens *Credential `json:"tokens"`
		}
		if json.Unmarshal([]byte(raw), &exported) != nil {
			return Credential{}, ErrCredential
		}
		if exported.Tokens != nil {
			c = *exported.Tokens
		} else if json.Unmarshal([]byte(raw), &c) != nil {
			return Credential{}, ErrCredential
		}
		claims, ok := codexClaims(c.AccessToken)
		if !ok || claims.AccountID == "" || c.AccountID != "" && c.AccountID != claims.AccountID {
			return Credential{}, ErrCredential
		}
		c.AccountID, c.ExpiresAt = claims.AccountID, claims.ExpiresAt
	case Claude:
		if strings.HasPrefix(raw, "sk-ant-oat") {
			c.AccessToken = raw
		} else {
			var exported struct {
				OAuth *struct {
					AccessToken  string `json:"accessToken"`
					RefreshToken string `json:"refreshToken"`
					ExpiresAt    int64  `json:"expiresAt"`
				} `json:"claudeAiOauth"`
			}
			if json.Unmarshal([]byte(raw), &exported) != nil {
				return Credential{}, ErrCredential
			}
			if exported.OAuth != nil {
				c.AccessToken, c.RefreshToken = exported.OAuth.AccessToken, exported.OAuth.RefreshToken
				if exported.OAuth.ExpiresAt > 0 {
					c.ExpiresAt = time.UnixMilli(exported.OAuth.ExpiresAt).UTC()
				}
			} else if json.Unmarshal([]byte(raw), &c) != nil {
				return Credential{}, ErrCredential
			}
		}
		c.AccountID = ""
		if !strings.HasPrefix(c.AccessToken, "sk-ant-oat") {
			return Credential{}, ErrCredential
		}
	default:
		return Credential{}, ErrCredential
	}
	c.Connection = ""
	if !safeToken(c.AccessToken) || c.RefreshToken != "" && !safeToken(c.RefreshToken) {
		return Credential{}, ErrCredential
	}
	return c, nil
}

func safeToken(token string) bool {
	if len(token) == 0 || len(token) > 32<<10 {
		return false
	}
	for _, ch := range token {
		if ch <= ' ' || ch >= 127 {
			return false
		}
	}
	return true
}

type tokenClaims struct {
	AccountID string
	ExpiresAt time.Time
}

// CodexRoutingAccount is a routing hint. The configured upstream verifies the
// bearer token and its entitlement.
func CodexRoutingAccount(token string) string {
	claims, _ := codexClaims(token)
	return claims.AccountID
}

func codexClaims(token string) (tokenClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Exp  int64 `json:"exp"`
		Auth struct {
			AccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err != nil || json.Unmarshal(payload, &claims) != nil || claims.Exp <= 0 || len(claims.Auth.AccountID) > 255 || !safeToken(claims.Auth.AccountID) {
		return tokenClaims{}, false
	}
	return tokenClaims{AccountID: claims.Auth.AccountID, ExpiresAt: time.Unix(claims.Exp, 0).UTC()}, true
}

var refreshClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func Refresh(ctx context.Context, kind string, current Credential) (Credential, error) {
	if current.RefreshToken == "" {
		return Credential{}, ErrReauth
	}
	var endpoint, clientID string
	switch kind {
	case Codex:
		endpoint, clientID = "https://auth.openai.com/oauth/token", "app_EMoamEEZ73f0CkXaXp7hrann"
	case Claude:
		endpoint, clientID = "https://platform.claude.com/v1/oauth/token", "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	default:
		return Credential{}, ErrCredential
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type": "refresh_token", "refresh_token": current.RefreshToken, "client_id": clientID,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Credential{}, ErrRefresh
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := refreshClient.Do(req)
	if err != nil {
		return Credential{}, ErrRefresh
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
		return Credential{}, ErrReauth
	case http.StatusOK:
	default:
		return Credential{}, ErrRefresh
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&token) != nil || !safeToken(token.AccessToken) {
		return Credential{}, ErrRefresh
	}
	if token.RefreshToken != "" {
		if !safeToken(token.RefreshToken) {
			return Credential{}, ErrRefresh
		}
		current.RefreshToken = token.RefreshToken
	}
	current.AccessToken = token.AccessToken
	if kind == Codex {
		claims, ok := codexClaims(token.AccessToken)
		if !ok || claims.AccountID != current.AccountID {
			return Credential{}, ErrReauth
		}
		current.ExpiresAt = claims.ExpiresAt
	} else {
		if token.ExpiresIn <= 0 || token.ExpiresIn > int64((365*24*time.Hour)/time.Second) || !strings.HasPrefix(token.AccessToken, "sk-ant-oat") {
			return Credential{}, ErrRefresh
		}
		current.ExpiresAt = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UTC()
	}
	return current, nil
}
