// Package httpx 提供 HTTP 公共约定：request_id、错误结构和 CORS。
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// APIError 与 docs/06 的错误结构对齐。
type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Param     any    `json:"param"`
	RequestID string `json:"request_id"`
	Retryable bool   `json:"retryable"`
}

func Abort(c *gin.Context, status int, code, message string, retryable bool) {
	AbortParam(c, status, code, message, nil, retryable)
}

// AbortParam 与 Abort 相同，但可带上安全的 param（例如 Google OAuth 的 error 码）。
func AbortParam(c *gin.Context, status int, code, message string, param any, retryable bool) {
	requestID, _ := c.Get(ContextRequestID)
	c.AbortWithStatusJSON(status, gin.H{
		"error": APIError{
			Code:      code,
			Message:   message,
			Param:     param,
			RequestID: stringify(requestID),
			Retryable: retryable,
		},
	})
}

func OK(c *gin.Context, body any) {
	c.JSON(http.StatusOK, body)
}

func Created(c *gin.Context, body any) {
	c.JSON(http.StatusCreated, body)
}

func Accepted(c *gin.Context, body any) {
	c.JSON(http.StatusAccepted, body)
}

func stringify(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
