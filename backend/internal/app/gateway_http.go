package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
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
	r.POST("/v1/me/api-keys/:id/rotate", a.requireAnyUser(), a.rotateAPIKey)
	r.POST("/v1/me/api-keys/:id/disable", a.requireAnyUser(), a.disableAPIKey)
	r.POST("/v1/me/api-keys/:id/expire", a.requireAnyUser(), a.expireAPIKey)
	r.POST("/v1/me/api-keys/:id/copy", a.requireAnyUser(), a.copyAPIKey)
	r.GET("/admin/api-keys", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listAdminAPIKeys)
	r.POST("/admin/api-keys/:id/disable", a.requireRoles("platform_admin", "tech_admin"), a.adminDisableAPIKey)
	r.GET("/v1/models", a.requireAPIKey(), a.listModels)
	r.GET("/v1/models/:model", a.requireAPIKey(), a.getModel)
	r.POST("/v1/chat/completions", a.requireAPIKey(), a.chatCompletions)
	r.POST("/v1/responses", a.requireAPIKey(), a.responses)
	r.POST("/v1/messages", a.requireAPIKey(), a.messages)
	r.GET("/v1/requests/:id/attempts", a.requireAPIKey(), a.listAttempts)
	r.GET("/admin/providers", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listProviders)
	r.POST("/admin/providers", a.requireRoles("platform_admin", "tech_admin"), a.createProvider)
	r.PATCH("/admin/providers/:id", a.requireRoles("platform_admin", "tech_admin"), a.patchProvider)
	r.POST("/admin/providers/:id/credentials", a.requireRoles("platform_admin", "tech_admin"), a.rotateProviderCredential)
	r.GET("/admin/providers/:id/accounts", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listProviderAccounts)
	r.POST("/admin/providers/:id/accounts", a.requireRoles("platform_admin", "tech_admin"), a.addProviderAccount)
	r.PATCH("/admin/providers/:id/accounts/:aid", a.requireRoles("platform_admin", "tech_admin"), a.patchProviderAccount)
	r.POST("/admin/providers/:id/health-check", a.requireRoles("platform_admin", "tech_admin"), a.healthCheckProvider)
	r.POST("/admin/providers/:id/sync", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.syncProvider)
	r.GET("/admin/models", a.requireRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.listAdminModels)
	r.POST("/admin/models", a.requireRoles("platform_admin", "ops_admin"), a.createAdminModel)
	r.GET("/admin/models/*id", a.requireRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.getAdminModel)
	r.PATCH("/admin/models/*id", a.requireRoles("platform_admin", "ops_admin"), a.patchAdminModel)
	r.POST("/admin/models/review", a.requireRoles("platform_admin", "ops_admin"), a.reviewAdminModel)
	r.POST("/admin/models/publish", a.requireRoles("platform_admin", "ops_admin"), a.publishAdminModel)
	r.POST("/admin/models/deprecate", a.requireRoles("platform_admin", "ops_admin"), a.deprecateAdminModel)
	r.GET("/admin/routes", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.listAdminRoutes)
	r.POST("/admin/routes", a.requireRoles("platform_admin", "tech_admin"), a.createAdminRoute)
	r.PATCH("/admin/routes/:id", a.requireRoles("platform_admin", "tech_admin"), a.patchAdminRoute)
	r.POST("/admin/models/attach", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.attachModelProvider)
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
		token := strings.TrimSpace(strings.TrimPrefix(a.tokenFromRequest(c), "Bearer "))
		principal, err := a.Identity.AuthenticateAPIKey(c.Request.Context(), token)
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "API Key 校验失败", true)
			return
		}
		if principal == nil {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
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
	var body struct {
		Name             string   `json:"name"`
		Allowlist        []string `json:"allowlist"`
		RPMLimit         int      `json:"rpm_limit"`
		ConcurrencyLimit int      `json:"concurrency_limit"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Name == "" {
		body.Name = "default"
	}
	key, err := a.Identity.CreateAPIKey(c.Request.Context(), *a.currentPrincipal(c), body.Name, a.Config.EncryptionKey, body.Allowlist, body.RPMLimit, body.ConcurrencyLimit)
	if err != nil {
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
	items, err := a.Identity.ListAPIKeySummaries(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 Key 失败", true)
		return
	}
	if q := strings.ToLower(c.Query("q")); q != "" {
		filtered := make([]identity.APIKeyView, 0, len(items))
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.ID+item.Name+item.Prefix+item.Status+item.UserID), q) {
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
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "api-keys.csv", []string{"id", "name", "prefix", "status", "user_id"}, items, func(item identity.APIKeyView) []string {
			return []string{item.ID, item.Name, item.Prefix, item.Status, item.UserID}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.APIKeyView) string { return item.ID })
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
	if !a.requireConfirm(c) {
		return
	}
	a.disableAPIKey(c)
}

func (a *App) listModels(c *gin.Context) {
	caller := a.currentAPIKey(c)
	items, err := a.Catalog.ListVisibleModels(c.Request.Context(), caller.ChannelOrgID, caller.Allowlist)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取模型失败", true)
		return
	}
	httpx.OK(c, gin.H{"data": items, "object": "list", "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getModel(c *gin.Context) {
	caller := a.currentAPIKey(c)
	item, err := a.Catalog.GetVisibleModel(c.Request.Context(), caller.ChannelOrgID, c.Param("model"), caller.Allowlist)
	if err != nil {
		httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "模型不可用", false)
		return
	}
	httpx.OK(c, item)
}

func (a *App) chatCompletions(c *gin.Context) {
	a.executeProtocol(c, "openai.chat")
}

func (a *App) responses(c *gin.Context) {
	out := a.executeProtocol(c, "openai.responses")
	if out == nil {
		return
	}
	payload := gin.H{
		"id": out.Response.ID, "model": out.Response.Model, "usage": out.Response.Usage,
		"output": out.Response.Choices, "request_id": c.GetString(httpx.ContextRequestID), "provider": out.Response.Provider,
	}
	c.JSON(http.StatusOK, payload)
}

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
	out, err := a.Gateway.Execute(c.Request.Context(), gateway.ExecuteInput{
		Caller:     *a.currentAPIKey(c),
		RequestID:  c.GetString(httpx.ContextRequestID),
		Protocol:   protocol,
		Hint:       hint,
		ForceFail:  c.GetHeader("X-Tokenhub-Force-Fail"),
		OmitUsage:  c.GetHeader("X-Tokenhub-Omit-Usage") == "1",
		CanarySlug: a.Ops.CanarySlug(c.Request.Context(), c.GetHeader("X-Tokenhub-Canary") == "1"),
		Chat:       chat,
	})
	if err != nil {
		switch {
		case errors.Is(err, gateway.ErrUnsupportedParam):
			msg := "不支持的参数"
			var pe gateway.ParamError
			if errors.As(err, &pe) && pe.Param != "" {
				msg = "不支持的参数 " + pe.Param
			}
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", msg, false)
		case errors.Is(err, gateway.ErrModelNotAllowed):
			httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "模型未授权", false)
		case errors.Is(err, gateway.ErrInsufficientBalance):
			httpx.Abort(c, http.StatusPaymentRequired, "insufficient_balance", "余额不足", false)
		case errors.Is(err, gateway.ErrChannelDisabled), errors.Is(err, identity.ErrChannelDisabled):
			httpx.Abort(c, http.StatusForbidden, "channel_disabled", "渠道已停用，已冻结新消费", false)
		case errors.Is(err, ops.ErrRateLimited):
			httpx.Abort(c, http.StatusTooManyRequests, "rate_limited", "API Key 超过限额", false)
		default:
			httpx.Abort(c, http.StatusServiceUnavailable, "provider_unavailable", "没有可用提供商", true)
		}
		return nil
	}
	if chat.Stream && protocol == "openai.chat" {
		c.Header("Content-Type", "text/event-stream")
		for _, chunk := range out.Stream {
			_, _ = c.Writer.WriteString("data: " + chunk + "\n\n")
		}
		_, _ = c.Writer.WriteString("data: [DONE]\n\n")
		c.Writer.Flush()
		return out
	}
	if protocol == "openai.responses" {
		return out
	}
	if protocol == "anthropic.messages" {
		payload := gin.H{
			"id": out.Response.ID, "type": "message", "role": "assistant", "model": out.Response.Model,
			"content": anthropicContent(out.Response),
			"usage":   out.Response.Usage, "request_id": c.GetString(httpx.ContextRequestID), "provider": out.Response.Provider,
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

func firstContent(resp gateway.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
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

func (a *App) listAttempts(c *gin.Context) {
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
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "providers.csv", []string{"id", "slug", "adapter", "status", "health"}, items, func(item catalog.ProviderView) []string {
			return []string{item.ID, item.Slug, item.Adapter, item.Status, item.Health}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item catalog.ProviderView) string { return item.ID })
}

func (a *App) healthCheckProvider(c *gin.Context) {
	health, err := a.Catalog.Probe(c.Request.Context(), c.Param("id"))
	if err != nil {
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

func (a *App) rotateProviderCredential(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Secret string `json:"secret"`
	}
	_ = c.ShouldBindJSON(&body)
	if strings.TrimSpace(body.Secret) == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "凭据不能为空", false)
		return
	}
	ref, err := a.Catalog.RotateCredential(c.Request.Context(), c.Param("id"), body.Secret, a.Config.EncryptionKey)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "轮换凭据失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.credential.rotate", ResourceType: "provider",
		ResourceID: c.Param("id"), After: map[string]string{"credential_ref": ref},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"credential_ref": ref, "request_id": c.GetString(httpx.ContextRequestID)})
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
	if q := strings.ToLower(c.Query("q")); q != "" {
		filtered := make([]catalog.ModelView, 0, len(items))
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.ID+item.Vendor+item.DisplayName+item.Status+item.SyncState), q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "models.csv", []string{"id", "vendor", "display_name", "status"}, items, func(item catalog.ModelView) []string {
			return []string{item.ID, item.Vendor, item.DisplayName, item.Status}
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
	item, err := a.Catalog.CreateModel(c.Request.Context(), body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建模型失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.create", ResourceType: "model", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func catalogPublicID(c *gin.Context) string {
	return strings.TrimPrefix(c.Param("id"), "/")
}

func (a *App) getAdminModel(c *gin.Context) {
	item, err := a.Catalog.GetAdminModel(c.Request.Context(), catalogPublicID(c))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "模型不存在", false)
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
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "模型不存在", false)
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

func (a *App) createAdminRoute(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body catalog.RouteInput
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.CreateRoute(c.Request.Context(), body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建路由失败", false)
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
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "路由不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "route.patch", ResourceType: "route", ResourceID: item.ID,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) syncProvider(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	result, err := a.Catalog.SyncProvider(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "同步 Provider 失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "provider.sync", ResourceType: "provider",
		ResourceID: c.Param("id"), After: map[string]string{"models": strings.Join(syncIDs(result), ",")},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": result, "request_id": c.GetString(httpx.ContextRequestID)})
}

func syncIDs(result *catalog.SyncResult) []string {
	if result == nil {
		return nil
	}
	out := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		out = append(out, item.ID)
	}
	return out
}

func (a *App) reviewAdminModel(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		PublicID string `json:"public_id"`
		Action   string `json:"action"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Catalog.ReviewModel(c.Request.Context(), body.PublicID, body.Action)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "审核模型失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "model.review", ResourceType: "model",
		ResourceID: item.ID, After: map[string]string{"sync_state": item.SyncState, "action": body.Action},
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
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
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "发布模型失败", false)
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
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "弃用模型失败", false)
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
