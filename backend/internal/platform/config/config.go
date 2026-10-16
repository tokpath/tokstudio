// Package config 用 Viper 读取配置。
// 密钥只来自环境变量或本地文件，默认值不能包含真实生产密钥。
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

// Config 是进程启动后的只读配置快照。
type Config struct {
	Env                  string
	HTTPAddr             string
	PublicBaseURL        string
	WebOrigin            string
	DatabaseURL          string
	RedisURL             string
	SMTPHost             string
	SMTPPort             int
	SMTPUsername         string
	SMTPPassword         string
	SMTPFrom             string
	SMTPSecurity         string
	OTELEndpoint         string
	OTELServiceName      string
	LogLevel             string
	EncryptionKey        string
	S3Endpoint           string
	S3Region             string
	S3Bucket             string
	S3AccessKey          string
	S3SecretKey          string
	S3ForcePathStyle     bool
	UpstreamURLAllowlist []string
	CloudflareAPIToken   string
	CloudflareZoneID     string
	CloudflareCNAME      string
	CloudflareBaseURL    string
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
	v.SetDefault("DATABASE_URL", "postgres://tokenhub:tokenhub@localhost:5432/tokenhub_v2?sslmode=disable")
	v.SetDefault("REDIS_URL", "redis://localhost:6379/0")
	v.SetDefault("OTEL_SERVICE_NAME", "tokenhub-api")
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("SMTP_PORT", 587)
	v.SetDefault("SMTP_SECURITY", "starttls")

	loadDotEnv(".env")
	loadDotEnv("../.env")

	cfg := &Config{
		Env:                  v.GetString("ENV"),
		HTTPAddr:             v.GetString("HTTP_ADDR"),
		PublicBaseURL:        v.GetString("PUBLIC_BASE_URL"),
		WebOrigin:            v.GetString("WEB_ORIGIN"),
		DatabaseURL:          v.GetString("DATABASE_URL"),
		RedisURL:             v.GetString("REDIS_URL"),
		SMTPHost:             v.GetString("SMTP_HOST"),
		SMTPPort:             v.GetInt("SMTP_PORT"),
		SMTPUsername:         v.GetString("SMTP_USERNAME"),
		SMTPPassword:         normalizeSecretValue(v.GetString("SMTP_PASSWORD")),
		SMTPFrom:             v.GetString("SMTP_FROM"),
		SMTPSecurity:         v.GetString("SMTP_SECURITY"),
		OTELEndpoint:         v.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OTELServiceName:      v.GetString("OTEL_SERVICE_NAME"),
		LogLevel:             v.GetString("LOG_LEVEL"),
		EncryptionKey:        firstNonEmpty(v.GetString("APP_KEY"), v.GetString("ENCRYPTION_KEY")),
		S3Endpoint:           v.GetString("S3_ENDPOINT"),
		S3Region:             v.GetString("S3_REGION"),
		S3Bucket:             v.GetString("S3_BUCKET"),
		S3AccessKey:          firstNonEmpty(v.GetString("S3_ACCESS_KEY"), os.Getenv("AWS_ACCESS_KEY_ID")),
		S3SecretKey:          firstNonEmpty(v.GetString("S3_SECRET_KEY"), os.Getenv("AWS_SECRET_ACCESS_KEY")),
		S3ForcePathStyle:     v.GetBool("S3_FORCE_PATH_STYLE"),
		UpstreamURLAllowlist: splitCSV(v.GetString("UPSTREAM_URL_ALLOWLIST")),
		CloudflareAPIToken:   v.GetString("CLOUDFLARE_API_TOKEN"),
		CloudflareZoneID:     v.GetString("CLOUDFLARE_ZONE_ID"),
		CloudflareCNAME:      v.GetString("CLOUDFLARE_CNAME_TARGET"),
		CloudflareBaseURL:    v.GetString("CLOUDFLARE_BASE_URL"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("TOKENHUB_DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return nil, fmt.Errorf("TOKENHUB_REDIS_URL is required")
	}
	if cfg.SMTPHost != "" || cfg.SMTPFrom != "" || cfg.SMTPUsername != "" || cfg.SMTPPassword != "" {
		if _, err := mail.NewSMTP(mail.SMTPOptions{Host: cfg.SMTPHost, Port: cfg.SMTPPort, From: cfg.SMTPFrom, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, Security: cfg.SMTPSecurity, Production: cfg.IsProduction()}); err != nil {
			return nil, fmt.Errorf("TOKENHUB_SMTP configuration is invalid")
		}
		if strings.TrimSpace(cfg.EncryptionKey) == "" {
			return nil, fmt.Errorf("TOKENHUB_APP_KEY is required for email delivery")
		}
	}
	return cfg, nil
}

// loadDotEnv never executes shell expressions and never overwrites an explicit environment value.
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
		value = normalizeSecretValue(value)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

// normalizeSecretValue 去掉首尾空白与一层成对引号，避免 .env 写成
// 防止本地配置文件的引号成为密码内容；不执行任何插值。
func normalizeSecretValue(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
	}
	return value
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

// RedactedMap contains readiness flags only, never secret fragments or lengths.
func (c *Config) RedactedMap() map[string]any {
	return map[string]any{
		"env": c.Env, "http_addr": c.HTTPAddr, "log_level": c.LogLevel,
		"database_configured": c.DatabaseURL != "", "redis_configured": c.RedisURL != "",
		"otel_endpoint_set": c.OTELEndpoint != "", "otel_service_name": c.OTELServiceName,
		"encryption_key_set": c.EncryptionKey != "", "s3_endpoint_set": c.S3Endpoint != "",
		"s3_bucket_set": c.S3Bucket != "", "s3_key_set": c.S3AccessKey != "",
		"smtp_configured":      c.SMTPHost != "" && c.SMTPFrom != "",
		"cloudflare_token_set": c.CloudflareAPIToken != "",
		"cloudflare_zone_set":  c.CloudflareZoneID != "", "cloudflare_cname_set": c.CloudflareCNAME != "",
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
