package app

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/identity"
)

func authAuthority(raw string) string {
	if raw != strings.TrimSpace(raw) {
		return ""
	}
	u, err := url.Parse("//" + raw)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, " ,\\\t\r\n") {
		return ""
	}
	return strings.ToLower(u.Host)
}
func authHostname(authority string) string {
	u, _ := url.Parse("//" + authority)
	if u == nil {
		return ""
	}
	return u.Hostname()
}
func loopbackAuthHost(authority string) bool {
	host := authHostname(authority)
	ip := net.ParseIP(host)
	return host == "localhost" || (ip != nil && ip.IsLoopback())
}

// Host is authoritative at the edge. Only an internal API connection may use
// a forwarded host, and only from a local/private reverse proxy. ?host= is
// deliberately reserved for public projections, never authentication.
func (a *App) authenticationHost(c *gin.Context) string {
	host := authAuthority(c.Request.Host)
	if a.Identity.KnownBrandHost(c.Request.Context(), host) {
		return host
	}
	remote, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	ip := net.ParseIP(remote)
	if (loopbackAuthHost(host) || authHostname(host) == "api") && ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		if forwarded := authAuthority(c.GetHeader("X-Forwarded-Host")); forwarded != "" {
			return forwarded
		}
	}
	return host
}
func localAuthTransport(c *gin.Context) bool {
	remote, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	ip := net.ParseIP(remote)
	return loopbackAuthHost(authAuthority(c.Request.Host)) && ip != nil && ip.IsLoopback()
}
func (a *App) authenticationBrand(c *gin.Context) (string, error) {
	host := a.authenticationHost(c)
	if host == "" {
		return "", identity.ErrInvalidCredentials
	}
	if a.Identity.KnownBrandHost(c.Request.Context(), host) {
		brand, err := a.Identity.BrandByHost(c.Request.Context(), host)
		if err != nil {
			return "", err
		}
		// SDK API domains are not browser login entry points.
		hostname := authHostname(host)
		if hostname != brand.PrimaryDomain && hostname != brand.AdminDomain {
			return "", identity.ErrInvalidCredentials
		}
		return brand.ID, nil
	}
	// Explicit instance origins are the official brand's deployment aliases.
	for _, origin := range []string{a.Config.WebOrigin, a.Config.PublicBaseURL} {
		u, err := url.Parse(origin)
		if err == nil && authAuthority(u.Host) == host {
			return identity.OfficialBrandID, nil
		}
	}
	if !a.Config.IsProduction() && loopbackAuthHost(host) && localAuthTransport(c) {
		return identity.OfficialBrandID, nil
	}
	return "", identity.ErrInvalidCredentials
}
func (a *App) browserBrandMatches(c *gin.Context, principal *identity.Principal) bool {
	host := a.authenticationHost(c)
	// Direct loopback test/dev clients also exercise scoped administration
	// without a virtual host. Public requests always require an exact brand.
	if !a.Config.IsProduction() && loopbackAuthHost(host) && localAuthTransport(c) && !a.Identity.KnownBrandHost(c.Request.Context(), host) {
		return true
	}
	brandID, err := a.authenticationBrand(c)
	return err == nil && principal.BrandID == brandID
}
func (a *App) googleConfigForRequest(c *gin.Context, testing bool) (*identity.GoogleOAuthConfig, string, int64, error) {
	brandID, err := a.authenticationBrand(c)
	if err != nil {
		return nil, "", 0, identity.ErrGoogleUnavailable
	}
	redirectURI := "https://" + a.authenticationHost(c) + "/login/oauth/google"
	cfg, version, err := a.Identity.ResolveGoogleConfig(c.Request.Context(), brandID, redirectURI, a.Config.EncryptionKey, testing)
	if err != nil || cfg != nil {
		return cfg, brandID, version, err
	}
	if brandID != identity.OfficialBrandID || !a.Config.GoogleTriad() {
		return nil, brandID, 0, identity.ErrGoogleUnavailable
	}
	callback, err := url.Parse(a.Config.GoogleRedirect)
	if err != nil || callback.Host == "" || callback.User != nil || callback.Fragment != "" || (callback.Scheme != "https" && !(callback.Scheme == "http" && !a.Config.IsProduction() && loopbackAuthHost(callback.Host))) {
		return nil, brandID, 0, identity.ErrGoogleUnavailable
	}
	if authAuthority(callback.Host) != a.authenticationHost(c) && !(!a.Config.IsProduction() && loopbackAuthHost(a.authenticationHost(c)) && localAuthTransport(c)) {
		return nil, brandID, 0, identity.ErrGoogleUnavailable
	}
	fingerprint := sha256.Sum256([]byte(a.Config.GoogleClientID + "\x00" + a.Config.GoogleClientSecret + "\x00" + a.Config.GoogleRedirect))
	version = -int64(binary.BigEndian.Uint64(fingerprint[:8])>>1 | 1)
	return &identity.GoogleOAuthConfig{ClientID: a.Config.GoogleClientID, ClientSecret: a.Config.GoogleClientSecret, RedirectURL: a.Config.GoogleRedirect}, brandID, version, nil
}
func (a *App) googleCallbackConfig(c *gin.Context, state string) (*identity.GoogleOAuthConfig, identity.OAuthIntent, error) {
	intent := a.Identity.GoogleIntent(c.Request.Context(), state)
	cfg, brandID, version, err := a.googleConfigForRequest(c, intent.TestOnly)
	if err != nil {
		return nil, intent, err
	}
	if cfg == nil || intent.BrandID != brandID || intent.ConfigVersion != version || intent.RedirectURI != cfg.RedirectURL {
		return nil, intent, identity.NewGoogleExchangeError("configuration_changed")
	}
	if !a.googleBrowserMatches(c, state) {
		return nil, intent, identity.NewGoogleExchangeError("state_mismatch")
	}
	return cfg, intent, nil
}
func googleSafeFailure(err error) string {
	var exchangeErr *identity.GoogleExchangeError
	if errors.As(err, &exchangeErr) {
		return exchangeErr.Reason
	}
	return "authentication_error"
}
