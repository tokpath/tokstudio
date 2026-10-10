package app

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
)

func (a *App) listCustomerSubchannels(c *gin.Context) {
	page, err := a.Identity.SearchCustomerSubchannels(c.Request.Context(), *a.currentPrincipal(c), c.Query("q"), c.Query("cursor"))
	if a.abortCustomerError(c, err) {
		return
	}
	httpx.OK(c, page)
}
func (a *App) customerPromotions(c *gin.Context) {
	items, err := a.Identity.CustomerPromotionChoices(c.Request.Context(), *a.currentPrincipal(c), c.Query("q"))
	if a.abortCustomerError(c, err) {
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
