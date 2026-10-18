package portal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	pc "github.com/tokpath/tokstudio/backend/internal/platform/crypto"
)

const googleAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"

// GoogleExchanger replaces the live Google token exchange in tests.
type GoogleExchanger func(context.Context, organization.GoogleCredential, string) (organization.GoogleProfile, error)

func (a *Application) googleRedirectURI(c *gin.Context) string {
	scheme := "https"
	if !a.Config.IsProduction() && c.Request.TLS == nil && !strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	return scheme + "://" + a.requestHost(c) + "/login/oauth/google"
}

func (a *Application) googleHostAllowed(c *gin.Context, brandID string) bool {
	host := bareHost(a.requestHost(c))
	if brandID == a.Installation.BrandID && a.officialHostname(host) {
		return true
	}
	resolved, err := a.Branding.ResolveHost(c.Request.Context(), nil, a.requestHost(c))
	return err == nil && resolved.ID == brandID
}

func (a *Application) googleAuthorizeURL(cred organization.GoogleCredential, state string) string {
	q := url.Values{}
	q.Set("client_id", cred.ClientID)
	q.Set("redirect_uri", cred.RedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "openid email profile")
	q.Set("state", state)
	q.Set("prompt", "select_account")
	return googleAuthorizeURL + "?" + q.Encode()
}

func (a *Application) exchangeGoogle(ctx context.Context, cred organization.GoogleCredential, code string) (organization.GoogleProfile, error) {
	if a.GoogleExchanger != nil {
		return a.GoogleExchanger(ctx, cred, code)
	}
	return exchangeGoogleHTTP(ctx, cred, code)
}

func exchangeGoogleHTTP(ctx context.Context, cred organization.GoogleCredential, code string) (organization.GoogleProfile, error) {
	if cred.ClientID == "" || cred.ClientSecret == "" || cred.RedirectURI == "" || strings.TrimSpace(code) == "" {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", cred.ClientID)
	form.Set("client_secret", cred.ClientSecret)
	form.Set("redirect_uri", cred.RedirectURI)
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if err != nil {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	var token struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if json.Unmarshal(body, &token) != nil || resp.StatusCode >= 300 || token.AccessToken == "" {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	infoReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	infoReq.Header.Set("Accept", "application/json")
	infoResp, err := client.Do(infoReq)
	if err != nil {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	defer infoResp.Body.Close()
	infoBody, err := io.ReadAll(io.LimitReader(infoResp.Body, 1<<20))
	if err != nil {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	var info struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		Name          string `json:"name"`
		EmailVerified bool   `json:"email_verified"`
	}
	if json.Unmarshal(infoBody, &info) != nil || infoResp.StatusCode >= 300 || info.Subject == "" || !info.EmailVerified || !strings.Contains(info.Email, "@") {
		return organization.GoogleProfile{}, organization.ErrGoogleUnavailable
	}
	return organization.GoogleProfile{Subject: info.Subject, Email: info.Email, Name: info.Name}, nil
}

func (a *Application) googleBrandCommand(c *gin.Context, op organization.Operator, actor, action, targetID, hostname, clientID, secret string, enabled bool, version int64) {
	target, err := a.brandingTarget(c, op, targetID)
	if err != nil || target.Kind == organization.Channel {
		handleError(c, organization.ErrForbidden)
		return
	}
	if action == "google_test" {
		redirect, err := a.googleTestRedirect(c, target.BrandID, hostname)
		if err != nil {
			handleError(c, err)
			return
		}
		cred, err := a.Organization.OpenGoogleCredential(c.Request.Context(), nil, target.BrandID, redirect, a.Config.AppKey, true)
		if err != nil {
			handleError(c, err)
			return
		}
		ticket, err := a.Organization.CreateGoogleTest(c.Request.Context(), nil, target.BrandID, redirect, cred.Version)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(200, gin.H{"id": target.BrandID, "status": "test_link", "version": cred.Version, "access_url": strings.TrimSuffix(redirect, "/login/oauth/google") + "/login/oauth/google/test?ticket=" + url.QueryEscape(ticket), "message": "在该品牌域名打开测试链接。测试只核对回调，不会创建客户或登录。"})
		return
	}
	view, err := a.Organization.SaveGoogleConfig(c.Request.Context(), nil, actor, target.BrandID, a.Config.AppKey, organization.GoogleConfigInput{ClientID: clientID, ClientSecret: secret, Enabled: enabled, ExpectedVersion: version})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"id": target.BrandID, "status": "saved", "version": view.Version, "message": "Google 登录配置已保存，当前版本 " + strconv.FormatInt(view.Version, 10) + "。请把品牌域名的 /login/oauth/google 登记到 Google 客户端，完成测试后再启用。"})
}

func (a *Application) googleTestRedirect(c *gin.Context, brandID, hostname string) (string, error) {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	if brandID == a.Installation.BrandID {
		if hostname == "" {
			if !a.googleHostAllowed(c, brandID) {
				return "", organization.ErrGoogleConfig
			}
			return a.googleRedirectURI(c), nil
		}
		if !a.officialHostname(bareHost(hostname)) {
			return "", organization.ErrForbidden
		}
		scheme := "https"
		if !a.Config.IsProduction() {
			scheme = "http"
		}
		return scheme + "://" + hostname + "/login/oauth/google", nil
	}
	if hostname == "" {
		return "", organization.ErrGoogleConfig
	}
	resolved, err := a.Branding.ResolveHost(c.Request.Context(), nil, hostname)
	if err != nil || resolved.ID != brandID {
		return "", organization.ErrForbidden
	}
	return "https://" + bareHost(hostname) + "/login/oauth/google", nil
}

func (a *Application) googleStart(c *gin.Context) {
	var in struct {
		InviteCode string `json:"invite_code"`
		Next       string `json:"next"`
	}
	if !body(c, &in) {
		return
	}
	if !a.googleHostAllowed(c, brand(c).ID) {
		handleError(c, organization.ErrGoogleUnavailable)
		return
	}
	if _, ok := key(c); !ok {
		return
	}
	redirect := a.googleRedirectURI(c)
	cred, err := a.Organization.OpenGoogleCredential(c.Request.Context(), nil, brand(c).ID, redirect, a.Config.AppKey, false)
	if err != nil {
		handleError(c, err)
		return
	}
	state, err := a.Organization.StartGoogle(c.Request.Context(), nil, organization.GoogleIntent{BrandID: brand(c).ID, RedirectURI: redirect, InviteCode: in.InviteCode, NextPath: safeCustomerNext(in.Next), ConfigVersion: cred.Version})
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"url": a.googleAuthorizeURL(cred, state)})
}

