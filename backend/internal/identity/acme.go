package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/acme"
)

var ErrACMEFailed = errors.New("acme issuance failed")

const (
	IssuerSandbox      = "sandbox"
	IssuerACME         = "acme"
	DirectoryLetsEnc   = "https://acme-v02.api.letsencrypt.org/directory"
	DirectoryLEStaging = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// ACME 走 RFC 8555：有 Directory 才真签发。空 Directory 表示沙箱，只改状态。
type ACME struct {
	Directory          string
	InsecureSkipVerify bool
	Force              bool
	mu                 sync.Mutex
	challenges         map[string]string
}

type ACMEResult struct {
	Issuer    string
	Directory string
	ExpiresAt *time.Time
}

func NewACME(directory string, insecure, force bool) *ACME {
	return &ACME{Directory: ResolveACMEDirectory(directory), InsecureSkipVerify: insecure, Force: force, challenges: map[string]string{}}
}

func UsePublicACME(domain string) bool {
	domain = strings.ToLower(strings.TrimSpace(strings.Split(domain, ":")[0]))
	if domain == "" || net.ParseIP(domain) != nil {
		return false
	}
	for _, suf := range []string{".localhost", ".local", ".test", ".invalid"} {
		if strings.HasSuffix(domain, suf) {
			return false
		}
	}
	return true
}

func ResolveACMEDirectory(raw string) string {
	raw = strings.TrimSpace(raw)
	switch strings.ToLower(raw) {
	case "", "sandbox", "off", "none":
		return ""
	case "letsencrypt", "le", "prod", "production":
		return DirectoryLetsEnc
	case "staging", "le-staging":
		return DirectoryLEStaging
	default:
		return raw
	}
}

func (a *ACME) Enabled() bool {
	return a != nil && a.Directory != ""
}

func (a *ACME) LookupChallenge(token string) string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.challenges[token]
}

func (a *ACME) putChallenge(token, keyAuth string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.challenges == nil {
		a.challenges = map[string]string{}
	}
	a.challenges[token] = keyAuth
}

func (a *ACME) deleteChallenge(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.challenges, token)
}

func (a *ACME) Issue(ctx context.Context, domain string) (*ACMEResult, error) {
	if !a.Enabled() {
		return &ACMEResult{Issuer: IssuerSandbox}, nil
	}
	domain = strings.ToLower(strings.TrimSpace(strings.Split(domain, ":")[0]))
	if domain == "" || net.ParseIP(domain) != nil {
		return nil, InvalidACME("ACME 需要可解析的域名，不能是空或 IP")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	client := &acme.Client{Key: key, DirectoryURL: a.Directory, HTTPClient: a.httpClient()}
	if _, err := client.Register(ctx, &acme.Account{Contact: []string{"mailto:ops@tokenhub.local"}}, acme.AcceptTOS); err != nil {
		return nil, wrapACME(err)
	}
	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(domain))
	if err != nil {
		return nil, wrapACME(err)
	}
	for _, u := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, u)
		if err != nil {
			return nil, wrapACME(err)
		}
		chal, ok := http01(authz)
		if !ok {
			return nil, InvalidACME("上游未提供 HTTP-01 挑战")
		}
		keyAuth, err := client.HTTP01ChallengeResponse(chal.Token)
		if err != nil {
			return nil, wrapACME(err)
		}
		a.putChallenge(chal.Token, keyAuth)
		defer a.deleteChallenge(chal.Token)
		if _, err := client.Accept(ctx, chal); err != nil {
			return nil, wrapACME(err)
		}
		if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
			return nil, wrapACME(err)
		}
	}
	if _, err := client.WaitOrder(ctx, order.URI); err != nil {
		return nil, wrapACME(err)
	}
	csr, err := newCSR(domain)
	if err != nil {
		return nil, err
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		return nil, wrapACME(err)
	}
	exp := certExpiry(der)
	return &ACMEResult{Issuer: IssuerACME, Directory: a.Directory, ExpiresAt: exp}, nil
}

func (a *ACME) httpClient() *http.Client {
	if !a.InsecureSkipVerify {
		return http.DefaultClient
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 30 * time.Second}
}

func http01(authz *acme.Authorization) (*acme.Challenge, bool) {
	for _, chal := range authz.Challenges {
		if chal.Type == "http-01" {
			return chal, true
		}
	}
	return nil, false
}

func newCSR(domain string) ([]byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.CertificateRequest{Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain}}
	return x509.CreateCertificateRequest(rand.Reader, tmpl, key)
}

func certExpiry(chain [][]byte) *time.Time {
	if len(chain) == 0 {
		return nil
	}
	cert, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil
	}
	exp := cert.NotAfter.UTC()
	return &exp
}

func wrapACME(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrACMEFailed, err)
}

func InvalidACME(msg string) error {
	return errors.Join(ErrACMEFailed, errors.New(msg))
}

func EncodeCertPEM(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
