package app

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerOEMManagementRoutes(r *gin.Engine) {
	r.GET("/channel/metrics", a.requireRoles("channel_admin"), a.oemMetrics)
	r.GET("/channel/media", a.requireRoles("channel_admin"), a.oemMedia)
	r.GET("/channel/audit-logs", a.requireRoles("channel_admin"), a.oemAudit)
	r.GET("/channel/alerts", a.requireRoles("channel_admin"), a.oemAlerts)
	r.GET("/channel/me/2fa", a.requireRoles("channel_admin"), a.requireOEM(), a.adminTOTPStatus)
	r.POST("/channel/me/2fa/setup", a.requireRoles("channel_admin"), a.requireOEM(), a.adminTOTPSetup)
	r.POST("/channel/me/2fa/enable", a.requireRoles("channel_admin"), a.requireOEM(), a.adminTOTPEnable)
	r.POST("/channel/me/2fa/disable", a.requireRoles("channel_admin"), a.requireOEM(), a.adminTOTPDisable)
}

func (a *App) requireOEM() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := a.oemScope(c); !ok {
			c.Abort()
		}
	}
}

// Scope is derived from persisted ownership, never from a client supplied brand.
// An explicit foreign channel fails closed instead of falling back to all data.
func (a *App) oemScope(c *gin.Context) ([]string, bool) {
	p := a.currentPrincipal(c)
	if p == nil || !p.HasRole("channel_admin") {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 OEM 管理员可使用此功能", false)
		return nil, false
	}
	ch, err := a.Identity.GetChannel(c.Request.Context(), *p, p.ChannelOrgID)
	if err != nil || ch.Type != identity.ChannelTypeC {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "仅 OEM 管理员可使用此功能", false)
		return nil, false
	}
	channels, err := a.Identity.ListChannels(c.Request.Context(), *p)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取管理范围失败", true)
		return nil, false
	}
	ids := []string{ch.ID}
	for _, child := range channels {
		if child.Type == identity.ChannelTypeB && child.ParentID == ch.ID {
			ids = append(ids, child.ID)
		}
	}
	if selected := c.Query("channel_id"); selected != "" {
		for _, id := range ids {
			if selected == id {
				return []string{id}, true
			}
		}
		httpx.Abort(c, 403, "permission_denied", "无权查看该渠道", false)
		return nil, false
	}
	return ids, true
}

// Existing channel ledgers keep one quota account per view. OEMs may select a
// direct B account; reseller callers cannot widen their own account scope.
func (a *App) channelForQuery(c *gin.Context) (string, bool) {
	p := a.currentPrincipal(c)
	channelID := p.VisibleChannelID()
	selected := c.Query("channel_id")
	if channelID == "" {
		return selected, true
	}
	if selected == "" || selected == channelID {
		return channelID, true
	}
	ids, ok := a.oemScope(c)
	if !ok {
		return "", false
	}
	return ids[0], true
}

func (a *App) oemMetrics(c *gin.Context) {
	ids, ok := a.oemScope(c)
	if !ok {
		return
	}
	total, items, err := a.Billing.BrandReport(c.Request.Context(), ids)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取指标失败", true)
		return
	}
	httpx.OK(c, gin.H{"totals": total, "items": items})
}

func (a *App) oemMedia(c *gin.Context) {
	ids, ok := a.oemScope(c)
	if !ok {
		return
	}
	items, err := a.Media.ListChannels(c.Request.Context(), ids, c.Query("q"))
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取媒体任务失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (a *App) oemAudit(c *gin.Context) {
	ids, ok := a.oemScope(c)
	if !ok {
		return
	}
	actors, err := a.Identity.ChannelActors(c.Request.Context(), *a.currentPrincipal(c), ids)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取操作人失败", true)
		return
	}
	items, err := a.Audit.SearchChannels(c.Request.Context(), ids, actors, c.Query("q"))
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取审计日志失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (a *App) oemAlerts(c *gin.Context) {
	ids, ok := a.oemScope(c)
	if !ok {
		return
	}
	pending, err := a.Billing.BrandPendingCounts(c.Request.Context(), ids)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取告警失败", true)
		return
	}
	failed, err := a.Media.FailedChannelJobCounts(c.Request.Context(), ids)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取告警失败", true)
		return
	}
	items := make([]gin.H, 0)
	names, err := a.Identity.BillingChannelCodes(c.Request.Context(), ids)
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "读取渠道失败", true)
		return
	}
	for _, channelID := range ids {
		if pending[channelID] > 0 {
			items = append(items, gin.H{"id": "reconciliation:" + channelID, "title": names[channelID] + " · 待对账请求", "count": pending[channelID], "href": "/channel/reconciliation?channel_id=" + url.QueryEscape(channelID)})
		}
		if failed[channelID] > 0 {
			items = append(items, gin.H{"id": "media:" + channelID, "title": names[channelID] + " · 近 24 小时失败的媒体任务", "count": failed[channelID], "href": "/channel/media?channel_id=" + url.QueryEscape(channelID)})
		}
	}
	httpx.OK(c, gin.H{"items": items})
}
