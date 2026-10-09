package app

import (
	"errors"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) poolChannelID(c *gin.Context, channelID string) string {
	if channelID == "" {
		return ""
	}
	if pool, err := a.Identity.ResolvePoolChannelID(c.Request.Context(), channelID); err == nil && pool != "" {
		return pool
	}
	return channelID
}

func (a *App) viewerChannelID(c *gin.Context) string {
	p := a.currentPrincipal(c)
	if p == nil {
		return ""
	}
	if id := p.VisibleChannelID(); id != "" {
		return id
	}
	if q := c.Query("channel_id"); q != "" {
		return q
	}
	return ""
}

// actorBookChannelID 记在操作者自己的账上：平台/财务归自营，B/C 归本渠道（各有积分池）。
func (a *App) actorBookChannelID(c *gin.Context) string {
	p := a.currentPrincipal(c)
	if p == nil {
		return ""
	}
	id := strings.TrimSpace(p.ChannelOrgID)
	if id == "" {
		id = identity.OfficialChannelID
	}
	return a.poolChannelID(c, id)
}

func (a *App) composePnL(c *gin.Context, selected string) {
	owner, channels, ok := a.paymentSettlementScope(c)
	if !ok {
		return
	}
	if selected != "" {
		allowed := false
		for _, channel := range channels {
			if selected == channel {
				allowed = true
				break
			}
		}
		if !allowed {
			httpx.Abort(c, 403, "permission_denied", "该资金主体不在当前品牌范围。", false)
			return
		}
	}
	mkt, err := a.Commission.MarketingTotals(c.Request.Context(), owner)
	if a.abortSupplierError(c, err) {
		return
	}
	supplier, err := a.Billing.SupplierTotal(c.Request.Context(), owner)
	if a.abortSupplierError(c, err) {
		return
	}
	item, err := a.Billing.ChannelPnL(c.Request.Context(), owner, mkt.FrozenMinor, mkt.IssuedMinor, supplier, channels)
	if a.abortSupplierError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"pnl": item})
}
func (a *App) channelPnL(c *gin.Context)      { a.composePnL(c, c.Query("channel_id")) }
func (a *App) adminChannelPnL(c *gin.Context) { a.composePnL(c, c.Param("id")) }

func (a *App) adminListSupplier(c *gin.Context)   { a.listSupplierFacts(c) }
func (a *App) channelListSupplier(c *gin.Context) { a.listSupplierFacts(c) }
func (a *App) listSupplierFacts(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	if a.supplierOperation(c) {
		return
	}
	items, err := a.Billing.ListSupplier(c.Request.Context(), scope.OwnerID, 0)
	if a.abortSupplierError(c, err) {
		return
	}
	actorIDs := []string{}
	for _, item := range items {
		actorIDs = append(actorIDs, item.ActorUserID)
	}
	people, err := a.Identity.BillingRecipientsByID(c.Request.Context(), actorIDs)
	if a.abortSupplierError(c, err) {
		return
	}
	type view struct {
		billing.SupplierView
		ActorEmail string `json:"actor_email"`
		Status     string `json:"status"`
	}
	out := []view{}
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	for _, item := range items {
		status := "recorded"
		if item.ReversalOf != "" {
			status = "reversal"
		} else if item.ReversedBy != "" {
			status = "reversed"
		}
		row := view{item, people[item.ActorUserID].Email, status}
		if (c.Query("status") == "" || c.Query("status") == status) && commissionMatches(row, q) {
			out = append(out, row)
		}
	}
	commissionPage(c, out, func(v view) string { return v.ID }, func(v view) time.Time { return v.CreatedAt })
}
func (a *App) abortSupplierError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, billing.ErrNotFound):
		httpx.Abort(c, 404, "not_found", "支出记录不存在，请核对。", false)
	case errors.Is(err, billing.ErrConflict):
		httpx.Abort(c, 409, "supplier_conflict", "原操作内容发生变化、参考号已登记或原流水已冲正，请查询原记录。", false)
	case errors.Is(err, billing.ErrInvalidAmount), errors.Is(err, billing.ErrInventedCost):
		httpx.Abort(c, 400, "invalid_request", "请核对真实付款金额、对象、时间及类别。", false)
	default:
		if c.Request.Method == http.MethodGet {
			httpx.Abort(c, 500, "read_failed", "读取财务记录失败，请重试。", true)
		} else {
			httpx.Abort(c, 500, "result_unconfirmed", "结果尚未确认，请回查或重试原操作。", true)
		}
	}
	return true
}

func (a *App) supplierOperation(c *gin.Context) bool {
	operationID := strings.TrimSpace(c.Query("operation_id"))
	if operationID == "" {
		return false
	}
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusUnauthorized, "unauthorized", "请先登录", false)
		return true
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return true
	}
	item, err := a.Billing.SupplierOperation(c.Request.Context(), p.UserID, scope.OwnerID, operationID)
	if errors.Is(err, billing.ErrNotFound) {
		httpx.OK(c, gin.H{"item": nil, "operation_status": "not_found"})
		return true
	}
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "原操作查询失败", true)
		return true
	}
	httpx.OK(c, gin.H{"item": item, "operation_status": "recorded"})
	return true
}

