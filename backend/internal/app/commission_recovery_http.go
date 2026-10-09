package app

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func (a *App) adminCommissionRecoveries(c *gin.Context) {
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	settlementIDs, err := a.Commission.SettlementIDsForChannels(c.Request.Context(), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	items, err := a.Billing.ListScopedCommissionRecoveries(c.Request.Context(), settlementIDs)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取待追回佣金失败，请重试。", true)
		return
	}
	ids := []string{}
	for _, item := range items {
		ids = append(ids, item.UserID)
		for _, receipt := range item.Receipts {
			ids = append(ids, receipt.ActorUserID)
		}
	}
	people, err := a.Identity.BillingRecipientsByID(c.Request.Context(), ids)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取收款人失败，请重试。", true)
		return
	}
	type view struct {
		billing.CommissionRecoveryView
		Recipient identity.BillingRecipient `json:"recipient"`
	}
	out := []view{}
	status, query := c.Query("status"), strings.ToLower(strings.TrimSpace(c.Query("q")))
	for _, item := range items {
		for i := range item.Receipts {
			item.Receipts[i].ActorEmail = people[item.Receipts[i].ActorUserID].Email
		}
		row := view{item, people[item.UserID]}
		if (status == "" || status == item.Status) && commissionMatches(row, query) {
			out = append(out, row)
		}
	}
	commissionPage(c, out, func(item view) string { return item.ID }, func(item view) time.Time { return item.CreatedAt })
}
func (a *App) myCommissionRecoveries(c *gin.Context) {
	items, err := a.Billing.ListCommissionRecoveries(c.Request.Context(), a.currentPrincipal(c).UserID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取佣金核对记录失败，请重试。", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
func (a *App) adminRecordCommissionRecovery(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	scope, ok := a.commissionWorkflowScope(c)
	if !ok {
		return
	}
	settlementIDs, err := a.Commission.SettlementIDsForChannels(c.Request.Context(), scope.Channels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	referenceIDs, err := a.Commission.SettlementIDsForChannels(c.Request.Context(), scope.ReferenceChannels)
	if a.abortCommissionWorkflowError(c, err) {
		return
	}
	var in billing.RecoveryReceiptInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, 400, "invalid_request", "请核对实际收回金额与时间。", false)
		return
	}
	var receipt *billing.RecoveryReceipt
	err = a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var created bool
		var err error
		receipt, created, err = a.Billing.RecordScopedCommissionRecoveryTx(tx, c.Param("id"), scope.ActorUserID, scope.OwnerID, settlementIDs, in, referenceIDs)
		if err != nil || !created {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: scope.ActorUserID, Action: "commission.recovery.received", ResourceType: "commission_recovery", ResourceID: c.Param("id"), After: receipt, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if err != nil {
		a.abortRecoveryError(c, err)
		return
	}
	httpx.OK(c, gin.H{"item": receipt})
}
func (a *App) abortRecoveryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billing.ErrNotFound):
		httpx.Abort(c, 404, "not_found", "待追回记录不存在，请刷新核对。", false)
	case errors.Is(err, billing.ErrInvalidAmount):
		httpx.Abort(c, 400, "invalid_amount", "金额须大于零且不超过剩余；请确认实际发生时间。参考号可选，最多200字节；说明最多1000字节。", false)
	case errors.Is(err, billing.ErrConflict):
		httpx.Abort(c, 409, "recovery_conflict", "该凭证已登记、原操作内容发生变化或记录已结清，请刷新核对收回明细，勿重复登记。", false)
	default:
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "尚未确认登记结果，请保留原金额、凭证和操作重试，不会重复登记。", true)
	}
}
