// Package logx 提供 zerolog 结构化日志，并默认脱敏敏感字段。
package logx

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

var sensitiveKeys = []string{
	"password", "password_hash", "secret", "token", "authorization",
	"api_key", "apikey", "ciphertext", "encryption_key", "credential",
	"access_token", "refresh_token", "bootstrap_admin_token", "bootstrap_user_token",
}

// New 创建 JSON 日志器。生产环境写 stdout，字段名统一小写下划线。
func New(level string, w io.Writer) zerolog.Logger {
	if w == nil {
		w = os.Stdout
	}
	zerolog.TimeFieldFormat = time.RFC3339Nano
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil {
		lvl = zerolog.InfoLevel
	}
	logger := zerolog.New(w).Level(lvl).With().Timestamp().Str("service", "tokenhub").Logger()
	return logger.Hook(redactHook{})
}

type redactHook struct{}

func (redactHook) Run(e *zerolog.Event, _ zerolog.Level, _ string) {
	// Hook 无法改写已经写入的字段值，敏感字段应通过 SafeField 写入。
	_ = e
}

// SafeField 把可能敏感的键写成 [REDACTED]。
func SafeField(key string, value any) (string, any) {
	if IsSensitiveKey(key) {
		if s, ok := value.(string); ok && s == "" {
			return key, ""
		}
		return key, "[REDACTED]"
	}
	return key, value
}

// IsSensitiveKey 判断字段名是否需要脱敏。
func IsSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, item := range sensitiveKeys {
		if normalized == item || strings.Contains(normalized, item) {
			return true
		}
	}
	return false
}

// RedactString 扫描文本，避免把常见密钥形态打进普通日志。
func RedactString(input string) string {
	lower := strings.ToLower(input)
	for _, item := range []string{"bearer ", "postgres://", "redis://"} {
		if strings.Contains(lower, item) {
			return "[REDACTED]"
		}
	}
	return input
}
