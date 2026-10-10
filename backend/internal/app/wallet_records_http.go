package app

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/platform/httpx"
	"github.com/tokpath/tokstudio/backend/internal/platform/pagecursor"
)

func (a *App) registerPersonalRecordsRoutes(r *gin.Engine) {
	r.GET("/v1/me/wallet-records", a.requireAnyUser(), a.personalRecords)
}
func (a *App) personalRecords(c *gin.Context) {
	userID := a.currentPrincipal(c).UserID
	cursor := c.Query("cursor")
	var result any
	var err error
	switch c.DefaultQuery("kind", "ledger") {
	case "ledger":
		result, err = a.Billing.PersonalLedger(c.Request.Context(), userID, cursor)
	case "orders":
		result, err = a.Payment.PersonalOrders(c.Request.Context(), userID, cursor)
	case "recoveries":
		result, err = a.Billing.PersonalRecoveries(c.Request.Context(), userID, cursor)
	default:
		err = pagecursor.ErrInvalid
	}
	if errors.Is(err, pagecursor.ErrInvalid) {
		httpx.Abort(c, 400, "invalid_cursor", "Records page expired. Start from the first page.", false)
		return
	}
	if err != nil {
		httpx.Abort(c, 500, "internal_error", "Records could not be loaded. Retry.", true)
		return
	}
	httpx.OK(c, result)
}
