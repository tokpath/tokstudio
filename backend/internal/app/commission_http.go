package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerCommissionRoutes(r *gin.Engine) {
	r.GET("/v1/partner/users", a.requireAnyUser(), a.partnerUsers)
	r.GET("/v1/partner/commissions", a.requireAnyUser(), a.partnerCommissions)
	r.GET("/v1/partner/settlements", a.requireAnyUser(), a.partnerSettlements)
	r.GET("/v1/partner/export", a.requireAnyUser(), a.partnerExport)
	r.GET("/channel/quota", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelQuota)
	r.GET("/channel/commissions", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelCommissions)
	r.GET("/channel/usage", a.requireRoles("channel_admin", "platform_admin", "finance_admin", "ops_admin"), a.channelUsage)
	r.GET("/channel/settlements", a.requireRoles("channel_admin", "platform_admin", "finance_admin"), a.channelSettlements)
	r.GET("/admin/settlements", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListSettlements)
	r.GET("/channel/promotion-codes", a.requireRoles("channel_admin", "platform_admin"), a.channelListPromos)
	r.POST("/channel/promotion-codes", a.requireRoles("channel_admin", "platform_admin"), a.adminCreatePromo)
	r.POST("/admin/acquisition-roles", a.requireRoles("platform_admin", "channel_admin"), a.adminCreateRole)
	r.GET("/admin/acquisition-roles", a.requireRoles("platform_admin", "channel_admin"), a.adminListRoles)
	r.GET("/admin/promotion-codes", a.requireRoles("platform_admin", "channel_admin", "ops_admin", "audit_readonly"), a.adminListPromos)
	r.POST("/admin/promotion-codes", a.requireRoles("platform_admin", "channel_admin"), a.adminCreatePromo)
	r.POST("/admin/channel-quotas/grant", a.requireRoles("platform_admin", "finance_admin"), a.adminGrantQuota)
	r.GET("/admin/channel-quotas/:channel_id", a.requireRoles("platform_admin", "finance_admin", "channel_admin"), a.adminGetQuota)
	r.GET("/admin/commissions", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminCommissions)
	r.POST("/admin/commissions/unfreeze", a.requireRoles("platform_admin", "finance_admin"), a.adminUnfreeze)
	r.POST("/admin/commissions/settle", a.requireRoles("platform_admin", "finance_admin"), a.adminSettle)
	r.POST("/admin/settlements/:id/payout", a.requireRoles("platform_admin", "finance_admin"), a.adminPayout)
	r.GET("/admin/commission-policy", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.adminPolicy)
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
	httpx.OK(c, gin.H{"usage": item, "request_id": c.GetString(httpx.ContextRequestID)})
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
	httpx.OKPage(c, items, 100, func(item commission.SettlementView) string { return item.ID })
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
	items, err := a.Identity.ListAcquisitionRoles(c.Request.Context(), channelID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取角色失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "roles.csv", []string{"id", "channel_org_id", "type", "level", "status"}, items, func(item identity.AcquisitionRoleView) []string {
			return []string{item.ID, item.ChannelOrgID, item.Type, strconv.Itoa(item.Level), item.Status}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item identity.AcquisitionRoleView) string { return item.ID })
}

func (a *App) adminCreatePromo(c *gin.Context) {
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

func (a *App) adminGetQuota(c *gin.Context) {
	item, err := a.Billing.ChannelQuota(c.Request.Context(), c.Param("channel_id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "渠道额度不存在", false)
		return
	}
	httpx.OK(c, gin.H{"quota": item, "request_id": c.GetString(httpx.ContextRequestID)})
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
	var body struct {
		UsageEventID string `json:"usage_event_id"`
		Now          bool   `json:"now"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.UsageEventID != "" {
		_ = a.Commission.ForceAvailableAt(c.Request.Context(), body.UsageEventID, time.Now().UTC().Add(-time.Second))
	}
	n, err := a.Commission.Unfreeze(c.Request.Context(), time.Now().UTC())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "解冻失败", true)
		return
	}
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
	_ = c.ShouldBindJSON(&body)
	if body.Method == "" {
		body.Method = "manual"
	}
	item, err := a.Commission.Payout(c.Request.Context(), c.Param("id"), body.Method, body.Reference, a.currentPrincipal(c).UserID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "打款失败", false)
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
