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
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerPaymentChannelRoutes(r *gin.Engine) {
	r.GET("/v1/payments/checkout", a.requireUserOrKey(), a.userPaymentCheckout)
	r.GET("/v1/payments/quote", a.requireUserOrKey(), a.userPaymentQuote)

	r.GET("/channel/payments/overview", a.requireRoles("channel_admin", "platform_admin", "finance_admin", "ops_admin"), a.channelPaymentOverview)
	r.GET("/channel/payments/adapters", a.requireRoles("channel_admin", "platform_admin", "finance_admin", "ops_admin"), a.channelPaymentAdapters)
	r.GET("/channel/payments/instances", a.requireRoles("channel_admin"), a.channelListPaymentInstances)
	r.POST("/channel/payments/instances", a.requireRoles("channel_admin"), a.channelCreatePaymentInstance)
	r.PATCH("/channel/payments/instances/:id", a.requireRoles("channel_admin"), a.channelPatchPaymentInstance)
	r.POST("/channel/payments/instances/:id/test", a.requireRoles("channel_admin"), a.channelTestPaymentInstance)
	r.POST("/channel/payments/instances/:id/go-live", a.requireRoles("channel_admin"), a.channelGoLivePaymentInstance)
	r.GET("/channel/payments/settings", a.requireRoles("channel_admin"), a.channelPaymentSettings)
	r.PATCH("/channel/payments/settings", a.requireRoles("channel_admin"), a.channelPatchPaymentSettings)
	r.GET("/channel/payments/orders", a.requireRoles("channel_admin", "finance_admin", "platform_admin"), a.channelListPaymentOrders)
	r.POST("/channel/payments/orders/:id/confirm", a.requireRoles("channel_admin", "finance_admin"), a.channelConfirmPayment)
	r.POST("/channel/payments/orders/:id/refund", a.requireRoles("channel_admin", "finance_admin"), a.channelRefundPayment)

	r.GET("/admin/payments/overview", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.channelPaymentOverview)
	r.GET("/admin/payments/instances", a.requireRoles("platform_admin", "finance_admin"), a.channelListPaymentInstances)
	r.POST("/admin/payments/instances", a.requireRoles("platform_admin", "finance_admin"), a.channelCreatePaymentInstance)
	r.PATCH("/admin/payments/instances/:id", a.requireRoles("platform_admin", "finance_admin"), a.channelPatchPaymentInstance)
	r.POST("/admin/payments/instances/:id/test", a.requireRoles("platform_admin", "finance_admin"), a.channelTestPaymentInstance)
	r.POST("/admin/payments/instances/:id/go-live", a.requireRoles("platform_admin", "finance_admin"), a.channelGoLivePaymentInstance)
	r.GET("/admin/payments/settings", a.requireRoles("platform_admin", "finance_admin"), a.channelPaymentSettings)
	r.PATCH("/admin/payments/settings", a.requireRoles("platform_admin", "finance_admin"), a.channelPatchPaymentSettings)
	r.GET("/admin/payments/recipients", a.requireRoles("platform_admin", "finance_admin"), a.paymentRecipients)
	r.POST("/admin/payments/offline", a.requireRoles("platform_admin", "finance_admin"), a.recordOfflinePayment)
	r.GET("/channel/payments/recipients", a.requireRoles("channel_admin"), a.paymentRecipients)
	r.POST("/channel/payments/offline", a.requireRoles("channel_admin"), a.recordOfflinePayment)
	r.GET("/admin/payments/offline/operations/:operation_id", a.requireRoles("platform_admin", "finance_admin"), a.offlinePaymentOperation)
	r.GET("/channel/payments/offline/operations/:operation_id", a.requireRoles("channel_admin"), a.offlinePaymentOperation)
	r.GET("/admin/payments/offline/preview", a.requireRoles("platform_admin", "finance_admin"), a.offlinePaymentPreview)
	r.GET("/channel/payments/offline/preview", a.requireRoles("channel_admin"), a.offlinePaymentPreview)
	r.GET("/admin/payments/:id", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.paymentOrderDetail)
	r.GET("/channel/payments/orders/:id", a.requireRoles("channel_admin"), a.paymentOrderDetail)
	r.GET("/admin/payments/:id/refund-preview", a.requireRoles("platform_admin", "finance_admin"), a.paymentRefundPreview)
	r.GET("/channel/payments/orders/:id/refund-preview", a.requireRoles("channel_admin"), a.paymentRefundPreview)

	r.GET("/admin/channels/:id/payments", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminChannelPayments)
	r.POST("/admin/channels/:id/payments/disable", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.adminDisableChannelPayments)
	r.GET("/admin/payment-adapters", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminPaymentAdapters)
	r.PATCH("/admin/payment-adapters/:adapter", a.requireRoles("platform_admin"), a.adminPatchPaymentAdapter)
}

