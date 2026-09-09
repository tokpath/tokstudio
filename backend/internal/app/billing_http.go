package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerBillingRoutes(r *gin.Engine) {
	r.GET("/v1/me/balance", a.requireUserOrKey(), a.getBalance)
	r.GET("/v1/balance", a.requireUserOrKey(), a.getBalance)
	r.GET("/v1/me/ledger", a.requireUserOrKey(), a.getLedger)
	r.GET("/v1/me/usage", a.requireUserOrKey(), a.getUsage)
	r.GET("/v1/usage", a.requireUserOrKey(), a.getUsage)
	r.GET("/v1/me/reconciliation", a.requireUserOrKey(), a.getMyReconciliation)
	r.POST("/v1/me/reconciliation/flag", a.requireUserOrKey(), a.flagMyReconciliation)
	r.POST("/v1/topups", a.requireUserOrKey(), a.createTopup)
	r.GET("/v1/topups/:id", a.requireUserOrKey(), a.getTopup)
	r.POST("/v1/topups/redeem", a.requireUserOrKey(), a.redeemTopup)
	r.POST("/v1/topups/:id/refund", a.requireRoles("platform_admin", "finance_admin"), a.refundTopup)

	r.POST("/admin/topups/:id/confirm", a.requireRoles("platform_admin", "finance_admin"), a.confirmTopup)
	r.POST("/admin/refunds", a.requireRoles("platform_admin", "finance_admin"), a.adminRefund)
	r.GET("/admin/ledger", a.requireRoles("platform_admin", "finance_admin", "audit_readonly"), a.adminLedger)
	r.GET("/admin/usage", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminUsage)
	r.GET("/admin/usage/pending", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminPendingUsage)
	r.GET("/admin/usage/pending/:id", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminPendingUsageDetail)
	r.POST("/admin/usage/pending/resolve", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.resolvePendingUsage)
	r.GET("/admin/billing/report", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.billingReport)
	r.GET("/admin/billing/export", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.billingExport)
	r.POST("/admin/commissions/recalc", a.requireRoles("platform_admin", "finance_admin"), a.recalcCommission)
	r.GET("/admin/price-books", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListPrices)
	r.POST("/admin/price-books", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.publishPrice)
	r.POST("/admin/usage/replay", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.replayUsage)
	r.GET("/channel/reconciliation", a.requireRoles("channel_admin", "platform_admin", "finance_admin", "ops_admin"), a.getChannelReconciliation)
	r.POST("/channel/reconciliation/flag", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.flagChannelReconciliation)
}

func (a *App) requireUserOrKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := a.Identity.Authenticate(c.Request.Context(), a.tokenFromRequest(c))
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "身份校验失败", true)
			return
		}
		if principal != nil {
			if !a.Identity.Allow(principal, c.Request.URL.Path, c.Request.Method) {
				httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
				return
			}
			c.Set("principal", principal)
			c.Next()
			return
		}
		token := a.tokenFromRequest(c)
		key, err := a.Identity.AuthenticateAPIKey(c.Request.Context(), trimBearer(token))
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "API Key 校验失败", true)
			return
		}
		if key == nil {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
			return
		}
		if !a.Identity.Allow(&key.Principal, c.Request.URL.Path, c.Request.Method) {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
			return
		}
		c.Set("principal", &key.Principal)
		c.Set("api_key", key)
		c.Next()
	}
}

func trimBearer(token string) string {
	const p = "Bearer "
	if len(token) > len(p) && token[:len(p)] == p {
		return token[len(p):]
	}
	return token
}

func (a *App) billingUser(c *gin.Context) (userID, channelID string) {
	if p := a.currentPrincipal(c); p != nil {
		return p.UserID, p.ChannelOrgID
	}
	if k := a.currentAPIKey(c); k != nil {
		return k.UserID, k.ChannelOrgID
	}
	return "", ""
}

