package httpx

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"

	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
)

const ContextRequestID = "request_id"

// RequestID 保证每个请求都有可追踪的 request_id。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" {
			requestID = id.RequestID()
		}
		c.Set(ContextRequestID, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

// AccessLog 写结构化访问日志，不记录 Authorization 原文。
func AccessLog(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		requestID, _ := c.Get(ContextRequestID)
		span := trace.SpanFromContext(c.Request.Context())
		event := logger.Info().
			Str("request_id", stringify(requestID)).
			Str("method", c.Request.Method).
			Str("path", c.FullPath()).
			Str("route", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Str("trace_id", span.SpanContext().TraceID().String())
		if auth := c.GetHeader("Authorization"); auth != "" {
			event = event.Interface(logx.SafeField("authorization", auth))
		}
		event.Msg("http_request")
	}
}

// CORS 仅允许配置的 Web Origin。
func CORS(webOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (origin == webOrigin || webOrigin == "*") {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID, Idempotency-Key")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
