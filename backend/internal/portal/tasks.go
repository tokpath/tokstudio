package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) jobsResource(c *gin.Context, p gin.H, base string) {
	p["status_supported"] = true
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
	p["description"] = "查看后台任务和失败原因。排除故障后可以重试原任务，系统自动控制重试次数并防止重复处理。"
	p["items"] = items
	p["columns"] = []gin.H{col("kind", "任务类型", "text"), col("attempts", "已尝试", "text"), col("status", "状态", "status"), col("last_error", "上次错误", "text"), col("available_at", "可执行时间", "datetime")}
	p["row_operations"] = []operation{{Name: "redrive", Label: "恢复原任务", Method: "POST", Path: base + "/{id}/redrive", Fields: []field{optional("reason", "备注（可选）", "textarea")}, Confirmation: "只恢复原任务，不创建新支付或模型请求。已产生的外部事实仍按原业务编号去重。"}}
	a.resourceResponse(c, p)
}
func (a *Application) redrive(c *gin.Context, op organization.Operator) {
	var in struct {
		Reason string
	}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "redrive:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		if e := a.Jobs.RedriveTx(tx, c.Param("id"), 3); e != nil {
			return nil, e
		}
		return commandResult(c.Param("id"), "pending"), nil
	})
	jsonResponse(c, raw, e)
}
