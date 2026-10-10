package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/ops"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

func (a *App) registerGatewayRoutes(r *gin.Engine) {
	r.POST("/v1/me/api-keys", a.requireAnyUser(), a.createAPIKey)
	r.GET("/v1/me/api-keys", a.requireAnyUser(), a.listAPIKeys)
	r.PUT("/v1/me/api-keys/:id/limits", a.requireAnyUser(), a.updateAPIKeyLimits)
	r.POST("/v1/me/api-keys/:id/enable", a.requireAnyUser(), a.enableAPIKey)
	r.POST("/admin/diagnostics/chat/completions", a.requireRoles("platform_admin", "tech_admin"), a.diagnosticKey(), a.chatCompletions)
	r.POST("/admin/diagnostics/responses", a.requireRoles("platform_admin", "tech_admin"), a.diagnosticKey(), a.responses)
	r.POST("/admin/diagnostics/messages", a.requireRoles("platform_admin", "tech_admin"), a.diagnosticKey(), a.messages)
	r.POST("/v1/me/api-keys/:id/rotate", a.requireAnyUser(), a.rotateAPIKey)
	r.POST("/v1/me/api-keys/:id/disable", a.requireAnyUser(), a.disableAPIKey)
	r.POST("/v1/me/api-keys/:id/expire", a.requireAnyUser(), a.expireAPIKey)
	r.POST("/v1/me/api-keys/:id/copy", a.requireAnyUser(), a.copyAPIKey)
	r.GET("/channel/api-keys", a.requireRoles("channel_admin"), a.listChannelAPIKeys)
	r.POST("/channel/api-keys/:id/disable", a.requireRoles("channel_admin"), a.channelDisableAPIKey)
	r.GET("/admin/api-keys", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listAdminAPIKeys)
	r.POST("/admin/api-keys/:id/disable", a.requireRoles("platform_admin", "tech_admin"), a.adminDisableAPIKey)
	r.GET("/v1/models", a.requireAPIKey(), a.listModels)
	r.GET("/v1/models/:model", a.requireAPIKey(), a.getModel)
	r.POST("/v1/chat/completions", a.requireAPIKey(), a.chatCompletions)
	r.POST("/v1/responses", a.requireAPIKey(), a.responses)
	r.POST("/v1/messages", a.requireAPIKey(), a.messages)
	r.GET("/v1/me/requests", a.requireUserOrKey(), a.listMyRequests)
	r.GET("/v1/requests/:id/attempts", a.requireAPIKey(), a.listAttempts)
	r.GET("/admin/diagnostics/requests/:id/attempts", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listDiagnosticAttempts)
	r.GET("/admin/providers", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listProviders)
	r.POST("/admin/providers", a.requireRoles("platform_admin", "tech_admin"), a.createProvider)
	r.GET("/admin/providers/:id", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.getProvider)
	r.PATCH("/admin/providers/:id", a.requireRoles("platform_admin", "tech_admin"), a.patchProvider)
	r.GET("/admin/providers/:id/accounts", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listProviderAccounts)
	r.POST("/admin/providers/:id/accounts", a.requireRoles("platform_admin", "tech_admin"), a.addProviderAccount)
	r.PATCH("/admin/providers/:id/accounts/:aid", a.requireRoles("platform_admin", "tech_admin"), a.patchProviderAccount)
	r.GET("/admin/providers/:id/upstream-models", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listProviderModels)
	r.POST("/admin/providers/:id/upstream-models/discover", a.requireRoles("platform_admin", "tech_admin"), a.discoverProviderModels)
	r.PUT("/admin/providers/:id/upstream-models", a.requireRoles("platform_admin", "tech_admin"), a.saveProviderModel)
	r.PATCH("/admin/providers/:id/upstream-models/status", a.requireRoles("platform_admin", "tech_admin"), a.setProviderModelStatus)
	r.POST("/admin/providers/:id/health-check", a.requireRoles("platform_admin", "tech_admin"), a.healthCheckProvider)
	r.GET("/admin/models", a.requireCatalogRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.listAdminModels)
	r.POST("/admin/models", a.requireCatalogRoles("platform_admin", "ops_admin"), a.createAdminModel)
	r.GET("/admin/models/*id", a.requireCatalogRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.getAdminModel)
	r.PATCH("/admin/models/*id", a.requireCatalogRoles("platform_admin", "ops_admin"), a.patchAdminModel)
	r.POST("/admin/models/review", a.requireCatalogRoles("platform_admin", "ops_admin"), a.reviewAdminModel)
	r.POST("/admin/models/publish", a.requireCatalogRoles("platform_admin", "ops_admin"), a.publishAdminModel)
	r.POST("/admin/models/deprecate", a.requireCatalogRoles("platform_admin", "ops_admin"), a.deprecateAdminModel)
	r.GET("/admin/routes", a.requireCatalogRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listAdminRoutes)
	r.GET("/admin/routes/:id", a.requireCatalogRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.getAdminRoute)
	r.POST("/admin/routes", a.requireCatalogRoles("platform_admin", "tech_admin"), a.createAdminRoute)
	r.PATCH("/admin/routes/:id", a.requireCatalogRoles("platform_admin", "tech_admin"), a.patchAdminRoute)
	r.POST("/admin/models/attach", a.requireCatalogRoles("platform_admin", "ops_admin", "tech_admin"), a.attachModelProvider)
}

