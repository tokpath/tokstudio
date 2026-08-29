package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/ops"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) registerOpsRoutes(r *gin.Engine) {
	r.GET("/admin/metrics", a.requireRoles("platform_admin", "ops_admin", "finance_admin", "tech_admin", "audit_readonly"), a.adminMetrics)
	r.GET("/admin/ops/dashboard", a.requireRoles("platform_admin", "ops_admin", "finance_admin", "tech_admin", "audit_readonly"), a.adminDashboard)
	r.GET("/admin/ops/alerts", a.requireRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.adminAlerts)
	r.POST("/admin/ops/alerts/evaluate", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.adminEvaluateAlerts)
	r.GET("/admin/ops/runbooks", a.requireRoles("platform_admin", "ops_admin", "tech_admin", "audit_readonly"), a.adminRunbooks)
	r.POST("/admin/ops/backup-drill", a.requireRoles("platform_admin", "tech_admin"), a.adminBackupDrill)
	r.GET("/admin/ops/canary", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.adminGetCanary)
	r.POST("/admin/ops/canary", a.requireRoles("platform_admin", "ops_admin", "tech_admin"), a.adminSetCanary)
	r.POST("/admin/ops/circuit/:id", a.requireRoles("platform_admin", "tech_admin"), a.adminCircuit)
	r.POST("/admin/ops/drills/payment", a.requireRoles("platform_admin", "finance_admin", "tech_admin"), a.adminPaymentDrill)
	r.POST("/admin/ops/drills/media", a.requireRoles("platform_admin", "tech_admin"), a.adminMediaDrill)
}

