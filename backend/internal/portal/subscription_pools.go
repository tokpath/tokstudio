package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/modelpricing"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) subscriptionPoolsResource(c *gin.Context, op organization.Operator) {
	ctx := c.Request.Context()
	rows, err := a.Catalog.SubscriptionPools(ctx, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	var statements map[string]modelpricing.SubscriptionStatement
	if len(rows) > 0 && a.Organization.RequirePermission(ctx, nil, subject(c).Staff.ID, op.ID, "platform.manage") == nil {
		statements, err = a.ModelPricing.LatestSubscriptionStatements(ctx)
		if err != nil {
			handleError(c, err)
			return
		}
	}
	type poolView struct {
		catalog.SubscriptionPool
		LatestStatement *modelpricing.SubscriptionStatement `json:"latest_statement,omitempty"`
	}
	items := make([]poolView, 0, len(rows))
	for _, row := range rows {
		item := poolView{SubscriptionPool: row}
		if statement, ok := statements[row.ProviderID]; ok {
			item.LatestStatement = &statement
		}
		items = append(items, item)
	}
	c.JSON(200, gin.H{"items": items})
}

func (a *Application) subscriptionPoolsCommand(c *gin.Context, op organization.Operator) {
	var in struct {
		Action  string                            `json:"action"`
		Pool    *catalog.SubscriptionPoolInput    `json:"pool"`
		Account *catalog.SubscriptionAccountInput `json:"account"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	raw, err := a.command(c, "subscription-pools:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		cat := a.Catalog.WithDB(tx)
		switch in.Action {
		case "save_pool":
			if in.Pool != nil && in.Account == nil {
				id, err := cat.SaveSubscriptionPool(ctx, op.ID, *in.Pool)
				return commandResult(id, "saved"), err
			}
		case "save_account":
			if in.Account != nil && in.Pool == nil {
				id, err := cat.SaveSubscriptionAccount(ctx, op.ID, *in.Account)
				return commandResult(id, "saved"), err
			}
		case "remove_account":
			if in.Account != nil && in.Pool == nil {
				err := cat.RemoveSubscriptionAccount(ctx, op.ID, in.Account.PoolID, in.Account.ID, in.Account.ExpectedVersion)
				return commandResult(in.Account.ID, "removed"), err
			}
		}
		return nil, catalog.ErrInvalidInput
	})
	jsonResponse(c, raw, err)
}
