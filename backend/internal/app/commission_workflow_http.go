package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func (a *App) registerCommissionWorkflowRoutes(r *gin.Engine) {
	r.GET("/admin/commission-context", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.commissionContext)
	r.GET("/channel/commission-context", a.requireRoles("channel_admin"), a.commissionContext)
	r.GET("/admin/commissions/settlement-preview", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.commissionSettlementPreview)
	r.GET("/channel/commissions/settlement-preview", a.requireRoles("channel_admin"), a.commissionSettlementPreview)
	r.GET("/admin/commission-operations/:operation_id", a.requireRoles("platform_admin", "finance_admin"), a.commissionOperation)
	r.GET("/channel/commission-operations/:operation_id", a.requireRoles("channel_admin"), a.commissionOperation)
	r.GET("/admin/settlements/:id", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.commissionSettlementDetail)
	r.GET("/channel/settlements/:id", a.requireRoles("channel_admin"), a.commissionSettlementDetail)
	r.GET("/channel/commission-recoveries", a.requireRoles("channel_admin"), a.adminCommissionRecoveries)
	r.POST("/channel/commission-recoveries/:id/receipts", a.requireRoles("channel_admin"), a.adminRecordCommissionRecovery)
	r.GET("/admin/commission-recovery-operations/:operation_id", a.requireRoles("platform_admin", "finance_admin"), a.commissionRecoveryOperation)
	r.GET("/channel/commission-recovery-operations/:operation_id", a.requireRoles("channel_admin"), a.commissionRecoveryOperation)
}
func (a *App) commissionContext(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	channels, err := a.Identity.BillingChannelCodes(c.Request.Context(), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	owner, err := a.Identity.GetChannel(c.Request.Context(), *a.currentPrincipal(c), scope.OwnerID)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	brand, err := a.Identity.BrandByID(c.Request.Context(), owner.BrandID)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"owner_id": scope.OwnerID, "owner_name": brand.Name, "owner_code": channels[scope.OwnerID], "channel_ids": scope.Channels, "channel_codes": channels})
}
func (a *App) commissionWorkflowScope(c *gin.Context) (commission.WorkflowScope, bool) {
	owner, channels, ok := a.paymentSettlementScope(c)
	if !ok {
		return commission.WorkflowScope{}, false
	}
	referenceChannels := append([]string{}, channels...)
	if selected := strings.TrimSpace(c.Query("channel_id")); selected != "" {
		allowed := false
		for _, channel := range channels {
			if channel == selected {
				allowed = true
				break
			}
		}
		if !allowed {
			httpx.Abort(c, 403, "permission_denied", "该推广归属不在当前品牌范围", false)
			return commission.WorkflowScope{}, false
		}
		channels = []string{selected}
	}
	policyOwner := owner
	if policyOwner == identity.OfficialChannelID {
		policyOwner = ""
	}
	return commission.WorkflowScope{OwnerID: owner, ActorUserID: a.currentPrincipal(c).UserID, PolicyChannelID: policyOwner, Channels: channels, ReferenceChannels: referenceChannels}, true
}
func (a *App) commissionSettlementPreview(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	preview, err := a.Commission.PreviewSettlement(c.Request.Context(), scope, time.Now().UTC(), c.Query("ignore_minimum") == "1")
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	roles := []string{}
	for _, group := range preview.Groups {
		roles = append(roles, group.BeneficiaryRoleID)
	}
	recipients, err := a.Identity.BillingRecipientsByRole(c.Request.Context(), roles)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	channels, err := a.Identity.BillingChannelCodes(c.Request.Context(), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"preview": preview, "recipients": recipients, "channel_codes": channels})
}
func (a *App) commissionOperation(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	operation, err := a.Commission.WorkflowOperation(c.Request.Context(), scope, c.Param("operation_id"))
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": operation})
}
func (a *App) commissionRecoveryOperation(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	ids, err := a.Commission.SettlementIDsForChannels(c.Request.Context(), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	item, err := a.Billing.CommissionRecoveryOperation(c.Request.Context(), scope.ActorUserID, scope.OwnerID, c.Param("operation_id"), ids)
	if err != nil {
		a.abortRecoveryError(c, err)
		return
	}
	httpx.OK(c, gin.H{"item": item, "operation_id": c.Param("operation_id")})
}

type commissionSettlementAdminView struct {
	commission.SettlementView
	Recipient   identity.BillingRecipient `json:"recipient"`
	ChannelCode string                    `json:"channel_code"`
}

func (a *App) settlementAdminViews(c *gin.Context, items []commission.SettlementView) ([]commissionSettlementAdminView, error) {
	roles, ids := []string{}, []string{}
	for _, item := range items {
		roles = append(roles, item.BeneficiaryRoleID)
		ids = append(ids, item.ChannelOrgID)
	}
	people, err := a.Identity.BillingRecipientsByRole(c.Request.Context(), roles)
	if err != nil {
		return nil, err
	}
	channels, err := a.Identity.BillingChannelCodes(c.Request.Context(), ids)
	if err != nil {
		return nil, err
	}
	out := []commissionSettlementAdminView{}
	for _, item := range items {
		out = append(out, commissionSettlementAdminView{item, people[item.BeneficiaryRoleID], channels[item.ChannelOrgID]})
	}
	return out, nil
}
func (a *App) commissionSettlementDetail(c *gin.Context) {
	p := a.currentPrincipal(c)
	if p.IsChannelStaff() {
		channel, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		if a.abortCommissionWorkflowError(c, err) {
			return
		}
		if channel.Type == identity.ChannelTypeB {
			own, err := a.Identity.PersonalReferral(c.Request.Context(), p.UserID)
			if a.abortCommissionWorkflowError(c, err) {
				return
			}
			item, entries, err := a.Commission.SettlementDetailForRoles(c.Request.Context(), c.Param("id"), channel.ID, own.RoleIDs)
			if a.abortCommissionWorkflowError(c, err) {
				return
			}
			views, err := a.settlementAdminViews(c, []commission.SettlementView{*item})
			if a.abortCommissionWorkflowError(c, err) {
				return
			}
			httpx.OK(c, gin.H{"item": views[0], "entries": entries})
			return
		}
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	item, entries, err := a.Commission.SettlementDetail(c.Request.Context(), c.Param("id"), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	views, err := a.settlementAdminViews(c, []commission.SettlementView{*item})
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": views[0], "entries": entries})
}
func (a *App) submitCommissionSettlement(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	var in commission.SettlementCreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, 400, "invalid_request", "请先读取结算预览", false)
		return
	}
	var items []commission.SettlementView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var created bool
		var err error
		items, created, err = a.Commission.CreatePreviewedSettlementTx(tx, scope, in)
		if err != nil || !created {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: scope.ActorUserID, Action: "commission.settle", ResourceType: "settlement_operation", ResourceID: in.OperationID, After: items, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"items": items, "operation_id": in.OperationID})
}
func (a *App) submitCommissionPayout(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	var in commission.PayoutInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, 400, "invalid_request", "请核对实际打款时间与对象", false)
		return
	}
	var item *commission.SettlementView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var created bool
		var err error
		item, created, err = a.Commission.RecordPayoutTx(tx, scope, c.Param("id"), in)
		if err != nil || !created {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: scope.ActorUserID, Action: "commission.payout", ResourceType: "settlement", ResourceID: item.ID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "operation_id": in.OperationID})
}
func (a *App) abortCommissionWorkflowError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, commission.ErrNotFound):
		httpx.Abort(c, 404, "not_found", "原记录暂未找到；结果未知时请保留原操作回查", false)
	case errors.Is(err, commission.ErrInvalid):
		httpx.Abort(c, 400, "invalid_request", "请核对对象、实际发生时间、发生确认及操作编号；参考号最多200字节，说明最多1000字节", false)
	case errors.Is(err, commission.ErrConflict):
		httpx.Abort(c, 409, "commission_conflict", "原操作参数、候选或策略已变化，请回查原记录并重新预览；不要重复登记", false)
	case errors.Is(err, commission.ErrBelowMinimum):
		httpx.Abort(c, 409, "nothing_to_settle", "当前没有符合条件的佣金，请重新预览", false)
	case errors.Is(err, commission.ErrSettlementChanged):
		httpx.Abort(c, 409, "settlement_changed", "结算单已变更或撤销，请刷新原单", false)
	case errors.Is(err, commission.ErrWalletMismatch):
		httpx.Abort(c, 409, "commission_wallet_mismatch", "原佣金入账或余额与结算单不符，未登记；请核对原流水", false)
	default:
		httpx.Abort(c, 500, "internal_error", "登记结果尚未确认，请回查或重试原操作", true)
	}
	return true
}