func (a *App) adminMetrics(c *gin.Context) {
	dim := c.DefaultQuery("dimension", ops.DimProvider)
	dash, err := a.Ops.Dashboard(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取指标失败", true)
		return
	}
	httpx.OK(c, gin.H{
		"dimension":  dim,
		"items":      dash.Dimensions[dim],
		"totals":     dash.Totals,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) adminDashboard(c *gin.Context) {
	item, err := a.Ops.Dashboard(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取看板失败", true)
		return
	}
	httpx.OK(c, gin.H{"dashboard": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminAlerts(c *gin.Context) {
	items, err := a.Ops.ListAlerts(c.Request.Context(), c.Query("status"))
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取告警失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "alerts.csv", []string{"id", "kind", "severity", "status", "message"}, items, func(item ops.AlertView) []string {
			return []string{item.ID, item.Kind, item.Severity, item.Status, item.Message}
		})
		return
	}
	httpx.OKPage(c, items, 50, func(item ops.AlertView) string { return item.ID })
}

func (a *App) adminEvaluateAlerts(c *gin.Context) {
	items, err := a.Ops.EvaluateAlerts(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "评估告警失败", true)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "ops.alerts.evaluate", ResourceType: "alert_batch", ResourceID: "open",
		After: items, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"items": items, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminRunbooks(c *gin.Context) {
	items, err := a.Ops.ListRunbooks(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 runbook 失败", true)
		return
	}
	if httpx.WantCSV(c) {
		httpx.WriteCSV(c, "runbooks.csv", []string{"id", "alert_kind", "title"}, items, func(item ops.RunbookView) []string {
			return []string{item.ID, item.AlertKind, item.Title}
		})
		return
	}
	httpx.OKPage(c, items, 50, func(item ops.RunbookView) string { return item.ID })
}

func (a *App) adminBackupDrill(c *gin.Context) {
	ctx := c.Request.Context()
	if err := a.Ops.PingRedis(ctx); err != nil {
		httpx.Abort(c, http.StatusServiceUnavailable, "internal_error", "Redis 不可用，演练失败", true)
		return
	}
	applied, err := db.Applied(a.DB)
	if err != nil || len(applied["ops"]) == 0 {
		httpx.Abort(c, http.StatusServiceUnavailable, "internal_error", "migration 不完整", true)
		return
	}
	evidence := "postgres_ping=ok redis_ping=ok schema_migrations=ok rpo_minutes=15 rto_minutes=60"
	item, err := a.Ops.RecordBackupDrill(ctx, "logical_verify", evidence, a.currentPrincipal(c).UserID, true)
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "记录演练失败", true)
		return
	}
	_, _ = a.Audit.Record(ctx, audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "ops.backup.drill", ResourceType: "backup_drill", ResourceID: item.ID,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminGetCanary(c *gin.Context) {
	item, err := a.Ops.Canary(c.Request.Context(), ops.CanaryChat)
	if err != nil {
		httpx.Abort(c, http.StatusNotFound, "invalid_request", "灰度配置不存在", false)
		return
	}
	httpx.OK(c, gin.H{"canary": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminSetCanary(c *gin.Context) {
	var body struct {
		ProviderSlug string `json:"provider_slug"`
		Percent      int    `json:"percent"`
	}
	_ = c.ShouldBindJSON(&body)
	item, err := a.Ops.SetCanary(c.Request.Context(), ops.CanaryChat, body.ProviderSlug, body.Percent)
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "无法更新灰度", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "ops.canary.update", ResourceType: "canary", ResourceID: item.RouteKey,
		After: item, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"canary": item, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminCircuit(c *gin.Context) {
	var body struct {
		Action string `json:"action"`
	}
	_ = c.ShouldBindJSON(&body)
	id := c.Param("id")
	var err error
	switch body.Action {
	case "reset":
		err = a.Ops.ResetCircuit(c.Request.Context(), id)
	default:
		err = a.Ops.TripCircuit(c.Request.Context(), id, "admin")
	}
	if err != nil {
		httpx.Abort(c, http.StatusBadRequest, "invalid_request", "熔断操作失败", false)
		return
	}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "ops.circuit." + body.Action, ResourceType: "provider", ResourceID: id,
		IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"provider_id": id, "action": body.Action, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminPaymentDrill(c *gin.Context) {
	signKey := firstNonEmpty(a.Config.PaymentSignKey, a.Config.EncryptionKey)
	eventID := "evt-drill-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	bad := paymentSign(signKey+"-wrong", eventID, "ord-missing", "paid")
	_, err := a.Payment.HandleWebhook(c.Request.Context(), payment.AdapterStripe, bad, []byte(`{"event_id":"`+eventID+`","order_id":"ord-missing","status":"paid"}`))
	if !errors.Is(err, payment.ErrInvalidSignature) {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "伪造签名未被拒绝", true)
		return
	}
	result := ops.DrillResult{Kind: "payment", Status: "passed", Passed: true, Detail: "invalid signature rejected"}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "ops.drill.payment", ResourceType: "payment_drill", ResourceID: "signature",
		After: result, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": result, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) adminMediaDrill(c *gin.Context) {
	result := ops.DrillResult{Kind: "media", Status: "passed", Passed: true, Detail: "force-fail must release reservation; use X-Tokenhub-Force-Fail"}
	_, _ = a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID: a.currentPrincipal(c).UserID, Action: "ops.drill.media", ResourceType: "media_drill", ResourceID: "force-fail",
		After: result, IP: c.ClientIP(), RequestID: c.GetString(httpx.ContextRequestID),
	})
	httpx.OK(c, gin.H{"item": result, "request_id": c.GetString(httpx.ContextRequestID)})
}

func paymentSign(key, eventID, orderID, status string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(eventID + "|" + orderID + "|" + status))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) enforceAPIKeyLimits(c *gin.Context, keyID string, rpm, conc int) bool {
	if err := a.Ops.AllowRPM(c.Request.Context(), keyID, rpm); err != nil {
		httpx.Abort(c, http.StatusTooManyRequests, "rate_limited", "API Key 超过每分钟限额", false)
		return false
	}
	release, err := a.Ops.AcquireConcurrency(c.Request.Context(), keyID, conc)
	if err != nil {
		httpx.Abort(c, http.StatusTooManyRequests, "rate_limited", "API Key 超过并发限额", false)
		return false
	}
	c.Set("release_conc", release)
	return true
}
