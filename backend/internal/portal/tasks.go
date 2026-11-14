package portal

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/jobs"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (a *Application) jobsResource(c *gin.Context, p gin.H, base string) {
	p["search_supported"], p["status_supported"] = false, false
	rows, e := a.taskExceptions(c.Request.Context(), c.Query("cursor"), 51)
	if e != nil {
		handleError(c, e)
		return
	}
	if len(rows) > 50 {
		rows = rows[:50]
		last := rows[49]
		raw, _ := json.Marshal(taskCursor{At: *last.ExhaustedAt, ID: last.ID})
		p["next_cursor"] = base64.RawURLEncoding.EncodeToString(raw)
	}
	items := []any{}
	for _, j := range rows {
		impact, scope, err := a.taskImpact(c.Request.Context(), j.Job)
		if err != nil {
			handleError(c, err)
			return
		}
		note, actions := "正在自动重试", []string{}
		if j.CurrentJobID == j.ID && j.CurrentStatus == "dead" {
			if j.MaxAttempts+3 <= jobs.MaxAttempts {
				actions, note = []string{"redrive"}, ""
			} else {
				note = "已达到恢复次数上限，请由技术人员排查。"
			}
		}
		row := entity(j.ID, j.CurrentStatus, 1, gin.H{
			"name": taskName(j.Kind), "impact_name": impact, "impact_scope": scope,
			"failure_reason": taskFailureReason(j.LastError), "last_failed_at": j.ExhaustedAt,
			"recovery_note": note,
		})
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "后台任务异常"
	p["description"] = "仅显示自动重试耗尽后仍未恢复的任务。同一事项的定时检查合并展示，后续执行成功后自动移除。请先排除故障，再恢复任务。"
	p["items"] = items
	p["columns"] = []gin.H{col("name", "任务名称", "text"), col("impact_name", "影响对象", "text"), col("failure_reason", "失败原因", "text"), col("last_failed_at", "最近失败时间", "datetime")}
	p["row_operations"] = []operation{{Name: "redrive", Label: "恢复任务", Method: "POST", Path: base + "/{id}/redrive", Fields: []field{}, Confirmation: "确认已排除故障，继续执行原任务。系统保留原业务编号，避免重复处理。"}}
	a.resourceResponse(c, p)
}
func (a *Application) redrive(c *gin.Context, op organization.Operator) {
	var in struct{}
	if !body(c, &in) {
		return
	}
	raw, e := a.command(c, "redrive:"+op.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		if !taskIDPattern.MatchString(c.Param("id")) {
			return nil, organization.ErrInvalidInput
		}
		if strings.HasPrefix(c.Param("id"), "refund_") {
			result := tx.Exec(`UPDATE payments_webhook_inbox SET state='pending',max_attempts=max_attempts+3,
 next_attempt_at=clock_timestamp()
 WHERE exhausted_at IS NOT NULL AND state='dead' AND max_attempts+3<=?
 AND md5(merchant_id||':'||event_id)=?`, jobs.MaxAttempts, strings.TrimPrefix(c.Param("id"), "refund_"))
			if result.Error != nil {
				return nil, result.Error
			}
			if result.RowsAffected != 1 {
				return nil, organization.ErrConflict
			}
			return commandResult(c.Param("id"), "pending"), nil
		}
		var job jobs.Job
		if err := tx.First(&job, "id=?", c.Param("id")).Error; err != nil {
			return nil, err
		}
		if job.Queue == "portal-maintenance" {
			var schedule scheduledTask
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", taskCycleSuffix.ReplaceAllString(job.BusinessKey, "")).Take(&schedule).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			var payload workerPayload
			_ = json.Unmarshal(schedule.Payload, &payload)
			if err == nil && (schedule.LastJobID == nil || *schedule.LastJobID != job.ID || schedule.Kind == "risks" && payload.OperatorID == a.Installation.OperatorID) {
				return nil, organization.ErrConflict
			}
		}
		if e := a.Jobs.RedriveTx(tx, job.ID, 3); e != nil {
			if errors.Is(e, jobs.ErrLeaseLost) {
				return nil, organization.ErrConflict
			}
			return nil, e
		}
		return commandResult(c.Param("id"), "pending"), nil
	})
	jsonResponse(c, raw, e)
}
