package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/page"
)

func (a *App) registerUsageWorkflowRoutes(r *gin.Engine) {
	admin := []string{"platform_admin", "finance_admin", "ops_admin", "audit_readonly"}
	channel := []string{"channel_admin", "oem_ops", "oem_finance", "oem_audit", "platform_admin", "finance_admin", "ops_admin", "audit_readonly"}
	r.GET("/v1/me/usage/summary", a.requireUserOrKey(), func(c *gin.Context) { a.usageSummary(c, "user") })
	r.GET("/admin/usage/summary", a.requireRoles(admin...), func(c *gin.Context) { a.usageSummary(c, "admin") })
	r.GET("/channel/usage/summary", a.requireRoles(channel...), func(c *gin.Context) { a.usageSummary(c, "channel") })
	r.POST("/admin/requests/:id/reconcile", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.reconcileRequestUsage)
	r.GET("/admin/requests", a.requireRoles(admin...), func(c *gin.Context) { a.workflowRequests(c, "admin") })
	r.GET("/channel/requests", a.requireRoles(channel...), func(c *gin.Context) { a.workflowRequests(c, "channel") })
	r.GET("/v1/me/requests/:id", a.requireUserOrKey(), func(c *gin.Context) { a.workflowRequestDetail(c, "user") })
	r.GET("/admin/requests/:id", a.requireRoles(admin...), func(c *gin.Context) { a.workflowRequestDetail(c, "admin") })
	r.GET("/channel/requests/:id", a.requireRoles(channel...), func(c *gin.Context) { a.workflowRequestDetail(c, "channel") })
}

func (a *App) usageWorkflowInput(c *gin.Context, surface string) (billing.QueryUsageInput, string, bool) {
	var in billing.QueryUsageInput
	zone := strings.TrimSpace(c.Query("time_zone"))
	if zone == "" {
		zone = "UTC"
	}
	location, err := time.LoadLocation(zone)
	if err != nil {
		httpx.Abort(c, 400, "invalid_request", "无效时区", false)
		return in, zone, false
	}
	since, until, err := queryWindowInLocation(c.Query("from"), c.Query("to"), location)
	if err != nil {
		httpx.Abort(c, 400, "invalid_request", "时间范围无效", false)
		return in, zone, false
	}
	in = billing.QueryUsageInput{UserID: strings.TrimSpace(c.Query("user_id")), APIKeyID: strings.TrimSpace(c.Query("api_key_id")), PublicModelID: strings.TrimSpace(c.Query("public_model_id")), State: strings.TrimSpace(c.Query("state")), Since: since, Until: until, Cursor: c.Query("cursor"), Query: strings.TrimSpace(c.Query("q")), RequestID: strings.TrimSpace(c.Query("request_id"))}
	if in.State == "" {
		in.State = strings.TrimSpace(c.Query("billing_state"))
	}
	if in.State != "" && !billing.KnownUsageState(in.State) {
		httpx.Abort(c, 400, "invalid_request", "账务状态无效", false)
		return in, zone, false
	}
	if surface == "user" {
		in.UserID, _ = a.billingUser(c)
		if k := a.currentAPIKey(c); k != nil {
			in.APIKeyID = k.APIKeyID
		}
	} else if surface == "channel" {
		p := a.currentPrincipal(c)
		if p.IsChannelStaff() {
			ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
			if err != nil {
				httpx.Abort(c, 500, "internal_error", "读取管理范围失败", true)
				return in, zone, false
			}
			if ch.Type == identity.ChannelTypeC {
				ids, ok := a.oemScope(c)
				if !ok {
					return in, zone, false
				}
				in.ChannelOrgIDs = ids
			} else {
				if selected := c.Query("channel_id"); selected != "" && selected != ch.ID {
					httpx.Abort(c, 403, "permission_denied", "无权查看该渠道", false)
					return in, zone, false
				}
				in.ChannelOrgIDs = []string{ch.ID}
			}
		} else {
			selected := c.Query("channel_id")
			if selected == "" {
				selected = identity.OfficialChannelID
			}
			// Finance/operations use the platform's own funds. Only the platform
			// administrator can explicitly inspect a different OEM workspace.
			if !p.IsPlatformAdmin() && selected != identity.OfficialChannelID {
				httpx.Abort(c, 403, "permission_denied", "无权查看该 OEM 范围", false)
				return in, zone, false
			}
			in.ChannelOrgIDs = []string{selected}
		}
	} else {
		in.ChannelOrgID = strings.TrimSpace(c.Query("channel_id"))
	}
	in.Limit = 50
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 && n <= 100 {
		in.Limit = n
	}
	if in.Cursor != "" {
		if _, _, err := page.Decode(in.Cursor); err != nil {
			httpx.Abort(c, 400, "invalid_request", "分页位置无效", false)
			return in, zone, false
		}
	}
	return in, zone, true
}

