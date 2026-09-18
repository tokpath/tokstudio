package app

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerPlanRoutes(r *gin.Engine) {
	r.GET("/v1/plans", a.listPublicPlans)
	r.GET("/v1/me/plans", a.requireUserOrKey(), a.listMyPlans)
	r.GET("/v1/me/subscriptions", a.requireUserOrKey(), a.listMySubscriptions)
	r.POST("/v1/me/subscriptions", a.requireUserOrKey(), a.createMySubscription)
	r.POST("/v1/me/subscriptions/:id/cancel", a.requireUserOrKey(), a.cancelMySubscription)
	r.GET("/v1/me/entitlements", a.requireUserOrKey(), a.listMyEntitlements)
	r.POST("/v1/payments/orders", a.requireUserOrKey(), a.createPaymentOrder)
	r.POST("/v1/payments/orders/:id/sync", a.requireUserOrKey(), a.syncPaymentOrder)
	r.GET("/v1/payments/orders/:id", a.requireUserOrKey(), a.getPaymentOrder)
	r.POST("/v1/payments/:adapter/webhook", a.paymentWebhook)

	r.GET("/channel/plans", a.requireRoles("channel_admin", "platform_admin", "ops_admin"), a.channelListPlans)
	r.POST("/channel/plans", a.requireRoles("channel_admin"), a.adminCreatePlan)
	r.GET("/admin/plans", a.requireRoles("platform_admin", "ops_admin", "channel_admin", "audit_readonly"), a.adminListPlans)
	r.POST("/admin/plans", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.adminCreatePlan)
	r.PATCH("/admin/plans/:id", a.requireRoles("platform_admin", "ops_admin"), a.adminPatchPlan)
	r.POST("/admin/plans/:id/review", a.requireRoles("platform_admin", "ops_admin"), a.adminReviewPlan)
	r.POST("/admin/entitlements/bonus", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.adminGrantBonus)
	r.GET("/admin/payments", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListPayments)
	r.POST("/admin/payments/:id/confirm", a.requireRoles("platform_admin", "finance_admin"), a.adminConfirmPayment)
	r.POST("/admin/payments/:id/refund", a.requireRoles("platform_admin", "finance_admin"), a.adminRefundPayment)
	r.POST("/admin/subscriptions/:id/force-period-end", a.requireRoles("platform_admin"), a.adminForcePeriodEnd)
	r.POST("/admin/subscriptions/process-renewals", a.requireRoles("platform_admin"), a.adminProcessRenewals)
}