func (a *App) abortPaymentErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var external *payment.ExternalReceiptConflict
	if errors.As(err, &external) {
		httpx.AbortParam(c, 409, "external_transaction_conflict", "该真实交易号已登记，请核对原订单", gin.H{"order_id": external.OrderID}, false)
		return true
	}
	switch {
	case errors.Is(err, payment.ErrReceiptConflict):
		httpx.Abort(c, http.StatusConflict, "operation_conflict", "该操作编号已绑定其他主体或内容，请查询原操作", false)
	case errors.Is(err, payment.ErrPreviewChanged):
		httpx.Abort(c, 409, "preview_changed", "发放换算规则已变化，本笔未登记，请重新核对预览", false)
	case errors.Is(err, payment.ErrCollectorRequired), errors.Is(err, identity.ErrChannelImmutable):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能管理所属品牌的收款和额度", false)
	case errors.Is(err, billing.ErrInsufficientQuota):
		httpx.Abort(c, http.StatusConflict, "insufficient_quota", "OEM 服务额度不足，划拨未完成", false)
	case errors.Is(err, identity.ErrNotFound), errors.Is(err, payment.ErrNotFound), errors.Is(err, payment.ErrInstanceNotFound):
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到支付配置或订单", false)
	case errors.Is(err, payment.ErrOrderNotPending):
		httpx.Abort(c, http.StatusConflict, "order_status_conflict", "订单状态已变化，请刷新后核对；仅已支付订单可退款", false)
	case errors.Is(err, billing.ErrInsufficientBalance):
		httpx.Abort(c, http.StatusConflict, "insufficient_balance", "用户可回收余额不足，退款未完成，请核对消费与预授权占用", false)
	case errors.Is(err, billing.ErrTopupNotPending):
		httpx.Abort(c, http.StatusConflict, "topup_status_conflict", "关联充值记录状态不允许此操作，请先核对入账情况", false)
	case errors.Is(err, payment.ErrInvalidAdapter):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "不支持的支付方式", false)
	case errors.Is(err, payment.ErrInvalidAmount):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "金额无效", false)
	case errors.Is(err, payment.ErrInstanceIncomplete):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "凭证不完整", false)
	case errors.Is(err, payment.ErrNotTested):
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请先测连通再上线", false)
	case errors.Is(err, payment.ErrAdapterDisabled), errors.Is(err, payment.ErrOnlineDisabled), errors.Is(err, payment.ErrMethodUnavailable):
		httpx.Abort(c, http.StatusForbidden, "payment_unavailable", "该支付方式当前不可用", false)
	case errors.Is(err, payment.ErrRefundDisabled):
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "该商户未开启退款", false)
	case errors.Is(err, payment.ErrProviderFailed):
		httpx.Abort(c, http.StatusBadGateway, "payment_provider_failed", "支付渠道请求失败", true)
	default:
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "暂未确认支付操作结果，请查询原订单或原操作", true)
	}
	return true
}

func (a *App) channelOrgForAdmin(c *gin.Context) string {
	p := a.currentPrincipal(c)
	if p == nil {
		return ""
	}
	if id := p.VisibleChannelID(); id != "" {
		return id
	}
	return strings.TrimSpace(c.Query("channel_id"))
}

