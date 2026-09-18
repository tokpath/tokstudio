package app

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func (a *App) adminCommissionRecoveries(c *gin.Context) {
	items, err := a.Billing.ListCommissionRecoveries(c.Request.Context(), "")
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
	for _, item := range items {
		for i := range item.Receipts {
			item.Receipts[i].ActorEmail = people[item.Receipts[i].ActorUserID].Email
		}
		out = append(out, view{item, people[item.UserID]})
	}
	httpx.OK(c, gin.H{"items": out})
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
	var in billing.RecoveryReceiptInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, 400, "invalid_request", "请填写收回金额和真实收款凭证。", false)
		return
	}
	var receipt *billing.RecoveryReceipt
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var created bool
		var err error
		receipt, created, err = a.Billing.RecordCommissionRecoveryTx(tx, c.Param("id"), a.currentPrincipal(c).UserID, in)
		if err != nil || !created {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "commission.recovery.received", ResourceType: "commission_recovery", ResourceID: c.Param("id"), After: receipt, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		return err
	})
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrNotFound):
			httpx.Abort(c, 404, "not_found", "待追回记录不存在，请刷新核对。", false)
		case errors.Is(err, billing.ErrInvalidAmount):
			httpx.Abort(c, 400, "invalid_amount", "收回金额必须大于零且不超过待收回金额；凭证为1–200字节，备注最多1000字节。请刷新核对后重试。", false)
		case errors.Is(err, billing.ErrConflict):
			httpx.Abort(c, 409, "recovery_conflict", "该凭证已登记、原操作内容发生变化或记录已结清，请刷新核对收回明细，勿重复登记。", false)
		default:
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "尚未确认登记结果，请保留原金额、凭证和操作重试，不会重复登记。", true)
		}
		return
	}
	httpx.OK(c, gin.H{"item": receipt})
}
