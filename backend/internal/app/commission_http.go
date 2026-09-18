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
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerCommissionRoutes(r *gin.Engine) {
	r.GET("/v1/partner/me", a.requireAnyUser(), a.partnerMe)
	r.GET("/v1/partner/users", a.requireAnyUser(), a.partnerUsers)
	r.GET("/v1/partner/commissions", a.requireAnyUser(), a.partnerCommissions)
	r.GET("/v1/partner/settlements", a.requireAnyUser(), a.partnerSettlements)
	r.GET("/v1/partner/export", a.requireAnyUser(), a.partnerExport)
	r.GET("/channel/quota", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelQuota)
	r.GET("/channel/allocations", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelAllocations)
	r.GET("/channel/commissions", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelCommissions)
	r.GET("/channel/usage", a.requireRoles("channel_admin", "platform_admin", "finance_admin", "ops_admin"), a.channelUsage)
	r.GET("/channel/settlements", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelSettlements)
	r.GET("/admin/settlements", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListSettlements)
	r.GET("/channel/promotion-codes", a.requireRoles("channel_admin", "platform_admin"), a.channelListPromos)
	r.POST("/channel/promotion-codes", a.requireRoles("channel_admin", "platform_admin"), a.adminCreatePromo)
	r.POST("/admin/acquisition-roles", a.requireRoles("platform_admin", "channel_admin"), a.adminCreateRole)
	r.GET("/admin/acquisition-roles", a.requireRoles("platform_admin", "channel_admin"), a.adminListRoles)
	r.GET("/admin/acquisition-roles/:id", a.requireRoles("platform_admin", "channel_admin"), a.adminGetRole)
	r.PATCH("/admin/acquisition-roles/:id", a.requireRoles("platform_admin", "channel_admin"), a.adminPatchRole)
	r.GET("/admin/promotion-codes", a.requireRoles("platform_admin", "channel_admin", "ops_admin", "audit_readonly"), a.adminListPromos)
	r.POST("/admin/promotion-codes", a.requireRoles("platform_admin", "channel_admin"), a.adminCreatePromo)
	r.POST("/admin/channel-quotas/grant", a.requireRoles("platform_admin", "finance_admin"), a.adminGrantQuota)
	r.POST("/channel/quotas/grant", a.requireRoles("channel_admin"), a.channelGrantQuota)
	r.GET("/admin/channel-quotas/:channel_id/issue-rule", a.requireRoles("platform_admin", "finance_admin", "channel_admin"), a.adminGetIssueRule)
	r.PATCH("/admin/channel-quotas/:channel_id/issue-rule", a.requireRoles("platform_admin", "finance_admin"), a.adminPatchIssueRule)
	r.GET("/admin/channel-quotas/:channel_id", a.requireRoles("platform_admin", "finance_admin", "channel_admin"), a.adminGetQuota)
	r.GET("/admin/commissions", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminCommissions)
	r.POST("/admin/commissions/unfreeze", a.requireRoles("platform_admin", "finance_admin"), a.adminUnfreeze)
	r.POST("/admin/commissions/settle", a.requireRoles("platform_admin", "finance_admin"), a.adminSettle)
	r.POST("/admin/settlements/:id/payout", a.requireRoles("platform_admin", "finance_admin"), a.adminPayout)
	r.GET("/admin/commission-policy", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminPolicy)
	r.PATCH("/admin/commission-policy", a.requireRoles("platform_admin", "finance_admin"), a.adminPatchPolicy)
	r.GET("/admin/eligibility-rules", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminEligibility)
	r.PATCH("/admin/eligibility-rules", a.requireRoles("platform_admin", "finance_admin"), a.adminPatchEligibility)
	r.GET("/channel/eligibility-rules", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelEligibility)
	r.PATCH("/channel/eligibility-rules", a.requireRoles("channel_admin"), a.channelPatchEligibility)
	r.GET("/channel/commission-policy", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelPolicy)
	r.PATCH("/channel/commission-policy", a.requireRoles("channel_admin"), a.channelPatchPolicy)
	r.GET("/channel/supplier-entries", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelListSupplier)
	r.POST("/channel/supplier-entries", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelRecordSupplier)
	r.POST("/channel/supplier-entries/:id/reverse", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelReverseSupplier)
	r.GET("/channel/pnl", a.requireRoles("channel_admin", "platform_admin", "finance_admin", "ops_admin"), a.channelPnL)
	r.GET("/admin/supplier-entries", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListSupplier)
	r.POST("/admin/supplier-entries", a.requireRoles("platform_admin", "finance_admin"), a.adminRecordSupplier)
	r.POST("/admin/supplier-entries/:id/reverse", a.requireRoles("platform_admin", "finance_admin"), a.adminReverseSupplier)
	r.GET("/admin/channels/:id/pnl", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminChannelPnL)
}