func queryWindowInLocation(from, to string, loc *time.Location) (time.Time, time.Time, error) {
	bound := func(raw string, end bool) (time.Time, error) {
		if len(raw) == 10 {
			t, err := time.ParseInLocation("2006-01-02", raw, loc)
			if err == nil {
				if end {
					t = t.AddDate(0, 0, 1)
				}
				return t.UTC(), nil
			}
		}
		return parseQueryBound(raw, end)
	}
	since, err := bound(strings.TrimSpace(from), false)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	until, err := bound(strings.TrimSpace(to), true)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		return time.Time{}, time.Time{}, errInvalidQueryRange
	}
	return since, until, nil
}
func (a *App) usageSummary(c *gin.Context, surface string) {
	in, zone, ok := a.usageWorkflowInput(c, surface)
	if !ok {
		return
	}
	result, err := a.Billing.UsageSummary(c.Request.Context(), in, zone)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取用量汇总失败，请重试", true)
		return
	}
	httpx.OK(c, result)
}
func (a *App) workflowRequests(c *gin.Context, surface string) {
	in, _, ok := a.usageWorkflowInput(c, surface)
	if !ok {
		return
	}
	query := gateway.QueryRequestsInput{UserID: in.UserID, APIKeyID: in.APIKeyID, ChannelOrgIDs: in.ChannelOrgIDs, PublicModelID: in.PublicModelID, Since: in.Since, Until: in.Until, Cursor: in.Cursor, Query: in.Query, Limit: in.Limit + 1, BillingState: in.State, Status: strings.TrimSpace(c.Query("result"))}
	if in.ChannelOrgID != "" {
		query.ChannelOrgIDs = []string{in.ChannelOrgID}
	}
	if in.RequestID != "" {
		query.RequestIDs = []string{in.RequestID}
	}
	if query.Status != "" && !gateway.KnownRequestResult(query.Status) {
		httpx.Abort(c, 400, "invalid_request", "请求结果无效", false)
		return
	}
	items, err := a.Gateway.ListScopedRequests(c.Request.Context(), query)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取请求失败，请重试", true)
		return
	}
	next := ""
	if len(items) > in.Limit {
		items = items[:in.Limit]
		next = gateway.RequestCursor(items[len(items)-1])
	}
	receipts, err := a.attachRequestBilling(c.Request.Context(), "", "", items)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取账务事实失败，请重试", true)
		return
	}
	httpx.OK(c, gin.H{"items": receipts, "next_cursor": next})
}
func (a *App) workflowRequestDetail(c *gin.Context, surface string) {
	in, _, ok := a.usageWorkflowInput(c, surface)
	if !ok {
		return
	}
	q := gateway.QueryRequestsInput{UserID: in.UserID, APIKeyID: in.APIKeyID, ChannelOrgIDs: in.ChannelOrgIDs, RequestIDs: []string{c.Param("id")}, Limit: 1}
	if in.ChannelOrgID != "" {
		q.ChannelOrgIDs = []string{in.ChannelOrgID}
	}
	items, err := a.Gateway.ListScopedRequests(c.Request.Context(), q)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取请求失败，请重试", true)
		return
	}
	if len(items) != 1 {
		httpx.Abort(c, 404, "not_found", "请求不存在或无权读取", false)
		return
	}
	facts, err := a.Billing.RequestFacts(c.Request.Context(), items[0].RequestID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取账务事实失败，请重试", true)
		return
	}
	attempts, err := a.Gateway.ListAttempts(c.Request.Context(), items[0].RequestID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取执行记录失败，请重试", true)
		return
	}
	safeAttempts := make([]gin.H, 0, len(attempts))
	for _, v := range attempts {
		safeAttempts = append(safeAttempts, gin.H{"id": v.ID, "attempt_no": v.AttemptNo, "status": v.Status, "http_status": v.HTTPStatus, "error_code": v.ErrorCode, "latency_ms": v.LatencyMS, "prompt_tokens": v.PromptTokens, "completion_tokens": v.CompletionTokens})
	}
	receipts, err := a.attachRequestBilling(c.Request.Context(), "", "", items)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取账务事实失败", true)
		return
	}
	projection := gin.H{"request": receipts[0], "attempts": safeAttempts, "authorization": facts.Authorization, "usage": publicUsageItems(facts.Usages), "charges": facts.Charges}
	if surface != "user" {
		projection["customer_id"] = items[0].UserID
		projection["commission_status"] = "unavailable"
		if service, ok := any(a.Commission).(interface {
			RequestFacts(context.Context, string) ([]commission.EntryView, error)
		}); ok {
			entries, err := service.RequestFacts(c.Request.Context(), items[0].RequestID)
			if err != nil {
				projection["commission_status"] = "read_error"
			} else {
				projection["commission_status"] = "ready"
				projection["commissions"] = entries
			}
		}
		if p := a.currentPrincipal(c); surface == "admin" && p.HasRole("platform_admin", "ops_admin", "audit_readonly") {
			projection["diagnostic_attempts"] = attempts
			projection["diagnostic_usage"] = facts.Usages
		}
	}
	httpx.OK(c, projection)
}

