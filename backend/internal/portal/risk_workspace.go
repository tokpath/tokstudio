package portal

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
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
	choices, staffNames, err := a.riskStaffChoices(ctx, op.ID)
	if err != nil {
		handleError(c, err)
		return
	}
	create.Fields = riskRuleFields(businessFields(create), choices, op.Kind == organization.Platform)
	p["operations"] = []operation{create}
	p["row_operations"] = []operation{}
	items := []any{}
	if view == "rules" {
		for _, rule := range rules {
			state := "disabled"
			if rule.Enabled {
				state = "enabled"
			}
			v := values(rule)
			v["name"], v["metric_name"] = rule.DisplayName(), operations.RiskMetricName(rule.Metric)
			v["direction_name"] = "达到或高于阈值"
			if rule.Direction == "below" {
				v["direction_name"] = "达到或低于阈值"
			}
			v["recipient_names"] = riskRecipientNames(rule.Recipients, staffNames)
			v["escalation_recipient_names"] = riskRecipientNames(rule.EscalationRecipients, staffNames)
			row := entity(rule.ID, state, rule.Version, v)
			row["allowed_actions"] = []string{"revise_rule"}
			items = append(items, row)
		}
		fields := riskRuleFields(append([]field(nil), create.Fields...), choices, op.Kind == organization.Platform)
		for i := range fields {
			if fields[i].Name == "id" {
				fields[i].Kind = "hidden"
				fields[i].Required = true
			}
		}
		p["row_operations"] = []operation{{Name: "revise_rule", Label: "修订此预警规则", Method: "POST", Path: base, Fields: fields, Description: "沿用此规则真实版本，修改通知阈值、负责人、提醒、升级或启停；不会把风险观察记录当作规则保存。"}}
		p["columns"] = []gin.H{col("name", "规则名称", "text"), col("metric_name", "指标", "text"), col("warning", "预警阈值", "decimal"), col("critical", "严重阈值", "decimal"), col("direction_name", "触发方向", "text"), col("recipient_names", "通知负责人", "text"), col("escalation_recipient_names", "升级负责人", "text"), col("reminder_seconds", "提醒间隔秒数", "text"), col("escalate_seconds", "升级等待秒数", "text"), col("enabled", "启用", "boolean")}
	} else {
		states, err := a.Operations.ListRisks(ctx, actor, op.ID)
		if err != nil {
			handleError(c, err)
			return
		}
		for _, state := range states {
			v := values(state)
			v["name"] = "历史预警规则"
			for _, rule := range rules {
				if rule.ID == state.RuleID {
					v["metric"] = rule.Metric
					v["metric_name"] = operations.RiskMetricName(rule.Metric)
					v["name"] = rule.DisplayName()
					break
				}
			}
			row := entity(state.RuleID, state.Status, 0, v)
			row["allowed_actions"] = []string{}
			items = append(items, row)
		}
		p["columns"] = []gin.H{col("name", "风险规则", "text"), col("metric_name", "指标", "text"), col("value", "实际测量值", "decimal"), col("status", "当前风险", "status"), col("opened_at", "本次风险开始", "datetime"), col("last_notified_at", "最近通知", "datetime")}
	}
	p["items"] = items
	a.resourceResponse(c, p)
}

func riskRuleFields(fields []field, choices []gin.H, platform bool) []field {
	for i := range fields {
		switch fields[i].Name {
		case "id", "expected_version":
			fields[i].Kind = "hidden"
		case "name":
			fields[i].Help = "用于风险观察与通知，最多100字；留空时按所选指标自动命名。"
		case "metric":
			fields[i].Help = "ATG指标使用ATG阈值；在途及待结算请求统计已受理、执行中、已提交及待补用量的请求。规则仅作用于当前经营方；待核实上游成本数仅供平台选择。"
			if !platform {
				options := []gin.H{}
				for _, option := range fields[i].Options {
					if option["value"] != "unverified_costs" {
						options = append(options, option)
					}
				}
				fields[i].Options = options
			}
		case "warning", "critical":
			fields[i].Help = "达到阈值即触发。低于方向的严重阈值不能高于预警阈值；高于方向则相反。"
		case "recipients", "escalation_recipients":
			fields[i].Options = choices
			fields[i].Help = "选择本经营方在岗人员，站内通知始终保留；邮箱已验证时同时发送邮件。"
		case "reminder_seconds":
			fields[i].Help = "风险持续时重复提醒的间隔，单位为秒；至少60秒，最多30天。"
		case "escalate_seconds":
			fields[i].Help = "预警从本次风险开始持续达到此时长后升为严重并通知升级负责人，单位为秒；至少60秒，最多30天。严重阈值可直接触发严重通知。"
		}
	}
	return fields
}

// ListRiskRules has already checked operations.manage in the exact workspace.
// People offered as risk owners are active members of that same workspace;
// managing a rule does not grant access to another operator's staff directory.
func (a *Application) riskStaffChoices(ctx context.Context, operator string) ([]gin.H, map[string]string, error) {
	var rows []struct {
		ID, DisplayName, Email, Status, MembershipStatus string
	}
	err := a.DB.WithContext(ctx).Table("portal_staff AS s").Select("s.id,s.display_name,s.email,s.status,m.status AS membership_status").Joins("JOIN organization_staff_memberships AS m ON m.staff_id=s.id").Where("m.operator_id=?", operator).Order("s.display_name,s.email,s.id").Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	choices := []gin.H{}
	names := map[string]string{}
	for _, row := range rows {
		label := strings.TrimSpace(row.DisplayName)
		if label == "" {
			label = row.Email
		} else if label != row.Email && row.Email != "" {
			label += " · " + row.Email
		}
		names[row.ID] = label
		if row.Status == "active" && row.MembershipStatus == "active" {
			choices = append(choices, gin.H{"value": row.ID, "label": label})
		}
	}
	return choices, names, nil
}

func riskRecipientNames(raw json.RawMessage, names map[string]string) string {
	var ids []string
	_ = json.Unmarshal(raw, &ids)
	labels := make([]string, 0, len(ids))
	for _, id := range ids {
		label := names[id]
		if label == "" {
			label = "已离岗负责人"
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return "未设置"
	}
	return strings.Join(labels, "、")
}
