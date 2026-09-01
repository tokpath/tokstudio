package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerPaymentChannelRoutes(r *gin.Engine) {
	r.GET("/v1/payments/checkout", a.requireUserOrKey(), a.userPaymentCheckout)
	r.GET("/v1/payments/quote", a.requireUserOrKey(), a.userPaymentQuote)

	r.GET("/channel/payments/overview", a.requireRoles("channel_admin"), a.channelPaymentOverview)
	r.GET("/channel/payments/adapters", a.requireRoles("channel_admin"), a.channelPaymentAdapters)
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

	r.GET("/admin/channels/:id/payments", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminChannelPayments)
	r.POST("/admin/channels/:id/payments/disable", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.adminDisableChannelPayments)
	r.GET("/admin/payment-adapters", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin"), a.adminPaymentAdapters)
	r.PATCH("/admin/payment-adapters/:adapter", a.requireRoles("platform_admin"), a.adminPatchPaymentAdapter)
}

func (a *App) abortPaymentErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, payment.ErrNotFound), errors.Is(err, payment.ErrInstanceNotFound):
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "未找到支付配置或订单", false)
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
	default:
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "支付操作失败", false)
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
	if p == nil || p.ChannelOrgID == "" {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "未绑定渠道", false)
		return "", false
	}
	return p.ChannelOrgID, true
}

func (a *App) paymentCallbackOrigin(c *gin.Context, channelOrgID string) string {
	origin := a.Config.PublicBaseURL
	p := a.currentPrincipal(c)
	if p == nil {
		return origin
	}
	if brand, _, err := a.Identity.ChannelBrand(c.Request.Context(), *p); err == nil && brand != nil {
		return payment.CallbackOrigin(a.Config.PublicBaseURL, brand.APIDomain)
	}
	return origin
}

func (a *App) channelPaymentOverview(c *gin.Context) {
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
	if _, ch, err := a.Identity.ChannelBrand(c.Request.Context(), *a.currentPrincipal(c)); err == nil && ch != nil {
		channelType = ch.Type
	}
	httpx.OK(c, gin.H{
		"item": item, "channel_type": channelType, "hint": paymentHint(channelType),
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func paymentHint(channelType string) string {
	switch channelType {
	case "B":
		return "资金进入本渠道商户。用户仍在 TokenHub 域名付款，系统按用户归属选你的通道。"
	case "C":
		return "资金进入本渠道商户。回调和收银台走你的品牌域名。"
	default:
		return "资金进入本渠道（平台）商户。用户在官网充值。"
	}
}

func (a *App) channelPaymentAdapters(c *gin.Context) {
	if _, ok := a.requireChannelOrg(c); !ok {
		return
	}
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

func (a *App) channelListPaymentOrders(c *gin.Context) {
	channelID := a.channelOrgForAdmin(c)
	if p := a.currentPrincipal(c); p != nil && p.HasRole("channel_admin") && !p.IsPlatformAdmin() && !p.HasRole("finance_admin") {
		channelID = p.ChannelOrgID
	}
	items, err := a.Payment.ListOrders(c.Request.Context(), payment.ListOrdersFilter{
		Status: c.Query("status"), ChannelOrgID: channelID, Adapter: c.Query("adapter"), Query: c.Query("q"),
	})
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OKPage(c, items, 100, func(item payment.OrderView) string { return item.ID })
}

func (a *App) channelConfirmPayment(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	item, err := a.Payment.ConfirmManual(c.Request.Context(), c.Param("id"))
	if a.abortPaymentErr(c, err) {
		return
	}
	if p := a.currentPrincipal(c); p != nil && p.HasRole("channel_admin") && !p.IsPlatformAdmin() && item.ChannelOrgID != p.ChannelOrgID {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不能操作其他渠道订单", false)
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
	item, err := a.Payment.GetOrder(c.Request.Context(), c.Param("id"), "")
	if a.abortPaymentErr(c, err) {
		return
	}
	if p := a.currentPrincipal(c); p != nil && p.HasRole("channel_admin") && !p.IsPlatformAdmin() && item.ChannelOrgID != p.ChannelOrgID {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "不能操作其他渠道订单", false)
		return
	}
	item, err = a.Payment.Refund(c.Request.Context(), c.Param("id"))
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
