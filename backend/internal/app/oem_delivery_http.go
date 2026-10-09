package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/endpoints"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"gorm.io/gorm"
	"net/url"
	"time"
)

func (a *App) registerOEMDeliveryRoutes(r *gin.Engine) {
	read := []string{"platform_admin", "ops_admin", "finance_admin", "tech_admin", "audit_readonly"}
	r.GET("/admin/oem-deliveries", a.requireRoles("platform_admin"), a.getOEMOperation)
	r.GET("/admin/channels/:id/admins/candidates", a.requireRoles("platform_admin", "channel_admin"), a.channelAdminCandidates)
	r.POST("/admin/oem-deliveries", a.requireRoles("platform_admin"), a.createOEMDelivery)
	r.GET("/admin/oem-deliveries/:id", a.requireRoles(read...), a.getOEMDelivery)
	r.PATCH("/admin/oem-deliveries/:id", a.requireRoles("platform_admin"), a.patchOEMDelivery)
	r.POST("/admin/oem-deliveries/:id/domains/check", a.requireRoles("platform_admin", "tech_admin"), a.checkOEMDomains)
	r.POST("/admin/oem-deliveries/:id/handoff", a.requireRoles("platform_admin"), a.handoffOEMDelivery)
	r.GET("/channel/delivery", a.requireRoles("channel_admin"), a.getOwnOEMDelivery)
}
func (a *App) abortDeliveryErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, identity.ErrDeliveryConflict) {
		httpx.Abort(c, 409, "version_conflict", "交付记录已变化，请重新读取后核对。", false)
	} else if errors.Is(err, identity.ErrDeliveryIncomplete) {
		httpx.Abort(c, 409, "delivery_incomplete", "必需项或接手验收证据未齐，未完成交接。", false)
	} else if errors.Is(err, identity.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
		httpx.Abort(c, 404, "not_found", "未找到该交付。", false)
	} else if errors.Is(err, identity.ErrChannelImmutable) {
		httpx.Abort(c, 403, "permission_denied", "无权处理该交付。", false)
	} else if errors.Is(err, identity.ErrPromotionInvalid) || errors.Is(err, identity.ErrBrandDomainTaken) {
		httpx.Abort(c, 400, "invalid_request", "请选择可用品牌或填写完整且未占用的品牌域名。", false)
	} else {
		httpx.Abort(c, 500, "read_failed", "交付读取或保存失败，请重试。", true)
	}
	return true
}
func (a *App) createOEMDelivery(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var in identity.OEMCreateInput
	if c.ShouldBindJSON(&in) != nil {
		httpx.Abort(c, 400, "invalid_request", "品牌字段无效", false)
		return
	}
	item, err := a.Identity.CreateOEM(c.Request.Context(), *a.currentPrincipal(c), in)
	if a.abortDeliveryErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "oem.delivery.create", ResourceType: "channel", ResourceID: item.ChannelOrgID, After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.Created(c, gin.H{"item": item})
}

type deliveryCheck struct {
	Key            string `json:"key"`
	Ready          bool   `json:"ready"`
	Reason         string `json:"reason"`
	Responsibility string `json:"responsibility"`
	Href           string `json:"href,omitempty"`
}
type deliveryProjection struct {
	Delivery        *identity.DeliveryView             `json:"delivery"`
	Channel         *identity.ChannelView              `json:"channel"`
	Brand           *identity.BrandView                `json:"brand"`
	Checks          []deliveryCheck                    `json:"checks"`
	Ready           bool                               `json:"ready"`
	CheckedAt       time.Time                          `json:"checked_at"`
	Administrators  []identity.AdminLoginEvidence      `json:"administrators"`
	RequestEvidence *gateway.SuccessfulRequestEvidence `json:"request_evidence,omitempty"`
	RegistrationURL string                             `json:"registration_url,omitempty"`
}