func (a *App) currentAPIKey(c *gin.Context) *identity.APIKeyPrincipal {
	value, ok := c.Get("api_key")
	if !ok {
		return nil
	}
	principal, _ := value.(*identity.APIKeyPrincipal)
	return principal
}

func (a *App) requireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.rejectPublicControls(c) {
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(a.tokenFromRequest(c), "Bearer "))
		if token == "" {
			httpx.Abort(c, http.StatusUnauthorized, "authentication_error", "未登录", false)
			return
		}
		principal, err := a.Identity.AuthenticateAPIKey(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, identity.ErrKeyExpired) {
				httpx.Abort(c, http.StatusForbidden, "key_expired", "该 Key 已过期，请延长有效期", false)
				return
			}
			if abortKeyBudget(c, err) {
				return
			}
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "API Key 校验失败", true)
			return
		}
		if principal == nil {
			httpx.Abort(c, http.StatusForbidden, "key_invalid", "API Key 无效，请检查或创建 Key", false)
			return
		}
		c.Set("api_key", principal)
		if !a.enforceAPIKeyLimits(c, principal.APIKeyID, principal.RPMLimit, principal.ConcurrencyLimit) {
			return
		}
		c.Next()
		if rel, ok := c.Get("release_conc"); ok {
			if fn, ok := rel.(func()); ok {
				fn()
			}
		}
	}
}

func (a *App) createAPIKey(c *gin.Context) {
	var body identity.APIKeyLimits
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_key_limits", "Key 限制无效", false)
		return
	}
	if body.Name == "" {
		body.Name = "default"
	}
	key, err := a.Identity.CreateAPIKeyWithLimits(c.Request.Context(), *a.currentPrincipal(c), a.Config.EncryptionKey, body)
	if err != nil {
		if errors.Is(err, identity.ErrKeyCreationConflict) {
			httpx.Abort(c, http.StatusConflict, "key_operation_conflict", "此次创建操作的参数不一致，请恢复原操作", false)
			return
		}
		if errors.Is(err, identity.ErrInvalidKeyLimits) {
			httpx.Abort(c, http.StatusBadRequest, "invalid_key_limits", "请选择指定模型或有效的 USD 上限", false)
			return
		}
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "创建 Key 失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.create", ResourceType: "api_key", ResourceID: key.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": key, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listAPIKeys(c *gin.Context) {
	items, err := a.Identity.ListAPIKeys(c.Request.Context(), *a.currentPrincipal(c), a.Config.EncryptionKey)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 Key 失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.view", ResourceType: "api_key", ResourceID: "*",
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OKPage(c, items, 20, func(item identity.APIKeyView) string { return item.ID })
}

func (a *App) listAdminAPIKeys(c *gin.Context) {
	// D38：平台不再管理用户 API Key。
	httpx.Abort(c, http.StatusGone, "gone", "平台不再管理用户 API Key，请到渠道台查看", false)
}

