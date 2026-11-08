package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) exitsResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows, e := a.Organization.ExitNotices(c.Request.Context(), actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	ownNotice := false
	for _, r := range rows {
		operator, e := a.Organization.GetOperator(c.Request.Context(), nil, r.OperatorID)
		if e != nil {
			handleError(c, e)
			return
		}
		state := "notified"
		kind := "OEM 停业告知"
		path := ""
		if r.Kind == organization.Channel {
			kind = "渠道退出通知"
			state = "pending"
			if operator.Lifecycle == "terminated" {
				state = "completed"
			}
			path = "/manage/" + op.ID + "/takeovers"
		}
		items = append(items, entity(r.ID, state, 0, gin.H{"operator_name": operator.Name, "kind_label": kind, "note": r.Note, "created_at": r.CreatedAt, "takeover_url": path}))
		if r.OperatorID == op.ID {
			ownNotice = true
		}
	}
	p["items"] = items
	p["description"] = "采购余额不退。OEM 自行处理停业及存量责任；渠道由所属品牌办理客户接管。"
	p["columns"] = []gin.H{col("operator_name", "经营方", "text"), col("kind_label", "事项", "text"), col("note", "说明", "text"), col("created_at", "通知时间", "datetime"), col("takeover_url", "查看渠道接管", "link")}
	p["operations"] = []operation{}
	if op.Kind != organization.Platform && op.Lifecycle != "terminated" && !ownNotice && a.Organization.RequireNoTakeover(c.Request.Context(), nil, op.ID) == nil {
		label, confirmation := "通知品牌办理退出", "通知所属品牌办理客户接管。采购余额不退；接管前继续由本渠道负责客户业务。"
		if op.Kind == organization.OEM {
			label = "告知平台停止经营"
			confirmation = "仅告知平台，不申请退款或接管。营业设置和客户、下属渠道的存量责任由本 OEM 自行处理。"
		}
		p["operations"] = []operation{{Name: "notify", Label: label, Method: "POST", Path: base, Permission: "cooperation.manage", Fields: []field{optional("note", "补充说明（可选）", "textarea")}, Confirmation: confirmation}}
	}
	a.resourceResponse(c, p)
}
func (a *Application) exitsCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Note string `json:"note"`
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "exit-notice:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		notice, e := a.Organization.ReportExit(c.Request.Context(), tx, actor, op.ID, in.Note)
		if e != nil {
			return nil, e
		}
		title, body := "渠道申请退出", "请在停业与接管页面查看并办理客户权益承接。双方资金交接在线下处理。"
		if op.Kind == organization.OEM {
			title = "OEM 已告知停止经营"
			body = "OEM 自行负责客户和下属渠道的存量事项，平台无需审批、退款或接管。"
		}
		recipients, e := a.Organization.OperatorNotificationStaff(c.Request.Context(), tx, notice.ParentID)
		if e != nil {
			return nil, e
		}
		for _, id := range recipients {
			if _, e = a.Operations.Emit(c.Request.Context(), tx, operations.Notification{OperatorID: notice.ParentID, RecipientID: id, BusinessKey: "remediation:exit-notice:" + notice.ID, Category: "mandatory", Severity: "info", Title: title, Body: body, Reference: notice.ID}); e != nil {
				return nil, e
			}
		}
		return commandResult(notice.ID, "notified"), nil
	})
	if e != nil {
		handleError(c, e)
		return
	}
	c.Data(200, "application/json", raw)
}
