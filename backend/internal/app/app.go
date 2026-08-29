// Package app 组装模块。模块之间只通过公开接口通信。
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/media"
	"github.com/tokpath/tokstudio/backend/internal/ops"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

type App struct {
	Config     *config.Config
	DB         *gorm.DB
	Redis      *redis.Client
	Identity   *identity.Service
	Audit      *audit.Service
	Outbox     *outbox.Service
	Catalog    *catalog.Service
	Billing    *billing.Service
	Gateway    *gateway.Service
	Media      *media.Service
	Plans      *plans.Service
	Payment    *payment.Service
	Commission *commission.Service
	Ops        *ops.Service
	Logger     zerolog.Logger
}

func New(cfg *config.Config, gdb *gorm.DB, rdb *redis.Client, logger zerolog.Logger) *App {
	outboxSvc := outbox.New(gdb)
	auditSvc := audit.New(gdb, outboxSvc)
	catalogSvc := catalog.New(gdb)
	catalogSvc.SetURLPolicy(cfg.IsProduction(), cfg.UpstreamURLAllowlist)
	billingSvc := billing.New(gdb, outboxSvc)
	plansSvc := plans.New(gdb, outboxSvc)
	billingSvc.SetCoverer(plansSvc)
	store := media.NewStore(cfg.MediaStorePath, firstNonEmpty(cfg.MediaSignKey, cfg.EncryptionKey), cfg.PublicBaseURL)
	mediaSvc := media.New(gdb, catalogSvc, billingSvc, outboxSvc, store, cfg.ArkBaseURL, cfg.OpenRouterBaseURL)
	paySvc := payment.New(gdb, outboxSvc, plansSvc, billingSvc, firstNonEmpty(cfg.PaymentSignKey, cfg.EncryptionKey))
	idSvc := identity.New(gdb)
	commSvc := commission.New(gdb, outboxSvc)
	billingSvc.SetCommissioner(&commissionBridge{identity: idSvc, comm: commSvc})
	gw := gateway.New(gdb, catalogSvc, billingSvc, cfg.BifrostURL)
	opsSvc := ops.New(gdb, rdb)
	opsSvc.SetSources(&trafficBridge{gateway: gw}, &moneyBridge{billing: billingSvc}, &healthBridge{catalog: catalogSvc}, &roleBridge{identity: idSvc})
	gw.SetBreaker(opsSvc)
	return &App{
		Config:     cfg,
		DB:         gdb,
		Redis:      rdb,
		Identity:   idSvc,
		Audit:      auditSvc,
		Outbox:     outboxSvc,
		Catalog:    catalogSvc,
		Billing:    billingSvc,
		Gateway:    gw,
		Media:      mediaSvc,
		Plans:      plansSvc,
		Payment:    paySvc,
		Commission: commSvc,
		Ops:        opsSvc,
		Logger:     logger,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func AllMigrations() []db.ModuleMigrations {
	outboxName, outboxFS := outbox.Migrations()
	identityName, identityFS := identity.Migrations()
	auditName, auditFS := audit.Migrations()
	catalogName, catalogFS := catalog.Migrations()
	gatewayName, gatewayFS := gateway.Migrations()
	billingName, billingFS := billing.Migrations()
	mediaName, mediaFS := media.Migrations()
	plansName, plansFS := plans.Migrations()
	paymentName, paymentFS := payment.Migrations()
	commissionName, commissionFS := commission.Migrations()
	opsName, opsFS := ops.Migrations()
	return []db.ModuleMigrations{
		{Module: outboxName, FS: outboxFS},
		{Module: identityName, FS: identityFS},
		{Module: auditName, FS: auditFS},
		{Module: catalogName, FS: catalogFS},
		{Module: gatewayName, FS: gatewayFS},
		{Module: billingName, FS: billingFS},
		{Module: mediaName, FS: mediaFS},
		{Module: plansName, FS: plansFS},
		{Module: paymentName, FS: paymentFS},
		{Module: commissionName, FS: commissionFS},
		{Module: opsName, FS: opsFS},
	}
}

func (a *App) Migrate() error {
	return db.Apply(a.DB, AllMigrations())
}

func (a *App) Bootstrap(ctx context.Context) error {
	channelToken := a.Config.BootstrapChannel
	if channelToken == "" && a.Config.BootstrapAdmin != "" {
		channelToken = a.Config.BootstrapAdmin + "-b"
	}
	if err := a.Identity.Bootstrap(ctx, a.Config.BootstrapAdmin, a.Config.BootstrapUser, channelToken); err != nil {
		return err
	}
	if err := a.Catalog.Seed(ctx); err != nil {
		return err
	}
	if err := a.Billing.Seed(ctx); err != nil {
		return err
	}
	if err := a.Plans.Seed(ctx); err != nil {
		return err
	}
	if err := a.Commission.Seed(ctx); err != nil {
		return err
	}
	return a.Ops.Seed(ctx)
}

func (a *App) Router() *gin.Engine {
	if a.Config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(httpx.RequestID())
	r.Use(otelgin.Middleware(a.Config.OTELServiceName))
	r.Use(httpx.AccessLog(a.Logger))
	r.Use(httpx.CORS(a.Config.WebOrigin))

	r.GET("/healthz", a.healthz)
	r.GET("/readyz", a.readyz)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	r.GET("/admin/me", a.requireRoles("platform_admin", "finance_admin", "ops_admin", "tech_admin", "channel_admin", "audit_readonly"), a.adminMe)
	r.GET("/admin/audit-logs", a.requireRoles("platform_admin", "audit_readonly"), a.listAudit)
	r.POST("/admin/audit-probes", a.requireRoles("platform_admin"), a.createAuditProbe)
	r.GET("/admin/outbox/stats", a.requireRoles("platform_admin", "tech_admin"), a.outboxStats)
	a.registerAuthRoutes(r)
	a.registerGatewayRoutes(r)
	a.registerBillingRoutes(r)
	a.registerMediaRoutes(r)
	a.registerPlanRoutes(r)
	a.registerCommissionRoutes(r)
	a.registerOpsRoutes(r)
	return r
}

func (a *App) healthz(c *gin.Context) {
	httpx.OK(c, gin.H{
		"status":     "ok",
		"service":    "tokenhub-api",
		"version":    "0.1.0-m7",
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	checks := gin.H{}
	ready := true

	if err := db.Ping(ctx, a.DB); err != nil {
		checks["postgres"] = "error"
		ready = false
	} else {
		checks["postgres"] = "ok"
	}
	if err := redisx.Ping(ctx, a.Redis); err != nil {
		checks["redis"] = "error"
		ready = false
	} else {
		checks["redis"] = "ok"
	}
	applied, err := db.Applied(a.DB)
	if err != nil || len(applied["outbox"]) == 0 || len(applied["identity"]) == 0 || len(applied["audit"]) == 0 || len(applied["catalog"]) == 0 || len(applied["gateway"]) == 0 || len(applied["billing"]) == 0 || len(applied["media"]) == 0 || len(applied["plans"]) == 0 || len(applied["payment"]) == 0 || len(applied["commission"]) == 0 || len(applied["ops"]) == 0 {
		checks["migrations"] = "error"
		ready = false
	} else {
		checks["migrations"] = "ok"
	}
	if age, ok, err := redisx.WorkerHeartbeatAge(ctx, a.Redis); err != nil {
		checks["outbox_worker"] = "error"
	} else if !ok || age > 15*time.Second {
		checks["outbox_worker"] = "stale"
	} else {
		checks["outbox_worker"] = "ok"
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}
	c.JSON(status, gin.H{
		"status":     state,
		"checks":     checks,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) currentPrincipal(c *gin.Context) *identity.Principal {
	value, ok := c.Get("principal")
	if !ok {
		return nil
	}
	principal, _ := value.(*identity.Principal)
	return principal
}

func (a *App) requireConfirm(c *gin.Context) bool {
	if c.GetHeader("X-Tokenhub-Confirm") != "1" && c.Query("confirm") != "1" {
		httpx.Abort(c, http.StatusConflict, "confirm_required", "敏感操作需要二次确认", false)
		return false
	}
	principal := a.currentPrincipal(c)
	if principal != nil && a.Identity.TOTPEnabled(c.Request.Context(), principal.UserID) {
		code := c.GetHeader("X-Tokenhub-TOTP")
		if code == "" {
			code = c.Query("totp")
		}
		if err := a.Identity.VerifyTOTP(c.Request.Context(), principal.UserID, code, a.Config.EncryptionKey); err != nil {
			httpx.Abort(c, http.StatusConflict, "totp_required", "管理员已启用 2FA，需要有效 TOTP", false)
			return false
		}
	}
	return true
}

func (a *App) requireRoles(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal, err := a.Identity.Authenticate(c.Request.Context(), a.tokenFromRequest(c))
		if err != nil {
			httpx.Abort(c, http.StatusInternalServerError, "internal_error", "身份校验失败", true)
			return
		}
		if principal == nil {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "未授权", false)
			return
		}
		if !principal.HasRole(roles...) {
			httpx.Abort(c, http.StatusForbidden, "permission_denied", "权限不足", false)
			return
		}
		c.Set("principal", principal)
		c.Next()
	}
}

func (a *App) adminMe(c *gin.Context) {
	principal := a.currentPrincipal(c)
	totp, _ := a.Identity.TOTPStatus(c.Request.Context(), principal.UserID)
	httpx.OK(c, gin.H{
		"user_id":    principal.UserID,
		"email":      principal.Email,
		"roles":      principal.Roles,
		"totp":       totp,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) listAudit(c *gin.Context) {
	entries, err := a.Audit.Search(c.Request.Context(), audit.SearchQuery{
		Action: c.Query("action"), ResourceType: c.Query("resource_type"),
		ResourceID: c.Query("resource_id"), ActorUserID: c.Query("actor_user_id"),
		RequestID: c.Query("request_id"), Query: c.Query("q"), Limit: 50,
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取审计失败", true)
		return
	}
	httpx.OK(c, gin.H{"items": entries, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) createAuditProbe(c *gin.Context) {
	if a.Config.IsProduction() && !a.Config.AllowDemoProbes {
		httpx.Abort(c, http.StatusForbidden, "permission_denied", "生产环境禁止探测写入", false)
		return
	}
	principal := a.currentPrincipal(c)
	entry, err := a.Audit.Record(c.Request.Context(), audit.RecordInput{
		ActorUserID:  principal.UserID,
		Action:       "audit.probe",
		ResourceType: "system",
		ResourceID:   "m0",
		After:        map[string]string{"result": "ok"},
		IP:           c.ClientIP(),
		RequestID:    c.GetString(httpx.ContextRequestID),
	})
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "写入审计失败", true)
		return
	}
	httpx.Created(c, gin.H{"item": entry, "request_id": c.GetString(httpx.ContextRequestID)})
}

func (a *App) outboxStats(c *gin.Context) {
	stats, err := a.Outbox.Stats(c.Request.Context())
	if err != nil {
		httpx.Abort(c, http.StatusInternalServerError, "internal_error", "读取 outbox 失败", true)
		return
	}
	httpx.OK(c, gin.H{"stats": stats, "request_id": c.GetString(httpx.ContextRequestID)})
}
