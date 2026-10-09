package app

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) channelOnboarding(c *gin.Context) {
	item, err := a.Identity.ChannelOnboarding(c.Request.Context(), *a.currentPrincipal(c), c.Param("id"))
	if a.abortCustomerError(c, err) {
		return
	}
	models, err := a.Catalog.ListChannelModels(c.Request.Context(), c.Param("id"), false)
	out := gin.H{"item": item, "models_available": err == nil}
	if err == nil {
		count := 0
		for _, model := range models {
			if model.EffectiveEnabled {
				count++
			}
		}
		out["model_count"] = count
	}
	httpx.OK(c, out)
}