type commissionPageCursor struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

// Stable ordering plus an immutable timestamp/ID cursor works beyond the old
// latest-100 slice. Search and status filtering happen on the server's full scope.
func commissionPage[T any](c *gin.Context, items []T, idOf func(T) string, atOf func(T) time.Time) {
	limit, cursor := httpx.Page(c, 20)
	total := len(items)
	sort.Slice(items, func(i, j int) bool {
		a, b := atOf(items[i]), atOf(items[j])
		if a.Equal(b) {
			return idOf(items[i]) > idOf(items[j])
		}
		return a.After(b)
	})
	if cursor != "" {
		var anchor commissionPageCursor
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(raw, &anchor) != nil || anchor.ID == "" || anchor.At.IsZero() {
			httpx.Abort(c, 400, "invalid_cursor", "分页位置无效，请返回第一页", false)
			return
		}
		filtered := []T{}
		for _, item := range items {
			at := atOf(item)
			if at.Before(anchor.At) || (at.Equal(anchor.At) && idOf(item) < anchor.ID) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		raw, _ := json.Marshal(commissionPageCursor{idOf(last), atOf(last)})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	httpx.OK(c, gin.H{"items": items, "limit": limit, "total": total, "next_cursor": next, "request_id": c.GetString(httpx.ContextRequestID)})
}
func commissionMatches(value any, q string) bool {
	if q == "" {
		return true
	}
	raw, _ := json.Marshal(value)
	return strings.Contains(strings.ToLower(string(raw)), q)
}
