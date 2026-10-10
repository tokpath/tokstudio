package gateway

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/catalog"
)

var errBlockedUpstream = errors.New("blocked upstream destination")

type upstreamHTTP struct {
	client        *http.Client
	allowedURL    *regexp.Regexp
	allowLoopback bool
	lookup        func(context.Context, string, string) ([]netip.Addr, error)
	dial          func(context.Context, string, string) (net.Conn, error)
}

func newUpstreamHTTP(settings Settings) *upstreamHTTP {
	guard := &upstreamHTTP{allowLoopback: settings.AllowTestLoopback && !settings.Production, lookup: net.DefaultResolver.LookupNetIP}
	var hosts []string
	for _, host := range catalog.UpstreamAllowedHosts(settings.UpstreamURLAllowlist) {
		host = strings.ToLower(strings.TrimSpace(host))
		// Entries are domains, never URLs, userinfo, ports or regex fragments.
		if host == "" || strings.ContainsAny(host, ":/\\@?#*%[]") {
			continue
		}
		hosts = append(hosts, `(?:[a-z0-9-]+\.)*`+regexp.QuoteMeta(host))
	}
	if guard.allowLoopback {
		hosts = append(hosts, `localhost`, `127(?:\.[0-9]{1,3}){3}`, `\[::1\]`)
	}
	scheme := "https"
	if !settings.Production {
		scheme = "https?"
	}
	guard.allowedURL = regexp.MustCompile(`(?i)^` + scheme + `://(?:` + strings.Join(hosts, "|") + `)(?::[0-9]{1,5})?(?:/[^?#\s\\]*)?$`)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	guard.dial = dialer.DialContext
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// An environment proxy would resolve targets outside our verified dial path.
	transport.Proxy = nil
	transport.DialContext = guard.dialContext
	transport.DialTLSContext = nil
	transport.TLSHandshakeTimeout = 10 * time.Second
	guard.client = &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return guard
}

func (g *upstreamHTTP) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || strings.Contains(u.Host, "%") {
		return errBlockedUpstream
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errBlockedUpstream
		}
	}
	host := strings.ToLower(u.Hostname())
	if host == "metadata" || host == "metadata.google.internal" || strings.HasSuffix(host, ".metadata.google.internal") {
		return errBlockedUpstream
	}
	if address, err := netip.ParseAddr(host); err == nil && !g.addressAllowed(address) {
		return errBlockedUpstream
	}
	return nil
}

var specialUpstreamNetworks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // Includes cloud metadata 100.100.100.200.
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
}

func (g *upstreamHTTP) addressAllowed(address netip.Addr) bool {
	address = address.Unmap()
	if address.Zone() != "" {
		return false
	}
	if address.IsLoopback() {
		return g.allowLoopback
	}
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLinkLocalUnicast() || address == netip.MustParseAddr("168.63.129.16") {
		return false
	}
	for _, network := range specialUpstreamNetworks {
		if network.Contains(address) {
			return false
		}
	}
	return true
}

func (g *upstreamHTTP) dialContext(ctx context.Context, network, authority string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		return nil, errBlockedUpstream
	}
	addresses, err := g.lookup(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, errBlockedUpstream
	}
	// Reject a mixed public/private answer as well. Dial only these inspected
	// addresses; never perform a second hostname lookup after the check.
	for _, address := range addresses {
		if !g.addressAllowed(address) {
			return nil, errBlockedUpstream
		}
	}
	var last error
	for _, address := range addresses {
		conn, err := g.dial(ctx, network, net.JoinHostPort(address.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}