func (a *Application) googleTestStart(c *gin.Context) {
	var in struct{ Ticket string }
	if !body(c, &in) {
		return
	}
	if !a.googleHostAllowed(c, brand(c).ID) {
		handleError(c, organization.ErrGoogleState)
		return
	}
	redirect := a.googleRedirectURI(c)
	intent, err := a.Organization.ConsumeGoogleTest(c.Request.Context(), nil, in.Ticket, redirect)
	if err != nil || intent.BrandID != brand(c).ID {
		handleError(c, organization.ErrGoogleState)
		return
	}
	cred, err := a.Organization.OpenGoogleCredential(c.Request.Context(), nil, intent.BrandID, redirect, a.Config.AppKey, true)
	if err != nil || cred.Version != intent.ConfigVersion {
		handleError(c, organization.ErrGoogleUnavailable)
		return
	}
	state, err := a.Organization.StartGoogle(c.Request.Context(), nil, intent)
	if err != nil {
		handleError(c, err)
		return
	}
	c.JSON(200, gin.H{"url": a.googleAuthorizeURL(cred, state)})
}

func (a *Application) googleFinish(c *gin.Context) {
	var in struct{ Code, State string }
	if !body(c, &in) {
		return
	}
	if _, ok := key(c); !ok {
		return
	}
	ctx := c.Request.Context()
	intent, err := a.Organization.ConsumeGoogleState(ctx, nil, in.State)
	if err != nil {
		handleError(c, err)
		return
	}
	if intent.BrandID != brand(c).ID || intent.RedirectURI != a.googleRedirectURI(c) {
		handleError(c, organization.ErrGoogleState)
		return
	}
	cred, err := a.Organization.OpenGoogleCredential(ctx, nil, intent.BrandID, intent.RedirectURI, a.Config.AppKey, intent.TestOnly)
	if err != nil || cred.Version != intent.ConfigVersion {
		handleError(c, organization.ErrGoogleUnavailable)
		return
	}
	profile, err := a.exchangeGoogle(ctx, cred, in.Code)
	if err != nil {
		if intent.TestOnly {
			_ = a.Organization.RecordGoogleVerification(ctx, nil, intent.BrandID, intent.RedirectURI, intent.ConfigVersion, false)
		}
		handleError(c, err)
		return
	}
	if intent.TestOnly {
		if err = a.Organization.RecordGoogleVerification(ctx, nil, intent.BrandID, intent.RedirectURI, intent.ConfigVersion, true); err != nil {
			handleError(c, err)
			return
		}
		c.JSON(200, gin.H{"kind": "google_test", "message": "这个域名的 Google 回调测试已通过。回到品牌配置并启用后，客户即可使用。"})
		return
	}
	result, err := a.Organization.FinishGoogleSignIn(ctx, nil, intent, profile, a.Incentives)
	if err != nil {
		handleError(c, err)
		return
	}
	now := time.Now().UTC()
	session := browserSession{TokenHash: pc.HashToken(result.AccessToken), Kind: "customer", BrandID: brand(c).ID, SubjectID: result.Principal.Customer.ID, CredentialVersion: result.Principal.CredentialVersion, ExpiresAt: result.ExpiresAt, CreatedAt: now}
	if err = a.DB.WithContext(ctx).Create(&session).Error; err != nil {
		handleError(c, err)
		return
	}
	a.setCookie(c, result.AccessToken, session.ExpiresAt)
	id := identity{Token: result.AccessToken, Customer: result.Principal, Session: session}
	c.Set("identity", &id)
	a.sessionResponse(c)
}
