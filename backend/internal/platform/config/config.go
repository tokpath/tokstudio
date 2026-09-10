// Package config 用 Viper 读取配置。
// 密钥只来自环境变量或本地文件，默认值不能包含真实生产密钥。
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config 是进程启动后的只读配置快照。
type Config struct {
	Env                    string
	HTTPAddr               string
	PublicBaseURL          string
	WebOrigin              string
	DatabaseURL            string
	RedisURL               string
	BootstrapAdmin         string
	BootstrapUser          string
	BootstrapChannel       string
	GoogleClientID         string
	GoogleClientSecret     string
	GoogleRedirect         string
	GoogleAllowMock        bool
	BifrostSandbox         bool
	OpenAIAPIKey           string
	AnthropicAPIKey        string
	GeminiAPIKey           string
	OpenRouterAPIKey       string
	OTELEndpoint           string
	OTELServiceName        string
	LogLevel               string
	EncryptionKey          string
	AllowDemoProbes        bool
	MediaStorePath         string
	MediaSignKey           string
	S3Endpoint             string
	S3Bucket               string
	S3AccessKey            string
	S3SecretKey            string
	S3Region               string
	ArkBaseURL             string
	ArkAPIKey              string
	OpenRouterBaseURL      string
	PaymentSignKey         string
	UpstreamURLAllowlist   []string
	EdgeCNAME              string
	CloudflareAPIToken     string
	CloudflareZoneID       string
	CloudflareCNAME        string
	CloudflareBaseURL      string
	ACMEDirectory          string
	ACMEInsecureSkipVerify bool
	ACMEForce              bool
}

// Load 从环境变量读取 TOKENHUB_* 配置。
func Load() (*Config, error) {
	v := viper.New()
	v.SetEnvPrefix("TOKENHUB")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("ENV", "development")
	v.SetDefault("HTTP_ADDR", ":8080")
	v.SetDefault("PUBLIC_BASE_URL", "http://localhost:8080")
	v.SetDefault("WEB_ORIGIN", "http://localhost:3000")
	v.SetDefault("DATABASE_URL", "postgres://tokenhub:tokenhub@localhost:5432/tokenhub?sslmode=disable")
	v.SetDefault("REDIS_URL", "redis://localhost:6379/0")
	v.SetDefault("OTEL_SERVICE_NAME", "tokenhub-api")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("ALLOW_DEMO_PROBES", true)

	loadDotEnv(".env")
	loadDotEnv("../.env")

	cfg := &Config{
		Env:                    v.GetString("ENV"),
		HTTPAddr:               v.GetString("HTTP_ADDR"),
		PublicBaseURL:          v.GetString("PUBLIC_BASE_URL"),
		WebOrigin:              v.GetString("WEB_ORIGIN"),
		DatabaseURL:            v.GetString("DATABASE_URL"),
		RedisURL:               v.GetString("REDIS_URL"),
		BootstrapAdmin:         v.GetString("BOOTSTRAP_ADMIN_TOKEN"),
		BootstrapUser:          v.GetString("BOOTSTRAP_USER_TOKEN"),
		BootstrapChannel:       v.GetString("BOOTSTRAP_CHANNEL_TOKEN"),
		GoogleClientID:         v.GetString("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:     v.GetString("GOOGLE_CLIENT_SECRET"),
		GoogleRedirect:         v.GetString("GOOGLE_REDIRECT_URL"),
		GoogleAllowMock:        v.GetBool("GOOGLE_ALLOW_MOCK"),
		BifrostSandbox:         resolveBifrostSandbox(v),
		OpenAIAPIKey:           v.GetString("OPENAI_API_KEY"),
		AnthropicAPIKey:        v.GetString("ANTHROPIC_API_KEY"),
		GeminiAPIKey:           v.GetString("GEMINI_API_KEY"),
		OpenRouterAPIKey:       v.GetString("OPENROUTER_API_KEY"),
		OTELEndpoint:           v.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OTELServiceName:        v.GetString("OTEL_SERVICE_NAME"),
		LogLevel:               v.GetString("LOG_LEVEL"),
		EncryptionKey:          v.GetString("ENCRYPTION_KEY"),
		AllowDemoProbes:        v.GetBool("ALLOW_DEMO_PROBES"),
		MediaStorePath:         v.GetString("MEDIA_STORE_PATH"),
		MediaSignKey:           v.GetString("MEDIA_SIGN_KEY"),
		S3Endpoint:             v.GetString("S3_ENDPOINT"),
		S3Bucket:               v.GetString("S3_BUCKET"),
		S3AccessKey:            v.GetString("S3_ACCESS_KEY"),
		S3SecretKey:            v.GetString("S3_SECRET_KEY"),
		S3Region:               v.GetString("S3_REGION"),
		ArkBaseURL:             v.GetString("ARK_BASE_URL"),
		ArkAPIKey:              v.GetString("ARK_API_KEY"),
		OpenRouterBaseURL:      v.GetString("OPENROUTER_BASE_URL"),
		PaymentSignKey:         v.GetString("PAYMENT_SIGN_KEY"),
		UpstreamURLAllowlist:   splitCSV(v.GetString("UPSTREAM_URL_ALLOWLIST")),
		EdgeCNAME:              v.GetString("EDGE_CNAME"),
		CloudflareAPIToken:     v.GetString("CLOUDFLARE_API_TOKEN"),
		CloudflareZoneID:       v.GetString("CLOUDFLARE_ZONE_ID"),
		CloudflareCNAME:        v.GetString("CLOUDFLARE_CNAME_TARGET"),
		CloudflareBaseURL:      v.GetString("CLOUDFLARE_BASE_URL"),
		ACMEDirectory:          v.GetString("ACME_DIRECTORY"),
		ACMEInsecureSkipVerify: v.GetBool("ACME_INSECURE_SKIP_VERIFY"),
		ACMEForce:              v.GetBool("ACME_FORCE"),
	}
	if cfg.EdgeCNAME == "" {
		cfg.EdgeCNAME = "edge.tokenhub.local"
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("TOKENHUB_DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("TOKENHUB_REDIS_URL is required")
	}
	return cfg, nil
}

// IsProduction 表示禁止演示探测和宽松启动。
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// GoogleTriad 表示 Client ID / Secret / Redirect URI 三件套齐全，可以走真实交换。
func (c *Config) GoogleTriad() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.GoogleClientID) != "" &&
		strings.TrimSpace(c.GoogleClientSecret) != "" &&
		strings.TrimSpace(c.GoogleRedirect) != ""
}

