package portal

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"strings"
	"time"
)

func (a *Application) auditResource(c *gin.Context, op organization.Operator, actor string, p gin.H) {
	ctx := c.Request.Context()
	choices := []gin.H{}
	allowed := map[string]bool{}
	addView := func(key, label string) {
		allowed[key] = true
		choices = append(choices, gin.H{"value": key, "label": label})
	}
	if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "reports.view") == nil {
		addView("organization", "组织与账号")
	}
	brandingAllowed := a.Organization.RequireBrandBranding(ctx, nil, actor, op.BrandID)
	if op.Kind == organization.Channel {
		brandingAllowed = a.Organization.RequireChannelBranding(ctx, nil, actor, op.ID)
	}
	if brandingAllowed == nil {
		addView("branding", "品牌外观")
	}
	if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "ledger.read") == nil {
		addView("commerce", "订单与交付")
		addView("incentives", "推广与佣金")
		addView("remediation", "退款、纠错与接管")
	}
	view := c.Query("view")
	if view == "" && len(choices) > 0 {
		view = choices[0]["value"].(string)
	}
	if !allowed[view] {
		handleError(c, organization.ErrForbidden)
		return
	}

	var rows []organization.AuditEvent
	var next string
	var e error
	type query func(context.Context, *gorm.DB, string, string, string, int) ([]organization.AuditEvent, string, error)
	queries := map[string]query{"organization": a.Organization.ListAuditEvents, "branding": a.Branding.ListAuditEvents, "commerce": a.Commerce.ListAuditEvents, "incentives": a.Incentives.ListAuditEvents}
	if view == "remediation" {
		at, id, err := organization.ParseAuditCursor(c.Query("cursor"))
		if err != nil {
			handleError(c, err)
			return
		}
		events, err := a.Remediation.ListEvents(ctx, actor, op.ID, 51, at, id)
		e = err
		for _, event := range events {
			rows = append(rows, organization.AuditEvent{ID: event.ID, OperatorID: event.OperatorID, SubjectID: event.SubjectID, Action: event.Action, ActorID: event.ActorID, Evidence: event.Evidence, Data: event.Data, CreatedAt: event.CreatedAt})
		}
		if len(rows) > 50 {
			rows = rows[:50]
			next = organization.NextAuditCursor(rows, 50)
		}
	} else if q, ok := queries[view]; ok {
		rows, next, e = q(ctx, nil, actor, op.ID, c.Query("cursor"), 50)
	} else {
		handleError(c, organization.ErrInvalidInput)
		return
	}
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		items = append(items, entity(r.ID, "recorded", 0, r))
	}
	p["title"] = "操作审计"
	p["summary"] = []gin.H{{"label": "请求内容", "value": "不长期保存"}, {"label": "异步结果暂存", "value": fmt.Sprint(int64(a.Usage.ResultRetention / time.Hour)), "unit": "小时"}, {"label": "导出文件有效期", "value": "24", "unit": "小时"}}
	p["description"] = "只显示各领域实际保存的操作主体、依据与前后记录。空白代表原事件未记录此字段。每个领域独立分页。"
	p["items"] = items
	p["next_cursor"] = next
	p["views"] = choices
	p["columns"] = []gin.H{col("action", "操作事实", "text"), col("subject_id", "关联主体", "text"), col("actor_id", "实际操作者", "text"), col("evidence", "依据", "text"), col("before", "变更前记录", "details"), col("after", "变更后记录", "details"), col("data", "原始业务事实", "details"), col("created_at", "记录时间", "datetime")}
	a.resourceResponse(c, p)
}
func (a *Application) approvalsResource(c *gin.Context, op organization.Operator, actor string, p gin.H) {
	ctx := c.Request.Context()
	items := []any{}
	if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "refunds.manage") == nil {
		cases, e := a.Remediation.Cases(ctx, op.ID, "")
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range cases {
			if r.State == "pending_review" {
				items = append(items, entity(r.ID, r.State, 1, gin.H{"kind": "客户退款", "amount_money": r.AmountMoney, "requested_by": r.RequestedBy, "created_at": r.CreatedAt, "workspace": "/manage/" + op.ID + "/refunds?record=" + r.ID}))
			}
		}
	}
	// The combined review page points to each authoritative workflow. It cannot
	// bypass the domain's independent reviewer and version requirements.
	if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "cooperation.manage") == nil && a.Organization.RequirePermission(ctx, nil, actor, op.ID, "refunds.manage") == nil {
		cases, e := a.Remediation.Takeovers(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range cases {
			if r.State == "awaiting_approval" {
				items = append(items, entity(r.ID, r.State, 1, gin.H{"kind": "渠道接管", "amount_money": r.SettlementDueMoney, "requested_by": r.RequestedBy, "created_at": r.CreatedAt, "workspace": "/manage/" + op.ID + "/takeovers?record=" + r.ID}))
			}
		}
	}
	if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "withdrawals.manage") == nil {
		rows, _, e := a.Incentives.WithdrawalsForOperator(ctx, nil, actor, op.ID, "", 100)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, r := range rows {
			if r.Status == "requested" {
				items = append(items, entity(r.ID, r.Status, 1, gin.H{"kind": "佣金提现", "amount_money": r.AmountMoney, "requested_by": r.CustomerID, "created_at": r.CreatedAt, "workspace": "/manage/" + op.ID + "/commissions?section=requests&record=" + r.ID}))
			}
		}
	}
	p["title"] = "待复核事项"
	p["description"] = "集中查看需要审核的事项，进入对应记录办理。是否需要独立复核，按各业务规则执行。"
	p["items"] = items
	p["columns"] = []gin.H{col("kind", "事项", "text"), col("amount_money", "责任结算币种", "money"), col("requested_by", "申请人", "text"), col("created_at", "申请时间", "datetime"), col("workspace", "进入原业务复核", "link")}
	a.resourceResponse(c, p)
}
func (a *Application) staffMembershipAction(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Role   string `json:"role"`
		Status string `json:"status"`
		Grants string `json:"grants"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "staff-membership:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		members, e := a.Organization.ListMemberships(c.Request.Context(), tx, actor, op.ID)
		if e != nil {
			return nil, e
		}
		staff := ""
		for _, member := range members {
			if member.ID == c.Param("id") {
				staff = member.StaffID
			}
		}
		if staff == "" {
			return nil, organization.ErrNotFound
		}
		grants := []string{}
		for _, grant := range strings.Split(in.Grants, ",") {
			if grant = strings.TrimSpace(grant); grant != "" {
				grants = append(grants, grant)
			}
		}
		e = a.Organization.SetMembership(c.Request.Context(), tx, organization.SetMembershipInput{ActorStaffID: actor, OperatorID: op.ID, StaffID: staff, Role: in.Role, Status: in.Status, Grants: grants})
		return commandResult(c.Param("id"), "updated"), e
	})
	jsonResponse(c, raw, e)
}
func (a *Application) platformRequestsResource(c *gin.Context, op organization.Operator, actor string, p gin.H) {
	from, to := time.Now().Add(-24*time.Hour), time.Now()
	var e error
	if c.Query("from") != "" {
		from, e = time.Parse(time.RFC3339, c.Query("from"))
		if e != nil {
			handleError(c, organization.ErrInvalidInput)
			return
		}
	}
	if c.Query("to") != "" {
		to, e = time.Parse(time.RFC3339, c.Query("to"))
		if e != nil {
			handleError(c, organization.ErrInvalidInput)
			return
		}
	}
	rows, e := a.Usage.OperationalRequests(c.Request.Context(), nil, op.ID, "", from, to, 100)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		items = append(items, entity(r.ID, r.Status, 0, r))
	}
	p["title"] = "请求与成本证据"
	p["description"] = "当前平台经营范围最近24小时的前100条业务事实；查看更长范围请使用受权导出。预估占用是调用前锁定的应收上限，实际用量计价是按真实用量和受理时销售价算出的应收。对照这两列可判断预估偏高或偏低。无正文、明文Key或供应商凭据。未知供应成本单列。"
	p["items"] = items
	p["columns"] = []gin.H{
		col("customer_id", "客户", "text"),
		col("model_id", "公开模型", "text"),
		col("status", "请求状态", "status"),
		col("reason", "失败原因", "text"),
		{"key": "estimate_atg", "label": "预估占用", "kind": "atg", "help": "调用前锁定的应收上限。高于实际用量计价说明占用偏保守，低于实际用量计价说明预估没有盖住真实应收。"},
		{"key": "actual_atg", "label": "实际用量计价", "kind": "atg", "help": "按真实用量和受理时的销售价计算的应收。客户余额足够时与实扣相同。"},
		col("charged_atg", "实扣ATG", "atg"),
		col("supplier_reference_costs", "参考成本（原币）", "text"),
		col("supplier_reported_costs", "供应商报告成本（原币）", "text"),
		col("unverified_attempts", "成本证据状态", "status"),
		col("accepted_at", "受理时间", "datetime"),
	}
	a.resourceResponse(c, p)
}
