package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

// A risk observation is a measured incident, never an editable rule revision.
// Alerts default to observations so overview counts continue to reflect risks.
func (a *Application) riskWorkspace(c *gin.Context, op organization.Operator, actor, resource, base string, p gin.H) {
	ctx := c.Request.Context()
	rules, err := a.Operations.ListRiskRules(ctx, actor, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	view := c.Query("view")
	if view == "" {
		view = "rules"
		if resource == "alerts" {
			view = "observations"
		}
	}
	if view != "rules" && view != "observations" {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	ruleView := gin.H{"value": "rules", "label": "预警规则"}
	observationView := gin.H{"value": "observations", "label": "风险观察"}
	p["views"] = []gin.H{ruleView, observationView}
	if resource == "alerts" {
		p["views"] = []gin.H{observationView, ruleView}
	}
	p["description"] = "规则配置决定何时通知、提醒和升级；风险观察保留实际测量状态。编辑规则不会修改采购余额、客户资产或历史风险事实。"
	create := operation{Name: "risk_rule", Label: "新增预警规则", Method: "POST", Path: base, Fields: []field{jsonField("新增风险规则")}}
	p["operations"] = []operation{create}
	p["row_operations"] = []operation{}
	items := []any{}
	if view == "rules" {
		for _, rule := range rules {
			state := "disabled"
			if rule.Enabled {
				state = "enabled"
			}
			row := entity(rule.ID, state, rule.Version, rule)
			row["allowed_actions"] = []string{"revise_rule"}
			items = append(items, row)
		}
		fields := businessFields(create)
		for i := range fields {
			if fields[i].Name == "id" {
				fields[i].Kind = "hidden"
			}
		}
		p["row_operations"] = []operation{{Name: "revise_rule", Label: "修订此预警规则", Method: "POST", Path: base, Fields: fields, Description: "沿用此规则真实版本，修改通知阈值、负责人、提醒、升级或启停；不会把风险观察记录当作规则保存。"}}
		p["columns"] = []gin.H{col("id", "规则编号", "text"), col("metric", "指标", "text"), col("warning", "预警阈值", "decimal"), col("critical", "严重阈值", "decimal"), col("direction", "触发方向", "text"), col("recipients", "通知负责人", "details"), col("escalation_recipients", "升级负责人", "details"), col("reminder_seconds", "提醒间隔秒数", "text"), col("escalate_seconds", "升级等待秒数", "text"), col("enabled", "启用", "boolean")}
	} else {
		states, err := a.Operations.ListRisks(ctx, actor, op.ID)
		if err != nil {
			handleError(c, err)
			return
		}
		for _, state := range states {
			v := values(state)
			for _, rule := range rules {
				if rule.ID == state.RuleID {
					v["metric"] = rule.Metric
					break
				}
			}
			row := entity(state.RuleID, state.Status, 0, v)
			row["allowed_actions"] = []string{}
			items = append(items, row)
		}
		p["columns"] = []gin.H{col("rule_id", "风险规则", "text"), col("metric", "指标", "text"), col("value", "实际测量值", "decimal"), col("status", "当前风险", "status"), col("opened_at", "本次风险开始", "datetime"), col("last_notified_at", "最近通知", "datetime")}
	}
	p["items"] = items
	a.resourceResponse(c, p)
}
