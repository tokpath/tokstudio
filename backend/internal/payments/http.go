package payments

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Transport struct {
	Client            *http.Client
	AllowTestLoopback bool
	AllowedRPCHosts   []string
}

func NewTransport(allowTestLoopback bool) *Transport {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	httpTransport := &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil {
			return nil, e
		}
		for _, ip := range ips {
			if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
				if !(allowTestLoopback && ip.IsLoopback()) {
					return nil, ErrForbidden
				}
			}
		}
		if len(ips) == 0 {
			return nil, ErrProvider
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	return &Transport{Client: &http.Client{Timeout: 15 * time.Second, Transport: httpTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, AllowTestLoopback: allowTestLoopback}
}
func (t *Transport) endpoint(m Merchant, official string) (string, error) {
	base := m.APIBaseURL
	if base == "" {
		base = official
	}
	u, e := url.Parse(base)
	if e != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrInvalid
	}
	expected, _ := url.Parse(official)
	if u.Scheme == "https" && u.Host == expected.Host {
		return strings.TrimRight(base, "/"), nil
	}
	ip, e := netip.ParseAddr(u.Hostname())
	if t != nil && t.AllowTestLoopback && e == nil && ip.IsLoopback() && u.Scheme == "http" {
		return strings.TrimRight(base, "/"), nil
	}
	return "", ErrForbidden
}
func (t *Transport) do(ctx context.Context, method, target string, body []byte, headers http.Header) ([]byte, http.Header, error) {
	if t == nil || t.Client == nil {
		return nil, nil, ErrNotConfigured
	}
	request, e := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if e != nil {
		return nil, nil, ErrInvalid
	}
	request.Header = headers.Clone()
	response, e := t.Client.Do(request)
	if e != nil {
		return nil, nil, ErrOutcomeUnknown
	}
	defer response.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if e != nil || len(raw) > 1<<20 {
		return nil, response.Header, ErrOutcomeUnknown
	}
	if response.StatusCode >= 500 || response.StatusCode == 408 {
		return nil, response.Header, ErrOutcomeUnknown
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.Header, ErrProvider
	}
	return raw, response.Header, nil
}
func required(m Merchant, fields ...string) []string {
	missing := []string{}
	for _, field := range fields {
		if strings.TrimSpace(m.Credentials[field]) == "" {
			missing = append(missing, field)
		}
	}
	return missing
}
func ready(m Merchant, d Driver) error {
	if m.ID == "" || m.OperatorID == "" || !m.Enabled || m.Driver != d.Name() || len(d.Validate(m)) > 0 {
		return ErrNotConfigured
	}
	return nil
}
