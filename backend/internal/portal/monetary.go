package portal

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"strings"
)

func (a *Application) requireMonetaryOwner(ctx context.Context, tx *gorm.DB, actor string) error {
	var count int64
	if e := tx.WithContext(ctx).Table("organization_staff_memberships").Where("operator_id=? AND staff_id=? AND role='owner' AND status='active'", a.Installation.OperatorID, actor).Count(&count).Error; e != nil {
		return e
	}
	if count != 1 {
		return organization.ErrForbidden
	}
	return nil
}
func (a *Application) monetarySettings(c *gin.Context) {
	row, e := ledger.GetMonetary(c.Request.Context(), a.DB)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, row)
}
func (a *Application) monetaryCommand(c *gin.Context) {
	actor := subject(c).Staff.ID
	if e := a.requireMonetaryOwner(c.Request.Context(), a.DB, actor); e != nil {
		handleError(c, e)
		return
	}
	if c.FullPath() == "/v1/monetary/cancel" {
		var in struct {
			ExpectedVersion int64 `json:"expected_version"`
		}
		if !body(c, &in) {
			return
		}
		raw, e := a.command(c, "monetary:cancel", in, func(tx *gorm.DB) (any, error) {
			if e := a.requireMonetaryOwner(c.Request.Context(), tx, actor); e != nil {
				return nil, e
			}
			row, e := ledger.CancelMonetary(c.Request.Context(), tx, actor, in.ExpectedVersion)
			return gin.H{"id": "platform", "status": row.Status, "version": row.Version}, e
		})
		jsonResponse(c, raw, e)
		return
	}
	var in ledger.MonetaryInput
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "monetary:configure", in, func(tx *gorm.DB) (any, error) {
		if e := a.requireMonetaryOwner(c.Request.Context(), tx, actor); e != nil {
			return nil, e
		}
		row, e := ledger.ConfigureMonetary(c.Request.Context(), tx, actor, in)
		return gin.H{"id": "platform", "status": row.Status, "version": row.Version}, e
	})
	jsonResponse(c, raw, e)
}

// Preparatory configuration remains available; transactions and grants wait for
// the one global monetary foundation, independently of local business switches.
func (a *Application) monetaryTransactionGuard(c *gin.Context) {
	if c.Request.Method == "GET" || c.Request.Method == "HEAD" {
		return
	}
	resource := c.Param("resource")
	if resource == "" {
		path := c.FullPath()
		if path == "/v1/auth/registrations" || path == "/v1/auth/verify" {
			resource = "registration"
		}
		parts := strings.Split(path, "/")
		if len(parts) > 3 && parts[2] == "me" {
			resource = parts[3]
		}
		if len(parts) > 4 && parts[2] == "operators" {
			resource = parts[4]
		}
	}
	if contains([]string{"orders", "topups", "playground", "activities", "payments", "refunds", "withdrawals", "disputes", "takeovers", "exits", "gifts", "procurement", "registration"}, resource) {
		if _, e := ledger.RequireMonetary(c.Request.Context(), a.DB); e != nil {
			handleError(c, e)
		}
	}
}