func (a *App) listPublicPlans(c *gin.Context) {
	items, err := a.Plans.ListPlans(c.Request.Context(), "", "", true)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取套餐失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listMyPlans(c *gin.Context) {
	_, channelID := a.billingUser(c)
	items, err := a.Plans.ListPlans(c.Request.Context(), channelID, "", true)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取套餐失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listMySubscriptions(c *gin.Context) {
	userID, _ := a.billingUser(c)
	items, err := a.Plans.ListSubscriptions(c.Request.Context(), userID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取订阅失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) createMySubscription(c *gin.Context) {
	var body struct {
		PlanID           string `json:"plan_id"`
		Adapter          string `json:"adapter"`
		PaymentMethodRef string `json:"payment_method_ref"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.PlanID == "" {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "需要 plan_id", false)
		return
	}
	if body.Adapter == "" {
		body.Adapter = payment.AdapterStripe
	}
	userID, channelID := a.billingUser(c)
	sub, err := a.Plans.CreateSubscription(c.Request.Context(), userID, channelID, body.PlanID, body.Adapter, body.PaymentMethodRef)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "无法订阅该套餐", false)
		return
	}
	plan, err := a.Plans.GetPlan(c.Request.Context(), body.PlanID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "套餐不存在", false)
		return
	}
	order, err := a.Payment.CreateOrder(c.Request.Context(), payment.CreateOrderInput{
		UserID: userID, ChannelOrgID: channelID, Adapter: body.Adapter, Purpose: payment.PurposeSubscription,
		ReferenceType: payment.PurposeSubscription, ReferenceID: sub.ID,
		AmountMinor: plan.PriceMinor, Currency: plan.Currency,
	})
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建支付单失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: userID, Action: "plans.subscription.create", ResourceType: "subscription", ResourceID: sub.ID,
		After: map[string]any{"plan_id": body.PlanID, "adapter": body.Adapter, "order_id": order.ID},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	checkout, err := a.Payment.Checkout(c.Request.Context(), order, a.Config.PublicBaseURL)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.Created(c, gin.H{
		"subscription": sub,
		"checkout":     checkout,
		"request_id":   c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) cancelMySubscription(c *gin.Context) {
	userID, _ := a.billingUser(c)
	item, err := a.Plans.Cancel(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "订阅不存在", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listMyEntitlements(c *gin.Context) {
	userID, _ := a.billingUser(c)
	items, err := a.Plans.ListEntitlements(c.Request.Context(), userID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取权益失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) createPaymentOrder(c *gin.Context) {
	raw, _ := io.ReadAll(c.Request.Body)
	if rec := a.replayIdempotency(c, raw); rec != nil {
		return
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	var body struct {
		Adapter     string `json:"adapter"`
		AmountMinor int64  `json:"amount_minor"`
		PayMajor    int64  `json:"pay_major"`
		Purpose     string `json:"purpose"`
		Currency    string `json:"currency"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || (body.AmountMinor <= 0 && body.PayMajor <= 0) {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "充值金额无效", false)
		return
	}
	if body.Adapter == "" {
		body.Adapter = payment.AdapterStripe
	}
	if body.Purpose == "" {
		body.Purpose = payment.PurposeWallet
	}
	userID, channelID := a.billingUser(c)
	order, err := a.Payment.CreateOrder(c.Request.Context(), payment.CreateOrderInput{
		UserID: userID, ChannelOrgID: channelID, Adapter: body.Adapter, Purpose: body.Purpose,
		AmountMinor: body.AmountMinor, PayMajor: body.PayMajor, Currency: body.Currency,
	})
	if a.abortPaymentErr(c, err) {
		return
	}
	checkout, err := a.Payment.Checkout(c.Request.Context(), order, a.Config.PublicBaseURL)
	if a.abortPaymentErr(c, err) {
		return
	}
	payload := gin.H{"checkout": checkout, "request_id": c.GetString(httpx.ContextRequestID)}
	a.rememberIdempotency(c, raw, http.StatusCreated, payload)
	httpx.Created(c, payload)
}

func (a *App) getPaymentOrder(c *gin.Context) {
	userID, _ := a.billingUser(c)
	if p := a.currentPrincipal(c); p != nil && p.HasRole("platform_admin", "finance_admin") {
		userID = ""
	}
	item, err := a.Payment.GetOrder(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "支付单不存在", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) syncPaymentOrder(c *gin.Context) {
	userID, _ := a.billingUser(c)
	item, err := a.Payment.SyncFromProvider(c.Request.Context(), c.Param("id"), userID)
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) paymentWebhook(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "回调体无效", false)
		return
	}
	item, err := a.Payment.HandleWebhook(c.Request.Context(), c.Param("adapter"), c.Request.Header, body)
	if err != nil {
		status := http.StatusBadRequest
		if err == payment.ErrInvalidSignature {
			status = http.StatusUnauthorized
		}
		httpx.Abort(c, status, "invalid_request", "支付回调校验失败", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) channelListPlans(c *gin.Context) {
	channelID := a.currentPrincipal(c).VisibleChannelID()
	if channelID == "" {
		channelID = c.Query("channel_id")
	}
	items, err := a.Plans.ListPlans(c.Request.Context(), channelID, c.Query("status"), false)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取套餐失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminListPlans(c *gin.Context) {
	channelID := ""
	if p := a.currentPrincipal(c); p != nil && p.HasRole("channel_admin") {
		channelID = p.ChannelOrgID
	}
	items, err := a.Plans.ListPlans(c.Request.Context(), channelID, c.Query("status"), false)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取套餐失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "plans.csv", []string{"id", "name", "status", "owner_type", "price_minor"}, items, func(item plans.PlanView) []string {
			return []string{item.ID, item.Name, item.Status, item.OwnerType, strconv.FormatInt(item.PriceMinor, 10)}
		})
		return
	}
	httpx.OKPage(c, items, 100, func(item plans.PlanView) string { return item.ID })
}

func (a *App) adminCreatePlan(c *gin.Context) {
	var in plans.CreatePlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "套餐字段无效", false)
		return
	}
	if p := a.currentPrincipal(c); p != nil && p.HasRole("channel_admin") && !p.IsPlatformAdmin() {
		in.OwnerType = plans.OwnerChannel
		in.OwnerID = p.ChannelOrgID
	}
	if in.OwnerID == "" {
		in.OwnerID = identity.OfficialChannelID
	}
	item, err := a.Plans.CreatePlan(c.Request.Context(), in)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建套餐失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "plans.plan.create", ResourceType: "product_plan", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchPlan(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Status != plans.StatusArchived {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "目前只支持下架 archived", false)
		return
	}
	item, err := a.Plans.ArchivePlan(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "套餐不存在", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "plans.plan.archive", ResourceType: "product_plan", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminReviewPlan(c *gin.Context) {
	var body struct {
		Action string `json:"action"`
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Plans.ReviewPlan(c.Request.Context(), c.Param("id"), body.Action, body.Reason)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "审核失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "plans.plan.review", ResourceType: "product_plan", ResourceID: item.ID,
		After: map[string]any{"action": body.Action, "status": item.Status, "reason": body.Reason},
		IP:    c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminGrantBonus(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var body struct {
		UserID    string `json:"user_id"`
		UnitType  string `json:"unit_type"`
		Amount    int64  `json:"amount"`
		ExpiresIn int    `json:"expires_in_seconds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.UserID == "" || body.Amount <= 0 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "赠送参数无效", false)
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "缺少有效的操作编号，请刷新后重试", false)
		return
	}
	recipient, err := a.Identity.Me(c.Request.Context(), identity.Principal{UserID: body.UserID})
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "暂时无法核对用户，请重试", true)
		return
	}
	if err != nil || recipient.Status != identity.UserStatusActive {
		httpx.Abort(c, http.StatusBadRequest, "invalid_recipient", "用户不存在或已停用，请重新选择", false)
		return
	}
	if body.ExpiresIn < 0 || body.ExpiresIn > 31536000 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "有效期应在 1 秒至 365 天之间", false)
		return
	}
	if body.UnitType == "" {
		body.UnitType = plans.UnitUSDCredit
	}
	exp := 24 * time.Hour
	if body.ExpiresIn > 0 {
		exp = time.Duration(body.ExpiresIn) * time.Second
	}
	item, err := a.Plans.GrantBonus(c.Request.Context(), a.currentPrincipal(c).UserID, key, body.UserID, body.UnitType, body.Amount, exp)
	if err != nil {
		if errors.Is(err, plans.ErrBonusConflict) {
			httpx.Abort(c, http.StatusConflict, "idempotency_conflict", "此操作编号已用于另一笔赠送，请核对原操作", false)
			return
		}
		if errors.Is(err, plans.ErrInvalidPlan) {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "赠送参数无效", false)
		} else {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "暂未确认发放结果，请用原操作重试", true)
		}
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "plans.bonus.grant", ResourceType: "entitlement", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminListPayments(c *gin.Context) {
	query := strings.TrimSpace(c.Query("q"))
	if len(query) > 200 {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "搜索内容过长", false)
		return
	}
	var userIDs []string
	var err error
	if query != "" {
		userIDs, err = a.Identity.MatchBillingUserIDs(c.Request.Context(), query)
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "搜索用户失败，请重试", true)
			return
		}
	}
	items, err := a.Payment.ListOrders(c.Request.Context(), payment.ListOrdersFilter{
		Status: c.Query("status"), ChannelOrgID: c.Query("channel_id"), Adapter: c.Query("adapter"), Query: query, MatchUserIDs: userIDs,
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取支付单失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "payments.csv", []string{"id", "user_id", "channel_org_id", "adapter", "purpose", "status", "amount_minor"}, items, func(item payment.OrderView) []string {
			return []string{item.ID, item.UserID, item.ChannelOrgID, item.Adapter, item.Purpose, item.Status, strconv.FormatInt(item.AmountMinor, 10)}
		})
		return
	}
	ids := make([]string, 0, len(items))
	channelIDs := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.UserID)
		channelIDs = append(channelIDs, item.ChannelOrgID)
	}
	users, err := a.Identity.BillingRecipientsByID(c.Request.Context(), ids)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取订单用户失败，请重试", true)
		return
	}
	channels, err := a.Identity.BillingChannelCodes(c.Request.Context(), channelIDs)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取订单渠道失败，请重试", true)
		return
	}
	type adminPayment struct {
		payment.OrderView
		UserEmail   string `json:"user_email"`
		UserName    string `json:"user_name"`
		ChannelCode string `json:"channel_code"`
	}
	result := make([]adminPayment, 0, len(items))
	for _, item := range items {
		u := users[item.UserID]
		result = append(result, adminPayment{item, u.Email, u.DisplayName, channels[item.ChannelOrgID]})
	}
	httpx.OKPage(c, result, 100, func(item adminPayment) string { return item.ID })
}

func (a *App) adminConfirmPayment(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	item, err := a.Payment.ConfirmManual(c.Request.Context(), c.Param("id"))
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.confirm", ResourceType: "payment_order", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminRefundPayment(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	item, err := a.Payment.Refund(c.Request.Context(), c.Param("id"))
	if a.abortPaymentErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "payment.refund", ResourceType: "payment_order", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminForcePeriodEnd(c *gin.Context) {
	if a.Config.IsProduction() && !a.Config.AllowDemoProbes {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "生产环境禁止拨时钟", false)
		return
	}
	end := time.Now().UTC().Add(-time.Second)
	if err := a.Plans.ForcePeriodEnd(c.Request.Context(), c.Param("id"), end); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "无法改到期时间", false)
		return
	}
	httpx.OK(c, gin.H{"ok": true, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminProcessRenewals(c *gin.Context) {
	if a.Config.IsProduction() && !a.Config.AllowDemoProbes {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "生产环境禁止手动续费扫描", false)
		return
	}
	n, err := a.Plans.ProcessRenewals(c.Request.Context(), time.Now().UTC(), a.Payment.RenewCharger())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "续费扫描失败", true)
		return
	}
	httpx.OK(c, gin.H{"processed": n, "request_id": c.GetString(httpx.ContextRequestID)})
}