// GoogleMockAllowed 仅在显式打开且不在 production / grok / test 预览时允许 mock。
// 默认禁止；缺三件套时不得静默 mock 成功。
func (c *Config) GoogleMockAllowed() bool {
	if c == nil || !c.GoogleAllowMock {
		return false
	}
	if c.IsProduction() {
		return false
	}
	host := strings.ToLower(c.PublicBaseURL + " " + c.WebOrigin)
	if strings.Contains(host, "grok.tokpath.com") || strings.Contains(host, "test.tokpath.com") {
		return false
	}
	return true
}

func resolveBifrostSandbox(v *viper.Viper) bool {
	if v.IsSet("BIFROST_SANDBOX") {
		return v.GetBool("BIFROST_SANDBOX")
	}
	return !strings.EqualFold(v.GetString("ENV"), "production")
}

// RedactedMap 返回可安全写入日志的配置摘要，绝不包含密钥原文。
func (c *Config) RedactedMap() map[string]any {
	return map[string]any{
		"env":                  c.Env,
		"http_addr":            c.HTTPAddr,
		"public_base_url":      c.PublicBaseURL,
		"web_origin":           c.WebOrigin,
		"database_configured":  c.DatabaseURL != "",
		"redis_configured":     c.RedisURL != "",
		"otel_endpoint_set":    c.OTELEndpoint != "",
		"otel_service_name":    c.OTELServiceName,
		"log_level":            c.LogLevel,
		"bootstrap_admin_set":  c.BootstrapAdmin != "",
		"bootstrap_user_set":   c.BootstrapUser != "",
		"encryption_key_set":   c.EncryptionKey != "",
		"bifrost_sandbox":      c.BifrostSandbox,
		"openai_key_set":       c.OpenAIAPIKey != "",
		"ark_url_set":          c.ArkBaseURL != "",
		"ark_key_set":          c.ArkAPIKey != "",
		"openrouter_url_set":   c.OpenRouterBaseURL != "",
		"openrouter_key_set":   c.OpenRouterAPIKey != "",
		"s3_endpoint_set":      c.S3Endpoint != "",
		"s3_bucket_set":        c.S3Bucket != "",
		"s3_key_set":           c.S3AccessKey != "",
		"acme_directory_set":   c.ACMEDirectory != "",
		"acme_force":           c.ACMEForce,
		"cloudflare_token_set": c.CloudflareAPIToken != "",
		"cloudflare_zone_set":  c.CloudflareZoneID != "",
		"cloudflare_cname_set": c.CloudflareCNAME != "",
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
