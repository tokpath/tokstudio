package portal

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tokpath/tokstudio/backend/internal/operations"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"gorm.io/gorm"
)

func (a *Application) publishRemediationEvent(ctx context.Context, tx *gorm.DB, e remediation.Event) error {
	// Events are durable facts. Messages deliberately contain no raw evidence,
	// provider payload, identity document, bank details or request content.
	severity := "info"
	title, body := "资金处理状态已更新", "请在本经营空间的资金处理记录核对最新状态与责任。"
	switch e.Action {
	case "refund.requested":
		title = "退款申请已建立"
		body = "已按原始来源保留退款责任，请在退款明细查看审批与执行进度。"
	case "refund.outcome_unknown":
		title = "退款结果仍待核实"
		body = "原退款结果尚未确定，相关占用保留，系统不会据此重新发起一笔付款。"
	case "refund.completed":
		title = "退款已核实完成"
		body = "原退款已按实际付款证据完成，请在退款明细核对。"
	case "refund.execution_recipient_mismatch", "refund.execution_amount_mismatch":
		severity = "warning"
		title = "实际退款执行需要复核"
		body = "实际退款执行需复核，资金现实与剩余责任已保留。请在原退款记录查看处理状态，未结责任不会因本次核实而消失。"
	case "refund.manual_execution_failed":
		severity = "warning"
		title = "退款执行已核实失败或取消"
		body = "外部执行方的终态已核实，剩余退款责任与资金占用保留。请在原退款记录核对后续办理。"
	case "refund.execution_closed":
		severity = "warning"
		title = "原退款执行已封存"
		body = "原付款指令已终止并保留历史事实，剩余本金仍待支付。后续付款须重新确认目的地并由财务核实授权。"
	}
	recipients, e2 := a.Organization.OperatorNotificationStaff(ctx, tx, e.OperatorID)
	if e2 != nil {
		return e2
	}
	for _, recipient := range recipients {
		if _, e2 = a.Operations.Emit(ctx, tx, operations.Notification{OperatorID: e.OperatorID, RecipientID: recipient, BusinessKey: "remediation:" + e.ID, Category: "mandatory", Severity: severity, Title: title, Body: body, Reference: e.SubjectID}); e2 != nil {
			return e2
		}
	}
	// Only a case's verified customer may receive a customer notice. Looking up
	// the current ownership routes historical responsibility notices after takeover
	// without granting the old operator customer access.
	var caseID string
	var safe struct {
		CaseID string `json:"case_id"`
	}
	_ = json.Unmarshal(e.Data, &safe)
	caseID = safe.CaseID
	if caseID == "" {
		caseID = e.SubjectID
	}
	record, e2 := a.Remediation.GetCase(ctx, tx, e.OperatorID, caseID)
	if errors.Is(e2, gorm.ErrRecordNotFound) {
		return nil
	}
	if e2 != nil {
		return e2
	}
	if record.CustomerID == nil {
		return nil
	}
	own, e2 := a.Organization.CurrentOwnership(ctx, tx, *record.CustomerID)
	if e2 != nil {
		return e2
	}
	_, e2 = a.Operations.Emit(ctx, tx, operations.Notification{OperatorID: own.OperatorID, RecipientID: *record.CustomerID, BusinessKey: "remediation:" + e.ID, Category: "mandatory", Severity: severity, Title: title, Body: body, Reference: record.ID})
	return e2
}
