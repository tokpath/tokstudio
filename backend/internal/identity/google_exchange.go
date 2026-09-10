package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleHTTPTimeout = 10 * time.Second
	maxOAuthBody      = 1 << 20
)

// GoogleOAuth 用授权码向 Google 换 token 和用户资料。端点可覆盖以便测试。
type GoogleOAuth struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	TokenURL     string
	UserInfoURL  string
	HTTPClient   *http.Client
}

func (g GoogleOAuth) tokenURL() string {
	if g.TokenURL != "" {
		return g.TokenURL
	}
	return googleTokenURL
}

func (g GoogleOAuth) userInfoURL() string {
	if g.UserInfoURL != "" {
		return g.UserInfoURL
	}
	return googleUserInfoURL
}

func (g GoogleOAuth) httpClient() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return &http.Client{Timeout: googleHTTPTimeout}
}

// Exchange 实现 GoogleExchanger。
func (g GoogleOAuth) Exchange(ctx context.Context, code string) (GoogleProfile, error) {
	code = strings.TrimSpace(code)
	if code == "" || g.ClientID == "" || g.ClientSecret == "" || g.RedirectURI == "" {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", g.ClientID)
	form.Set("client_secret", g.ClientSecret)
	form.Set("redirect_uri", g.RedirectURI)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.httpClient().Do(req)
	if err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxOAuthBody))
	if err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	var tokenBody struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &tokenBody); err != nil || tokenBody.AccessToken == "" {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, g.userInfoURL(), nil)
	if err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	infoReq.Header.Set("Authorization", "Bearer "+tokenBody.AccessToken)
	infoResp, err := g.httpClient().Do(infoReq)
	if err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	defer infoResp.Body.Close()
	infoRaw, err := io.ReadAll(io.LimitReader(infoResp.Body, maxOAuthBody))
	if err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	if infoResp.StatusCode < 200 || infoResp.StatusCode >= 300 {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	var info struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
	}
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	if info.Subject == "" || info.Email == "" || !googleEmailVerified(info.EmailVerified) {
		return GoogleProfile{}, ErrInvalidCredentials
	}
	return GoogleProfile{Subject: info.Subject, Email: normalizeEmail(info.Email)}, nil
}

func googleEmailVerified(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true")
	default:
		return false
	}
}