// Preserve complete CSV scope; pagination applies only to interactive lists.
func (a *App) workflowUsage(c *gin.Context, surface string) {
	in, _, ok := a.usageWorkflowInput(c, surface)
	if !ok {
		return
	}
	in.Unlimited = httpx.WantCSV(c)
	if !in.Unlimited {
		in.Limit++
	}
	items, err := a.Billing.QueryUsage(c.Request.Context(), in)
	if err != nil {
		code, status := "internal_error", 500
		if errors.Is(err, page.ErrCursor) {
			code, status = "invalid_request", 400
		}
		httpx.Abort(c, status, code, "读取用量失败，请重试", true)
		return
	}
	limit := in.Limit - 1
	next := ""
	if !in.Unlimited && len(items) > limit {
		items = items[:limit]
		next = billing.UsageCursor(items[len(items)-1])
	}
	if in.Unlimited {
		httpx.WriteCSV(c, "usage.csv", []string{"request_id", "api_key_id", "public_model_id", "amount_usd_micro", "state", "occurred_at"}, items, func(v billing.UsageView) []string {
			return []string{v.RequestID, v.APIKeyID, v.PublicModelID, strconv.FormatInt(v.CustomerMinor, 10), v.State, v.OccurredAt.UTC().Format(time.RFC3339Nano)}
		})
		return
	}
	if surface == "admin" {
		httpx.OK(c, gin.H{"items": items, "next_cursor": next})
	} else {
		httpx.OK(c, gin.H{"items": publicUsageItems(items), "next_cursor": next})
	}
}

func (a *App) reconcileRequestUsage(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Usage map[string]int `json:"usage"`
	}
	if c.ShouldBindJSON(&body) != nil {
		httpx.Abort(c, 400, "invalid_request", "需要本次真实用量", false)
		return
	}
	item, err := a.Billing.ReplayActualUsage(c.Request.Context(), c.Param("id"), body.Usage)
	if err != nil {
		code, status := "invalid_request", 400
		if errors.Is(err, billing.ErrConflict) {
			code, status = "idempotency_conflict", 409
		}
		httpx.Abort(c, status, code, "原请求核对未完成，请检查真实用量与当前状态", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.usage.reconcile", ResourceType: "usage_event", ResourceID: item.UsageEventID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.OK(c, gin.H{"item": item})
}
