package app

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"strconv"
)

func (a *App) offlinePaymentOperation(c *gin.Context) {
	owner, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	item, err := a.Payment.OfflineOperation(c.Request.Context(), c.Param("operation_id"), a.currentPrincipal(c).UserID, owner)
	if errors.Is(err, payment.ErrNotFound) {
		httpx.OK(c, gin.H{"operation_status": "not_found"})
		return
	}
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"operation_status": "recorded", "item": item})
}
func (a *App) offlinePaymentPreview(c *gin.Context) {
	owner, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	credit, err := strconv.ParseInt(c.Query("credit_minor"), 10, 64)
	if err != nil {
		httpx.Abort(c, 400, "invalid_request", "发放金额无效", false)
		return
	}
	item, err := a.Payment.PreviewOfflineReceipt(c.Request.Context(), owner, c.Query("user_id"), credit)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) paymentOrderDetail(c *gin.Context) {
	owner, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	item, err := a.Payment.OrderFacts(c.Request.Context(), c.Param("id"), owner)
	if a.abortPaymentErr(c, err) {
		return
	}
	users, err := a.Identity.BillingRecipientsByID(c.Request.Context(), []string{item.Order.UserID})
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取订单客户失败，请重试", true)
		return
	}
	channels, err := a.Identity.BillingChannelCodes(c.Request.Context(), []string{item.Order.ChannelOrgID, item.Order.PayeeChannelOrgID})
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取原归属失败，请重试", true)
		return
	}
	httpx.OK(c, gin.H{"item": item, "customer": users[item.Order.UserID], "channel_codes": channels})
}
func (a *App) paymentRefundPreview(c *gin.Context) {
	owner, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	item, err := a.Payment.PreviewRefund(c.Request.Context(), c.Param("id"), owner)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
