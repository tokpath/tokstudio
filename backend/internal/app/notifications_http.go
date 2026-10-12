package app

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/notifications"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/mail"
)

func (a *App) registerNotificationRoutes(r *gin.Engine) {
	r.GET("/v1/me/notifications", a.requireAnyUser(), a.listNotifications)
	r.GET("/v1/me/notifications/unread-count", a.requireAnyUser(), func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		p := a.currentPrincipal(c)
		count, err := a.Notifications.Unread(c.Request.Context(), p.UserID, p.BrandID)
		if a.notificationError(c, err) {
			return
		}
		httpx.OK(c, gin.H{"unread": count})
	})
	r.POST("/v1/me/notifications/read-all", a.requireAnyUser(), a.readNotifications)
	r.POST("/v1/me/notifications/:id/read", a.requireAnyUser(), a.readNotifications)
	r.GET("/v1/me/notification-preferences", a.requireAnyUser(), a.notificationPreferences)
	r.PUT("/v1/me/notification-preferences", a.requireAnyUser(), a.notificationPreferences)
	r.GET("/admin/notifications/smtp", a.requireRoles("platform_admin", "tech_admin"), a.smtpSettings)
	r.PUT("/admin/notifications/smtp", a.requireRoles("platform_admin", "tech_admin"), a.smtpSettings)
	r.POST("/admin/notifications/smtp/test", a.requireRoles("platform_admin", "tech_admin"), a.testSMTP)
	r.GET("/admin/notifications/deliveries", a.requireRoles("platform_admin", "tech_admin", "ops_admin", "audit_readonly"), a.notificationDeliveries)
	r.POST("/admin/notifications/deliveries/:id/retry", a.requireRoles("platform_admin", "tech_admin", "ops_admin"), a.retryNotificationDelivery)
}
func (a *App) notificationError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, notifications.ErrNotFound):
		httpx.Abort(c, 404, "notification_not_found", "通知不存在", false)
	case errors.Is(err, notifications.ErrIneligible):
		httpx.Abort(c, 409, "notification_ineligible", "请确认 SMTP 已配置、自己的邮箱已验证，且任务未过期或已发送", false)
	case errors.Is(err, mail.ErrSettings):
		httpx.Abort(c, 400, "invalid_smtp_settings", "请检查 SMTP 地址、端口、发件邮箱及安全连接配置", false)
	case errors.Is(err, mail.ErrUnavailable):
		httpx.Abort(c, 503, "smtp_unavailable", "邮件服务不可用，请检查 SMTP 及平台加密配置", true)
	default:
		httpx.Abort(c, 500, "internal_error", "通知服务暂不可用", true)
	}
	return true
}
func (a *App) listNotifications(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	p := a.currentPrincipal(c)
	page, err := a.Notifications.List(c.Request.Context(), p.UserID, p.BrandID, c.Query("before"), c.Query("unread") == "1")
	if a.notificationError(c, err) {
		return
	}
	httpx.OK(c, page)
}
func (a *App) readNotifications(c *gin.Context) {
	p := a.currentPrincipal(c)
	if a.notificationError(c, a.Notifications.Read(c.Request.Context(), p.UserID, p.BrandID, c.Param("id"))) {
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}
func (a *App) notificationPreferences(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	p := a.currentPrincipal(c)
	if c.Request.Method == http.MethodPut {
		var body struct {
			EmailEnabled *bool `json:"email_enabled"`
		}
		if c.ShouldBindJSON(&body) != nil || body.EmailEnabled == nil {
			httpx.Abort(c, 400, "invalid_request", "请设置邮件通知偏好", false)
			return
		}
		if a.notificationError(c, a.Notifications.SetEmailEnabled(c.Request.Context(), p.UserID, p.BrandID, *body.EmailEnabled)) {
			return
		}
	}
	enabled, err := a.Notifications.EmailEnabled(c.Request.Context(), p.UserID, p.BrandID)
	if a.notificationError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"email_enabled": enabled, "smtp_available": a.Mail.Available()})
}
func (a *App) smtpSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.Method == http.MethodPut {
		if !a.requireConfirm(c) {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
		var body mail.SettingsInput
		if c.ShouldBindJSON(&body) != nil {
			httpx.Abort(c, 400, "invalid_request", "配置字段无效", false)
			return
		}
		item, err := a.Mail.Update(c.Request.Context(), body)
		if a.notificationError(c, err) {
			return
		}
		_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "notifications.smtp.configure", ResourceType: "mail_settings", ResourceID: "platform", After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
		httpx.OK(c, gin.H{"item": item})
		return
	}
	item, err := a.Mail.Settings(c.Request.Context())
	if a.notificationError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"item": item})
}
func (a *App) testSMTP(c *gin.Context) {
	if !a.emailRateLimit(c, "smtp_test") {
		return
	}
	notificationID, err := a.Notifications.QueueTest(c.Request.Context(), a.currentPrincipal(c).UserID)
	if a.notificationError(c, err) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "notifications.smtp.test", ResourceType: "notification", ResourceID: notificationID, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	c.JSON(http.StatusAccepted, gin.H{"notification_id": notificationID, "status": "pending"})
}
func (a *App) notificationDeliveries(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, err := a.Notifications.Deliveries(c.Request.Context(), c.Query("status"))
	if a.notificationError(c, err) {
		return
	}
	httpx.OK(c, page)
}
func (a *App) retryNotificationDelivery(c *gin.Context) {
	if !a.emailRateLimit(c, "notification_retry") {
		return
	}
	if a.notificationError(c, a.Notifications.Retry(c.Request.Context(), c.Param("id"))) {
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{ActorUserID: a.currentPrincipal(c).UserID, Action: "notifications.email.retry", ResourceType: "email_delivery", ResourceID: c.Param("id"), IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID)})
	httpx.OK(c, gin.H{"ok": true})
}
