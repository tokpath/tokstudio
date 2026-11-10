package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/branding"
	"net/url"
	"strings"
)

// TLSAsk is deliberately outside brand middleware: Caddy's internal request
// host is api:8080, while the candidate hostname is its domain query parameter.
func (a *Application) tlsAsk(c *gin.Context) {
	host, e := branding.NormalizeDomain(c.Query("domain"))
	if e != nil {
		c.Status(403)
		return
	}
	if a.officialHostname(host) {
		c.Status(200)
		return
	}
	_, e = a.Branding.TLSBinding(c.Request.Context(), host)
	if e != nil {
		c.Status(403)
		return
	}
	c.Status(200)
}
func (a *Application) officialHostname(host string) bool {
	if strings.EqualFold(host, a.Config.OfficialHost) {
		return true
	}
	u, e := url.Parse(a.Config.PublicBaseURL)
	return e == nil && u.Hostname() != "" && strings.EqualFold(host, u.Hostname())
}

func (a *Application) domainProof(c *gin.Context) {
	nonce := c.Query("nonce")
	if len(nonce) != 64 {
		c.Status(404)
		return
	}
	d, e := a.Branding.TLSBinding(c.Request.Context(), c.Request.Host)
	if e != nil {
		c.Status(404)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.String(200, branding.DomainProof(a.Config.AppKey, d.Hostname, d.VerificationToken, nonce))
}
