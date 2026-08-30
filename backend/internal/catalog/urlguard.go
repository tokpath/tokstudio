package catalog

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ErrBlockedURL 表示上游 Base URL 不安全或不在白名单。
var ErrBlockedURL = errors.New("blocked upstream url")

var defaultAllowHosts = []string{
	"api.openai.com",
	"api.anthropic.com",
	"openrouter.ai",
	"generativelanguage.googleapis.com",
	"ark.cn-beijing.volces.com",
	"ark.ap-southeast.volces.com",
}

// ValidateUpstreamURL 校验 Provider Base URL。
// 空地址允许（沙箱适配器）。任意环境都拒绝链路本地/元数据地址。
// 生产环境只允许 https，且主机必须落在 allowlist。
func ValidateUpstreamURL(raw string, production bool, extraAllow []string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return ErrBlockedURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrBlockedURL
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || isBlockedHost(host) {
		return ErrBlockedURL
	}
	if !production {
		return nil
	}
	if parsed.Scheme != "https" {
		return ErrBlockedURL
	}
	allowed := append([]string{}, defaultAllowHosts...)
	allowed = append(allowed, extraAllow...)
	if !hostAllowed(host, allowed) {
		return ErrBlockedURL
	}
	return nil
}

func hostAllowed(host string, allow []string) bool {
	for _, item := range allow {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if host == item || strings.HasSuffix(host, "."+item) {
			return true
		}
	}
	return false
}

func isBlockedHost(host string) bool {
	if host == "metadata.google.internal" || host == "metadata" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	return false
}
