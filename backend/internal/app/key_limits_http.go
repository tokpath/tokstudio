package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

// Public control validation looks only at gateway controls, never message or
// tool content. It runs before replay so rejected controls cannot replay a charge.
func (a *App) rejectPublicControls(c *gin.Context) bool {
	if c.GetBool("internal_diagnostic") {
		return false
	}
	control := map[string]bool{"provider_id": true, "provider_only": true, "provider_ignore": true, "provider_order": true, "providers": true, "routing_policy": true, "route_strategy": true, "retry_policy": true, "diagnostics": true, "provider": true, "provider.only": true, "provider.ignore": true, "provider.order": true, "routing": true, "route": true, "route_id": true, "upstream": true, "upstream_model": true, "upstream_model_id": true, "fallback": true, "retries": true, "canary": true, "sandbox_mode": true, "force_fail": true, "omit_usage": true}
	reject := func(field string) bool {
		httpx.Abort(c, http.StatusBadRequest, "unsupported_control_parameter", "不支持的内部控制参数 "+field, false)
		return true
	}
	for key := range c.Request.URL.Query() {
		if control[strings.ToLower(key)] {
			return reject(key)
		}
	}
	for _, key := range []string{"X-Tokenhub-Force-Fail", "X-Tokenhub-Omit-Usage", "X-Tokenhub-Sandbox-Mode", "X-Tokenhub-Canary", "X-Tokenhub-Provider", "X-Tokenhub-Route", "X-Tokenhub-Test-Key"} {
		if _, ok := c.Request.Header[http.CanonicalHeaderKey(key)]; ok {
			return reject(key)
		}
	}
	if c.Request.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 8<<20+1))
		if err != nil || len(raw) > 8<<20 {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效或过大", false)
			return true
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) == nil {
			for key := range fields {
				if control[strings.ToLower(key)] {
					return reject(key)
				}
			}
		}
	}
	return false
}

// Explicit platform diagnostics use a staff session and a separately supplied
// test Key. Public API credentials alone cannot authorize this entry point.
func (a *App) diagnosticKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, err := a.Identity.AuthenticateAPIKey(c.Request.Context(), c.GetHeader("X-Tokenhub-Test-Key"))
		if err != nil || key == nil || key.UserID != a.currentPrincipal(c).UserID {
			httpx.Abort(c, http.StatusForbidden, "key_invalid", "诊断需要有效的测试 Key", false)
			return
		}
		c.Set("api_key", key)
		c.Set("internal_diagnostic", true)
		_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "gateway.diagnostic", ResourceType: "api_key", ResourceID: key.APIKeyID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		if !a.enforceAPIKeyLimits(c, key.APIKeyID, key.RPMLimit, key.ConcurrencyLimit) {
			return
		}
		c.Next()
		if release, ok := c.Get("release_conc"); ok {
			if fn, ok := release.(func()); ok {
				fn()
			}
		}
	}
}

func (a *App) updateAPIKeyLimits(c *gin.Context) {
	var body identity.APIKeyLimits
	if c.ShouldBindJSON(&body) != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_key_limits", "Key 限制无效", false)
		return
	}
	item, err := a.Identity.UpdateAPIKeyLimits(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), body)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidKeyLimits):
			httpx.Abort(c, http.StatusBadRequest, "invalid_key_limits", "请选择模型范围、有效的 USD 上限和有效期", false)
		default:
			httpx.Abort(c, http.StatusNotFound, "key_not_found", "API Key 不存在或暂不可编辑", false)
		}
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.limits", ResourceType: "api_key", ResourceID: item.ID, After: map[string]any{"model_mode": item.ModelMode, "allowlist": item.Allowlist, "budget_limit_minor": item.BudgetLimitMinor, "expires_at": item.ExpiresAt}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) enableAPIKey(c *gin.Context) {
	item, err := a.Identity.EnableAPIKey(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "key_not_found", "API Key 不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.enable", ResourceType: "api_key", ResourceID: item.ID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.OK(c, gin.H{"item": item})
}
func abortKeyBudget(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, identity.ErrKeyBudgetExceeded):
		httpx.Abort(c, http.StatusPaymentRequired, "key_budget_exceeded", "该 Key 的 USD 上限不足；可编辑上限或等待处理中请求完成", false)
	case errors.Is(err, billing.ErrPriceEstimateUnavailable):
		httpx.Abort(c, http.StatusBadRequest, "price_estimate_unavailable", "此模型的定价暂无法预估，请联系当前品牌支持", false)
	case errors.Is(err, identity.ErrKeyNotUsable):
		httpx.Abort(c, http.StatusForbidden, "key_unusable", "该 Key 已停用或过期", false)
	case errors.Is(err, identity.ErrKeyModelNotAllowed):
		httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "该 Key 不允许此模型", false)
	default:
		return false
	}
	return true
}
