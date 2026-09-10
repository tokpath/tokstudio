package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	IssuerCloudflare     = "cloudflare"
	defaultCloudflareAPI = "https://api.cloudflare.com/client/v4"
)

var ErrCloudflareFailed = errors.New("cloudflare custom hostname failed")

type CloudflareOptions struct {
	APIToken    string
	ZoneID      string
	CNAMETarget string
	BaseURL     string
	HTTPClient  *http.Client
}

// Cloudflare 登记 SSL for SaaS Custom Hostname。Token+Zone 齐才启用。
type Cloudflare struct {
	token   string
	zoneID  string
	cname   string
	baseURL string
	http    *http.Client
}

type HostnameResult struct {
	ID        string
	Status    string
	Issuer    string
	ExpiresAt *time.Time
}

func NewCloudflare(opts CloudflareOptions) *Cloudflare {
	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base == "" {
		base = defaultCloudflareAPI
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Cloudflare{
		token:   strings.TrimSpace(opts.APIToken),
		zoneID:  strings.TrimSpace(opts.ZoneID),
		cname:   strings.TrimSpace(opts.CNAMETarget),
		baseURL: base,
		http:    client,
	}
}

func (c *Cloudflare) Ready() bool {
	return c != nil && c.token != "" && c.zoneID != ""
}

func (c *Cloudflare) CNAMETarget() string {
	if c == nil {
		return ""
	}
	return c.cname
}

func (c *Cloudflare) Ensure(ctx context.Context, hostname string) (*HostnameResult, error) {
	if !c.Ready() {
		return nil, wrapCloudflare(errors.New("cloudflare token or zone missing"))
	}
	hostname = strings.ToLower(strings.TrimSpace(strings.Split(hostname, ":")[0]))
	if hostname == "" || !UsePublicACME(hostname) {
		return nil, wrapCloudflare(errors.New("custom hostname 需要公网形态域名"))
	}
	created, err := c.create(ctx, hostname)
	if err == nil {
		return created, nil
	}
	if !isAlreadyExists(err) {
		return nil, err
	}
	return c.getByHostname(ctx, hostname)
}

func (c *Cloudflare) Get(ctx context.Context, hostname string) (*HostnameResult, error) {
	if !c.Ready() {
		return nil, wrapCloudflare(errors.New("cloudflare token or zone missing"))
	}
	hostname = strings.ToLower(strings.TrimSpace(strings.Split(hostname, ":")[0]))
	if hostname == "" || !UsePublicACME(hostname) {
		return nil, wrapCloudflare(errors.New("custom hostname 需要公网形态域名"))
	}
	return c.getByHostname(ctx, hostname)
}

func (c *Cloudflare) create(ctx context.Context, hostname string) (*HostnameResult, error) {
	body, _ := json.Marshal(map[string]any{
		"hostname": hostname,
		"ssl": map[string]any{
			"method": "http",
			"type":   "dv",
		},
	})
	resp, err := c.do(ctx, http.MethodPost, "/zones/"+url.PathEscape(c.zoneID)+"/custom_hostnames", body)
	if err != nil {
		return nil, err
	}
	return parseHostnameResult(resp)
}

func (c *Cloudflare) getByHostname(ctx context.Context, hostname string) (*HostnameResult, error) {
	path := "/zones/" + url.PathEscape(c.zoneID) + "/custom_hostnames?hostname=" + url.QueryEscape(hostname)
	raw, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var env struct {
		Success bool            `json:"success"`
		Errors  []cfAPIError    `json:"errors"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, wrapCloudflare(err)
	}
	if !env.Success {
		return nil, wrapCloudflare(fmt.Errorf("cloudflare: %s", cfErrorText(env.Errors)))
	}
	var list []cfHostname
	if err := json.Unmarshal(env.Result, &list); err != nil {
		var one cfHostname
		if err2 := json.Unmarshal(env.Result, &one); err2 != nil {
			return nil, wrapCloudflare(err)
		}
		list = []cfHostname{one}
	}
	if len(list) == 0 {
		return nil, wrapCloudflare(errors.New("custom hostname not found after create conflict"))
	}
	return list[0].toResult(), nil
}

func (c *Cloudflare) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, wrapCloudflare(err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, wrapCloudflare(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, wrapCloudflare(err)
	}
	var env struct {
		Success bool         `json:"success"`
		Errors  []cfAPIError `json:"errors"`
	}
	_ = json.Unmarshal(raw, &env)
	if resp.StatusCode == http.StatusConflict || cfAlreadyExists(env.Errors) {
		return nil, alreadyExistsError()
	}
	if resp.StatusCode >= 400 || !env.Success {
		msg := cfErrorText(env.Errors)
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return nil, wrapCloudflare(fmt.Errorf("cloudflare HTTP %d: %s", resp.StatusCode, msg))
	}
	return raw, nil
}

type cfAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfHostname struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	SSL    struct {
		Status    string `json:"status"`
		ExpiresOn string `json:"expires_on"`
	} `json:"ssl"`
}

func (h cfHostname) toResult() *HostnameResult {
	status := "pending"
	if strings.EqualFold(h.SSL.Status, "active") {
		status = "issued"
	}
	var exp *time.Time
	if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(h.SSL.ExpiresOn)); err == nil {
		t := ts.UTC()
		exp = &t
	}
	return &HostnameResult{ID: h.ID, Status: status, Issuer: IssuerCloudflare, ExpiresAt: exp}
}

func parseHostnameResult(raw []byte) (*HostnameResult, error) {
	var env struct {
		Success bool         `json:"success"`
		Errors  []cfAPIError `json:"errors"`
		Result  cfHostname   `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, wrapCloudflare(err)
	}
	if !env.Success {
		return nil, wrapCloudflare(fmt.Errorf("cloudflare: %s", cfErrorText(env.Errors)))
	}
	if env.Result.ID == "" && env.Result.SSL.Status == "" {
		return nil, wrapCloudflare(errors.New("empty custom hostname result"))
	}
	return env.Result.toResult(), nil
}

type alreadyExists struct{}

func (alreadyExists) Error() string { return "custom hostname already exists" }

func alreadyExistsError() error { return alreadyExists{} }

func isAlreadyExists(err error) bool {
	var dup alreadyExists
	return errors.As(err, &dup)
}

func cfAlreadyExists(errs []cfAPIError) bool {
	for _, item := range errs {
		if item.Code == 1406 || item.Code == 1407 {
			return true
		}
		if strings.Contains(strings.ToLower(item.Message), "already exists") {
			return true
		}
	}
	return false
}

func cfErrorText(errs []cfAPIError) string {
	parts := make([]string, 0, len(errs))
	for _, item := range errs {
		if strings.TrimSpace(item.Message) != "" {
			parts = append(parts, item.Message)
		}
	}
	return strings.Join(parts, "; ")
}

func wrapCloudflare(err error) error {
	if err == nil {
		return nil
	}
	return errors.Join(ErrCloudflareFailed, err)
}
