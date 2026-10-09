// Package endpoints performs explicit, non-billable checks of public brand entrypoints.
package endpoints

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"golang.org/x/net/html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Result struct {
	Host      string     `json:"host"`
	OK        bool       `json:"ok"`
	Reason    string     `json:"reason,omitempty"`
	CheckedAt time.Time  `json:"checked_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func publicIP(ip net.IP) bool {
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return false
	}
	for _, raw := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		_, block, _ := net.ParseCIDR(raw)
		if block.Contains(ip) {
			return false
		}
	}
	return true
}
func Verify(ctx context.Context, host, path, brandID string) Result {
	result := Result{Host: host, CheckedAt: time.Now().UTC()}
	if strings.ContainsAny(host, ":/@?#\\") || strings.TrimSpace(host) != host || host == "" {
		result.Reason = "invalid_domain"
		return result
	}
	parsed, err := url.Parse("https://" + host + path)
	if err != nil || parsed.Hostname() != host {
		result.Reason = "invalid_domain"
		return result
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		hostname, port, err := net.SplitHostPort(address)
		if err != nil || hostname != host || port != "443" {
			return nil, errors.New("invalid endpoint")
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", hostname)
		if err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, errors.New("no address")
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, errors.New("non-public endpoint")
			}
		}
		return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 || req.URL.Scheme != "https" || req.URL.Hostname() != host {
			return errors.New("invalid redirect")
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		result.Reason = "invalid_domain"
		return result
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		result.Reason = "dns_tls_or_route_failed"
		return result
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		result.Reason = "route_failed"
		return result
	}
	if brandID != "" && !responseBrandMatches(res.Body, path, brandID) {
		result.Reason = "brand_mismatch"
		return result
	}

	if res.TLS == nil || len(res.TLS.PeerCertificates) == 0 {
		result.Reason = "tls_unverified"
		return result
	}
	expires := res.TLS.PeerCertificates[0].NotAfter
	result.ExpiresAt = &expires
	result.OK = true
	return result
}

func responseBrandMatches(body io.Reader, path, brandID string) bool {
	if path == "/v1/public/brand" {
		var data struct {
			Brand struct {
				ID string `json:"id"`
			} `json:"brand"`
		}
		return json.NewDecoder(io.LimitReader(body, 32768)).Decode(&data) == nil && data.Brand.ID == brandID
	}
	tokenizer := html.NewTokenizer(io.LimitReader(body, 2<<20))
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			return false
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken {
			continue
		}
		token := tokenizer.Token()
		if token.Data != "meta" {
			continue
		}
		name, content := "", ""
		for _, attr := range token.Attr {
			if attr.Key == "name" {
				name = attr.Val
			}
			if attr.Key == "content" {
				content = attr.Val
			}
		}
		if name == "tokstudio-brand-id" {
			return content == brandID
		}
	}
}
