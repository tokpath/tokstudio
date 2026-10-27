package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"time"
)

func (a *Application) restrictionsResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows, e := a.Organization.ListScopeFreezes(c.Request.Context(), nil, actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, row := range rows {
		items = append(items, restrictionEntity(row))
	}
	p["title"] = "账号与业务范围限制"
	p["description"] = "按具体动作限制当前经营范围，保留资产、历史资格与已受理责任。每个决策记录原因、客户说明和复核到期，解除需沿用当前版本。"
	p["items"] = items
	p["columns"] = []gin.H{col("scope_kind", "范围", "text"), col("scope_id", "主体", "text"), col("action", "受限动作", "text"), col("explanation", "对客户说明", "text"), col("review_due_at", "复核到期", "datetime")}
	p["operations"] = []operation{{Name: "freeze", Label: "新增业务限制", Method: "POST", Path: base, Fields: []field{f("scope_kind", "对象类型", "hidden"), f("scope_id", "限制对象", "text"), selectField("action", "限制动作", "registration|注册", "new_purchase|新购", "renewal|续费", "model_usage|模型调用", "activity|营销活动", "withdrawal|佣金提现", "wallet_refund|余额退款", "customer_login|登录", "all_new_business|所有新增业务"), defaultField("enabled", "启用限制", "hidden", true), defaultField("expected_version", "当前范围动作版本（首次0）", "integer", 0), f("explanation", "处理说明（向客户展示）", "textarea"), f("review_due_at", "复核时间", "datetime")}, Confirmation: "只对明确范围与动作生效，不转移或删除客户资产。设置限制必须有复核到期。"}}

	p["row_operations"] = []operation{{Name: "revise", Label: "变更此项已有业务限制", Method: "POST", Path: base, Fields: []field{f("scope_kind", "原范围类型", "hidden"), f("scope_id", "原范围主体", "hidden"), f("action", "原限制动作", "hidden"), f("expected_version", "当前版本", "hidden"), f("enabled", "继续限制", "boolean"), f("explanation", "处理说明（向客户展示）", "textarea"), restrictionReviewField()}}}
	for i := range items {
		r := items[i].(gin.H)
		v := r["values"].(gin.H)
		v["expected_version"] = r["version"]
	}
	a.resourceResponse(c, p)
}
func (a *Application) restrictionsCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		ScopeKind   string     `json:"scope_kind"`
		ScopeID     string     `json:"scope_id"`
		Action      string     `json:"action"`
		Enabled     bool       `json:"enabled"`
		Expected    int64      `json:"expected_version"`
		Evidence    string     `json:"evidence"`
		Explanation string     `json:"explanation"`
		ReviewDueAt *time.Time `json:"review_due_at"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "restriction:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		if in.Evidence == "" {
			in.Evidence = in.Explanation
		}
		out, e := a.Organization.SetScopeFreeze(c.Request.Context(), tx, organization.ScopeFreezeInput{ActorStaffID: actor, OperatorID: op.ID, ScopeKind: in.ScopeKind, ScopeID: in.ScopeID, Action: in.Action, Enabled: in.Enabled, ExpectedVersion: in.Expected, Evidence: in.Evidence, Explanation: in.Explanation, ReviewDueAt: in.ReviewDueAt})
		return gin.H{"id": out.ScopeID, "status": "recorded", "version": out.Version}, e
	})
	jsonResponse(c, raw, e)
}

func restrictionEntity(row organization.ScopeFreeze) gin.H {
	state := "resolved"
	if row.Enabled {
		state = "active"
	}
	return entity(row.ScopeKind+":"+row.ScopeID+":"+row.Action, state, row.Version, row)
}
func restrictionReviewField() field {
	v := f("review_due_at", "复核时间", "datetime")
	v.VisibleWhen = &fieldVisibility{Field: "enabled", Values: []string{"true"}}
	return v
}
