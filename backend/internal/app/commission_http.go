package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerCommissionRoutes(r *gin.Engine) {
	a.registerCommissionWorkflowRoutes(r)
	r.GET("/admin/commission-recoveries", a.requireRoles("platform_admin", "finance_admin", "audit_readonly"), a.adminCommissionRecoveries)
	r.POST("/admin/commission-recoveries/:id/receipts", a.requireRoles("platform_admin", "finance_admin"), a.adminRecordCommissionRecovery)
	r.GET("/v1/me/commission-recoveries", a.requireAnyUser(), a.myCommissionRecoveries)
	r.GET("/v1/partner/me", a.requireAnyUser(), a.partnerMe)
	r.GET("/v1/partner/users", a.requireAnyUser(), a.partnerUsers)
	r.GET("/v1/partner/commissions", a.requireAnyUser(), a.partnerCommissions)
	r.GET("/v1/partner/settlements", a.requireAnyUser(), a.partnerSettlements)
	r.GET("/v1/partner/export", a.requireAnyUser(), a.partnerExport)
	r.GET("/channel/settlements/manage", a.requireRoles("channel_admin"), a.adminListSettlements)
	r.POST("/channel/commissions/settle", a.requireRoles("channel_admin"), a.adminSettle)
	r.POST("/channel/commissions/unfreeze", a.requireRoles("channel_admin"), a.adminUnfreeze)
	r.POST("/channel/settlements/:id/payout", a.requireRoles("channel_admin"), a.adminPayout)
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
	r.GET("/admin/channel-quotas/:channel_id/operations", a.requireRoles("platform_admin", "finance_admin"), a.adminQuotaOperation)
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
	if p.IsChannelStaff() {
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

// Financial results in the account belong to its own memberships. Customer
// visibility retains the existing professional/management scope separately.
func (a *App) partnerIncomeScope(c *gin.Context) (string, []string, bool) {
	p := a.currentPrincipal(c)
	if p == nil {
		return "", nil, false
	}
	if p.IsPlatformAdmin() || p.HasRole("finance_admin", "ops_admin") || p.IsChannelStaff() {
		return a.partnerScope(c)
	}
	own, err := a.Identity.PersonalReferral(c.Request.Context(), p.UserID)
	if err != nil || len(own.RoleIDs) == 0 {
		return "", nil, false
	}
	return "", own.RoleIDs, true
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
	if p.IsChannelStaff() {
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
	if !p.IsPlatformAdmin() && !p.HasRole("finance_admin", "ops_admin") && !p.IsChannelStaff() {
		mem, err := a.Identity.MemberRole(c.Request.Context(), p.UserID)
		if err != nil || mem.Status != "active" || mem.Type == identity.AcqPromoter {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "此账户没有专业推广客户查看权限", false)
			return
		}
	}
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
	channelID, roleIDs, ok := a.partnerIncomeScope(c)
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
	channelID, roleIDs, ok := a.partnerIncomeScope(c)
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
	channelID, roleIDs, ok := a.partnerIncomeScope(c)
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
	if errors.Is(err, billing.ErrNotFound) {
		httpx.Abort(c, 404, "not_found", "OEM 服务额度池尚未建立。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "读取额度失败。", true)
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

func (a *App) channelUsage(c *gin.Context) { a.workflowUsage(c, "channel") }

func (a *App) channelSettlements(c *gin.Context) {
	if p := a.currentPrincipal(c); p.IsChannelStaff() {
		ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		if err == nil && ch.Type == identity.ChannelTypeC {
			a.adminListSettlements(c)
			return
		}
	}
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		a.adminListSettlements(c)
		return
	}
	if selected := c.Query("channel_id"); selected != "" && selected != channelID {
		httpx.Abort(c, 403, "permission_denied", "只能查看本渠道结算", false)
		return
	}
	own, err := a.Identity.PersonalReferral(c.Request.Context(), a.currentPrincipal(c).UserID)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	items, err := a.Commission.ListSettlements(c.Request.Context(), channelID, own.RoleIDs)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取结算单失败", true)
		return
	}
	views, err := a.settlementAdminViews(c, items)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	out := []commissionSettlementAdminView{}
	for _, item := range views {
		if (c.Query("status") == "" || item.Status == c.Query("status")) && commissionMatches(item, strings.ToLower(strings.TrimSpace(c.Query("q")))) {
			out = append(out, item)
		}
	}
	commissionPage(c, out, func(item commissionSettlementAdminView) string { return item.ID }, func(item commissionSettlementAdminView) time.Time { return item.CreatedAt })
}

func (a *App) adminListSettlements(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	items, err := a.Commission.ListBrandSettlements(c.Request.Context(), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	views, err := a.settlementAdminViews(c, items)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	status, query := strings.TrimSpace(c.Query("status")), strings.ToLower(strings.TrimSpace(c.Query("q")))
	out := []commissionSettlementAdminView{}
	for _, item := range views {
		if (status == "" || item.Status == status) && commissionMatches(item, query) {
			out = append(out, item)
		}
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "settlements.csv", []string{"id", "channel_org_id", "status", "amount_minor"}, out, func(item commissionSettlementAdminView) []string {
			return []string{item.ID, item.ChannelOrgID, item.Status, strconv.FormatInt(item.AmountMinor, 10)}
		})
		return
	}
	commissionPage(c, out, func(item commissionSettlementAdminView) string { return item.ID }, func(item commissionSettlementAdminView) time.Time { return item.CreatedAt })
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
	if p := a.currentPrincipal(c); p.IsChannelStaff() && !p.IsPlatformAdmin() {
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
	if p := a.currentPrincipal(c); p.IsChannelStaff() {
		ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		if err == nil && ch.Type == identity.ChannelTypeC {
			a.adminCommissions(c)
			return
		}
	}
	if channelID == "" {
		a.adminCommissions(c)
		return
	}
	if selected := c.Query("channel_id"); selected != "" && selected != channelID {
		httpx.Abort(c, 403, "permission_denied", "只能查看本渠道佣金", false)
		return
	}
	own, err := a.Identity.PersonalReferral(c.Request.Context(), a.currentPrincipal(c).UserID)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	items, err := a.Commission.ListEntries(c.Request.Context(), channelID, own.RoleIDs, "")
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取佣金失败", true)
		return
	}
	out := []commission.EntryView{}
	for _, item := range items {
		if (c.Query("status") == "" || item.Status == c.Query("status")) && commissionMatches(item, strings.ToLower(strings.TrimSpace(c.Query("q")))) {
			out = append(out, item)
		}
	}
	commissionPage(c, out, func(item commission.EntryView) string { return item.ID }, func(item commission.EntryView) time.Time { return item.CreatedAt })
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
	if p := a.currentPrincipal(c); p.IsChannelStaff() && !p.IsPlatformAdmin() {
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
	if p := a.currentPrincipal(c); p.IsChannelStaff() && !p.IsPlatformAdmin() {
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
	if p := a.currentPrincipal(c); p.IsChannelStaff() && !p.IsPlatformAdmin() {
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
		OperationID  string `json:"operation_id"`
	}
	if c.ShouldBindJSON(&body) != nil || body.ChannelOrgID == "" || body.OperationID == "" || body.AmountMinor == 0 {
		httpx.Abort(c, 400, "invalid_request", "请填写组织、有效金额和原操作编号。", false)
		return
	}
	ownerID, err := a.Identity.ResolvePaymentOwnerID(c.Request.Context(), body.ChannelOrgID)
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "组织读取失败，未调整额度。", true)
		return
	}
	if ownerID != body.ChannelOrgID || ownerID == identity.OfficialChannelID {
		httpx.Abort(c, 403, "permission_denied", "仅向 OEM 服务额度池划拨，渠道客户由品牌方直接管理", false)
		return
	}
	operation, err := a.Billing.AdjustQuota(c.Request.Context(), a.currentPrincipal(c).UserID, body.ChannelOrgID, body.OperationID, body.AmountMinor)
	if errors.Is(err, billing.ErrConflict) {
		httpx.Abort(c, 409, "operation_conflict", "该操作编号已绑定其他内容，请核对原记录。", false)
		return
	}
	if errors.Is(err, billing.ErrInsufficientQuota) || errors.Is(err, billing.ErrInvalidAmount) {
		httpx.Abort(c, 400, "invalid_request", "金额无效或回收金额超过可用额度，未修改额度。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "result_unconfirmed", "调整结果待确认，请重试同一操作。", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.quota.grant", ResourceType: "quota_operation", ResourceID: operation.ID, After: operation, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.OK(c, gin.H{"quota": operation.Quota, "operation": operation})
}
func (a *App) adminQuotaOperation(c *gin.Context) {
	if !a.canReadChannelQuota(c, c.Param("channel_id")) {
		return
	}
	item, err := a.Billing.QuotaOperation(c.Request.Context(), a.currentPrincipal(c).UserID, c.Param("channel_id"), c.Query("operation_id"))
	if errors.Is(err, billing.ErrNotFound) {
		httpx.Abort(c, 404, "not_found", "尚未查到该操作，请继续核对原操作。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "读取原操作失败。", true)
		return
	}
	httpx.OK(c, gin.H{"operation": item})
}

func (a *App) channelGrantQuota(c *gin.Context) {
	httpx.Abort(c, http.StatusGone, "unsupported_operation", "渠道不持有额度池，请在支付页面直接给客户划拨额度", false)
}

func (a *App) adminGetQuota(c *gin.Context) {
	if !a.canReadChannelQuota(c, c.Param("channel_id")) {
		return
	}
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
	if p.IsChannelStaff() && !p.IsPlatformAdmin() && !p.HasRole("finance_admin") && p.ChannelOrgID != channelID {
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
	ownerID, err := a.Identity.ResolvePaymentOwnerID(c.Request.Context(), channelID)
	if err != nil || ownerID != channelID || ownerID == identity.OfficialChannelID {
		httpx.Abort(c, 403, "permission_denied", "仅 OEM 有服务额度换算规则", false)
		return
	}
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
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	items, err := a.Commission.ListBrandEntries(c.Request.Context(), scope.Channels, c.Query("usage_event_id"))
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	roles, channels := []string{}, []string{}
	for _, item := range items {
		roles = append(roles, item.BeneficiaryRoleID)
		channels = append(channels, item.ChannelOrgID)
	}
	people, err := a.Identity.BillingRecipientsByRole(c.Request.Context(), roles)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	codes, err := a.Identity.BillingChannelCodes(c.Request.Context(), channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	type entry struct {
		commission.EntryView
		Recipient   identity.BillingRecipient `json:"recipient"`
		ChannelCode string                    `json:"channel_code"`
	}
	status, q := c.Query("status"), strings.ToLower(strings.TrimSpace(c.Query("q")))
	out := []entry{}
	for _, item := range items {
		view := entry{item, people[item.BeneficiaryRoleID], codes[item.ChannelOrgID]}
		if (status == "" || item.Status == status) && commissionMatches(view, q) {
			out = append(out, view)
		}
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "commissions.csv", []string{"id", "kind", "status", "amount_minor", "channel_org_id"}, out, func(item entry) []string {
			return []string{item.ID, item.Kind, item.Status, strconv.FormatInt(item.AmountMinor, 10), item.ChannelOrgID}
		})
		return
	}
	commissionPage(c, out, func(item entry) string { return item.ID }, func(item entry) time.Time { return item.CreatedAt })
}

func (a *App) adminUnfreeze(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		OperationID  string `json:"operation_id"`
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
	if body.OperationID != "" {
		scope, ok := a.commissionWorkflowScope(c)
		if !ok {
			return
		}
		n := 0
		err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
			var created bool
			var err error
			n, created, err = a.Commission.UnfreezeWorkflowTx(tx, scope, commission.UnfreezeInput{OperationID: body.OperationID, UsageEventID: body.UsageEventID})
			if err != nil || !created {
				return err
			}
			_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: scope.ActorUserID, Action: "commission.unfreeze", ResourceType: "commission_operation", ResourceID: body.OperationID, After: map[string]any{"unfrozen": n}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
			return err
		})
		if a.abortCommissionWorkflowError(c, err) {
			return
		}
		httpx.OK(c, gin.H{"unfrozen": n})
		return
	}
	_, channels, ok := a.paymentSettlementScope(c)
	if !ok {
		return
	}
	n, err := a.Commission.UnfreezeForChannels(c.Request.Context(), time.Now().UTC(), strings.TrimSpace(body.UsageEventID), channels)
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
	a.submitCommissionSettlement(c)
}

func (a *App) adminPayout(c *gin.Context) {
	a.submitCommissionPayout(c)
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
		if errors.Is(err, commission.ErrConflict) {
			httpx.Abort(c, http.StatusConflict, "version_conflict", "策略已被更新，请重新读取并核对差异", false)
			return
		}
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

func (a *App) paymentSettlementScope(c *gin.Context) (string, []string, bool) {
	ownerID, ok := a.requireChannelOrg(c)
	if !ok {
		return "", nil, false
	}
	channels, err := a.Identity.BrandChannelIDs(c.Request.Context(), ownerID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取结算范围失败", true)
		return "", nil, false
	}
	return ownerID, channels, true
}