func (a *App) listChannelAPIKeys(c *gin.Context) {
	principal := a.currentPrincipal(c)
	channelID := principal.ChannelOrgID
	if channelID == "" {
		httpx.Abort(c, http.StatusForbidden, "forbidden", "缺少渠道归属", false)
		return
	}
	items, err := a.Identity.ListAPIKeySummariesForChannel(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 Key 失败", true)
		return
	}
	if q := strings.ToLower(c.Query("q")); q != "" {
		filtered := make([]identity.APIKeyView, 0, len(items))
		for _, item := range items {
			hay := strings.ToLower(item.ID + item.Name + item.Prefix + item.Status + item.UserID + item.UserEmail)
			if strings.Contains(hay, q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if status := c.Query("status"); status != "" {
		filtered := make([]identity.APIKeyView, 0, len(items))
		for _, item := range items {
			if item.Status == status {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	httpx.OKPage(c, items, 100, func(item identity.APIKeyView) string { return item.ID })
}

func (a *App) channelDisableAPIKey(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	principal := a.currentPrincipal(c)
	channelID := principal.ChannelOrgID
	if channelID == "" {
		httpx.Abort(c, http.StatusForbidden, "forbidden", "缺少渠道归属", false)
		return
	}
	item, err := a.Identity.DisableChannelAPIKey(c.Request.Context(), channelID, c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "API Key 不存在或不属于本渠道", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "api_key.channel_disable", ResourceType: "api_key", ResourceID: item.ID,
		After: map[string]string{"status": item.Status, "channel_org_id": channelID},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) rotateAPIKey(c *gin.Context) {
	item, err := a.Identity.RotateAPIKey(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), a.Config.EncryptionKey)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "API Key 不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.rotate", ResourceType: "api_key", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) disableAPIKey(c *gin.Context) {
	item, err := a.Identity.DisableAPIKey(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "API Key 不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.disable", ResourceType: "api_key", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) expireAPIKey(c *gin.Context) {
	var body struct {
		ExpiresAt string `json:"expires_at"`
	}
	_ = c.ShouldBindJSON(&body)
	when := time.Now().UTC()
	if body.ExpiresAt != "" {
		if parsed, err := time.Parse(time.RFC3339, body.ExpiresAt); err == nil {
			when = parsed
		}
	}
	item, err := a.Identity.ExpireAPIKey(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), when)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "API Key 不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.expire", ResourceType: "api_key", ResourceID: item.ID,
		After: map[string]string{"expires_at": when.Format(time.RFC3339)},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) copyAPIKey(c *gin.Context) {
	keyID, err := a.Identity.ConfirmOwnedKey(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "API Key 不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "api_key.copy", ResourceType: "api_key", ResourceID: keyID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"copied": true, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminDisableAPIKey(c *gin.Context) {
	// D38：平台不再禁用用户 API Key。
	httpx.Abort(c, http.StatusGone, "gone", "平台不再管理用户 API Key，请由渠道管理员禁用", false)
}

func (a *App) listModels(c *gin.Context) {
	caller := a.currentAPIKey(c)
	if caller.ModelMode == "selected" && len(caller.Allowlist) == 0 {
		httpx.OK(c, gin.H{"data": []any{}})
		return
	}
	items, err := a.Catalog.ListVisibleModels(c.Request.Context(), caller.ChannelOrgID, caller.Allowlist)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取模型失败", true)
		return
	}
	httpx.OK(c, gin.H{"data": items, "object": "list", "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getModel(c *gin.Context) {
	caller := a.currentAPIKey(c)
	if !caller.AllowsModel(c.Param("model")) {
		httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "该 Key 不允许此模型", false)
		return
	}
	item, err := a.Catalog.GetVisibleModel(c.Request.Context(), caller.ChannelOrgID, c.Param("model"), caller.Allowlist)
	if err != nil {
		if errors.Is(err, catalog.ErrUnknownModel) {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "模型不存在", false)
			return
		}
		httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "模型不可用", false)
		return
	}
	httpx.OK(c, item)
}

func (a *App) chatCompletions(c *gin.Context) {
	a.executeProtocol(c, "openai.chat")
}

func (a *App) responses(c *gin.Context) { a.executeProtocol(c, "openai.responses") }

func (a *App) messages(c *gin.Context) {
	a.executeProtocol(c, "anthropic.messages")
}

func (a *App) executeProtocol(c *gin.Context, protocol string) *gateway.ExecuteOutput {
	rawBody, _ := io.ReadAll(c.Request.Body)
	if rec := a.replayIdempotency(c, rawBody); rec != nil {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(rawBody, &raw); err != nil && len(rawBody) > 0 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return nil
	}
	if raw == nil {
		raw = map[string]any{}
	}
	body := rawBody
	if len(body) == 0 {
		body, _ = json.Marshal(raw)
	}
	var chat gateway.ChatRequest
	_ = json.Unmarshal(body, &chat)
	if chat.Model == "" {
		if model, _ := raw["model"].(string); model != "" {
			chat.Model = model
		}
	}
	if protocol != "openai.chat" && chat.Stream {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "该协议暂不支持 stream，请使用 Chat Completions", false)
		return nil
	}
	if protocol == "openai.responses" {
		if _, ok := raw["input"].(string); !ok {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Responses 当前支持字符串 input", false)
			return nil
		}
		for _, field := range []string{"tools", "previous_response_id", "conversation", "include", "store"} {
			if _, ok := raw[field]; ok {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Responses 暂不支持参数 "+field, false)
				return nil
			}
		}
		if text, ok := raw["input"].(string); ok {
			chat.Messages = []gateway.ChatMessage{{Role: "user", Content: text}}
		}
		if n, ok := raw["max_output_tokens"].(float64); ok {
			v := int(n)
			chat.MaxTokens = &v
		}
	}
	if protocol == "anthropic.messages" {
		for _, field := range []string{"tool_choice", "thinking", "betas"} {
			if _, ok := raw[field]; ok {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages 暂不支持参数 "+field+"；工具调用请使用 Chat Completions", false)
				return nil
			}
		}
		if value, exists := raw["tools"]; exists {
			tools, ok := value.([]any)
			if !ok {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages tools 无效", false)
				return nil
			}
			converted := make([]map[string]any, 0, len(tools))
			for _, item := range tools {
				tool, ok := item.(map[string]any)
				if !ok {
					httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages tool 无效", false)
					return nil
				}
				name, _ := tool["name"].(string)
				schema, ok := tool["input_schema"].(map[string]any)
				if name == "" || !ok {
					httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages tool 需要 name/input_schema", false)
					return nil
				}
				converted = append(converted, map[string]any{"type": "function", "function": map[string]any{"name": name, "description": tool["description"], "parameters": schema}})
			}
			chat.Tools, _ = json.Marshal(converted)
		}
		rows, ok := raw["messages"].([]any)
		if !ok || len(rows) == 0 {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages 需要消息列表", false)
			return nil
		}
		chat.Messages = nil
		if system, exists := raw["system"]; exists {
			text, ok := system.(string)
			if !ok {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages system 当前支持字符串", false)
				return nil
			}
			chat.Messages = append(chat.Messages, gateway.ChatMessage{Role: "system", Content: text})
		}
		for _, item := range rows {
			row, ok := item.(map[string]any)
			if !ok {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages 消息无效", false)
				return nil
			}
			role, _ := row["role"].(string)
			text, ok := row["content"].(string)
			if !ok || (role != "user" && role != "assistant") {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "Messages 当前支持 user/assistant 字符串内容；内容块与工具结果请使用 Chat Completions", false)
				return nil
			}
			chat.Messages = append(chat.Messages, gateway.ChatMessage{Role: role, Content: text})
		}
	}
	if len(chat.Messages) == 0 {
		if msgs, ok := raw["messages"].([]any); ok {
			for _, msg := range msgs {
				row, _ := msg.(map[string]any)
				chat.Messages = append(chat.Messages, gateway.ChatMessage{
					Role: fmt.Sprint(row["role"]), Content: fmt.Sprint(row["content"]),
				})
			}
		}
	}
	hint := gateway.ParseHint(c.Query("provider.only"), c.Query("provider.ignore"), c.Query("provider.order"))
	execCtx := c.Request.Context()
	var live *sseWriter
	if chat.Stream && protocol == "openai.chat" {
		live = &sseWriter{c: c}
		execCtx = gateway.WithStreamSink(execCtx, live)
	}
	out, err := a.Gateway.Execute(execCtx, gateway.ExecuteInput{
		Caller:      *a.currentAPIKey(c),
		RequestID:   c.GetString(httpx.ContextRequestID),
		Protocol:    protocol,
		Hint:        hint,
		ForceFail:   c.GetHeader("X-Tokenhub-Force-Fail"),
		OmitUsage:   c.GetHeader("X-Tokenhub-Omit-Usage") == "1",
		SandboxMode: c.GetHeader("X-Tokenhub-Sandbox-Mode"),
		CanarySlug:  a.Ops.CanarySlug(c.Request.Context(), c.GetHeader("X-Tokenhub-Canary") == "1"),
		Chat:        chat,
	})
	if err != nil {
		if live != nil && live.started {
			return out
		}
		if abortKeyBudget(c, err) {
			return nil
		}
		switch {
		case errors.Is(err, billing.ErrConflict):
			httpx.Abort(c, http.StatusConflict, "idempotency_conflict", "请求与原预留不一致或已处理，请查询原请求", false)
		case errors.Is(err, gateway.ErrRequestUnknown):
			httpx.Abort(c, http.StatusGatewayTimeout, "request_outcome_unknown", "请求结果未知，预留额度保留；请查询原请求", true)
		case errors.Is(err, gateway.ErrUnsupportedParam):
			msg := "不支持的参数"
			var pe gateway.ParamError
			if errors.As(err, &pe) && pe.Param != "" {
				msg = "不支持的参数 " + pe.Param
			}
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", msg, false)
		case errors.Is(err, catalog.ErrUnknownModel):
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "模型不存在", false)
		case errors.Is(err, gateway.ErrModelNotAllowed):
			httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "模型未授权", false)
		case errors.Is(err, gateway.ErrInsufficientBalance):
			httpx.Abort(c, http.StatusPaymentRequired, "insufficient_balance", "余额不足", false)
		case errors.Is(err, gateway.ErrChannelDisabled), errors.Is(err, identity.ErrChannelDisabled):
			httpx.Abort(c, http.StatusForbidden, "channel_disabled", "渠道已停用，已冻结新消费", false)
		case errors.Is(err, ops.ErrRateLimited):
			httpx.Abort(c, http.StatusTooManyRequests, "rate_limited", "API Key 超过限额", false)
		default:
			httpx.Abort(c, http.StatusServiceUnavailable, "provider_unavailable", "该模型暂不可用", true)
		}
		return nil
	}
	if !c.GetBool("internal_diagnostic") {
		out.Response.Provider = ""
	}
	if chat.Stream && protocol == "openai.chat" {
		if live == nil {
			live = &sseWriter{c: c}
		}
		if !live.started {
			for _, chunk := range out.Stream {
				if err := live.Emit(chunk); err != nil {
					return out
				}
			}
		}
		if _, err := c.Writer.WriteString("data: [DONE]\n\n"); err == nil {
			c.Writer.Flush()
		}
		return out
	}
	if protocol == "openai.responses" {
		payload := gin.H{"id": out.Response.ID, "model": out.Response.Model, "usage": protocolUsage(out.Response.Usage), "object": "response", "status": "completed", "output": []gin.H{{"type": "message", "role": "assistant", "content": []gin.H{{"type": "output_text", "text": gatewayResponseText(out.Response)}}}}, "request_id": c.GetString(httpx.ContextRequestID)}
		a.rememberIdempotency(c, rawBody, http.StatusOK, payload)
		c.JSON(http.StatusOK, payload)
		return out
	}
	if protocol == "anthropic.messages" {
		payload := gin.H{
			"id": out.Response.ID, "type": "message", "role": "assistant", "model": out.Response.Model,
			"content": anthropicContent(out.Response),
			"usage":   protocolUsage(out.Response.Usage), "request_id": c.GetString(httpx.ContextRequestID),
		}
		a.rememberIdempotency(c, rawBody, http.StatusOK, payload)
		c.JSON(http.StatusOK, payload)
		return out
	}
	a.rememberIdempotency(c, rawBody, http.StatusOK, out.Response)
	c.JSON(http.StatusOK, out.Response)
	return out
}

func (a *App) idempotencyActor(c *gin.Context) string {
	if key := a.currentAPIKey(c); key != nil && key.APIKeyID != "" {
		return key.APIKeyID
	}
	if p := a.currentPrincipal(c); p != nil {
		return p.UserID
	}
	return "anon"
}

func (a *App) replayIdempotency(c *gin.Context, rawBody []byte) *redisx.IdemRecord {
	header := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if header == "" || a.Redis == nil {
		return nil
	}
	rec, err := redisx.RecallIdempotency(c.Request.Context(), a.Redis, a.idempotencyActor(c), header, redisx.HashBody(rawBody))
	if errors.Is(err, redisx.ErrIdempotencyConflict) {
		httpx.Abort(c, http.StatusConflict, "idempotency_conflict", "相同幂等键对应不同请求体", false)
		return &redisx.IdemRecord{}
	}
	if err != nil || rec == nil {
		return nil
	}
	c.Data(rec.Status, "application/json", rec.Body)
	c.Abort()
	return rec
}

func (a *App) rememberIdempotency(c *gin.Context, rawBody []byte, status int, payload any) {
	header := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if header == "" || a.Redis == nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = redisx.RememberIdempotency(c.Request.Context(), a.Redis, a.idempotencyActor(c), header, redisx.HashBody(rawBody), status, body)
}

func anthropicContent(resp gateway.ChatResponse) []gin.H {
	if len(resp.Choices) == 0 {
		return []gin.H{}
	}
	msg := resp.Choices[0].Message
	out := make([]gin.H, 0, 1+len(msg.ToolCalls))
	if msg.Content != "" {
		out = append(out, gin.H{"type": "text", "text": msg.Content})
	}
	for _, call := range msg.ToolCalls {
		var input any
		if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
			input = call.Function.Arguments
		}
		out = append(out, gin.H{"type": "tool_use", "id": call.ID, "name": call.Function.Name, "input": input})
	}
	return out
}