func (a *App) partnerScope(c *gin.Context) (channelID string, roleIDs []string, ok bool) {
	p := a.currentPrincipal(c)
	if p == nil {
		return "", nil, false
	}
	if p.IsPlatformAdmin() || p.HasRole("finance_admin", "ops_admin") {
		return "", nil, true
	}
	if p.HasRole("channel_admin") {
		return p.ChannelOrgID, nil, true
	}
	mem, err := a.Identity.MemberRole(c.Request.Context(), p.UserID)
	if err != nil {
		return "", nil, false
	}
	ids, err := a.Identity.RoleIDsInScope(c.Request.Context(), mem.ID)
	if err != nil {
		return "", nil, false
	}
	return mem.ChannelOrgID, ids, true
}

func (a *App) partnerMe(c *gin.Context) {
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
		return
	}
	if p.IsPlatformAdmin() || p.HasRole("finance_admin", "ops_admin") {
		httpx.OK(c, gin.H{
			"role_type": "platform", "channel_org_id": "", "scope_role_ids": []string{},
			"sees_downline": true, "request_id": c.GetString(httpx.ContextRequestID),
		})
		return
	}
	if p.HasRole("channel_admin") {
		httpx.OK(c, gin.H{
			"role_type": "channel_admin", "channel_org_id": p.ChannelOrgID, "scope_role_ids": []string{},
			"sees_downline": true, "request_id": c.GetString(httpx.ContextRequestID),
		})
		return
	}
	mem, err := a.Identity.MemberRole(c.Request.Context(), p.UserID)
	if err != nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不是推广主体", false)
		return
	}
	ids, err := a.Identity.RoleIDsInScope(c.Request.Context(), mem.ID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取推广范围失败", true)
		return
	}
	httpx.OK(c, gin.H{
		"role_id": mem.ID, "role_type": mem.Type, "parent_role_id": mem.ParentID,
		"channel_org_id": mem.ChannelOrgID, "scope_role_ids": ids,
		"sees_downline": mem.Type != identity.AcqKOL2,
		"request_id":    c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) partnerUsers(c *gin.Context) {
	p := a.currentPrincipal(c)
	if _, _, ok := a.partnerScope(c); !ok {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不是推广主体", false)
		return
	}
	mask := !p.IsPlatformAdmin()
	items, err := a.Identity.ListScopedUsers(c.Request.Context(), *p, mask)
	if err != nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "无权查看用户", false)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) partnerCommissions(c *gin.Context) {
	channelID, roleIDs, ok := a.partnerScope(c)
	if !ok {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不是推广主体", false)
		return
	}
	items, err := a.Commission.ListEntries(c.Request.Context(), channelID, roleIDs, c.Query("usage_event_id"))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取佣金失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) partnerSettlements(c *gin.Context) {
	channelID, roleIDs, ok := a.partnerScope(c)
	if !ok {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不是推广主体", false)
		return
	}
	items, err := a.Commission.ListSettlements(c.Request.Context(), channelID, roleIDs)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取结算单失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) partnerExport(c *gin.Context) {
	channelID, roleIDs, ok := a.partnerScope(c)
	if !ok {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不是推广主体", false)
		return
	}
	items, err := a.Commission.ListEntries(c.Request.Context(), channelID, roleIDs, "")
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "导出失败", true)
		return
	}
	var b strings.Builder
	b.WriteString("id,kind,amount_minor,status,usage_event_id,beneficiary_role_id\n")
	for _, item := range items {
		b.WriteString(item.ID + "," + item.Kind + "," + strconv.FormatInt(item.AmountMinor, 10) + "," + item.Status + "," + item.UsageEventID + "," + item.BeneficiaryRoleID + "\n")
	}
	c.Header("Content-Type", "text/csv")
	c.String(http.StatusOK, b.String())
}

