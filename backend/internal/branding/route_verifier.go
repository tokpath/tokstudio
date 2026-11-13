package branding

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func DomainProof(key, host, token, nonce string) string {
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte("tokenhub-domain\x00" + host + "\x00" + token + "\x00" + nonce))
	return hex.EncodeToString(h.Sum(nil))
}
func ownership(ctx context.Context, resolver TXTResolver, host, token string) (bool, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	records, err := resolver.LookupTXT(ctx, "_tokenhub."+host)
	if err != nil {
		if dnsRecordMissing(err) {
			return false, nil
		}
		return false, ErrVerificationUnavailable
	}
	for _, r := range records {
		if strings.TrimSpace(r) == "tokenhub-verification="+token {
			return true, nil
		}
	}
	return false, nil
}

func dnsRecordMissing(err error) bool {
	var dns *net.DNSError
	return errors.As(err, &dns) && dns.IsNotFound
}

func tlsEvidenceFailed(err error) bool {
	var certificate *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	return errors.As(err, &certificate) || errors.As(err, &record)
}

func (v DNSAndTLSVerifier) VerifyOwnership(ctx context.Context, host, token string) (bool, error) {
	return ownership(ctx, v.Resolver, host, token)
}
func (v *CloudflareVerifier) VerifyOwnership(ctx context.Context, host, token string) (bool, error) {
	return ownership(ctx, v.settings.Resolver, host, token)
}

// RouteVerifier proves the verified HTTPS hostname reaches this installation's
// binding. A valid certificate for an unrelated website is not sufficient.
type RouteVerifier struct {
	Provider DomainVerifier
	Key      string
}

func (v RouteVerifier) VerifyOwnership(ctx context.Context, host, token string) (bool, error) {
	if p, ok := v.Provider.(interface {
		VerifyOwnership(context.Context, string, string) (bool, error)
	}); ok {
		return p.VerifyOwnership(ctx, host, token)
	}
	return false, nil
}
func (v RouteVerifier) Verify(ctx context.Context, host, token string) (DomainVerification, error) {
	out, e := v.Provider.Verify(ctx, host, token)
	if e != nil || !out.OwnershipVerified || !out.TLSReady {
		return out, e
	}
	out.RouteReady = false
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil && !dnsRecordMissing(e) {
		return out, ErrVerificationUnavailable
	}
	if e != nil || len(ips) == 0 {
		out.FailureReason = "Domain routing is not ready"
		return out, nil
	}
	for _, ip := range ips {
		if !publicDomainAddress(ip.IP) {
			out.FailureReason = "domain resolves to a restricted network address"
			return out, nil
		}
	}
	nonce := make([]byte, 32)
	if _, e = rand.Read(nonce); e != nil {
		return out, e
	}
	n := hex.EncodeToString(nonce)
	expected := DomainProof(v.Key, host, token, n)
	transport := &http.Transport{TLSClientConfig: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		var err error
		for _, ip := range ips {
			var c net.Conn
			c, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), "443"))
			if err == nil {
				return c, nil
			}
		}
		return nil, err
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", "https://"+host+"/.well-known/tokenhub-domain?nonce="+n, nil)
	if e != nil {
		return out, e
	}
	res, e := client.Do(req)
	if e != nil {
		if !tlsEvidenceFailed(e) {
			return out, ErrVerificationUnavailable
		}
	} else {
		defer res.Body.Close()
		if res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests || res.StatusCode == http.StatusRequestTimeout {
			return out, ErrVerificationUnavailable
		}
		b, err := io.ReadAll(io.LimitReader(res.Body, 256))
		if err != nil {
			return out, ErrVerificationUnavailable
		}
		out.RouteReady = res.StatusCode == http.StatusOK && hmac.Equal([]byte(expected), b)
	}
	if !out.RouteReady {
		out.FailureReason = "Domain does not route to this brand on TokenHub"
	}
	return out, nil
}