func (a *App) listMyRequests(c *gin.Context) { a.workflowRequests(c, "user") }

type requestReceiptView struct {
	ID                  string    `json:"id"`
	RequestID           string    `json:"request_id"`
	PublicModelID       string    `json:"public_model_id"`
	APIKeyID            string    `json:"api_key_id,omitempty"`
	Result              string    `json:"result"`
	BillingState        string    `json:"billing_state,omitempty"`
	CustomerAmountMinor int64     `json:"customer_amount_minor"`
	ErrorCode           string    `json:"error_code,omitempty"`
	HTTPStatus          int       `json:"http_status,omitempty"`
	StartedAt           time.Time `json:"started_at"`
	PromptTokens        int64     `json:"prompt_tokens,omitempty"`
	CompletionTokens    int64     `json:"completion_tokens,omitempty"`
	ReasoningTokens     int64     `json:"reasoning_tokens,omitempty"`
}

func (a *App) attachRequestBilling(ctx context.Context, userID, apiKeyID string, items []gateway.RequestView) ([]requestReceiptView, error) {
	out := make([]requestReceiptView, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.RequestID)
	}
	usageByReq := map[string]billing.UsageView{}
	if len(ids) > 0 {
		usages, err := a.Billing.QueryUsage(ctx, billing.QueryUsageInput{
			UserID: userID, APIKeyID: apiKeyID, RequestIDs: ids, Unlimited: true,
		})
		if err != nil {
			return nil, err
		}
		for _, usage := range usages {
			if _, ok := usageByReq[usage.RequestID]; ok {
				continue
			}
			usageByReq[usage.RequestID] = usage
		}
	}
	for _, item := range items {
		row := requestReceiptView{
			ID: item.RequestID, RequestID: item.RequestID, PublicModelID: item.PublicModelID,
			APIKeyID: item.APIKeyID, Result: item.Status, ErrorCode: item.ErrorCode,
			HTTPStatus: item.HTTPStatus, StartedAt: item.StartedAt,
		}
		if usage, ok := usageByReq[item.RequestID]; ok {
			row.BillingState = usage.State
			row.CustomerAmountMinor = usage.CustomerMinor
			row.PromptTokens = usage.PromptTokens
			row.CompletionTokens = usage.CompletionTokens
			row.ReasoningTokens = usage.ReasoningTokens
		}
		out = append(out, row)
	}
	return out, nil
}

