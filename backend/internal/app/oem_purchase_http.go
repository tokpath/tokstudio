package app

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
)

func (a *App) registerOEMPurchaseRoutes(r *gin.Engine) {
	r.GET("/admin/oem-purchases", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminOEMPurchases)
	r.POST("/admin/oem-purchases", a.requireRoles("platform_admin", "finance_admin"), a.completeOEMPurchase)
	r.GET("/admin/oem-purchases/operations", a.requireRoles("platform_admin", "finance_admin"), a.oemPurchaseOperation)
	r.GET("/admin/oem-purchases/:id", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminOEMPurchase)
	r.POST("/admin/oem-purchases/:id/reverse", a.requireRoles("platform_admin", "finance_admin"), a.reverseOEMPurchase)
	r.GET("/channel/oem-purchases", a.requireRoles("channel_admin"), a.channelOEMPurchases)
}
func (a *App) adminOEMPurchases(c *gin.Context) { a.listOEMPurchases(c, c.Query("oem_channel_id")) }
func (a *App) channelOEMPurchases(c *gin.Context) {
	if _, ok := a.oemScope(c); !ok {
		return
	}
	a.listOEMPurchases(c, a.currentPrincipal(c).ChannelOrgID)
}
func (a *App) listOEMPurchases(c *gin.Context, owner string) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	items, err := a.Billing.ListOEMPurchases(c.Request.Context(), owner, c.Query("cursor"), limit)
	if errors.Is(err, billing.ErrNotFound) {
		httpx.Abort(c, 404, "not_found", "该采购分页位置不在当前范围。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "读取采购交易失败，请重试。", true)
		return
	}
	if a.currentPrincipal(c).IsChannelStaff() && !a.currentPrincipal(c).IsPlatformAdmin() {
		for i := range items {
			items[i].ActorUserID = ""
			items[i].Note = ""
			items[i].ReversedBy = ""
			items[i].ReversalOperationID = ""
		}
	}
	httpx.OKPage(c, items, limit, func(v billing.OEMPurchaseView) string { return v.ID })
}
func (a *App) oemPurchaseOperation(c *gin.Context) {
	item, err := a.Billing.OEMPurchaseOperation(c.Request.Context(), a.currentPrincipal(c).UserID, c.Query("oem_channel_id"), c.Query("operation_id"))
	if errors.Is(err, billing.ErrNotFound) {
		httpx.Abort(c, 404, "not_found", "尚未查到原交易，请保留原操作核对。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "读取原交易失败。", true)
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) completeOEMPurchase(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body billing.OEMPurchaseInput
	if c.ShouldBindJSON(&body) != nil {
		httpx.Abort(c, 400, "invalid_request", "采购信息无效。", false)
		return
	}
	var item *billing.OEMPurchaseView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		item, err = a.Billing.CompleteOEMPurchaseTx(tx, a.currentPrincipal(c).UserID, body)
		if err != nil {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.oem_purchase.complete", ResourceType: "oem_purchase", ResourceID: item.ID, After: item, RequestID: c.GetString(httpx.ContextRequestID), IP: c.ClientIP()})
		return err
	})
	var duplicate *billing.OEMPurchaseReferenceConflict
	if errors.As(err, &duplicate) {
		original, readErr := a.Billing.OEMPurchase(c.Request.Context(), "", duplicate.PurchaseID)
		if readErr != nil {
			httpx.Abort(c, 500, "read_failed", "该真实交易号已登记，但原记录暂时读取失败，请保留原操作核对。", true)
			return
		}
		httpx.AbortParam(c, 409, "external_reference_conflict", "该真实交易号已登记，本次没有新增交易或额度。请核对下方原采购记录。", gin.H{"item": original}, false)
		return
	}
	if errors.Is(err, billing.ErrConflict) {
		httpx.Abort(c, 409, "operation_conflict", "原操作已绑定其他采购内容，未再次划入额度。", false)
		return
	}
	if errors.Is(err, billing.ErrInvalidAmount) {
		httpx.Abort(c, 400, "invalid_request", "请核对OEM、实际收款、协议USD销售金额、服务额度和实际收款时间，并确认款项已收到。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "result_unconfirmed", "结果待确认，请核对或重试原操作。", true)
		return
	}
	httpx.OK(c, gin.H{"item": item, "platform_owner_id": identity.OfficialChannelID})
}

func (a *App) adminOEMPurchase(c *gin.Context) {
	item, err := a.Billing.OEMPurchase(c.Request.Context(), "", c.Param("id"))
	if errors.Is(err, billing.ErrNotFound) {
		httpx.Abort(c, 404, "not_found", "未找到原采购交易。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "原采购记录读取失败。", true)
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) reverseOEMPurchase(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body billing.OEMPurchaseReversalInput
	if c.ShouldBindJSON(&body) != nil {
		httpx.Abort(c, 400, "invalid_request", "请填写原操作与撤销原因。", false)
		return
	}
	var item *billing.OEMPurchaseView
	err := a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		item, err = a.Billing.ReverseOEMPurchaseTx(tx, a.currentPrincipal(c).UserID, c.Param("id"), body)
		if err != nil {
			return err
		}
		_, err = a.Audit.RecordTx(tx, audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "billing.oem_purchase.reverse", ResourceType: "oem_purchase", ResourceID: item.ID, After: item, RequestID: c.GetString(httpx.ContextRequestID), IP: c.ClientIP()})
		return err
	})
	if errors.Is(err, billing.ErrInsufficientQuota) {
		httpx.Abort(c, 409, "quota_not_recoverable", "原OEM可用额度不足，无法全额收回；没有撤销销售或改变客户余额。请先核对原额度。", false)
		return
	}
	if errors.Is(err, billing.ErrConflict) {
		httpx.Abort(c, 409, "operation_conflict", "原采购已撤销或此操作绑定其他内容，请核对原记录。", false)
		return
	}
	if errors.Is(err, billing.ErrNotFound) {
		httpx.Abort(c, 404, "not_found", "原采购交易不存在。", false)
		return
	}
	if errors.Is(err, billing.ErrInvalidAmount) {
		httpx.Abort(c, 400, "invalid_request", "请填写原操作与撤销原因。", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "result_unconfirmed", "撤销结果待确认，请查询或重试原操作。", true)
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
