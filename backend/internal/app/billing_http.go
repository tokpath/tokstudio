package app

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
	r.POST("/v1/topups", a.requireUserOrKey(), a.createTopup)
	r.GET("/v1/topups/:id", a.requireUserOrKey(), a.getTopup)
	r.POST("/v1/topups/redeem", a.requireUserOrKey(), a.redeemTopup)
	r.POST("/v1/topups/:id/refund", a.requireRoles("platform_admin", "finance_admin"), a.refundTopup)

	r.POST("/admin/topups/:id/confirm", a.requireRoles("platform_admin", "finance_admin"), a.confirmTopup)
	r.POST("/admin/refunds", a.requireRoles("platform_admin", "finance_admin"), a.adminRefund)
	r.GET("/admin/ledger", a.requireRoles("platform_admin", "finance_admin", "audit_readonly"), a.adminLedger)
	r.GET("/admin/usage", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminUsage)
	r.GET("/admin/billing/report", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.billingReport)
	r.GET("/admin/billing/export", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.billingExport)
	r.POST("/admin/commissions/recalc", a.requireRoles("platform_admin", "finance_admin"), a.recalcCommission)
	r.GET("/admin/price-books", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListPrices)
	r.POST("/admin/price-books", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.publishPrice)
	r.POST("/admin/usage/replay", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.replayUsage)
}

func (a *App) requireUserOrKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := a.Identity.Authenticate(c.Request.Context(), a.tokenFromRequest(c))
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "身份校验失败", true)
			return
		}
		if principal != nil {
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

func (a *App) getUsage(c *gin.Context) {
	userID, _ := a.billingUser(c)
	if a.currentPrincipal(c) != nil && a.currentPrincipal(c).HasRole("platform_admin", "finance_admin", "ops_admin", "audit_readonly") && c.Query("all") == "1" {
		userID = ""
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := a.Billing.ListUsage(c.Request.Context(), userID, limit)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 usage 失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
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
	items, err := a.Billing.ListUsage(c.Request.Context(), c.Query("user_id"), limit)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 usage 失败", true)
		return
	}
	if c.Query("format") == "csv" {
		var b strings.Builder
		b.WriteString("id,request_id,state,customer_amount_minor,public_model_id\n")
		for _, item := range items {
			b.WriteString(fmt.Sprintf("%s,%s,%s,%d,%s\n", item.ID, item.RequestID, item.State, item.CustomerMinor, item.PublicModelID))
		}
		c.Header("Content-Type", "text/csv")
		c.String(http.StatusOK, b.String())
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
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
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
		httpx.WriteCSV(c, "price-books.csv", []string{"id", "public_id", "status"}, items, func(item catalog.PriceBookView) []string {
			return []string{item.ID, item.PublicID, item.Status}
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
	httpx.OK(c, gin.H{"price": snap, "request_id": c.GetString(httpx.ContextRequestID)})
}