func dimKeys(keys []string) []gin.H {
	out := make([]gin.H, 0, len(keys))
	for _, key := range keys {
		out = append(out, gin.H{"key": key})
	}
	return out
}

func (a *App) listAttempts(c *gin.Context) {
	key := a.currentAPIKey(c)
	items, err := a.Gateway.ListOwnedAttempts(c.Request.Context(), c.Param("id"), key.UserID, key.APIKeyID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "请求不存在", false)
		return
	}
	safe := make([]gin.H, 0, len(items))
	for _, item := range items {
		safe = append(safe, gin.H{"id": item.ID, "request_id": item.RequestID, "attempt_no": item.AttemptNo, "status": item.Status, "http_status": item.HTTPStatus, "error_code": item.ErrorCode, "latency_ms": item.LatencyMS, "prompt_tokens": item.PromptTokens, "completion_tokens": item.CompletionTokens, "total_tokens": item.TotalTokens})
	}
	httpx.OK(c, gin.H{"items": safe, "request_id": c.GetString(httpx.ContextRequestID)})
}
func (a *App) listDiagnosticAttempts(c *gin.Context) {
	items, err := a.Gateway.ListAttempts(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "请求不存在", false)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listProviders(c *gin.Context) {
	items, err := a.Catalog.ListProviders(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 Provider 失败", true)
		return
	}
	items = catalog.FilterProviders(items, c.Query("q"))
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "providers.csv", []string{"id", "slug", "adapter", "status", "health", "models"}, items, func(item catalog.ProviderView) []string {
			return []string{item.ID, item.Slug, item.Adapter, item.Status, item.Health, mappedPublicIDs(item)}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item catalog.ProviderView) string { return item.ID })
}

