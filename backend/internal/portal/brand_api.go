package portal

import (
	"context"
	"net/url"
	"strings"
)

// brandAPIEndpoint is the single source for customer copyable API addresses.
// The official installation already has a configured host; OEMs require a
// verified and enabled custom domain. Channels use the same brand identifier.
func (a *Application) brandAPIEndpoint(ctx context.Context, brandID string) string {
	if brandID == a.Installation.BrandID {
		return officialAPIEndpoint(a.Config)
	}
	endpoint, err := a.Branding.APIEndpoint(ctx, nil, brandID)
	if err != nil {
		return ""
	}
	return endpoint
}

func officialAPIEndpoint(c Config) string {
	host := strings.ToLower(strings.TrimSpace(c.OfficialHost))
	if host == "" {
		return ""
	}
	// Preserve an explicitly configured development origin and its port. In
	// production the public origin must belong to the official hostname.
	if c.Config != nil {
		u, err := url.Parse(c.PublicBaseURL)
		if err == nil && strings.EqualFold(u.Hostname(), host) && (u.Scheme == "https" || (!c.IsProduction() && u.Scheme == "http")) && u.User == nil {
			return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/v1"}).String()
		}
	}
	return "https://" + host + "/v1"
}
