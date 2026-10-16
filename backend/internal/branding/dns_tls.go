package branding

import (
	"context"
	"crypto/tls"
	"net"
	"net/netip"
	"strings"
	"time"
)

// DNSAndTLSVerifier verifies a real DNS TXT challenge and a normal TLS
// certificate/hostname handshake. Cloudflare provisioning adapters can also
// implement DomainVerifier when provider-specific readiness is required.
type DNSAndTLSVerifier struct {
	Resolver *net.Resolver
	Dialer   *net.Dialer
}

func (v DNSAndTLSVerifier) Verify(ctx context.Context, hostname, challenge string) (DomainVerification, error) {
	hostname, err := NormalizeDomain(hostname)
	if err != nil {
		return DomainVerification{}, err
	}
	if challenge == "" {
		return DomainVerification{}, ErrInvalidInput
	}
	resolver := v.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := v.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 10 * time.Second}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	records, err := resolver.LookupTXT(ctx, "_tokenhub."+hostname)
	if err != nil {
		return DomainVerification{FailureReason: "DNS verification record is not available"}, nil
	}
	matched := false
	for _, r := range records {
		if strings.TrimSpace(r) == "tokenhub-verification="+challenge {
			matched = true
			break
		}
	}
	if !matched {
		return DomainVerification{FailureReason: "DNS verification challenge does not match"}, nil
	}
	out := DomainVerification{OwnershipVerified: true, ProviderReference: "dns:" + hostname}
	tlsDialer := tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12}}
	addresses, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil || len(addresses) == 0 {
		out.FailureReason = "domain has no verified public network address"
		return out, nil
	}
	for _, address := range addresses {
		if !publicDomainAddress(address.IP) {
			out.FailureReason = "domain resolves to a restricted network address"
			return out, nil
		}
	}
	// Pin the already-checked address. Resolving the hostname again during dial
	// would allow rebinding between validation and the TLS network connection.
	for _, address := range addresses {
		conn, err := tlsDialer.DialContext(ctx, "tcp", net.JoinHostPort(address.IP.String(), "443"))
		if err != nil {
			continue
		}
		_ = conn.Close()
		out.TLSReady = true
		out.ProviderReference = "dns-tls:" + hostname
		return out, nil
	}
	out.FailureReason = "TLS certificate or hostname is not ready"
	return out, nil
}

func publicDomainAddress(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsUnspecified() {
		return false
	}
	for _, block := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001::/32", "2001:db8::/32", "2002::/16", "64:ff9b::/96"} {
		if netip.MustParsePrefix(block).Contains(addr) {
			return false
		}
	}
	return true
}
