package portal

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"strings"
)

func (a *Application) receiptValuationsResource(c *gin.Context, op organization.Operator, actor string) {
	rows, next, e := a.Payments.PendingReceiptValuations(c.Request.Context(), actor, op.ID, c.Query("cursor"), 50)
	if e != nil {
		handleError(c, e)
		return
	}
	p := page("收款估值待办")
	var allPending int
	e = a.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		rows, e := a.Payments.PendingReceiptObligations(c.Request.Context(), tx, op.ID, nil)
		allPending = len(rows)
		return e
	})
	if e != nil {
		handleError(c, e)
		return
	}
	p["summary"] = []gin.H{{"label": "真实原币款待核实/入账事项", "value": fmt.Sprint(allPending), "detail": "包含估值、精度及收款匹配待办；未折算或假定为0 USD"}}
	p["description"] = "原币实际款已记录，尚未计入USD或交付。以真实付款时点的独立估值证据核实；不能让客户重复付款，也不能用下单时旧估值追认。"
	items := []any{}
	for _, r := range rows {
		v := values(r)
		v["actual_paid_at"] = v["paid_at"]
		delete(v, "paid_at") // Known immutable provider time is displayed, never resubmitted with browser datetime precision loss.
		v["currency"] = r.Money.Currency
		items = append(items, entity(r.FactID, r.State, 0, v))
	}
	p["items"], p["next_cursor"] = items, next
	p["columns"] = []gin.H{col("order_id", "原订单", "text"), col("driver", "收款协议", "text"), col("money", "已实际收到原币", "details"), col("actual_paid_at", "原事实实际付款时间", "datetime"), col("observed_at", "观察时间", "datetime"), col("reason", "待核实原因", "text")}
	fields := []field{}
	for _, v := range executionValuationFields() {
		if strings.HasPrefix(v.Name, "valuation.") {
			fields = append(fields, v)
		}
	}
	fields = append(fields, optional("paid_at", "独立证明的真实付款时间（原事实缺失时必填）", "datetime"), optional("payment_time_evidence", "独立付款时点证明（补时间时必填）", "textarea"), f("evidence_reference", "本次估值核实证据", "textarea"))
	p["row_operations"] = []operation{{Name: "confirm", Label: "核实原币实际付款时点估值", Method: "POST", Path: "/api/v1/operators/" + op.ID + "/receipt-valuations/{id}/confirm", Fields: fields, Confirmation: "确认独立真实付款时点和覆盖该时刻的本次估值窗口。只使原已收款事实一次入账，不生成新收款或重复交付。"}}
	a.resourceResponse(c, p)
}
func (a *Application) confirmReceiptValuation(c *gin.Context, op organization.Operator, actor string) {
	var in payments.ReceiptValuationConfirmation
	if !body(c, &in) {
		return
	}
	in.BusinessKey, _ = key(c)
	out, e := a.Payments.ConfirmReceiptValuation(c.Request.Context(), actor, op.ID, c.Param("id"), in)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"id": out.FactID, "status": out.State, "version": out.Version, "message": "原已收款事实估值已核实，请查看原订单分配与交付状态"})
}
