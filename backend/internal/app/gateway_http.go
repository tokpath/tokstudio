package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerGatewayRoutes(r *gin.Engine) {
	r.POST("/v1/me/api-keys", a.requireAnyUser(), a.createAPIKey)
	r.GET("/v1/me/api-keys", a.requireAnyUser(), a.listAPIKeys)
	r.GET("/v1/models", a.requireAPIKey(), a.listModels)
	r.GET("/v1/models/:model", a.requireAPIKey(), a.getModel)
	r.POST("/v1/chat/completions", a.requireAPIKey(), a.chatCompletions)
	r.POST("/v1/responses", a.requireAPIKey(), a.responses)
	r.POST("/v1/messages", a.requireAPIKey(), a.messages)
	r.GET("/v1/requests/:id/attempts", a.requireAPIKey(), a.listAttempts)
	r.GET("/admin/providers", a.requireRoles("platform_admin", "tech_admin"), a.listProviders)
	r.POST("/admin/providers/:id/health-check", a.requireRoles("platform_admin", "tech_admin"), a.healthCheckProvider)
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
		c.Next()
	}
}

func (a *App) createAPIKey(c *gin.Context) {
	var body struct {
		Name      string   `json:"name"`
		Allowlist []string `json:"allowlist"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Name == "" {
		body.Name = "default"
	}
	key, err := a.Identity.CreateAPIKey(c.Request.Context(), *a.currentPrincipal(c), body.Name, a.Config.EncryptionKey, body.Allowlist)
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
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
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
	c.JSON(http.StatusOK, gin.H{
		"id": out.Response.ID, "model": out.Response.Model, "usage": out.Response.Usage,
		"output": out.Response.Choices, "request_id": c.GetString(httpx.ContextRequestID), "provider": out.Response.Provider,
	})
}

func (a *App) messages(c *gin.Context) {
	a.executeProtocol(c, "anthropic.messages")
}

func (a *App) executeProtocol(c *gin.Context, protocol string) *gateway.ExecuteOutput {
	var raw map[string]any
	if err := c.ShouldBindJSON(&raw); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return nil
	}
	body, _ := json.Marshal(raw)
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
		Caller:    *a.currentAPIKey(c),
		RequestID: c.GetString(httpx.ContextRequestID),
		Protocol:  protocol,
		Hint:      hint,
		ForceFail: c.GetHeader("X-Tokenhub-Force-Fail"),
		OmitUsage: c.GetHeader("X-Tokenhub-Omit-Usage") == "1",
		Chat:      chat,
	})
	if err != nil {
		switch {
		case errors.Is(err, gateway.ErrUnsupportedParam):
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "不支持的参数 logit_bias", false)
		case errors.Is(err, gateway.ErrModelNotAllowed):
			httpx.Abort(c, http.StatusForbidden, "model_not_allowed", "模型未授权", false)
		case errors.Is(err, gateway.ErrInsufficientBalance):
			httpx.Abort(c, http.StatusPaymentRequired, "insufficient_balance", "余额不足", false)
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
		c.JSON(http.StatusOK, gin.H{
			"id": out.Response.ID, "type": "message", "role": "assistant", "model": out.Response.Model,
			"content": []gin.H{{"type": "text", "text": firstContent(out.Response)}},
			"usage":   out.Response.Usage, "request_id": c.GetString(httpx.ContextRequestID), "provider": out.Response.Provider,
		})
		return out
	}
	c.JSON(http.StatusOK, out.Response)
	return out
}

func firstContent(resp gateway.ChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return resp.Choices[0].Message.Content
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
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) healthCheckProvider(c *gin.Context) {
	health, err := a.Catalog.Probe(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "Provider 不存在", false)
		return
	}
	httpx.OK(c, gin.H{"health": health, "request_id": c.GetString(httpx.ContextRequestID)})
}
