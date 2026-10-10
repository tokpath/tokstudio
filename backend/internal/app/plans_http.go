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
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

func (a *App) registerPlanRoutes(r *gin.Engine) {
	r.GET("/v1/plans", a.listPublicPlans)
	r.GET("/v1/me/plans", a.requireUserOrKey(), a.listMyPlans)
	r.GET("/v1/me/subscriptions", a.requireUserOrKey(), a.listMySubscriptions)
	r.POST("/v1/me/subscriptions", a.requireUserOrKey(), a.createMySubscription)
	r.GET("/v1/me/subscription-purchases/:operation", a.requireAnyUser(), a.getMySubscriptionPurchase)
	r.POST("/v1/me/subscriptions/:id/cancel", a.requireUserOrKey(), a.cancelMySubscription)
	r.GET("/v1/me/entitlements", a.requireUserOrKey(), a.listMyEntitlements)
	r.POST("/v1/payments/orders", a.requireUserOrKey(), a.createPaymentOrder)
	r.GET("/v1/me/wallet-purchases/:operation", a.requireAnyUser(), a.getMyWalletPurchase)
	r.POST("/v1/payments/orders/:id/sync", a.requireUserOrKey(), a.syncPaymentOrder)
	r.GET("/v1/payments/orders/:id", a.requireUserOrKey(), a.getPaymentOrder)
	r.POST("/v1/payments/:adapter/webhook", a.paymentWebhook)

	r.GET("/channel/plans", a.requireRoles("channel_admin", "platform_admin", "ops_admin"), a.channelListPlans)
	r.POST("/channel/plans", a.requireRoles("channel_admin"), a.adminCreatePlan)
	r.GET("/admin/plans", a.requireRoles("platform_admin", "ops_admin", "channel_admin", "audit_readonly"), a.adminListPlans)
	r.GET("/admin/plans/eligible-channels", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.adminPlanEligibleChannels)
	r.POST("/admin/plans", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.adminCreatePlan)
	r.PATCH("/admin/plans/:id", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.adminPatchPlan)
	r.POST("/admin/plans/:id/review", a.requireRoles("platform_admin", "ops_admin", "channel_admin"), a.adminReviewPlan)
	r.POST("/admin/entitlements/bonus", a.requireRoles("platform_admin", "finance_admin", "ops_admin"), a.adminGrantBonus)
	r.GET("/admin/payments", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.adminListPayments)
	r.POST("/admin/payments/:id/confirm", a.requireRoles("platform_admin", "finance_admin"), a.adminConfirmPayment)
	r.POST("/admin/payments/:id/refund", a.requireRoles("platform_admin", "finance_admin"), a.adminRefundPayment)
	r.POST("/admin/subscriptions/:id/force-period-end", a.requireRoles("platform_admin"), a.adminForcePeriodEnd)
	r.POST("/admin/subscriptions/process-renewals", a.requireRoles("platform_admin"), a.adminProcessRenewals)
}

func (a *App) planBrandOwnerID(c *gin.Context, channelID string) (string, error) {
	if channelID == "" {
		return identity.OfficialChannelID, nil
	}
	marketID, err := a.Identity.ResolveMarketChannelID(c.Request.Context(), channelID)
	if err != nil {
		return "", err
	}
	if marketID == "" {
		return identity.OfficialChannelID, nil
	}
	return marketID, nil
}

func (a *App) requirePlanPublisher(c *gin.Context) bool {
	p := a.currentPrincipal(c)
	if p == nil {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
		return false
	}
	if !p.IsChannelStaff() || p.IsPlatformAdmin() || p.HasRole("ops_admin") {
		return true
	}
	ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
	if err != nil || ch.Type != identity.ChannelTypeC {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "套餐由所属品牌平台统一管理", false)
		return false
	}
	return true
}

