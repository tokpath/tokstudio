package portal

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
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
			delete(v, "reminder_seconds")
			delete(v, "escalate_seconds")
			v["reminder_minutes"] = riskMinutes(rule.ReminderSeconds)
			v["escalate_minutes"] = riskMinutes(rule.EscalateSeconds)
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
		p["columns"] = []gin.H{col("name", "规则名称", "text"), col("metric_name", "指标", "text"), col("warning", "预警阈值", "decimal"), col("critical", "严重阈值", "decimal"), col("direction_name", "触发方向", "text"), col("recipient_names", "通知负责人", "text"), col("escalation_recipient_names", "升级负责人", "text"), col("reminder_minutes", "提醒间隔（分钟）", "decimal"), col("escalate_minutes", "升级等待时间（分钟）", "decimal"), col("enabled", "启用", "boolean")}
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
			fields[i].Help = "仅统计本经营方，不汇总下属经营方。剩余销售空间按上级供货口径计算，不是直接可售出的客户ATG；计算方式为：采购可用余额＋获准超卖额度−已承诺责任−待交付责任。采购可用余额是向上级采购并已交付的ATG扣除调用预占后的余额，不含未付款采购和超卖额度；这两个指标及已用超卖额度不适用于没有上级采购关系的平台。在途及待结算请求包含已受理、执行中、已提交及待补用量的请求。"
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
		case "reminder_minutes":
			fields[i].Help = "风险持续时重复提醒的间隔，按分钟填写；至少1分钟，最多43200分钟（30天）。60分钟为1小时。"
		case "escalate_minutes":
			fields[i].Help = "本次预警持续达到此时长后升为严重并通知升级负责人，按分钟填写；至少1分钟，最多43200分钟（30天）。1440分钟为1天；达到严重阈值时会直接通知。"
		}
	}
	return fields
}

// Keep the scheduler and existing saved durations in seconds. The portal uses
// minutes, including fractional minutes, without changing saved rule timing.
func riskMinutes(seconds int64) string {
	return strconv.FormatFloat(float64(seconds)/60, 'f', -1, 64)
}

func riskSeconds(minutes ledger.Decimal) (int64, bool) {
	if minutes.Cmp(ledger.FromInt64(1)) < 0 || minutes.Cmp(ledger.FromInt64(43200)) > 0 {
		return 0, false
	}
	value, _ := minutes.Rat().Float64()
	return int64(math.Round(value * 60)), true
}

type riskRuleInput struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Metric               string          `json:"metric"`
	Warning              ledger.Decimal  `json:"warning"`
	Critical             ledger.Decimal  `json:"critical"`
	Direction            string          `json:"direction"`
	Recipients           json.RawMessage `json:"recipients"`
	EscalationRecipients json.RawMessage `json:"escalation_recipients"`
	ReminderMinutes      ledger.Decimal  `json:"reminder_minutes"`
	EscalateMinutes      ledger.Decimal  `json:"escalate_minutes"`
	Enabled              bool            `json:"enabled"`
	ExpectedVersion      int64           `json:"expected_version"`
}

func (in riskRuleInput) rule(operator string) (operations.RiskRule, bool) {
	reminder, reminderOK := riskSeconds(in.ReminderMinutes)
	escalate, escalateOK := riskSeconds(in.EscalateMinutes)
	return operations.RiskRule{OperatorID: operator, ID: in.ID, Name: in.Name, Metric: in.Metric, Warning: in.Warning, Critical: in.Critical, Direction: in.Direction, Recipients: in.Recipients, EscalationRecipients: in.EscalationRecipients, ReminderSeconds: reminder, EscalateSeconds: escalate, Enabled: in.Enabled}, reminderOK && escalateOK
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
