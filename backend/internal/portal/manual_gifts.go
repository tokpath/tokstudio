package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"time"
)

func (a *Application) manualGiftsResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows, next, e := a.Commerce.ListManualGifts(c.Request.Context(), nil, actor, op.ID, "", c.Query("cursor"), 50, c.Query("q"), c.Query("status"))
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		items = append(items, entity(r.ID, "delivered", 0, r))
	}
	p["search_supported"], p["status_supported"] = true, true
	p["title"] = "定向赠送与服务补偿"
	p["description"] = "独立赠送来源，PaidUSD为0，不修改正确原交易、现金或佣金。必须有明确原因、凭据和真实销售覆盖。列表可按客户编号查询。"
	p["items"] = items
	p["next_cursor"] = next
	p["columns"] = []gin.H{col("customer_id", "当前服务客户", "text"), col("amount_atg", "独立赠送ATG", "atg"), col("reason", "原因", "text"), col("evidence", "关联事实与依据", "text"), col("effective_at", "本次生效", "datetime"), col("expires_at", "到期（空为不限期）", "datetime"), col("grant_id", "独立权益来源", "text")}
	p["operations"] = []operation{{Name: "issue", Label: "核实并发放独立赠送", Method: "POST", Path: base, Fields: []field{f("customer_id", "本经营方当前客户", "text"), selectField("reason", "明确赠送原因", "compensation|独立服务补偿", "targeted_gift|受权定向赠送"), f("amount_atg", "本次新增赠送ATG", "decimal"), optional("expires_at", "到期时刻（留空不限期）", "datetime"), f("evidence", "授权、事实和原事项关联依据", "textarea")}, Confirmation: "确认独立发放本次赠送，不改原正确交易，不将赠送视作有偿消费或分佣基数。新增责任受真实覆盖检查。"}}
	a.resourceResponse(c, p)
}
func (a *Application) issueManualGift(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		CustomerID string         `json:"customer_id"`
		AmountATG  ledger.Decimal `json:"amount_atg"`
		Reason     string         `json:"reason"`
		Evidence   string         `json:"evidence"`
		ExpiresAt  *time.Time     `json:"expires_at"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	key, _ := key(c)
	raw, e := a.command(c, "manual-gift:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		ownership, e := a.Organization.CurrentOwnership(ctx, tx, in.CustomerID)
		if e != nil {
			return nil, e
		}
		if ownership.OperatorID != op.ID {
			return nil, organization.ErrForbidden
		}
		out, e := a.Commerce.IssueManualGift(ctx, tx, commerce.ManualGiftInput{ActorStaffID: actor, OperatorID: op.ID, CustomerID: in.CustomerID, BusinessKey: key, Reason: in.Reason, Evidence: in.Evidence, AmountATG: in.AmountATG, ExpiresAt: in.ExpiresAt, ExpectedOwnershipVersion: ownership.Version})
		return commandResult(out.ID, "delivered"), e
	})
	jsonResponse(c, raw, e)
}
