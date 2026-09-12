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
	GoogleAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL     = "https://oauth2.googleapis.com/token"
	googleUserInfoURL  = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// GoogleOAuthConfig 是真实 Google token/profile 交换的参数。
// TokenURL / UserInfoURL 仅供测试注入；生产用 Google 官方地址。
type GoogleOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	TokenURL     string
	UserInfoURL  string
	HTTPClient   *http.Client
}

func (c GoogleOAuthConfig) tokenURL() string {
	if strings.TrimSpace(c.TokenURL) != "" {
		return c.TokenURL
	}
	return googleTokenURL
}

func (c GoogleOAuthConfig) userInfoURL() string {
	if strings.TrimSpace(c.UserInfoURL) != "" {
		return c.UserInfoURL
	}
	return googleUserInfoURL
}

func (c GoogleOAuthConfig) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// NewGoogleExchange 用授权码换 access token，再拉 OpenID profile。
// 错误只带 Google 的 error 码，不含 code / token / secret。
func NewGoogleExchange(cfg GoogleOAuthConfig) GoogleExchanger {
	return func(ctx context.Context, code string) (GoogleProfile, error) {
		return cfg.exchange(ctx, code)
	}
}

func (c GoogleOAuthConfig) exchange(ctx context.Context, code string) (GoogleProfile, error) {
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "" || strings.TrimSpace(c.RedirectURL) == "" {
		return GoogleProfile{}, ErrGoogleUnavailable
	}
	if strings.TrimSpace(code) == "" {
		return GoogleProfile{}, NewGoogleExchangeError("invalid_request")
	}

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", c.ClientID)
	form.Set("client_secret", c.ClientSecret)
	form.Set("redirect_uri", c.RedirectURL)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("network_error")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.client().Do(req)
	if err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("network_error")
	}
	defer resp.Body.Close()
	tokenBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("network_error")
	}
	var token googleTokenResponse
	if err := json.Unmarshal(tokenBody, &token); err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("empty_token")
	}
	if resp.StatusCode >= 300 || token.AccessToken == "" {
		if token.Error != "" {
			return GoogleProfile{}, NewGoogleExchangeError(token.Error)
		}
		return GoogleProfile{}, NewGoogleExchangeError("empty_token")
	}

	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userInfoURL(), nil)
	if err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("userinfo_error")
	}
	infoReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	infoReq.Header.Set("Accept", "application/json")

	infoResp, err := c.client().Do(infoReq)
	if err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("network_error")
	}
	defer infoResp.Body.Close()
	infoBody, err := io.ReadAll(io.LimitReader(infoResp.Body, 1<<20))
	if err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("network_error")
	}
	var info googleUserInfo
	if err := json.Unmarshal(infoBody, &info); err != nil {
		return GoogleProfile{}, NewGoogleExchangeError("userinfo_error")
	}
	if infoResp.StatusCode >= 300 || info.Subject == "" || !strings.Contains(info.Email, "@") {
		return GoogleProfile{}, NewGoogleExchangeError("empty_profile")
	}
	return GoogleProfile{Subject: info.Subject, Email: normalizeEmail(info.Email)}, nil
}

type googleTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

type googleUserInfo struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
}
