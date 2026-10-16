package portal

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) jobsResource(c *gin.Context, p gin.H, base string) {
	rows, e := a.Jobs.List(c.Request.Context(), c.Query("cursor"), c.Query("queue"), c.Query("status"), 51)
	if e != nil {
		handleError(c, e)
		return
	}
	if len(rows) > 50 {
		rows = rows[:50]
		p["next_cursor"] = rows[49].ID
	}
	items := []any{}
	for _, j := range rows {
		row := entity(j.ID, j.Status, 1, j)
		row["allowed_actions"] = []string{}
		if j.Status == "dead" {
			row["allowed_actions"] = []string{"redrive"}
		}
		items = append(items, row)
	}
	p["title"] = "持久任务与异常恢复"
	p["description"] = "展示原任务与有界重试。恢复保留原业务编号和单调fencing历史；外部支付未知结果必须查询原交易。"
	p["items"] = items
	p["columns"] = []gin.H{col("queue", "队列", "text"), col("kind", "任务类型", "text"), col("business_key", "原业务编号", "text"), col("attempts", "已尝试", "text"), col("max_attempts", "重试上限", "text"), col("status", "状态", "status"), col("last_error", "上次错误", "text"), col("available_at", "可执行时间", "datetime")}
	p["row_operations"] = []operation{{Name: "redrive", Label: "恢复原任务", Method: "POST", Path: base + "/{id}/redrive", Fields: []field{f("additional_attempts", "追加尝试次数1–10", "integer"), f("reason", "人工核实与恢复原因", "textarea")}, Confirmation: "只恢复原任务，不创建新支付或模型请求。已产生的外部事实仍按原业务编号去重。"}}
	a.resourceResponse(c, p)
}
func (a *Application) redrive(c *gin.Context, op organization.Operator) {
	var in struct {
		Additional int `json:"additional_attempts"`
		Reason     string
	}
	if !body(c, &in) {
		return
	}
	if in.Reason == "" {
		handleError(c, operations.ErrInvalid)
		return
	}
	raw, e := a.command(c, "redrive:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		if e := a.Jobs.RedriveTx(tx, c.Param("id"), in.Additional); e != nil {
			return nil, e
		}
		return commandResult(c.Param("id"), "pending"), nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) retentionResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows, e := a.Operations.ListRetention(c.Request.Context(), actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		status := "retained"
		if r.PurgedAt != nil {
			status = "purged"
		}
		items = append(items, entity(r.ID, status, 1, r))
	}
	policy := a.Operations.RetentionPolicy()
	p["title"] = "临时数据保留与保护"
	p["description"] = "临时内容最长保留30天，私有导出24小时；合法复核保护有90天上限。账本、履约与审计事实不随临时内容删除。"
	p["items"] = items
	p["summary"] = []gin.H{{"label": "临时内容上限", "value": "30", "unit": "天"}, {"label": "合法保护上限", "value": "90", "unit": "天"}}
	p["columns"] = []gin.H{col("kind", "内容类型", "text"), col("reference", "事实引用", "text"), col("expires_at", "到期", "datetime"), col("legal_hold_until", "保护截至", "datetime"), col("state", "状态", "status")}
	p["row_operations"] = []operation{{Name: "hold", Label: "记录必要复核保护", Method: "POST", Path: base + "/{id}/hold", Fields: []field{f("reason", "复核/保留依据", "textarea"), f("until", "保护截止时间", "datetime")}}}
	_ = policy
	a.resourceResponse(c, p)
}
func (a *Application) holdTemporary(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Reason string
		Until  time.Time
	}
	if !body(c, &in) {
		return
	}
	e := a.Operations.HoldTemporary(c.Request.Context(), actor, op.ID, c.Param("id"), in.Reason, in.Until)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(c.Param("id"), "held"))
}

var _ json.RawMessage
