package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
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
		c.JSON(200, gin.H{"status": "ready"})
	})
	r.GET("/metrics", a.metricsAccess, a.Metrics.Handler())
	return r
}
