package branding

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"golang.org/x/net/dns/dnsmessage"
)

func TestPublicDomainAddressRejectsPrivateAndTransitionNetworks(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.168.1.1", "198.18.0.1", "192.0.2.1", "0.0.0.0", "224.0.0.1", "::1", "fd00::1", "fe80::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2002:7f00:1::1", "2001:db8::1"} {
		if publicDomainAddress(net.ParseIP(ip)) {
			t.Fatal("accepted restricted address", ip)
		}
	}
	for _, ip := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicDomainAddress(net.ParseIP(ip)) {
			t.Fatal("rejected public address", ip)
		}
	}
}
func dnsFixture(t *testing.T, address [4]byte) *net.Resolver {
	t.Helper()
	conn, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, peer, e := conn.ReadFrom(buf)
			if e != nil {
				return
			}
			var p dnsmessage.Parser
			header, e := p.Start(buf[:n])
			if e != nil {
				continue
			}
			question, e := p.Question()
			if e != nil {
				continue
			}
			builder := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: header.ID, Response: true, RecursionDesired: header.RecursionDesired, RecursionAvailable: true})
			_ = builder.StartQuestions()
			_ = builder.Question(question)
			_ = builder.StartAnswers()
			resource := dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET, TTL: 1}
			if question.Type == dnsmessage.TypeTXT {
				resource.Type = dnsmessage.TypeTXT
				_ = builder.TXTResource(resource, dnsmessage.TXTResource{TXT: []string{"tokenhub-verification=proof"}})
			}
			if question.Type == dnsmessage.TypeA {
				resource.Type = dnsmessage.TypeA
				_ = builder.AResource(resource, dnsmessage.AResource{A: address})
			}
			response, e := builder.Finish()
			if e == nil {
				_, _ = conn.WriteTo(response, peer)
			}
		}
	}()
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", conn.LocalAddr().String())
	}}
}
func TestDNSVerifierRejectsVerifiedDomainPointingToLoopbackBeforeDial(t *testing.T) {
	var calls atomic.Int64
	v := DNSAndTLSVerifier{Resolver: dnsFixture(t, [4]byte{127, 0, 0, 1}), Dialer: &net.Dialer{Control: func(string, string, syscall.RawConn) error { calls.Add(1); return errors.New("must not dial") }}}
	result, e := v.Verify(context.Background(), "tenant.example", "proof")
	if e != nil || !result.OwnershipVerified || result.TLSReady || calls.Load() != 0 || !strings.Contains(result.FailureReason, "restricted") {
		t.Fatal(result, e, calls.Load())
	}
}
func TestDNSVerifierPinsValidatedIPInsteadOfResolvingHostnameAgain(t *testing.T) {
	var target string
	v := DNSAndTLSVerifier{Resolver: dnsFixture(t, [4]byte{8, 8, 8, 8}), Dialer: &net.Dialer{Control: func(_, address string, _ syscall.RawConn) error {
		target = address
		return errors.New("test prevents external connection")
	}}}
	result, e := v.Verify(context.Background(), "tenant.example", "proof")
	if e != nil || !result.OwnershipVerified || result.TLSReady || target != "8.8.8.8:443" {
		t.Fatal("TLS dial did not pin validated public address", target, result, e)
	}
}
