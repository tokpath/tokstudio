package app

import (
	"errors"
	"net/http"
	"strings"

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

func (a *App) composePnL(c *gin.Context, channelID string) {
	pool := a.poolChannelID(c, channelID)
	if pool == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "缺少渠道", false)
		return
	}
	mkt, err := a.Commission.MarketingTotals(c.Request.Context(), pool)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取营销汇总失败", true)
		return
	}
	supplier, err := a.Billing.SupplierTotal(c.Request.Context(), pool)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取供应商汇总失败", true)
		return
	}
	item, err := a.Billing.ChannelPnL(c.Request.Context(), pool, mkt.FrozenMinor, mkt.IssuedMinor, supplier)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "渠道不存在", false)
		return
	}
	httpx.OK(c, gin.H{"pnl": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelPnL(c *gin.Context) {
	a.composePnL(c, a.viewerChannelID(c))
}

func (a *App) adminChannelPnL(c *gin.Context) {
	a.composePnL(c, c.Param("id"))
}

func (a *App) adminListSupplier(c *gin.Context) {
	items, err := a.Billing.ListSupplier(c.Request.Context(), c.Query("channel_id"), 50)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取供应商支出失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelListSupplier(c *gin.Context) {
	channelID := a.actorBookChannelID(c)
	items, err := a.Billing.ListSupplier(c.Request.Context(), channelID, 50)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取供应商支出失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
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
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
		return
	}
	var body billing.SupplierInput
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	body.ChannelOrgID = a.actorBookChannelID(c)
	if body.ChannelOrgID == "" || body.SourceType == "" || body.IdempotencyKey == "" || body.AmountMinor <= 0 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "必填：amount_minor、source_type、idempotency_key", false)
		return
	}
	item, err := a.Billing.RecordSupplier(c.Request.Context(), p.UserID, body)
	if err != nil {
		if errors.Is(err, billing.ErrInventedCost) {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "禁止估算 attempt 成本", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "记供应商支出失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: p.UserID, Action: "billing.supplier.record", ResourceType: "supplier_entry", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminReverseSupplier(c *gin.Context) {
	a.reverseSupplier(c, "")
}

func (a *App) channelReverseSupplier(c *gin.Context) {
	a.reverseSupplier(c, a.actorBookChannelID(c))
}

func (a *App) reverseSupplier(c *gin.Context, scopedChannel string) {
	if !a.requireConfirm(c) {
		return
	}
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	if scopedChannel != "" {
		orig, err := a.Billing.GetSupplier(c.Request.Context(), c.Param("id"))
		if err != nil {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "流水不存在", false)
			return
		}
		if orig.ChannelOrgID != scopedChannel {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能冲正本渠道流水", false)
			return
		}
	}
	item, err := a.Billing.ReverseSupplier(c.Request.Context(), p.UserID, c.Param("id"), body.Reason)
	if err != nil {
		if errors.Is(err, billing.ErrNotFound) {
			httpx.Abort(c, http.StatusNotFound, "invalid_request", "流水不存在", false)
			return
		}
		if errors.Is(err, billing.ErrConflict) {
			httpx.Abort(c, http.StatusConflict, "conflict", "冲正对象不合法", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "冲正失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: p.UserID, Action: "billing.supplier.reverse", ResourceType: "supplier_entry", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
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
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 C 渠道可改分佣比例", false)
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