func (a *App) getProvider(c *gin.Context) {
	item, err := a.Catalog.GetProvider(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "Provider 不存在", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func mappedPublicIDs(item catalog.ProviderView) string {
	ids := make([]string, 0, len(item.Models))
	for _, model := range item.Models {
		if model.PublicID != "" {
			ids = append(ids, model.PublicID)
		}
	}
	return strings.Join(ids, " ")
}

func (a *App) healthCheckProvider(c *gin.Context) {
	health, err := a.Catalog.Probe(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, catalog.ErrProbeUnsupported) {
			httpx.Abort(c, http.StatusNotImplemented, "probe_not_supported", "此提供商尚不支持主动连通性探测，请完成真实上游联调；未修改路由健康标记。", false)
			return
		}
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "Provider 不存在", false)
		return
	}
	httpx.OK(c, gin.H{"health": health, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) createProvider(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.ProviderInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.CreateProvider(c.Request.Context(), body)
	if err != nil {
		msg := "创建 Provider 失败"
		if errors.Is(err, catalog.ErrBlockedURL) {
			msg = "上游 Base URL 不在白名单或指向私网"
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", msg, false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.create", ResourceType: "provider", ResourceID: item.ID,
		After: map[string]string{"slug": item.Slug, "adapter": item.Adapter},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchProvider(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.ProviderInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.PatchProvider(c.Request.Context(), c.Param("id"), body)
	if err != nil {
		if errors.Is(err, catalog.ErrBlockedURL) {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "上游 Base URL 不在白名单或指向私网", false)
			return
		}
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "Provider 不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.patch", ResourceType: "provider", ResourceID: item.ID,
		After: map[string]string{"status": item.Status, "health": item.Health},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listProviderModels(c *gin.Context) {
	items, err := a.Catalog.ListProviderModels(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "提供商不存在", false)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) discoverProviderModels(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	items, err := a.Catalog.DiscoverProviderModels(c.Request.Context(), c.Param("id"), a.Config.EncryptionKey)
	if err != nil {
		if errors.Is(err, catalog.ErrDiscoveryUnavailable) {
			httpx.Abort(c, http.StatusUnprocessableEntity, "discovery_unavailable", err.Error(), false)
			return
		}
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "探测上游模型失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.models.discover", ResourceType: "provider",
		ResourceID: c.Param("id"), After: map[string]any{"count": len(items)},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) saveProviderModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.ProviderModelInput
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "模型报价无效", false)
		return
	}
	item, err := a.Catalog.SaveProviderModel(c.Request.Context(), c.Param("id"), body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请填写上游模型标识和有效成本价", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.model.price", ResourceType: "provider_model",
		ResourceID: item.ProviderID + "/" + item.UpstreamModelID,
		After:      map[string]any{"unit_costs": item.UnitCosts},
		IP:         c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) setProviderModelStatus(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		UpstreamModelID string `json:"upstream_model_id"`
		Enabled         *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请选择上游模型和状态", false)
		return
	}
	item, err := a.Catalog.SetProviderModelEnabled(c.Request.Context(), c.Param("id"), body.UpstreamModelID, *body.Enabled)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "上游模型不存在", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "更新上游模型状态失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.model.status", ResourceType: "provider_model",
		ResourceID: item.ProviderID + "/" + item.UpstreamModelID,
		After:      map[string]any{"status": item.Status}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listProviderAccounts(c *gin.Context) {
	items, err := a.Catalog.ListAccounts(c.Request.Context(), c.Param("id"), c.Query("q"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "Provider 不存在", false)
		return
	}
	httpx.OKPage(c, items, 100, func(item catalog.AccountView) string { return item.ID })
}

func (a *App) addProviderAccount(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.AccountInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.AddAccount(c.Request.Context(), c.Param("id"), a.Config.EncryptionKey, body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "添加上游账号失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.account.add", ResourceType: "provider",
		ResourceID: c.Param("id"), After: map[string]string{"account_id": item.ID, "fingerprint": item.Fingerprint},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchProviderAccount(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.AccountInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.PatchAccount(c.Request.Context(), c.Param("id"), c.Param("aid"), body)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "账号不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.account.patch", ResourceType: "provider_account",
		ResourceID: item.ID, After: map[string]string{"status": item.Status},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listAdminModels(c *gin.Context) {
	items, err := a.Catalog.ListAdminModels(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取模型失败", true)
		return
	}
	items = catalog.FilterAdminModels(items, c.Query("status"), c.Query("sync_state"), c.Query("q"))
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "models.csv", []string{"id", "vendor", "display_name", "status", "sync_state"}, items, func(item catalog.ModelView) []string {
			return []string{item.ID, item.Vendor, item.DisplayName, item.Status, item.SyncState}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item catalog.ModelView) string { return item.ID })
}

func (a *App) createAdminModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.ModelInput
	_ = c.ShouldBindJSON(&body)
	item, price, err := a.Catalog.CreateModel(c.Request.Context(), body, a.currentPrincipal(c).UserID)
	if err != nil {
		a.abortCatalogModelWrite(c, err, "创建模型失败", "创建模型失败")
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.create", ResourceType: "model", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	if price != nil {
		_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
			ActorUserID: a.currentPrincipal(c).UserID, Action: "catalog.price.publish", ResourceType: "price_version",
			ResourceID: price.VersionID, After: price, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
		})
	}
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func catalogPublicID(c *gin.Context) string {
	return strings.TrimPrefix(c.Param("id"), "/")
}

func (a *App) getAdminModel(c *gin.Context) {
	item, err := a.Catalog.GetAdminModel(c.Request.Context(), catalogPublicID(c))
	if err != nil {
		a.abortCatalogModelWrite(c, err, "模型不存在", "读取模型失败")
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchAdminModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.ModelInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.PatchModel(c.Request.Context(), catalogPublicID(c), body)
	if err != nil {
		a.abortCatalogModelWrite(c, err, "模型不存在", "保存模型失败")
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.patch", ResourceType: "model", ResourceID: item.ID,
		After: map[string]string{"status": item.Status},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listAdminRoutes(c *gin.Context) {
	items, err := a.Catalog.ListRoutes(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取路由失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "routes.csv", []string{"id", "public_model_id", "strategy", "status"}, items, func(item catalog.RouteView) []string {
			return []string{item.ID, item.PublicModelID, item.Strategy, item.Status}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item catalog.RouteView) string { return item.ID })
}

func (a *App) getAdminRoute(c *gin.Context) {
	item, err := a.Catalog.GetRoute(c.Request.Context(), c.Param("id"))
	if err != nil {
		a.abortCatalogRouteWrite(c, err, "读取路由失败")
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) createAdminRoute(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.RouteInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.CreateRoute(c.Request.Context(), body)
	if err != nil {
		a.abortCatalogRouteWrite(c, err, "创建路由失败")
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "route.create", ResourceType: "route", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) patchAdminRoute(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.RouteInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.PatchRoute(c.Request.Context(), c.Param("id"), body)
	if err != nil {
		a.abortCatalogRouteWrite(c, err, "保存路由失败")
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "route.patch", ResourceType: "route", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) reviewAdminModel(c *gin.Context) {
	httpx.Abort(c, http.StatusGone, "invalid_request", "模型审核流程已移除，请完善模型后直接发布", false)
}

func (a *App) publishAdminModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		PublicID string `json:"public_id"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.PublishModel(c.Request.Context(), body.PublicID)
	if err != nil {
		a.abortCatalogModelWrite(c, err, "模型不存在", "发布模型失败")
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.publish", ResourceType: "model",
		ResourceID: item.ID, After: map[string]string{"status": item.Status},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) deprecateAdminModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		PublicID string `json:"public_id"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.DeprecateModel(c.Request.Context(), body.PublicID)
	if err != nil {
		a.abortCatalogModelWrite(c, err, "模型不存在", "弃用模型失败")
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.deprecate", ResourceType: "model",
		ResourceID: item.ID, After: map[string]string{"status": item.Status},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) attachModelProvider(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		PublicID   string `json:"public_id"`
		ProviderID string `json:"provider_id"`
		Upstream   string `json:"upstream_model_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if err := a.Catalog.AttachProvider(c.Request.Context(), body.PublicID, body.ProviderID, body.Upstream); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "挂载 Provider 失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.attach", ResourceType: "model", ResourceID: body.PublicID,
		After: map[string]string{"provider_id": body.ProviderID},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"ok": true, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) abortCatalogModelWrite(c *gin.Context, err error, notFoundMsg, fallbackMsg string) {
	switch {
	case errors.Is(err, catalog.ErrModelIncomplete):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "请填写模型类型和对应售价", false)
	case errors.Is(err, catalog.ErrModelExists):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "模型已存在，请编辑已有模型", false)
	case errors.Is(err, catalog.ErrUnknownModel):
		httpx.Abort(c, http.StatusNotFound, "invalid_request", notFoundMsg, false)
	case errors.Is(err, catalog.ErrInvalidInput):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", fallbackMsg, false)
	default:
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", fallbackMsg, false)
	}
}

func (a *App) abortCatalogRouteWrite(c *gin.Context, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "模型或路由不存在", false)
	case errors.Is(err, catalog.ErrRouteExists):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "该模型已有路由组", false)
	case errors.Is(err, catalog.ErrRouteIncomplete):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "启用前请配置有效的提供商与上游模型标识", false)
	case errors.Is(err, catalog.ErrProviderModelUnpriced):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "先在提供商详情配置该上游模型的成本价", false)
	case errors.Is(err, catalog.ErrModelIncomplete):
		httpx.Abort(c, http.StatusConflict, "invalid_request", "请先补齐模型类型和对应售价", false)
	default:
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", fallbackMsg, false)
	}
}

func gatewayResponseText(resp gateway.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
}

func protocolUsage(usage map[string]int) gin.H {
	if len(usage) == 0 {
		return gin.H{}
	}
	result := gin.H{"input_tokens": usage["prompt_tokens"], "output_tokens": usage["completion_tokens"], "total_tokens": usage["total_tokens"]}
	if n := usage["reasoning_tokens"]; n > 0 {
		result["output_tokens_details"] = gin.H{"reasoning_tokens": n}
	}
	return result
}