func (a *App) adminRecordSupplier(c *gin.Context) {
	a.writeSupplier(c)
}

func (a *App) channelRecordSupplier(c *gin.Context) {
	a.writeSupplier(c)
}

func (a *App) writeSupplier(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	var body billing.SupplierInput
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, 400, "invalid_request", "请求体无效。", false)
		return
	}
	body.ChannelOrgID = scope.OwnerID
	if !body.Confirmed || body.OccurredAt.IsZero() || body.OccurredAt.After(time.Now().UTC().Add(5*time.Minute)) || strings.TrimSpace(body.VendorName) == "" || len(body.VendorName) > 200 || len(body.Memo) > 1000 || len(body.BankRef) > 200 || (body.Currency != "" && body.Currency != "USD") {
		httpx.Abort(c, 400, "invalid_request", "请确认已实际支付，并填写付款对象、USD金额和实际付款时间；参考号可选。", false)
		return
	}
	var item *billing.SupplierView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var created bool
		var e error
		item, created, e = a.Billing.RecordSupplierTx(tx, scope.ActorUserID, body)
		if e != nil || !created {
			return e
		}
		_, e = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: scope.ActorUserID, Action: "billing.supplier.record", ResourceType: "supplier_entry", ResourceID: item.ID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return e
	})
	if a.abortSupplierError(c, err) {
		return
	}
	httpx.Created(c, gin.H{"item": item})
}

func (a *App) adminReverseSupplier(c *gin.Context) {
	a.reverseSupplier(c, "")
}

func (a *App) channelReverseSupplier(c *gin.Context) {
	a.reverseSupplier(c, a.actorBookChannelID(c))
}

func (a *App) reverseSupplier(c *gin.Context, _ string) {
	if !a.requireConfirm(c) {
		return
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	var body struct {
		Reason      string `json:"reason"`
		OperationID string `json:"operation_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Reason) == "" || body.OperationID == "" {
		httpx.Abort(c, 400, "invalid_request", "请填写冲正原因并保留原操作编号。", false)
		return
	}
	var item *billing.SupplierView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var created bool
		var e error
		item, created, e = a.Billing.ReverseSupplierTx(tx, scope.ActorUserID, c.Param("id"), body.OperationID, body.Reason, scope.OwnerID)
		if e != nil || !created {
			return e
		}
		_, e = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: scope.ActorUserID, Action: "billing.supplier.reverse", ResourceType: "supplier_entry", ResourceID: item.ID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return e
	})
	if a.abortSupplierError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item})
}

func (a *App) channelPolicy(c *gin.Context) {
	channelID := a.viewerChannelID(c)
	if p := a.currentPrincipal(c); p != nil && channelID == "" {
		channelID = p.ChannelOrgID
	}
	if market, err := a.Identity.ResolveMarketChannelID(c.Request.Context(), channelID); err == nil {
		channelID = market
	}
	item, err := a.Commission.PolicyFor(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "佣金策略不存在", false)
		return
	}
	httpx.OK(c, gin.H{"policy": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelPatchPolicy(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	p := a.currentPrincipal(c)
	if p == nil || p.ChannelOrgID == "" {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
		return
	}
	ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
	if err != nil || ch.Type != identity.ChannelTypeC {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 OEM 可改分佣比例", false)
		return
	}
	var body commission.PolicyView
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "佣金策略字段无效", false)
		return
	}
	before, _ := a.Commission.PolicyFor(c.Request.Context(), ch.ID)
	item, err := a.Commission.UpdateChannelPolicy(c.Request.Context(), ch.ID, body)
	if err != nil {
		if errors.Is(err, commission.ErrConflict) {
			httpx.Abort(c, http.StatusConflict, "version_conflict", "策略已被更新，请重新读取并核对差异", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "佣金策略不合法：直接+间接不能超过总佣金", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: p.UserID, Action: "commission.policy.update",
		ResourceType: "commission_policy", ResourceID: item.ID,
		Before: before, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"policy": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) grantSignupGift(c *gin.Context, userID string) {
	if a.Billing == nil || a.Commission == nil || userID == "" {
		return
	}
	ctx := c.Request.Context()
	attr, err := a.Identity.GetAttribution(ctx, userID)
	if err != nil || attr.AcquisitionRoleID == "" {
		return
	}
	if a.Identity.RoleAllowsCommission(ctx, attr.AcquisitionRoleID) {
		return
	}
	elig, err := a.Identity.EffectiveEligibility(ctx, attr.ChannelOrgID)
	if err != nil || elig.GiftMinor <= 0 {
		return
	}
	pool := a.poolChannelID(c, attr.ChannelOrgID)
	_ = a.Billing.GrantGift(ctx, userID, "gift:register:"+userID, elig.GiftMinor)
	_ = a.Commission.RecordSignupCredit(ctx, pool, userID, elig.GiftMinor)
}