func (a *App) canManagePlan(c *gin.Context, planID string) bool {
	if !a.requirePlanPublisher(c) {
		return false
	}
	item, err := a.Plans.GetPlan(c.Request.Context(), planID)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "套餐不存在", false)
		return false
	}
	p := a.currentPrincipal(c)
	allowed := p != nil && ((p.IsChannelStaff() && !p.IsPlatformAdmin() && item.OwnerType == plans.OwnerChannel && item.OwnerID == p.ChannelOrgID) ||
		(p.HasRole("platform_admin", "ops_admin") && item.OwnerType == plans.OwnerPlatform && item.OwnerID == identity.OfficialChannelID))
	if !allowed {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能管理所属品牌的套餐", false)
	}
	return allowed
}

func (a *App) listPublicPlans(c *gin.Context) {
	brand, err := a.Identity.BrandByHost(c.Request.Context(), a.requestHost(c))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取品牌失败", true)
		return
	}
	ownerID, err := a.Identity.ChannelIDByBrand(c.Request.Context(), brand.ID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取品牌套餐失败", true)
		return
	}
	items, err := a.Plans.ListPlansFiltered(c.Request.Context(), plans.ListPlanFilter{BrandOwnerID: ownerID, PublishedOnly: true})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取套餐失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) listMyPlans(c *gin.Context) {
	_, channelID := a.billingUser(c)
	ownerID, err := a.planBrandOwnerID(c, channelID)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "渠道无效", false)
		return
	}
	items, err := a.Plans.ListPlansFiltered(c.Request.Context(), plans.ListPlanFilter{AudienceChannelID: channelID, BrandOwnerID: ownerID, PublishedOnly: true})
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
		AutoRenew        bool   `json:"auto_renew"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.PlanID == "" {
		httpx.Abort(c, 400, "invalid_request", "需要 plan_id", false)
		return
	}
	if body.Adapter == "" {
		body.Adapter = payment.AdapterStripe
	}
	if body.Adapter == payment.AdapterManual {
		httpx.Abort(c, 403, "permission_denied", "线下收款由品牌财务登记", false)
		return
	}
	userID, channelID := a.billingUser(c)
	ownerID, err := a.planBrandOwnerID(c, channelID)
	if err != nil {
		httpx.Abort(c, 400, "invalid_request", "渠道无效", false)
		return
	}
	if body.AutoRenew || strings.TrimSpace(body.PaymentMethodRef) != "" {
		httpx.Abort(c, 400, "invalid_request", "当前购买仅支持手动续费", false)
		return
	}
	operation := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if operation == "" {
		operation = id.New("purchase")
	}
	sub, order, err := a.Payment.CreateSubscriptionPurchase(c.Request.Context(), payment.SubscriptionPurchaseInput{UserID: userID, ChannelID: channelID, BrandOwnerID: ownerID, PlanID: body.PlanID, Adapter: body.Adapter, MethodRef: body.PaymentMethodRef, OperationID: operation, AutoRenew: body.AutoRenew})
	if errors.Is(err, payment.ErrPurchaseConflict) {
		httpx.Abort(c, 409, "idempotency_conflict", "此购买操作的套餐或支付方式已改变，请回查原订单", false)
		return
	}
	if err != nil {
		if errors.Is(err, plans.ErrNotFound) || errors.Is(err, plans.ErrNotPublished) {
			httpx.Abort(c, 400, "invalid_request", "该套餐当前不可购买", false)
			return
		}
		a.abortPaymentErr(c, err)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: userID, Action: "plans.subscription.create", ResourceType: "subscription", ResourceID: sub.ID, After: map[string]any{"plan_id": body.PlanID, "adapter": body.Adapter, "order_id": order.ID}, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	checkout, err := a.Payment.Checkout(c.Request.Context(), order, a.paymentCallbackOrigin(c, order.PayeeChannelOrgID))
	if a.abortPaymentErr(c, err) {
		return
	}
	httpx.Created(c, gin.H{"subscription": sub, "checkout": checkout, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) getMySubscriptionPurchase(c *gin.Context) {
	item, err := a.Payment.GetSubscriptionPurchase(c.Request.Context(), a.currentPrincipal(c).UserID, c.Param("operation"))
	if errors.Is(err, payment.ErrNotFound) {
		httpx.OK(c, gin.H{"item": nil})
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取购买结果失败，请重试", true)
		return
	}
	httpx.OK(c, gin.H{"item": item})
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
	if body.Adapter == payment.AdapterManual {
		httpx.Abort(c, 403, "permission_denied", "线下收款由品牌财务登记", false)
		return
	}
	if body.Purpose == "" {
		body.Purpose = payment.PurposeWallet
	}
	if body.Purpose != payment.PurposeWallet {
		httpx.Abort(c, 400, "invalid_request", "请从套餐页面购买套餐", false)
		return
	}
	if body.PayMajor <= 0 {
		httpx.Abort(c, 400, "invalid_request", "请填写充值金额，由收银台计算到账额度", false)
		return
	}
	userID, channelID := a.billingUser(c)
	operation := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if operation == "" {
		operation = id.New("wallet_purchase")
	}
	order, err := a.Payment.CreateWalletPurchase(c.Request.Context(), payment.CreateOrderInput{
		UserID: userID, ChannelOrgID: channelID, Adapter: body.Adapter, Purpose: body.Purpose,
		PayMajor: body.PayMajor,
	}, operation)
	if errors.Is(err, payment.ErrPurchaseConflict) {
		httpx.Abort(c, 409, "idempotency_conflict", "此充值操作的金额或支付方式已改变，请回查原订单", false)
		return
	}
	if a.abortPaymentErr(c, err) {
		return
	}
	checkout, err := a.Payment.Checkout(c.Request.Context(), order, a.paymentCallbackOrigin(c, order.PayeeChannelOrgID))
	if a.abortPaymentErr(c, err) {
		return
	}
	payload := gin.H{"checkout": checkout, "request_id": c.GetString(httpx.ContextRequestID)}
	httpx.Created(c, payload)
}

func (a *App) getMyWalletPurchase(c *gin.Context) {
	order, err := a.Payment.GetWalletPurchase(c.Request.Context(), a.currentPrincipal(c).UserID, c.Param("operation"))
	if errors.Is(err, payment.ErrNotFound) {
		httpx.OK(c, gin.H{"item": nil})
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "充值结果尚未确认，请重试", true)
		return
	}
	httpx.OK(c, gin.H{"item": order})
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
	if !a.requirePlanPublisher(c) {
		return
	}
	filter := plans.ListPlanFilter{Status: c.Query("status")}
	if p := a.currentPrincipal(c); p != nil && p.IsChannelStaff() && !p.IsPlatformAdmin() {
		filter.OwnerChannelID = p.ChannelOrgID
	} else {
		filter.BrandOwnerID = identity.OfficialChannelID
	}
	items, err := a.Plans.ListPlansFiltered(c.Request.Context(), filter)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取套餐失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminListPlans(c *gin.Context) {
	if !a.requirePlanPublisher(c) {
		return
	}
	filter := plans.ListPlanFilter{Status: c.Query("status"), Name: strings.TrimSpace(c.Query("name")), BillingPeriod: c.Query("billing_period")}
	if p := a.currentPrincipal(c); p != nil {
		if p.HasRole("platform_admin", "ops_admin") {
			filter.BrandOwnerID = identity.OfficialChannelID
		} else if p.IsChannelStaff() {
			filter.OwnerChannelID = p.ChannelOrgID
		}
	}
	if target := c.Query("channel_id"); target != "" {
		if filter.OwnerChannelID != "" {
			if _, err := a.Identity.GetChannel(c.Request.Context(), *a.currentPrincipal(c), target); err != nil {
				httpx.Abort(c, http.StatusForbidden, "forbidden", "无权查看该渠道", false)
				return
			}
		}
		ownerID, err := a.planBrandOwnerID(c, target)
		if err != nil {
			httpx.Abort(c, http.StatusBadRequest, "invalid_request", "渠道无效", false)
			return
		}
		if filter.BrandOwnerID != "" && filter.BrandOwnerID != ownerID {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "只能查看本品牌的套餐", false)
			return
		}
		filter.BrandOwnerID = ownerID
		filter.TargetChannelID = target
	}
	items, err := a.Plans.ListPlansFiltered(c.Request.Context(), filter)
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

func (a *App) adminPlanEligibleChannels(c *gin.Context) {
	if !a.requirePlanPublisher(c) {
		return
	}
	ownerID := identity.OfficialChannelID
	if p := a.currentPrincipal(c); p != nil && p.IsChannelStaff() && !p.IsPlatformAdmin() {
		ownerID = p.ChannelOrgID
	}
	channels, err := a.Identity.ListPlanSubchannels(c.Request.Context(), ownerID)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取下属渠道失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": channels, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminCreatePlan(c *gin.Context) {
	if !a.requirePlanPublisher(c) {
		return
	}
	var in plans.CreatePlanInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "套餐字段无效", false)
		return
	}
	if p := a.currentPrincipal(c); p != nil && p.IsChannelStaff() && !p.IsPlatformAdmin() {
		in.OwnerType = plans.OwnerChannel
		in.OwnerID = p.ChannelOrgID
	} else {
		in.OwnerType = plans.OwnerPlatform
		in.OwnerID = identity.OfficialChannelID
	}
	if in.OwnerID == "" {
		in.OwnerID = identity.OfficialChannelID
	}
	if in.ChannelScope == plans.ChannelScopeSelected {
		allowed, err := a.Identity.ListPlanSubchannels(c.Request.Context(), in.OwnerID)
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取渠道失败", true)
			return
		}
		valid := map[string]bool{}
		for _, ch := range allowed {
			if ch.Status == "active" {
				valid[ch.ID] = true
			}
		}
		for _, id := range in.ChannelIDs {
			if !valid[id] {
				httpx.Abort(c, http.StatusBadRequest, "invalid_request", "适用渠道无效", false)
				return
			}
		}
	}
	item, err := a.Plans.CreatePlan(c.Request.Context(), in, audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "创建套餐失败", false)
		return
	}
	httpx.Created(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPatchPlan(c *gin.Context) {
	if !a.canManagePlan(c, c.Param("id")) {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Status != plans.StatusArchived {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "目前只支持下架 archived", false)
		return
	}
	item, err := a.Plans.ChangePlanStatus(c.Request.Context(), c.Param("id"), "archive", audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, plans.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpx.Abort(c, status, "invalid_request", "当前状态不能下架", false)
		return
	}
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminReviewPlan(c *gin.Context) {
	if !a.canManagePlan(c, c.Param("id")) {
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Plans.ChangePlanStatus(c.Request.Context(), c.Param("id"), body.Action, audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, plans.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpx.Abort(c, status, "invalid_request", "当前状态不能执行该操作", false)
		return
	}
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
	ownerID, ok := a.requireChannelOrg(c)
	if !ok {
		return
	}
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
	limit, cursor := httpx.Page(c, 20)
	if httpx.WantCSV(c) {
		limit = 0
		cursor = ""
	}
	items, err := a.Payment.ListOrders(c.Request.Context(), payment.ListOrdersFilter{
		Limit: limit, Cursor: cursor,
		PayeeChannelOrgID: ownerID, Status: c.Query("status"), ChannelOrgID: c.Query("channel_id"), Adapter: c.Query("adapter"), Query: query, MatchUserIDs: userIDs,
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
	next := ""
	if limit > 0 && len(result) > limit {
		result = result[:limit]
		next = result[len(result)-1].ID
	}
	httpx.OK(c, gin.H{"items": result, "next_cursor": next, "limit": limit})
}

func (a *App) adminConfirmPayment(c *gin.Context) {
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

func (a *App) adminRefundPayment(c *gin.Context) {
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
