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
	Env             string
	HTTPAddr        string
	PublicBaseURL   string
	WebOrigin       string
	DatabaseURL     string
	RedisURL        string
	BootstrapAdmin   string
	BootstrapUser    string
	BootstrapChannel string
	GoogleClientID   string
	GoogleRedirect   string
	OTELEndpoint    string
	OTELServiceName string
	LogLevel        string
	EncryptionKey   string
	AllowDemoProbes bool
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
		Env:             v.GetString("ENV"),
		HTTPAddr:        v.GetString("HTTP_ADDR"),
		PublicBaseURL:   v.GetString("PUBLIC_BASE_URL"),
		WebOrigin:       v.GetString("WEB_ORIGIN"),
		DatabaseURL:     v.GetString("DATABASE_URL"),
		RedisURL:        v.GetString("REDIS_URL"),
		BootstrapAdmin:   v.GetString("BOOTSTRAP_ADMIN_TOKEN"),
		BootstrapUser:    v.GetString("BOOTSTRAP_USER_TOKEN"),
		BootstrapChannel: v.GetString("BOOTSTRAP_CHANNEL_TOKEN"),
		GoogleClientID:   v.GetString("GOOGLE_CLIENT_ID"),
		GoogleRedirect:   v.GetString("GOOGLE_REDIRECT_URL"),
		OTELEndpoint:    v.GetString("OTEL_EXPORTER_OTLP_ENDPOINT"),
		OTELServiceName: v.GetString("OTEL_SERVICE_NAME"),
		LogLevel:        v.GetString("LOG_LEVEL"),
		EncryptionKey:   v.GetString("ENCRYPTION_KEY"),
		AllowDemoProbes: v.GetBool("ALLOW_DEMO_PROBES"),
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

func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

// RedactedMap 返回可安全写入日志的配置摘要，绝不包含密钥原文。
func (c *Config) RedactedMap() map[string]any {
	return map[string]any{
		"env":               c.Env,
		"http_addr":         c.HTTPAddr,
		"public_base_url":   c.PublicBaseURL,
		"web_origin":        c.WebOrigin,
		"database_configured": c.DatabaseURL != "",
		"redis_configured":    c.RedisURL != "",
		"otel_endpoint_set":   c.OTELEndpoint != "",
		"otel_service_name":   c.OTELServiceName,
		"log_level":           c.LogLevel,
		"bootstrap_admin_set": c.BootstrapAdmin != "",
		"bootstrap_user_set":  c.BootstrapUser != "",
		"encryption_key_set":  c.EncryptionKey != "",
	}
}