func (a *App) getBalance(c *gin.Context) {
	userID, channelID := a.billingUser(c)
	view, err := a.Billing.Balance(c.Request.Context(), userID, channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取余额失败", true)
		return
	}
	httpx.OK(c, gin.H{"balance": view, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getLedger(c *gin.Context) {
	userID, _ := a.billingUser(c)
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := a.Billing.ListLedger(c.Request.Context(), userID, limit)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取流水失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getMyReconciliation(c *gin.Context) {
	userID, channelID := a.billingUser(c)
	view, err := a.Billing.ReconcileWindow(c.Request.Context(), billing.ReconcileInput{
		UserID:        userID,
		ChannelOrgID:  channelID,
		Scope:         billing.ScopeUser,
		APIKeyID:      strings.TrimSpace(c.Query("api_key_id")),
		PublicModelID: strings.TrimSpace(c.Query("public_model_id")),
		Since:         parseQueryTime(c.Query("from")),
		Until:         parseQueryTime(c.Query("to")),
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取对账失败", true)
		return
	}
	httpx.OK(c, gin.H{"item": view, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) flagMyReconciliation(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	userID, _ := a.billingUser(c)
	item, err := a.Billing.FlagPending(c.Request.Context(), billing.FlagPendingInput{
		Key:    flagKey(c),
		UserID: userID,
	})
	a.respondFlagPending(c, item, err)
}

func (a *App) getChannelReconciliation(c *gin.Context) {
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = strings.TrimSpace(c.Query("channel_id"))
	}
	view, err := a.Billing.ReconcileWindow(c.Request.Context(), billing.ReconcileInput{
		ChannelOrgID:  channelID,
		Scope:         billing.ScopeChannel,
		APIKeyID:      strings.TrimSpace(c.Query("api_key_id")),
		PublicModelID: strings.TrimSpace(c.Query("public_model_id")),
		Since:         parseQueryTime(c.Query("from")),
		Until:         parseQueryTime(c.Query("to")),
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道对账失败", true)
		return
	}
	httpx.OK(c, gin.H{"item": view, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) flagChannelReconciliation(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = strings.TrimSpace(c.Query("channel_id"))
	}
	item, err := a.Billing.FlagPending(c.Request.Context(), billing.FlagPendingInput{
		Key:          flagKey(c),
		ChannelOrgID: channelID,
	})
	a.respondFlagPending(c, item, err)
}

func flagKey(c *gin.Context) string {
	var body struct {
		ID        string `json:"id"`
		RequestID string `json:"request_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if strings.TrimSpace(body.ID) != "" {
		return strings.TrimSpace(body.ID)
	}
	return strings.TrimSpace(body.RequestID)
}

func (a *App) respondFlagPending(c *gin.Context, item *billing.UsageGapView, err error) {
	if err != nil {
		if errors.Is(err, billing.ErrAlreadyMatched) {
			httpx.Abort(c, http.StatusConflict, "idempotency_conflict", "匹配行无需送入待对账", false)
			return
		}
		if errors.Is(err, billing.ErrNotFound) {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "对账行不存在", false)
			return
		}
		if errors.Is(err, billing.ErrInvalidAmount) {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 id 或 request_id", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "送入待对账失败", false)
		return
	}
	resourceID := item.ID
	if resourceID == "" {
		resourceID = item.RequestID
	}
	if p := a.currentPrincipal(c); p != nil {
		_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
			ActorUserID: p.UserID, Action: "billing.usage.flag_pending", ResourceType: "usage_event",
			ResourceID: resourceID, After: item,
			IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
		})
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getUsage(c *gin.Context) {
	userID, _ := a.billingUser(c)
	in := billing.QueryUsageInput{
		UserID:        userID,
		APIKeyID:      strings.TrimSpace(c.Query("api_key_id")),
		PublicModelID: strings.TrimSpace(c.Query("public_model_id")),
		State:         strings.TrimSpace(c.Query("state")),
		Since:         parseQueryTime(c.Query("from")),
		Until:         parseQueryTime(c.Query("to")),
	}
	if a.currentPrincipal(c) != nil && a.currentPrincipal(c).HasRole("platform_admin", "finance_admin", "ops_admin", "audit_readonly") && c.Query("all") == "1" {
		in.UserID = ""
		in.ChannelOrgID = strings.TrimSpace(c.Query("channel_id"))
	}
	if k := a.currentAPIKey(c); k != nil {
		in.UserID = k.UserID
		in.APIKeyID = k.APIKeyID
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	in.Limit = limit
	items, err := a.Billing.QueryUsage(c.Request.Context(), in)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 usage 失败", true)
		return
	}
	keys, _ := a.Billing.DimMoneyScoped(c.Request.Context(), "api_key", in.UserID, in.ChannelOrgID)
	models, _ := a.Billing.DimMoneyScoped(c.Request.Context(), "model", in.UserID, in.ChannelOrgID)
	httpx.OK(c, gin.H{
		"items": items, "keys": keys, "models": models,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) createTopup(c *gin.Context) {
	var body struct {
		AmountMinor int64  `json:"amount_minor"`
		Method      string `json:"payment_method"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.AmountMinor <= 0 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "充值金额无效", false)
		return
	}
	userID, channelID := a.billingUser(c)
	item, err := a.Billing.CreateTopup(c.Request.Context(), userID, channelID, body.AmountMinor, body.Method)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建充值失败", false)
		return
	}
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getTopup(c *gin.Context) {
	userID, _ := a.billingUser(c)
	if a.currentPrincipal(c) != nil && a.currentPrincipal(c).HasRole("platform_admin", "finance_admin") {
		userID = ""
	}
	item, err := a.Billing.GetTopup(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "充值订单不存在", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) redeemTopup(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	_ = c.ShouldBindJSON(&body)
	userID, channelID := a.billingUser(c)
	item, err := a.Billing.Redeem(c.Request.Context(), userID, channelID, body.Code)
	if err != nil {
		if errors.Is(err, billing.ErrInsufficientQuota) {
			httpx.Abort(c, http.StatusPaymentRequired, "insufficient_quota", "渠道可用额度不足，无法发放服务额度", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "兑换码无效或已用尽", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: userID, Action: "billing.redeem", ResourceType: "topup", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) confirmTopup(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	principal := a.currentPrincipal(c)
	item, err := a.Billing.ConfirmTopup(c.Request.Context(), c.Param("id"), principal.UserID)
	if err != nil {
		if errors.Is(err, billing.ErrInsufficientQuota) {
			httpx.Abort(c, http.StatusPaymentRequired, "insufficient_quota", "渠道可用额度不足，无法发放服务额度", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "确认入账失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: principal.UserID, Action: "billing.topup.confirm", ResourceType: "topup", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) refundTopup(c *gin.Context) {
	item, err := a.Billing.RefundTopup(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "充值退款失败", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminRefund(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		RequestID string `json:"request_id"`
		TopupID   string `json:"topup_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.TopupID != "" {
		item, err := a.Billing.RefundTopup(c.Request.Context(), body.TopupID)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "充值退款失败", false)
			return
		}
		_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
			ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.topup.refund", ResourceType: "topup", ResourceID: item.ID,
			After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
		})
		httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
		return
	}
	if body.RequestID == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 request_id 或 topup_id", false)
		return
	}
	item, err := a.Billing.RefundCharge(c.Request.Context(), body.RequestID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "账单退款失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.charge.refund", ResourceType: "customer_charge", ResourceID: item.ChargeID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminLedger(c *gin.Context) {
	userID := c.Query("user_id")
	if userID == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 user_id", false)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := a.Billing.ListLedger(c.Request.Context(), userID, limit)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取流水失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminUsage(c *gin.Context) {
	limit, _ := httpx.Page(c, 50)
	items, err := a.Billing.QueryUsage(c.Request.Context(), billing.QueryUsageInput{
		UserID:        c.Query("user_id"),
		APIKeyID:      c.Query("api_key_id"),
		ChannelOrgID:  c.Query("channel_id"),
		PublicModelID: c.Query("public_model_id"),
		State:         strings.TrimSpace(c.Query("state")),
		Since:         parseQueryTime(c.Query("from")),
		Until:         parseQueryTime(c.Query("to")),
		Limit:         limit,
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 usage 失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "usage.csv",
			[]string{"id", "request_id", "user_id", "api_key_id", "public_model_id", "provider_id", "prompt_tokens", "completion_tokens", "reasoning_tokens", "customer_amount_minor", "wholesale_amount_minor", "state", "occurred_at"},
			items, func(item billing.UsageView) []string {
				return []string{
					item.ID, item.RequestID, item.UserID, item.APIKeyID, item.PublicModelID, item.ProviderID,
					strconv.FormatInt(item.PromptTokens, 10), strconv.FormatInt(item.CompletionTokens, 10),
					strconv.FormatInt(item.ReasoningTokens, 10), strconv.FormatInt(item.CustomerMinor, 10),
					strconv.FormatInt(item.WholesaleMinor, 10), item.State, item.OccurredAt.UTC().Format("2006-01-02T15:04:05Z"),
				}
			})
		return
	}
	httpx.OK(c, gin.H{"items": items, "limit": limit, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) billingExport(c *gin.Context) {
	report, err := a.Billing.Report(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "导出失败", true)
		return
	}
	c.Header("Content-Type", "text/csv")
	c.String(http.StatusOK, "metric,amount_minor\nrevenue,%d\nupstream_cost,%d\ncommission,%d\ngross_profit,%d\npending_reconciliation,%d\n",
		report.RevenueMinor, report.UpstreamMinor, report.CommissionMinor, report.GrossProfitMinor, report.PendingCount)
}

func (a *App) billingReport(c *gin.Context) {
	report, err := a.Billing.Report(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取报表失败", true)
		return
	}
	httpx.OK(c, gin.H{"report": report, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) recalcCommission(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		UsageEventID string `json:"usage_event_id"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Billing.RecalcCommission(c.Request.Context(), body.UsageEventID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "佣金重算失败", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) replayUsage(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body billing.SettleInput
	if err := c.ShouldBindJSON(&body); err != nil || body.RequestID == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 request_id", false)
		return
	}
	item, err := a.Billing.Settle(c.Request.Context(), body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "usage 回放失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.usage.replay", ResourceType: "usage_event",
		ResourceID: firstNonEmpty(item.UsageEventID, body.RequestID), After: item,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPendingUsage(c *gin.Context) {
	limit, _ := httpx.Page(c, 50)
	items, err := a.Billing.ListPendingReconciliation(c.Request.Context(), billing.QueryUsageInput{
		UserID:        c.Query("user_id"),
		APIKeyID:      c.Query("api_key_id"),
		ChannelOrgID:  c.Query("channel_id"),
		PublicModelID: c.Query("public_model_id"),
		State:         strings.TrimSpace(c.Query("status")),
		Since:         parseQueryTime(c.Query("from")),
		Until:         parseQueryTime(c.Query("to")),
		Limit:         limit,
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取待对账失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "limit": limit, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPendingUsageDetail(c *gin.Context) {
	item, err := a.Billing.GetUsageGap(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, billing.ErrNotFound) {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "待对账记录不存在", false)
			return
		}
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取用量缺口失败", true)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) resolvePendingUsage(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body billing.ResolvePendingInput
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 ids 或 request_ids", false)
		return
	}
	item, err := a.Billing.ResolvePending(c.Request.Context(), body)
	if err != nil {
		if errors.Is(err, billing.ErrAlreadyCharged) {
			httpx.Abort(c, http.StatusConflict, "idempotency_conflict", "已结算账单不能标记已解，禁止估算扣款", false)
			return
		}
		if errors.Is(err, billing.ErrNotFound) {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "待对账记录不存在", false)
			return
		}
		if errors.Is(err, billing.ErrInvalidAmount) {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 ids 或 request_ids", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "标记已解失败", false)
		return
	}
	resourceID := ""
	if len(item.Items) > 0 {
		resourceID = firstNonEmpty(item.Items[0].ID, item.Items[0].RequestID)
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.usage.resolve", ResourceType: "usage_event",
		ResourceID: resourceID, After: item,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func parseQueryTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func (a *App) adminListPrices(c *gin.Context) {
	items, err := a.Catalog.ListPriceBooks(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取价格失败", true)
		return
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		filtered := items[:0]
		for _, item := range items {
			if strings.Contains(item.PublicID, q) || strings.Contains(item.Status, q) || strings.Contains(item.ID, q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "price-books.csv",
			[]string{"id", "public_id", "status", "effective_at", "upstream", "wholesale", "sell", "channel"},
			items, func(item catalog.PriceBookView) []string {
				return []string{
					item.ID, item.PublicID, item.Status,
					item.EffectiveAt.UTC().Format("2006-01-02T15:04:05Z"),
					item.Upstream, item.Wholesale, item.Sell, item.Channel,
				}
			})
		return
	}
	httpx.OKPage(c, items, 100, func(item catalog.PriceBookView) string { return item.ID })
}

func (a *App) publishPrice(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "价格无效", false)
		return
	}
	model, _ := body["model"].(string)
	if model == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 model", false)
		return
	}
	delete(body, "model")
	snap, err := a.Catalog.PublishPrice(c.Request.Context(), model, body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "发布价格失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "catalog.price.publish", ResourceType: "price_version",
		ResourceID: snap.VersionID, After: snap, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"price": snap, "request_id": c.GetString(httpx.ContextRequestID)})
}
