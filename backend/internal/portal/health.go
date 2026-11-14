package portal

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"gorm.io/gorm"
)

// InternalRouter has no brand or business endpoints. Deployment keeps this
// listener private; metrics additionally require a configured bearer or loopback.
func (a *Application) InternalRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), a.Metrics.HTTP())
	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/readyz", func(c *gin.Context) {
		if db.Readiness(c.Request.Context(), a.DB, AllMigrations()) != nil {
			c.JSON(503, gin.H{"status": "schema_not_ready"})
			return
		}
		if a.Redis == nil || a.Redis.Ping(c.Request.Context()).Err() != nil || a.ObjectStore == nil || a.ObjectStore.Ready(c.Request.Context()) != nil {
			c.JSON(503, gin.H{"status": "dependency_not_ready"})
			return
		}
		// Only the worker runs this listener. Its discovery loop must have
		// actually completed recently, in addition to the dependency checks.
		if time.Since(time.Unix(0, a.workerHeartbeat.Load())) > 2*time.Minute {
			c.JSON(503, gin.H{"status": "worker_not_ready"})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})
	r.GET("/metrics", a.metricsAccess, a.Metrics.Handler())
	return r
}

// ReleaseCheck is a one-shot operational check. Long-running business
// exceptions stay visible in the operator's worklists.
func (a *Application) ReleaseCheck(ctx context.Context) error {
	return a.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var overdue bool
		if err := tx.Raw(`SELECT EXISTS (
 SELECT 1 FROM durable_jobs WHERE (status='pending' AND available_at<now()-interval '15 minutes')
 OR (status='running' AND lease_until<now()-interval '15 minutes')
 UNION ALL
 SELECT 1 FROM commerce_events e WHERE e.published_at IS NULL AND e.created_at<now()-interval '15 minutes'
 AND NOT EXISTS (SELECT 1 FROM durable_jobs j WHERE j.queue='portal-commerce-notifications'
                 AND j.kind='commerce_notification' AND j.business_key=e.id::text)
 UNION ALL
 SELECT 1 FROM payments_webhook_inbox WHERE (state='pending' AND next_attempt_at<now()-interval '15 minutes')
 OR (state='processing' AND lease_until<now()-interval '15 minutes')
 UNION ALL
 SELECT 1 FROM operations_exports WHERE expires_at>now() AND
 ((state='pending' AND created_at<now()-interval '15 minutes') OR
 (state='processing' AND lease_until<now()-interval '15 minutes')))`).Row().Scan(&overdue); err != nil {
			return err
		}
		if overdue {
			return errors.New("background work is overdue; inspect the queues before completing release")
		}
		findings, err := a.Ledger.OperationalIntegrity(ctx, tx, "")
		if err != nil {
			return err
		}
		if len(findings) != 0 {
			return errors.New("ledger reconciliation failed; inspect the operator's reconciliation page")
		}
		return nil
	})
}
