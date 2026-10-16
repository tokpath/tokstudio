package portal

import (
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func (a *Application) disputesResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	rows, e := a.Remediation.Chargebacks(c.Request.Context(), op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		row := entity(r.ID, r.State, 1, r)
		actions := []string{"recover"}
		if r.State == "disputed" {
			actions = append(actions, "restrict", "resolve")
		}
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "争议、拒付与收款风险"
	p["description"] = "外部真实拒付先登记损失，再追溯原收款来源。无效付款与经营方收款风险分别处理，不能把正常客户权益直接抹去。"
	p["items"] = items
	p["columns"] = []gin.H{col("receipt_id", "原收款来源", "text"), col("provider_case_id", "外部争议编号", "text"), col("amount_usd", "实际损失", "usd"), col("recovered_usd", "已核实追回", "usd"), col("reason", "原因", "text"), col("created_at", "登记时间", "datetime")}
	p["operations"] = []operation{{Name: "record", Label: "登记已核实外部扣回", Method: "POST", Path: base, Fields: []field{f("receipt_id", "原收款编号", "text"), f("provider", "外部通道", "text"), f("provider_case_id", "外部唯一争议编号", "text"), f("amount_usd", "已核实实际扣回USD", "decimal"), f("reason", "争议原因", "text"), f("evidence", "通道或银行真实证据", "textarea")}, Confirmation: "确认这是已核实的外部扣回事实。登记将记录真实收款损失，不能用一般投诉替代资金证据。"}}
	p["row_operations"] = []operation{{Name: "restrict", Label: "追溯并限制来源资产", Method: "POST", Path: base + "/{id}/restrict", Fields: []field{f("evidence", "限制依据与客户告知", "textarea")}, Confirmation: "只限制真实关联的原收款来源资产。范围由服务器追溯。"}, {Name: "resolve", Label: "复核风险归类", Method: "POST", Path: base + "/{id}/resolve", Fields: []field{selectField("classification", "事实归类", "invalid_payment|付款无效，按来源纠正", "merchant_collection_risk|经营收款风险，保留客户权益", "reversed|外部已撤销且全部追回"), f("evidence", "独立复核与归类证据", "textarea")}, Confirmation: "不同归类影响不同责任。请核实真实付款与收款风险，独立复核要求由服务器执行。"}, {Name: "recover", Label: "登记真实追回资金", Method: "POST", Path: base + "/{id}/recover", Fields: []field{f("amount_usd", "本次真实追回USD", "decimal"), f("evidence", "银行或通道追回凭证", "textarea")}}}
	a.resourceResponse(c, p)
}
func (a *Application) disputesCommand(c *gin.Context, op organization.Operator, actor string) {
	var in remediation.ChargebackInput
	if !body(c, &in) {
		return
	}
	in.OperatorID = op.ID
	in.ActorID = actor
	out, e := a.Remediation.RecordChargeback(c.Request.Context(), in)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) disputesAction(c *gin.Context, op organization.Operator, actor, action string) {
	var in struct {
		Evidence       string         `json:"evidence"`
		Classification string         `json:"classification"`
		Amount         ledger.Decimal `json:"amount_usd"`
	}
	if !body(c, &in) {
		return
	}
	id := c.Param("id")
	ctx := c.Request.Context()
	var e error
	state := "updated"
	switch action {
	case "restrict":
		e = a.Remediation.RestrictChargeback(ctx, actor, op.ID, id, in.Evidence)
	case "resolve":
		out, err := a.Remediation.ResolveChargeback(ctx, actor, op.ID, id, in.Classification, in.Evidence)
		e = err
		state = out.State
	case "recover":
		key, _ := key(c)
		out, err := a.Remediation.RecordChargebackRecovery(ctx, actor, op.ID, id, key, in.Evidence, in.Amount)
		e = err
		state = out.State
	default:
		e = remediation.ErrInvalid
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(id, state))
}
