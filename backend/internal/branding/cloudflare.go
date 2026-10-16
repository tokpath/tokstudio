package branding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type TXTResolver interface {
	LookupTXT(context.Context, string) ([]string, error)
}
type CloudflareSettings struct {
	APIToken, ZoneID, CNAME string
	// APIBaseURL is normally omitted. Tests can supply an isolated HTTP server.
	APIBaseURL string
	Resolver   TXTResolver
}
type CloudflareVerifier struct {
	settings CloudflareSettings
	client   *http.Client
}

// NewCloudflareVerifier uses Cloudflare for SaaS custom-hostname issuance.
// The zone must already have its fallback origin and SaaS entitlement set up.
// No API call or external provisioning occurs at construction time.
func NewCloudflareVerifier(in CloudflareSettings, client *http.Client) (*CloudflareVerifier, error) {
	if strings.TrimSpace(in.APIToken) == "" || strings.ContainsAny(in.APIToken, "\r\n") || !regexp.MustCompile(`^[0-9a-fA-F]{32}$`).MatchString(in.ZoneID) {
		return nil, ErrInvalidInput
	}
	if in.APIBaseURL == "" {
		in.APIBaseURL = "https://api.cloudflare.com/client/v4"
	}
	u, e := url.Parse(in.APIBaseURL)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"))) {
		return nil, ErrInvalidInput
	}
	in.APIBaseURL = strings.TrimRight(in.APIBaseURL, "/")
	if in.CNAME != "" {
		in.CNAME, e = NormalizeDomain(in.CNAME)
		if e != nil {
			return nil, e
		}
	}
	if in.Resolver == nil {
		in.Resolver = net.DefaultResolver
	}
	c := http.Client{Timeout: 15 * time.Second}
	if client != nil {
		c = *client
	}
	if c.Timeout == 0 || c.Timeout > 15*time.Second {
		c.Timeout = 15 * time.Second
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &CloudflareVerifier{settings: in, client: &c}, nil
}

type cfHostname struct {
	ID                    string            `json:"id"`
	Hostname              string            `json:"hostname"`
	Status                string            `json:"status"`
	CustomMetadata        map[string]string `json:"custom_metadata"`
	OwnershipVerification struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"ownership_verification"`
	SSL struct {
		Status            string `json:"status"`
		ValidationRecords []struct {
			TXTName     string `json:"txt_name"`
			TXTValue    string `json:"txt_value"`
			CNAME       string `json:"cname"`
			CNAMETarget string `json:"cname_target"`
		} `json:"validation_records"`
	} `json:"ssl"`
}
type cfEnvelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Code int `json:"code"`
	} `json:"errors"`
}

func (v *CloudflareVerifier) call(ctx context.Context, method, path string, body any, out any) error {
	var data []byte
	var e error
	if body != nil {
		data, e = json.Marshal(body)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, v.settings.APIBaseURL+path, bytes.NewReader(data))
	if e != nil {
		return ErrVerificationUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+v.settings.APIToken)
	req.Header.Set("Content-Type", "application/json")
	res, e := v.client.Do(req)
	if e != nil {
		return fmt.Errorf("%w: Cloudflare request failed", ErrVerificationUnavailable)
	}
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if e != nil {
		return ErrVerificationUnavailable
	}
	var envelope cfEnvelope
	if e = json.Unmarshal(raw, &envelope); e != nil || res.StatusCode < 200 || res.StatusCode >= 300 || !envelope.Success {
		return fmt.Errorf("%w: Cloudflare response status %d", ErrVerificationUnavailable, res.StatusCode)
	}
	if e = json.Unmarshal(envelope.Result, out); e != nil {
		return ErrVerificationUnavailable
	}
	return nil
}
func (v *CloudflareVerifier) Verify(ctx context.Context, hostname, challenge string) (DomainVerification, error) {
	hostname, e := NormalizeDomain(hostname)
	if e != nil || challenge == "" {
		return DomainVerification{}, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	records, e := v.settings.Resolver.LookupTXT(ctx, "_tokenhub."+hostname)
	matched := false
	for _, record := range records {
		if strings.TrimSpace(record) == "tokenhub-verification="+challenge {
			matched = true
		}
	}
	if e != nil || !matched {
		return DomainVerification{FailureReason: "DNS ownership challenge does not match"}, nil
	}
	hash := sha256.Sum256([]byte(challenge))
	expected := hex.EncodeToString(hash[:])
	base := "/zones/" + v.settings.ZoneID + "/custom_hostnames"
	var candidates []cfHostname
	if e = v.call(ctx, http.MethodGet, base+"?hostname="+url.QueryEscape(hostname), nil, &candidates); e != nil {
		return DomainVerification{}, e
	}
	var host cfHostname
	for _, candidate := range candidates {
		if candidate.Hostname == hostname {
			if host.ID != "" {
				return DomainVerification{}, ErrConflict
			}
			host = candidate
		}
	}
	if host.ID == "" {
		body := map[string]any{"hostname": hostname, "custom_metadata": map[string]string{"tokenhub_verification_hash": expected}, "ssl": map[string]any{"method": "txt", "type": "dv", "settings": map[string]string{"min_tls_version": "1.2"}}}
		if e = v.call(ctx, http.MethodPost, base, body, &host); e != nil {
			return DomainVerification{}, e
		}
	}
	// Recover a created hostname after a timeout by exact-host lookup. A foreign
	// preexisting hostname is never silently adopted into this brand binding.
	if host.ID == "" || host.Hostname != hostname || host.CustomMetadata["tokenhub_verification_hash"] != expected {
		return DomainVerification{}, ErrConflict
	}
	out := DomainVerification{OwnershipVerified: true, ProviderReference: "cloudflare:" + v.settings.ZoneID + ":" + host.ID, ProviderStatus: host.Status, TLSStatus: host.SSL.Status}
	if v.settings.CNAME != "" {
		out.Instructions = append(out.Instructions, DNSInstruction{Type: "CNAME", Name: hostname, Value: v.settings.CNAME, Purpose: "route"})
	}
	if record := host.OwnershipVerification; record.Type == "txt" && record.Name != "" && record.Value != "" {
		out.Instructions = append(out.Instructions, DNSInstruction{Type: "TXT", Name: record.Name, Value: record.Value, Purpose: "cloudflare_ownership"})
	}
	for _, r := range host.SSL.ValidationRecords {
		if r.TXTName != "" && r.TXTValue != "" {
			out.Instructions = append(out.Instructions, DNSInstruction{Type: "TXT", Name: r.TXTName, Value: r.TXTValue, Purpose: "certificate"})
		}
		if r.CNAME != "" && r.CNAMETarget != "" {
			out.Instructions = append(out.Instructions, DNSInstruction{Type: "CNAME", Name: r.CNAME, Value: r.CNAMETarget, Purpose: "certificate"})
		}
	}
	out.TLSReady = host.Status == "active" && host.SSL.Status == "active"
	if !out.TLSReady {
		out.FailureReason = "Cloudflare hostname or certificate validation is pending"
	}
	return out, nil
}