func (a *App) requireChannelOrg(c *gin.Context) (string, bool) {
	p := a.currentPrincipal(c)
	if p != nil && !p.IsChannelStaff() && p.HasRole("platform_admin", "finance_admin", "ops_admin", "audit_readonly") {
		return identity.OfficialChannelID, true
	}
	if p != nil && p.IsChannelStaff() {
		ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		if err == nil && ch.Type == identity.ChannelTypeC {
			return ch.ID, true
		}
	}
	httpx.Abort(c, http.StatusForbidden, "permission_denied", "收款与额度由所属品牌管理", false)
	return "", false
}

func (a *App) canManagePayment(c *gin.Context) bool {
	ownerID, ok := a.requireChannelOrg(c)
	if !ok {
		return false
	}
	item, err := a.Payment.GetOrder(c.Request.Context(), c.Param("id"), "")
	if a.abortPaymentErr(c, err) {
		return false
	}
	if item.PayeeChannelOrgID != ownerID {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不能操作其他品牌的订单", false)
		return false
	}
	return true
}

func (a *App) paymentRecipients(c *gin.Context) {
	ownerID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	items, err := a.Identity.SearchBrandBillingRecipients(c.Request.Context(), c.Query("q"), ownerID)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取客户失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (a *App) recordOfflinePayment(c *gin.Context) {
	ownerID, ok := a.requireChannelOrg(c)
	if !ok || !a.requireConfirm(c) {
		return
	}
	var in payment.OfflineReceiptInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, 400, "invalid_request", "收款信息无效", false)
		return
	}
	item, err := a.Payment.RecordOfflineReceipt(c.Request.Context(), ownerID, in, a.Audit, audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.Created(c, gin.H{"item": item})
}

func (a *App) paymentCallbackOrigin(c *gin.Context, ownerID string) string {
	origin := a.Config.PublicBaseURL
	if ownerID == identity.OfficialChannelID {
		return origin
	}
	brand, _, err := a.Identity.ChannelBrand(c.Request.Context(), identity.Principal{ChannelOrgID: ownerID})
	if err == nil && brand != nil && !strings.Contains(brand.APIDomain, "localhost") {
		return payment.CallbackOrigin(origin, brand.APIDomain)
	}
	return origin
}

