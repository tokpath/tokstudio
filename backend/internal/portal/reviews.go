package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"strings"
	"time"
)

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
