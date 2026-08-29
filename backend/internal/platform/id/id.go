// Package id 生成带前缀的稳定公共 ID，便于日志和账务关联。
package id

import (
	"strings"

	"github.com/google/uuid"
)

// New 生成形如 prefix_01H... 的 ID。prefix 只允许小写字母。
func New(prefix string) string {
	prefix = strings.TrimSpace(strings.ToLower(prefix))
	if prefix == "" {
		prefix = "id"
	}
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

// RequestID 生成请求 ID。
func RequestID() string {
	return New("req")
}