func (a *App) channelPaymentOverview(c *gin.Context) {
	p := a.currentPrincipal(c)
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	origin := a.paymentCallbackOrigin(c, channelID)
	item, err := a.Payment.Overview(c.Request.Context(), channelID, origin)
	if a.abortPaymentErr(c, err) {
		return
	}
	channelType := ""
	if p != nil {
		if _, ch, err := a.Identity.ChannelBrand(c.Request.Context(), *p); err == nil && ch != nil {
			channelType = ch.Type
		}
	}
	httpx.OK(c, gin.H{
		"item": item, "channel_type": channelType, "hint": paymentHint(channelType),
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func paymentHint(channelType string) string {
	switch channelType {
	case "B":
		return "资金进入所属品牌的商户。渠道只负责推广和用户管理。"
	case "C":
		return "用户付款直接进入 OEM 商户，包括下属渠道的客户。"
	default:
		return "用户付款直接进入平台商户，包括直属渠道的客户。"
	}
}

func (a *App) channelPaymentAdapters(c *gin.Context) {
	httpx.OK(c, gin.H{"items": a.Payment.Registry().Specs(), "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelListPaymentInstances(c *gin.Context) {
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	items, err := a.Payment.ListInstances(c.Request.Context(), channelID, c.Query("adapter"))
	if a.abortPaymentErr(c, err) {
		return
	}
	origin := a.paymentCallbackOrigin(c, channelID)
	for i := range items {
		items[i].WebhookURL = payment.WebhookURL(origin, items[i].Adapter)
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelCreatePaymentInstance(c *gin.Context) {
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	var in payment.InstanceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	item, err := a.Payment.CreateInstance(c.Request.Context(), channelID, in)
	if a.abortPaymentErr(c, err) {
		return
	}
	item.WebhookURL = payment.WebhookURL(a.paymentCallbackOrigin(c, channelID), item.Adapter)
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.instance.create", ResourceType: "payment_instance", ResourceID: item.ID,
		After: map[string]any{"adapter": item.Adapter, "mode": item.Mode},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelPatchPaymentInstance(c *gin.Context) {
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	var in payment.InstanceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	if len(in.Credentials) > 0 && !a.requireConfirm(c) {
		return
	}
	item, err := a.Payment.PatchInstance(c.Request.Context(), channelID, c.Param("id"), in)
	if a.abortPaymentErr(c, err) {
		return
	}
	item.WebhookURL = payment.WebhookURL(a.paymentCallbackOrigin(c, channelID), item.Adapter)
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.instance.update", ResourceType: "payment_instance", ResourceID: item.ID,
		After: map[string]any{"adapter": item.Adapter, "state": item.State},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelTestPaymentInstance(c *gin.Context) {
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	item, err := a.Payment.TestInstance(c.Request.Context(), channelID, c.Param("id"))
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelGoLivePaymentInstance(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	item, err := a.Payment.GoLiveInstance(c.Request.Context(), channelID, c.Param("id"))
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.instance.go_live", ResourceType: "payment_instance", ResourceID: item.ID,
		After: map[string]any{"adapter": item.Adapter, "mode": item.Mode},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelPaymentSettings(c *gin.Context) {
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	item, err := a.Payment.GetSettings(c.Request.Context(), channelID)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelPatchPaymentSettings(c *gin.Context) {
	channelID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
	var in payment.SettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "请求体无效", false)
		return
	}
	if in.OnlineDisabled != nil && !a.requireConfirm(c) {
		return
	}
	item, err := a.Payment.PatchSettings(c.Request.Context(), channelID, in)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelListPaymentOrders(c *gin.Context) { a.adminListPayments(c) }

func (a *App) channelConfirmPayment(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	if !a.canManagePayment(c) {
		return
	}
	var fact struct {
		OccurredAt time.Time `json:"occurred_at"`
	}
	if err := c.ShouldBindJSON(&fact); err != nil {
		httpx.Abort(c, 400, "invalid_request", "请填写实际收款时间", false)
		return
	}
	item, err := a.Payment.ConfirmRecorded(c.Request.Context(), c.Param("id"), a.currentPrincipal(c).UserID, fact.OccurredAt)
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.confirm", ResourceType: "payment_order", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelRefundPayment(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	if !a.canManagePayment(c) {
		return
	}
	var fact struct {
		OccurredAt time.Time `json:"occurred_at"`
	}
	if err := c.ShouldBindJSON(&fact); err != nil {
		httpx.Abort(c, 400, "invalid_request", "退款信息无效", false)
		return
	}
	item, err := a.Payment.RefundRecorded(c.Request.Context(), c.Param("id"), a.currentPrincipal(c).UserID, fact.OccurredAt)
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.refund", ResourceType: "payment_order", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) userPaymentCheckout(c *gin.Context) {
	_, channelID := a.billingUser(c)
	item, err := a.Payment.UserCheckout(c.Request.Context(), channelID)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) userPaymentQuote(c *gin.Context) {
	_, channelID := a.billingUser(c)
	adapter := c.Query("adapter")
	major, _ := strconv.ParseInt(c.Query("pay_major"), 10, 64)
	item, err := a.Payment.QuoteForChannel(c.Request.Context(), channelID, adapter, major)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminChannelPayments(c *gin.Context) {
	item, err := a.Payment.Overview(c.Request.Context(), c.Param("id"), a.Config.PublicBaseURL)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminDisableChannelPayments(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	disabled := true
	item, err := a.Payment.PatchSettings(c.Request.Context(), c.Param("id"), payment.SettingsInput{OnlineDisabled: &disabled})
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.channel.disable", ResourceType: "channel_org", ResourceID: c.Param("id"),
		After: map[string]any{"online_disabled": true},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPaymentAdapters(c *gin.Context) {
	flags, err := a.Payment.AdapterFlags(c.Request.Context())
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"items": a.Payment.Registry().Specs(), "flags": flags, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchPaymentAdapter(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 enabled", false)
		return
	}
	item, err := a.Payment.SetAdapterEnabled(c.Request.Context(), c.Param("adapter"), *body.Enabled)
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.adapter.flag", ResourceType: "payment_adapter", ResourceID: item.Adapter,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}
