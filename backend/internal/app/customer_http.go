package app

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"net/http"
	"strings"
)

func (a *App) registerCustomerRoutes(r *gin.Engine) {
	r.GET("/admin/channels/:id/onboarding", a.requireRoles("platform_admin", "channel_admin", "ops_admin", "audit_readonly"), a.channelOnboarding)
	r.GET("/admin/professional-customers", a.requireRoles("platform_admin", "channel_admin"), a.professionalCustomerChoices)
	r.POST("/admin/professional-customers", a.requireRoles("platform_admin", "channel_admin"), a.createProfessionalCustomer)
	r.GET("/admin/professional-customers/search", a.requireRoles("platform_admin", "channel_admin", "ops_admin", "audit_readonly"), a.searchProfessionalCustomers)
	r.GET("/admin/professional-customers/:id", a.requireRoles("platform_admin", "channel_admin", "ops_admin", "audit_readonly"), a.professionalCustomerDetail)
	r.GET("/admin/customer-scopes", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.customerScopes)
	r.GET("/channel/subchannels", a.requireRoles("channel_admin"), a.listCustomerSubchannels)
	r.GET("/admin/customer-promotions", a.requireRoles("platform_admin"), a.customerPromotions)
	r.GET("/channel/customer-scopes", a.requireRoles("channel_admin"), a.customerScopes)
	r.GET("/admin/customers", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.listCustomers)
	r.GET("/admin/customers/:id", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "audit_readonly"), a.customerDetail)
	r.GET("/channel/customers", a.requireRoles("channel_admin"), a.listCustomers)
	r.GET("/channel/customers/:id", a.requireRoles("channel_admin"), a.customerDetail)
}
func (a *App) customerScopes(c *gin.Context) {
	items, err := a.Identity.CustomerScopes(c.Request.Context(), *a.currentPrincipal(c))
	if a.abortCustomerError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
func (a *App) customerQueryInput(c *gin.Context) identity.CustomerQuery {
	limit, cursor := httpx.Page(c, 25)
	return identity.CustomerQuery{Q: c.Query("q"), Status: c.Query("status"), ChannelID: c.Query("channel_id"), BrandID: c.Query("brand_id"), Limit: limit, Cursor: cursor}
}
func (a *App) listCustomers(c *gin.Context) { a.writeCustomerPage(c, a.customerQueryInput(c)) }
func (a *App) writeCustomerPage(c *gin.Context, in identity.CustomerQuery) {
	page, err := a.Identity.SearchCustomers(c.Request.Context(), *a.currentPrincipal(c), in)
	if a.abortCustomerError(c, err) {
		return
	}
	if c.Query("format") == "csv" {
		httpx.WriteCSV(c, "customers.csv", []string{"id", "email", "name", "status", "channel", "brand"}, page.Items, func(item identity.CustomerView) []string {
			return []string{item.ID, item.Email, item.DisplayName, item.Status, item.ChannelCode, item.BrandName}
		})
		return
	}
	httpx.OK(c, page)
}
func (a *App) abortCustomerError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, identity.ErrNotFound):
		httpx.Abort(c, 404, "not_found", "客户不存在或不在可见范围", false)
	case errors.Is(err, identity.ErrChannelImmutable):
		httpx.Abort(c, 403, "permission_denied", "无权查看该客户范围", false)
	case errors.Is(err, identity.ErrCustomerCursor), errors.Is(err, identity.ErrInvalidProfile):
		httpx.Abort(c, 400, "invalid_request", "搜索条件或分页已失效，请从第一页重试", false)
	default:
		httpx.Abort(c, 500, "internal_error", "读取客户失败，请重试", true)
	}
	return true
}
func (a *App) customerDetail(c *gin.Context) {
	p := a.currentPrincipal(c)
	item, err := a.Identity.GetCustomer(c.Request.Context(), *p, c.Param("id"))
	if a.abortCustomerError(c, err) {
		return
	}
	if selected := strings.TrimSpace(c.Query("channel_id")); selected != "" && selected != item.ChannelOrgID {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "客户不属于所选渠道", false)
		return
	}
	operational := p.HasRole("platform_admin", "ops_admin", "channel_admin", "oem_ops", "audit_readonly", "oem_audit")
	finance := p.HasRole("platform_admin", "finance_admin", "audit_readonly", "oem_finance", "oem_audit")
	if p.HasRole("channel_admin") {
		ch, e := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		finance = finance || (e == nil && ch.Type == identity.ChannelTypeC)
	}
	canRecord := false
	owner, e := a.Identity.ResolvePaymentOwnerID(c.Request.Context(), item.ChannelOrgID)
	if e == nil {
		canRecord = (p.HasRole("platform_admin", "finance_admin") && owner == identity.OfficialChannelID) || (p.HasRole("channel_admin", "oem_finance") && owner == p.ChannelOrgID)
	}
	paymentOwner := identity.OfficialChannelID
	readPayments := p.HasRole("platform_admin", "finance_admin", "ops_admin", "audit_readonly")
	if p.IsChannelStaff() && !p.IsPlatformAdmin() {
		ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
		readPayments = err == nil && ch.Type == identity.ChannelTypeC
		paymentOwner = p.ChannelOrgID
	}
	readPayments = readPayments && e == nil && owner == paymentOwner
	channels, err := a.Identity.CustomerChannels(c.Request.Context(), *p, "")
	if a.abortCustomerError(c, err) {
		return
	}
	for _, id := range channels {
		if id == identity.OfficialChannelID {
			channels = append(channels, "")
			break
		}
	}
	canManage := (p.IsPlatformAdmin() && e == nil && owner == identity.OfficialChannelID) || (p.IsChannelStaff() && p.HasRole("channel_admin", "oem_ops"))
	for _, role := range item.Roles {
		if role != "end_user" {
			canManage = p.IsPlatformAdmin() && e == nil && owner == identity.OfficialChannelID
			break
		}
	}
	canProfessional := item.Status == identity.UserStatusActive && item.ProfessionalRoleID == "" && ((p.IsPlatformAdmin() && owner == identity.OfficialChannelID) || (p.HasRole("channel_admin") && owner == p.ChannelOrgID))
	for _, role := range item.Roles {
		if role != "end_user" {
			canProfessional = false
		}
	}
	out := gin.H{"item": item, "permissions": gin.H{"operations": operational, "finance": finance, "audit": p.HasRole("platform_admin", "audit_readonly", "oem_audit"), "manage": canManage, "professional_create": canProfessional, "attribution": p.IsPlatformAdmin() && e == nil && owner == identity.OfficialChannelID, "record_payment": canRecord, "read_payments": readPayments}, "errors": gin.H{}}
	failures := out["errors"].(gin.H)
	usage, e := a.Billing.CustomerUsage(c.Request.Context(), item.ID, channels)
	if e != nil {
		failures["usage"] = "消费统计暂不可用"
	} else {
		out["usage"] = usage
	}
	activity, e := a.Gateway.CustomerActivity(c.Request.Context(), item.ID, channels)
	if e != nil {
		failures["activity"] = "请求记录暂不可用"
	} else {
		out["activity"] = activity
	}
	if finance {
		var scopeError error
		if !p.IsPlatformAdmin() {
			scopeError = a.Billing.CustomerCreditScope(c.Request.Context(), item.ID, channels)
		}
		if !p.IsPlatformAdmin() && scopeError == nil {
			inScope, e := a.Payment.CustomerOrdersInScope(c.Request.Context(), item.ID, paymentOwner)
			if e != nil {
				scopeError = e
			} else if !inScope {
				scopeError = billing.ErrCustomerCreditScope
			}
			if scopeError == nil {
				inScope, e = a.Plans.CustomerSubscriptionsInScope(c.Request.Context(), item.ID, channels)
				if e != nil {
					scopeError = e
				} else if !inScope {
					scopeError = billing.ErrCustomerCreditScope
				}
			}
		}
		if scopeError != nil {
			failures["credit"] = "余额含其他品牌历史记录，当前权限无法读取"
			failures["entitlements"] = "额度含其他品牌历史记录，当前权限无法读取"
			if !errors.Is(scopeError, billing.ErrCustomerCreditScope) {
				failures["credit"] = "余额范围校验暂不可用"
				failures["entitlements"] = "额度范围校验暂不可用"
			}
		} else {
			credit, e := a.Billing.CustomerCredit(c.Request.Context(), item.ID)
			if e != nil {
				failures["credit"] = "余额暂不可用"
			} else {
				out["credit"] = credit
			}
			entitlements, e := a.Plans.ListEntitlements(c.Request.Context(), item.ID)
			if e != nil {
				failures["entitlements"] = "额度暂不可用"
			} else {
				out["entitlements"] = entitlements
			}
		}
		if readPayments {
			orders, e := a.Payment.CustomerOrders(c.Request.Context(), item.ID, paymentOwner)
			if e != nil {
				failures["orders"] = "订单统计暂不可用"
			} else {
				out["orders"] = orders
			}
		}
	}

	httpx.OK(c, out)
}