func (a *App) channelQuota(c *gin.Context) {
	channelID := a.currentPrincipal(c).ChannelOrgID
	if a.currentPrincipal(c).IsPlatformAdmin() && c.Query("channel_id") != "" {
		channelID = c.Query("channel_id")
	}
	item, err := a.Billing.ChannelQuota(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "渠道额度不存在", false)
		return
	}
	httpx.OK(c, gin.H{"quota": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelAllocations(c *gin.Context) {
	channelID := a.currentPrincipal(c).ChannelOrgID
	if a.currentPrincipal(c).IsPlatformAdmin() && c.Query("channel_id") != "" {
		channelID = c.Query("channel_id")
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	items, err := a.Billing.ListAllocations(c.Request.Context(), channelID, limit)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取额度发放失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelUsage(c *gin.Context) {
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = c.Query("channel_id")
	}
	item, err := a.Billing.ChannelUsage(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道用量失败", true)
		return
	}
	keys, _ := a.Billing.DimMoneyScoped(c.Request.Context(), "api_key", "", channelID)
	models, _ := a.Billing.DimMoneyScoped(c.Request.Context(), "model", "", channelID)
	items, _ := a.Billing.QueryUsage(c.Request.Context(), billing.QueryUsageInput{
		ChannelOrgID:  channelID,
		APIKeyID:      c.Query("api_key_id"),
		PublicModelID: c.Query("public_model_id"),
		Limit:         50,
	})
	httpx.OK(c, gin.H{
		"usage": item, "keys": keys, "models": models, "items": items,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) channelSettlements(c *gin.Context) {
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = c.Query("channel_id")
	}
	items, err := a.Commission.ListSettlements(c.Request.Context(), channelID, nil)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取结算单失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminListSettlements(c *gin.Context) {
	items, err := a.Commission.ListSettlements(c.Request.Context(), c.Query("channel_id"), nil)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取结算单失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "settlements.csv", []string{"id", "channel_org_id", "status", "amount_minor"}, items, func(item commission.SettlementView) []string {
			return []string{item.ID, item.ChannelOrgID, item.Status, strconv.FormatInt(item.AmountMinor, 10)}
		})
		return
	}
	roleIDs, channelIDs := []string{}, []string{}
	for _, item := range items {
		roleIDs = append(roleIDs, item.BeneficiaryRoleID)
		channelIDs = append(channelIDs, item.ChannelOrgID)
	}
	recipients, err := a.Identity.BillingRecipientsByRole(c.Request.Context(), roleIDs)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取收款人失败，请重试。", true)
		return
	}
	channels, err := a.Identity.BillingChannelCodes(c.Request.Context(), channelIDs)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取渠道失败，请重试。", true)
		return
	}
	type adminSettlement struct {
		commission.SettlementView
		Recipient   identity.BillingRecipient `json:"recipient"`
		ChannelCode string                    `json:"channel_code"`
	}
	out := make([]adminSettlement, 0, len(items))
	for _, item := range items {
		out = append(out, adminSettlement{item, recipients[item.BeneficiaryRoleID], channels[item.ChannelOrgID]})
	}
	httpx.OKPage(c, out, 100, func(item adminSettlement) string { return item.ID })
}

func (a *App) channelListPromos(c *gin.Context) {
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = c.Query("channel_id")
	}
	items, err := a.Identity.ListPromotionCodes(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取推广码失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminListPromos(c *gin.Context) {
	channelID := c.Query("channel_id")
	if p := a.currentPrincipal(c); p.HasRole("channel_admin") && !p.IsPlatformAdmin() {
		channelID = p.ChannelOrgID
	}
	items, err := a.Identity.ListPromotionCodes(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取推广码失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "promotion-codes.csv", []string{"id", "code", "channel_org_id", "status"}, items, func(item identity.PromotionView) []string {
			return []string{item.ID, item.Code, item.ChannelOrgID, item.Status}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.PromotionView) string { return item.ID })
}

func (a *App) channelCommissions(c *gin.Context) {
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = c.Query("channel_id")
	}
	items, err := a.Commission.ListEntries(c.Request.Context(), channelID, nil, "")
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取佣金失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminCreateRole(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		ChannelOrgID string `json:"channel_org_id"`
		Type         string `json:"type"`
		ParentID     string `json:"parent_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if p := a.currentPrincipal(c); p.HasRole("channel_admin") && !p.IsPlatformAdmin() {
		body.ChannelOrgID = p.ChannelOrgID
	}
	item, err := a.Identity.CreateAcquisitionRole(c.Request.Context(), body.ChannelOrgID, body.Type, body.ParentID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "无法创建推广角色", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "identity.acquisition.create", ResourceType: "acquisition_role", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminListRoles(c *gin.Context) {
	channelID := c.Query("channel_id")
	if p := a.currentPrincipal(c); p.HasRole("channel_admin") && !p.IsPlatformAdmin() {
		channelID = p.ChannelOrgID
	}
	items, err := a.Identity.ListAcquisitionRoles(c.Request.Context(), channelID, c.Query("type"))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取角色失败", true)
		return
	}
	if q := strings.ToLower(c.Query("q")); q != "" {
		filtered := make([]identity.AcquisitionRoleView, 0, len(items))
		for _, item := range items {
			hay := strings.ToLower(item.ID + item.Type + item.ChannelOrgID + item.Status + item.ParentID)
			if strings.Contains(hay, q) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "roles.csv", []string{"id", "channel_org_id", "type", "level", "status", "parent_id"}, items, func(item identity.AcquisitionRoleView) []string {
			return []string{item.ID, item.ChannelOrgID, item.Type, strconv.Itoa(item.Level), item.Status, item.ParentID}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.AcquisitionRoleView) string { return item.ID })
}

func (a *App) adminGetRole(c *gin.Context) {
	item, err := a.Identity.GetAcquisitionRole(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchRole(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Identity.PatchAcquisitionRole(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), body.Status)
	if err != nil {
		a.writeAuthError(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "identity.acquisition.patch", ResourceType: "acquisition_role", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminCreatePromo(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		ChannelOrgID      string `json:"channel_org_id"`
		AcquisitionRoleID string `json:"acquisition_role_id"`
		Code              string `json:"code"`
	}
	_ = c.ShouldBindJSON(&body)
	if p := a.currentPrincipal(c); p.HasRole("channel_admin") && !p.IsPlatformAdmin() {
		body.ChannelOrgID = p.ChannelOrgID
	}
	item, err := a.Identity.CreatePromotionCode(c.Request.Context(), body.ChannelOrgID, body.AcquisitionRoleID, body.Code)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "无法创建推广码", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "identity.promotion.create", ResourceType: "promotion_code", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminGrantQuota(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		ChannelOrgID string `json:"channel_org_id"`
		AmountMinor  int64  `json:"amount_minor"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ChannelOrgID == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "额度参数无效", false)
		return
	}
	item, err := a.Billing.GrantChannelQuota(c.Request.Context(), body.ChannelOrgID, body.AmountMinor, a.currentPrincipal(c).UserID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "调整额度失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.quota.grant", ResourceType: "quota", ResourceID: body.ChannelOrgID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"quota": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelGrantQuota(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	p := a.currentPrincipal(c)
	if p == nil || p.ChannelOrgID == "" {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
		return
	}
	parent, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
	if err != nil || parent.Type != identity.ChannelTypeC {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 C 渠道可向下属 B 划拨积分", false)
		return
	}
	var body struct {
		ChannelOrgID string `json:"channel_org_id"`
		AmountMinor  int64  `json:"amount_minor"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.ChannelOrgID == "" || body.AmountMinor <= 0 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "额度参数无效", false)
		return
	}
	child, err := a.Identity.GetChannel(c.Request.Context(), *p, body.ChannelOrgID)
	if err != nil || child.Type != identity.ChannelTypeB || child.ParentID != parent.ID {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能划给本 C 下的 B", false)
		return
	}
	item, err := a.Billing.TransferChannelQuota(c.Request.Context(), parent.ID, child.ID, body.AmountMinor, p.UserID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "划拨失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: p.UserID, Action: "billing.quota.wholesale", ResourceType: "quota", ResourceID: child.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"quota": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminGetQuota(c *gin.Context) {
	item, err := a.Billing.ChannelQuota(c.Request.Context(), c.Param("channel_id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "渠道额度不存在", false)
		return
	}
	httpx.OK(c, gin.H{"quota": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) canReadChannelQuota(c *gin.Context, channelID string) bool {
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
		return false
	}
	if p.HasRole("channel_admin") && !p.IsPlatformAdmin() && !p.HasRole("finance_admin") && p.ChannelOrgID != channelID {
		ch, err := a.Identity.GetChannel(c.Request.Context(), *p, channelID)
		if err != nil || ch.ParentID != p.ChannelOrgID {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能查看本渠道额度", false)
			return false
		}
	}
	return true
}

func (a *App) adminGetIssueRule(c *gin.Context) {
	channelID := c.Param("channel_id")
	if !a.canReadChannelQuota(c, channelID) {
		return
	}
	item, err := a.Billing.IssueRule(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "换算规则不存在", false)
		return
	}
	httpx.OK(c, gin.H{"rule": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchIssueRule(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	channelID := c.Param("channel_id")
	var body struct {
		IssueRatioBPS int64 `json:"issue_ratio_bps"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "换算比字段无效", false)
		return
	}
	before, _ := a.Billing.IssueRule(c.Request.Context(), channelID)
	item, err := a.Billing.SetIssueRule(c.Request.Context(), channelID, body.IssueRatioBPS)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "换算比必须在 1000–100000 BPS（0.1x–10x）", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.quota.issue_rule",
		ResourceType: "quota_issue_rule", ResourceID: channelID,
		Before: before, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"rule": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminCommissions(c *gin.Context) {
	items, err := a.Commission.ListEntries(c.Request.Context(), c.Query("channel_id"), nil, c.Query("usage_event_id"))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取佣金失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "commissions.csv", []string{"id", "kind", "status", "amount_minor", "channel_org_id"}, items, func(item commission.EntryView) []string {
			return []string{item.ID, item.Kind, item.Status, strconv.FormatInt(item.AmountMinor, 10), item.ChannelOrgID}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item commission.EntryView) string { return item.ID })
}

func (a *App) adminUnfreeze(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		UsageEventID string `json:"usage_event_id"`
		Now          bool   `json:"now"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "解冻参数无效。", false)
		return
	}
	if body.Now {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "只能解冻已到期佣金，不能跳过冻结期。", false)
		return
	}
	n, err := a.Commission.UnfreezeUsage(c.Request.Context(), time.Now().UTC(), strings.TrimSpace(body.UsageEventID))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "解冻失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "commission.unfreeze", ResourceType: "commission_batch", ResourceID: firstNonEmpty(body.UsageEventID, "due"),
		After: map[string]any{"unfrozen": n, "usage_event_id": body.UsageEventID},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"unfrozen": n, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminSettle(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	items, err := a.Commission.CreateMonthlySettlement(c.Request.Context(), time.Now().UTC(), c.Query("ignore_minimum") == "1")
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "生成结算单失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "commission.settle", ResourceType: "settlement_batch", ResourceID: "monthly",
		After: items, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPayout(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Method    string `json:"method"`
		Reference string `json:"reference"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请填写有效的打款凭证。", false)
		return
	}
	if body.Method == "" {
		body.Method = "manual"
	}
	item, err := a.Commission.Payout(c.Request.Context(), c.Param("id"), body.Method, body.Reference, a.currentPrincipal(c).UserID)
	if err != nil {
		switch {
		case errors.Is(err, commission.ErrWalletMismatch):
			httpx.Abort(c, http.StatusConflict, "commission_wallet_mismatch", "佣金钱包入账或余额与结算单不一致，未登记打款。请联系财务核对原佣金流水后再操作。", false)
		case errors.Is(err, commission.ErrInvalid):
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请填写真实的线下打款凭证（1–200 字节）；仅支持人工登记。", false)
		case errors.Is(err, commission.ErrConflict):
			httpx.Abort(c, http.StatusConflict, "payout_conflict", "该结算单已登记其他凭证，请刷新核对，不要重复登记。", false)
		case errors.Is(err, commission.ErrSettlementChanged):
			httpx.Abort(c, http.StatusConflict, "settlement_changed", "结算单已因佣金变更失效，请刷新并重新生成结算单。", false)
		case errors.Is(err, commission.ErrNotFound):
			httpx.Abort(c, http.StatusNotFound, "not_found", "结算单不存在，请刷新列表。", false)
		default:
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "尚未确认登记结果，请用原凭证重试，不会重复登记。", true)
		}
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "commission.payout", ResourceType: "settlement", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPolicy(c *gin.Context) {
	item, err := a.Commission.ActivePolicy(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "佣金策略不存在", false)
		return
	}
	httpx.OK(c, gin.H{"policy": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchPolicy(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body commission.PolicyView
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "佣金策略字段无效", false)
		return
	}
	before, _ := a.Commission.ActivePolicy(c.Request.Context())
	item, err := a.Commission.UpdatePolicy(c.Request.Context(), body)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "佣金策略不合法：各档 BPS 之和不能超过上限", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "commission.policy.update",
		ResourceType: "commission_policy", ResourceID: item.ID,
		Before: before, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"policy": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminEligibility(c *gin.Context) {
	item, err := a.Identity.PlatformEligibility(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "达线规则不存在", false)
		return
	}
	httpx.OK(c, gin.H{"rule": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchEligibility(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body identity.EligibilityView
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "达线规则字段无效", false)
		return
	}
	before, _ := a.Identity.PlatformEligibility(c.Request.Context())
	item, err := a.Identity.UpdatePlatformEligibility(c.Request.Context(), body.SpendMinor, body.TopupMinor, body.GiftMinor)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "达线规则不合法", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "eligibility.rule.update",
		ResourceType: "eligibility_rule", ResourceID: item.ID,
		Before: before, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"rule": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelEligibility(c *gin.Context) {
	p := a.currentPrincipal(c)
	channelID := ""
	if p != nil {
		channelID = p.ChannelOrgID
	}
	item, err := a.Identity.EffectiveEligibility(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "达线规则不存在", false)
		return
	}
	httpx.OK(c, gin.H{"rule": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelPatchEligibility(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
		return
	}
	var body identity.EligibilityView
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "达线规则字段无效", false)
		return
	}
	item, err := a.Identity.UpdateChannelEligibility(c.Request.Context(), *p, body.SpendMinor, body.TopupMinor, body.GiftMinor)
	if err != nil {
		if errors.Is(err, identity.ErrChannelImmutable) {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 C 渠道可改达线规则", false)
			return
		}
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "达线规则不合法", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: p.UserID, Action: "eligibility.rule.update",
		ResourceType: "eligibility_rule", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"rule": item, "request_id": c.GetString(httpx.ContextRequestID)})
}