func (a *App) deliveryProjection(ctx context.Context, p identity.Principal, channelID string) (*deliveryProjection, error) {
	delivery, err := a.Identity.OEMDelivery(ctx, p, channelID)
	if err != nil {
		return nil, err
	}
	channel, err := a.Identity.GetChannel(ctx, p, channelID)
	if err != nil {
		return nil, err
	}
	brand, err := a.Identity.BrandByID(ctx, channel.BrandID)
	if err != nil {
		return nil, err
	}
	view := &deliveryProjection{Delivery: delivery, Channel: channel, Brand: brand, Ready: true, CheckedAt: time.Now().UTC(), Checks: []deliveryCheck{}}
	add := func(key string, ready bool, reason, owner, href string) {
		view.Checks = append(view.Checks, deliveryCheck{key, ready, reason, owner, href})
		if !ready {
			view.Ready = false
		}
	}
	add("organization", channel.Status == "active", "organization_disabled", "platform", "/admin/channels/"+channelID)
	domainsReady := true
	for _, name := range []string{"primary", "api", "admin"} {
		item, ok := delivery.DomainEvidence[name].(map[string]any)
		expected := brand.PrimaryDomain
		if name == "api" {
			expected = brand.APIDomain
		}
		if name == "admin" {
			expected = brand.AdminDomain
		}
		checked, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(item["checked_at"]))
		expires, _ := time.Parse(time.RFC3339Nano, fmt.Sprint(item["expires_at"]))
		if !ok || item["ok"] != true || item["host"] != expected || checked.IsZero() || checked.Add(24*time.Hour).Before(time.Now()) || expires.Before(time.Now()) {
			domainsReady = false
		}
	}
	add("domains", domainsReady, "domains_unverified", "platform_tech", "/admin/brands?brand="+brand.ID)
	models, err := a.Catalog.ListChannelModels(ctx, channelID, true)
	if err != nil {
		return nil, err
	}
	goodModels := 0
	for _, model := range models {
		if !model.EffectiveEnabled {
			continue
		}
		readiness, err := a.Catalog.ModelReadiness(ctx, model.PublicID)
		if err != nil {
			return nil, err
		}
		if !readiness.Callable {
			continue
		}
		snapshot, err := a.Catalog.PriceSnapshot(ctx, model.PublicID)
		if err != nil {
			return nil, err
		}
		effective, err := a.Catalog.PriceForChannel(ctx, channelID, model.PublicID, snapshot.Raw)
		if err != nil {
			return nil, err
		}
		if err := a.Catalog.ValidateChannelPrice(ctx, channelID, model.PublicID, effective); err == nil {
			goodModels++
		} else if !errors.Is(err, catalog.ErrModelIncomplete) && !errors.Is(err, catalog.ErrInvalidInput) {
			return nil, err
		}
	}
	add("models", goodModels > 0, "models_or_prices_incomplete", "platform_ops", "/admin/oem-deliveries/"+channelID+"#models")
	quota, err := a.Billing.ChannelQuota(ctx, channelID)
	if err != nil && !errors.Is(err, billing.ErrNotFound) {
		return nil, err
	}
	add("quota", quota != nil && quota.AvailableMinor > 0, "quota_empty", "platform_finance", "/admin/oem-deliveries/"+channelID+"#quota")
	paymentReady := delivery.SalesMode == "offline"
	if delivery.SalesMode == "online" {
		payment, err := a.Payment.Overview(ctx, channelID, "https://"+brand.APIDomain)
		if err != nil {
			return nil, err
		}
		for _, lane := range payment.Lanes {
			if lane.State == "live" {
				paymentReady = true
			}
		}
	}
	add("payment", paymentReady, "online_payment_not_ready", "oem_finance", "")
	if delivery.SellPlans {
		items, err := a.Plans.ListPlansFiltered(ctx, plans.ListPlanFilter{BrandOwnerID: channelID, AudienceChannelID: channelID, PublishedOnly: true})
		if err != nil {
			return nil, err
		}
		add("plans", len(items) > 0, "plans_not_published", "oem_ops", "")
	}
	admins, err := a.Identity.OEMAdminEvidence(ctx, channelID)
	if err != nil {
		return nil, err
	}
	view.Administrators = admins
	hasLogin := false
	for _, admin := range admins {
		if admin.LoginAt != nil {
			hasLogin = true
		}
	}
	add("administrator", hasLogin, "administrator_login_missing", "platform_and_receiver", "/admin/oem-deliveries/"+channelID+"#admins")
	requests, err := a.Gateway.SuccessfulRequestsForChannel(ctx, channelID, delivery.CreatedAt)
	if err != nil {
		return nil, err
	}
	for _, request := range requests {
		usage, err := a.Billing.QueryUsage(ctx, billing.QueryUsageInput{ChannelOrgID: channelID, State: billing.UsageConfirmed, RequestID: request.RequestID, Limit: 1})
		if err != nil {
			return nil, err
		}
		if len(usage) > 0 {
			proof := request
			view.RequestEvidence = &proof
			break
		}
	}
	add("request", view.RequestEvidence != nil, "request_evidence_missing", "receiver", "")
	codes, err := a.Identity.ListPromotionCodes(ctx, channelID)
	if err != nil {
		return nil, err
	}
	for _, code := range codes {
		if code.Status == "active" && code.AcquisitionRoleID == "" {
			view.RegistrationURL = "https://" + brand.PrimaryDomain + "/login?" + url.Values{"promotion_code": {code.Code}, "next": {"/enter"}}.Encode()
			break
		}
	}
	return view, nil
}
func (a *App) getOEMDelivery(c *gin.Context) {
	view, err := a.deliveryProjection(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if a.abortDeliveryErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": view})
}
func (a *App) getOwnOEMDelivery(c *gin.Context) {
	p := a.currentPrincipal(c)
	view, err := a.deliveryProjection(c.Request.Context(), *p, p.ChannelOrgID)
	if a.abortDeliveryErr(c, err) {
		return
	}
	for i := range view.Checks {
		switch view.Checks[i].Key {
		case "organization":
			view.Checks[i].Href = "/channel"
		case "models":
			view.Checks[i].Href = "/channel/models"
		case "quota":
			view.Checks[i].Href = "/channel/ledger"
		case "domains":
			view.Checks[i].Href = "/channel/brand"
		case "administrator":
			view.Checks[i].Href = "/channel/staff"
		case "payment":
			view.Checks[i].Href = "/channel/payments/lanes"
		case "plans":
			view.Checks[i].Href = "/channel/plans"
		case "request":
			view.Checks[i].Href = "/app/catalog"
		}
	}
	httpx.OK(c, gin.H{"item": view})
}
func (a *App) patchOEMDelivery(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var in identity.DeliveryUpdateInput
	if c.ShouldBindJSON(&in) != nil {
		httpx.Abort(c, 400, "invalid_request", "字段无效", false)
		return
	}
	item, err := a.Identity.UpdateOEMDelivery(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), in)
	if a.abortDeliveryErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) checkOEMDomains(c *gin.Context) {
	p := a.currentPrincipal(c)
	view, err := a.deliveryProjection(c.Request.Context(), *p, c.Param("id"))
	if a.abortDeliveryErr(c, err) {
		return
	}
	evidence := map[string]any{"primary": endpoints.Verify(c.Request.Context(), view.Brand.PrimaryDomain, "/", view.Brand.ID), "api": endpoints.Verify(c.Request.Context(), view.Brand.APIDomain, "/v1/public/brand", view.Brand.ID), "admin": endpoints.Verify(c.Request.Context(), view.Brand.AdminDomain, "/login", view.Brand.ID)}
	err = a.Identity.RecordOEMDomainEvidence(c.Request.Context(), *p, c.Param("id"), evidence)
	if a.abortDeliveryErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"evidence": evidence})
}
func (a *App) handoffOEMDelivery(c *gin.Context) {
	if !a.requireConfirm(c) {
		return
	}
	var in struct {
		ExpectedVersion int64  `json:"expected_version"`
		ReceiverUserID  string `json:"receiver_user_id"`
	}
	if c.ShouldBindJSON(&in) != nil {
		httpx.Abort(c, 400, "invalid_request", "接手人字段无效", false)
		return
	}
	view, err := a.deliveryProjection(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if a.abortDeliveryErr(c, err) {
		return
	}
	if !view.Ready || view.Delivery.Phase == "paused" {
		a.abortDeliveryErr(c, identity.ErrDeliveryIncomplete)
		return
	}
	evidence := map[string]any{"checks": view.Checks, "checked_at": view.CheckedAt, "request_id": view.RequestEvidence.RequestID, "administrators": view.Administrators, "brand_id": view.Brand.ID, "sales_mode": view.Delivery.SalesMode}
	item, err := a.Identity.CompleteOEMDelivery(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"), in.ReceiverUserID, in.ExpectedVersion, evidence)
	if a.abortDeliveryErr(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "oem.delivery.handoff", ResourceType: "channel", ResourceID: c.Param("id"), After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.OK(c, gin.H{"item": item})
}

func (a *App) getOEMOperation(c *gin.Context) {
	item, err := a.Identity.OEMOperation(c.Request.Context(), *a.currentPrincipal(c), c.Query("operation_id"))
	if a.abortDeliveryErr(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) channelAdminCandidates(c *gin.Context) {
	if !a.canManageChannelAdmins(c) {
		return
	}
	items, err := a.Identity.ChannelAdminCandidates(c.Request.Context(), c.Param("id"), c.Query("q"))
	if err != nil {
		httpx.Abort(c, 500, "read_failed", "读取接手人失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
