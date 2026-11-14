package portal

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func (a *Application) disputesResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	q := a.DB.WithContext(c.Request.Context()).Where("operator_id=?", op.ID)
	if record := c.Query("record"); record != "" {
		q = q.Where("id::text=?", record)
	}
	if term := strings.TrimSpace(c.Query("q")); term != "" {
		term = "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(term) + "%"
		q = q.Where("(id::text ILIKE ? OR receipt_id::text ILIKE ? OR provider_case_id ILIKE ? OR reason ILIKE ?)", term, term, term, term)
	}
	if status := c.Query("status"); status != "" {
		q = q.Where("state=?", status)
	}
	at, cursorID, e := organization.ParseAuditCursor(c.Query("cursor"))
	if e != nil {
		handleError(c, e)
		return
	}
	if at != nil {
		q = q.Where("(created_at,id)<(?,?::uuid)", *at, cursorID)
	}
	var rows []remediation.Chargeback
	if e := q.Order("created_at DESC,id DESC").Limit(51).Find(&rows).Error; e != nil {
		handleError(c, e)
		return
	}
	p["search_supported"], p["status_supported"] = true, true
	if len(rows) > 50 {
		last := rows[49]
		p["next_cursor"] = organization.NextAuditCursor([]organization.AuditEvent{{ID: last.ID, CreatedAt: last.CreatedAt}}, 1)
		rows = rows[:50]
	}
	items := []any{}
	for _, r := range rows {
		row := entity(r.ID, r.State, 1, r)
		actions := []string{}
		if r.RecoveredMoney.Cmp(r.AmountMoney) < 0 {
			actions = append(actions, "recover")
		}
		if r.State == "disputed" {
			actions = append(actions, "restrict", "resolve")
		}
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "争议、拒付与收款风险"
	p["description"] = "外部真实拒付先登记损失，再追溯原收款来源。无效付款与经营方收款风险分别处理，不能把正常客户权益直接抹去。"
	p["items"] = items
	p["columns"] = []gin.H{col("receipt_id", "原收款来源", "text"), col("provider_case_id", "外部争议编号", "text"), col("amount_money", "实际损失", "money"), col("recovered_money", "已核实追回", "money"), col("reason", "原因", "text"), col("created_at", "登记时间", "datetime")}
	p["operations"] = []operation{{Name: "record", Label: "登记已核实外部扣回", Method: "POST", Path: base, Fields: []field{f("receipt_id", "原收款编号", "text"), f("provider_case_id", "外部唯一争议编号", "text"), f("amount_money", "已核实实际扣回结算币种", "decimal"), f("reason", "争议原因", "text"), f("evidence", "通道或银行真实证据", "textarea")}, Confirmation: "确认这是已核实的外部扣回事实。登记将记录真实收款损失，不能用一般投诉替代资金证据。"}}
	p["row_operations"] = []operation{{Name: "restrict", Label: "追溯并限制来源资产", Method: "POST", Path: base + "/{id}/restrict", Fields: []field{f("evidence", "限制依据与客户告知", "textarea")}, Confirmation: "只限制真实关联的原收款来源资产。范围由服务器追溯。"}, {Name: "resolve", Label: "复核风险归类", Method: "POST", Path: base + "/{id}/resolve", Fields: []field{selectField("classification", "事实归类", "invalid_payment|付款无效，按来源纠正", "merchant_collection_risk|经营收款风险，保留客户权益", "reversed|外部已撤销且全部追回"), f("evidence", "归类证据", "textarea")}, Confirmation: "不同归类影响不同责任。请核实真实付款与收款风险后再提交。"}, {Name: "recover", Label: "登记真实追回资金", Method: "POST", Path: base + "/{id}/recover", Fields: []field{f("amount_money", "本次真实追回结算币种", "decimal"), f("evidence", "银行或通道追回凭证", "textarea")}}}
	a.resourceResponse(c, p)
}
func (a *Application) disputesCommand(c *gin.Context, op organization.Operator, actor string) {
	var in remediation.ChargebackInput
	if !body(c, &in) {
		return
	}
	in.OperatorID = op.ID
	in.ActorID = actor
	receipt, err := a.Commerce.GetReceipt(c.Request.Context(), nil, op.ID, in.ReceiptID)
	if err != nil {
		handleError(c, err)
		return
	}
	in.Provider = receipt.Provider
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
		Amount         ledger.Decimal `json:"amount_money"`
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
