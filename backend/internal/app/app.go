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
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/outbox"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
)

type App struct {
	Config   *config.Config
	DB       *gorm.DB
	Redis    *redis.Client
	Identity *identity.Service
	Audit    *audit.Service
	Outbox   *outbox.Service
	Logger   zerolog.Logger
}

func New(cfg *config.Config, gdb *gorm.DB, rdb *redis.Client, logger zerolog.Logger) *App {
	outboxSvc := outbox.New(gdb)
	auditSvc := audit.New(gdb, outboxSvc)
	return &App{
		Config:   cfg,
		DB:       gdb,
		Redis:    rdb,
		Identity: identity.New(gdb),
		Audit:    auditSvc,
		Outbox:   outboxSvc,
		Logger:   logger,
	}
}

func AllMigrations() []db.ModuleMigrations {
	outboxName, outboxFS := outbox.Migrations()
	identityName, identityFS := identity.Migrations()
	auditName, auditFS := audit.Migrations()
	return []db.ModuleMigrations{
		{Module: outboxName, FS: outboxFS},
		{Module: identityName, FS: identityFS},
		{Module: auditName, FS: auditFS},
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
	return a.Identity.Bootstrap(ctx, a.Config.BootstrapAdmin, a.Config.BootstrapUser, channelToken)
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
	return r
}

func (a *App) healthz(c *gin.Context) {
	httpx.OK(c, gin.H{
		"status":     "ok",
		"service":    "tokenhub-api",
		"version":    "0.1.0-m1",
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
	if err != nil || len(applied["outbox"]) == 0 || len(applied["identity"]) == 0 || len(applied["audit"]) == 0 {
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
	httpx.OK(c, gin.H{
		"user_id":    principal.UserID,
		"email":      principal.Email,
		"roles":      principal.Roles,
		"request_id": c.GetString(httpx.ContextRequestID),
	})
}

func (a *App) listAudit(c *gin.Context) {
	entries, err := a.Audit.List(c.Request.Context(), 50)
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
