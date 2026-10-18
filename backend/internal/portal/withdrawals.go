package portal

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

type withdrawalPreview struct {
	ID, CustomerID, OperatorID string
	Quote                      json.RawMessage `gorm:"type:jsonb"`
	ExpiresAt, CreatedAt       time.Time
}

func (withdrawalPreview) TableName() string { return "portal_withdrawal_quotes" }
func (a *Application) withdrawalQuote(c *gin.Context) {
	var in struct {
		Amount ledger.Decimal `json:"amount_usd"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	raw, e := a.command(c, "withdrawal-quote:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		quote, e := a.Incentives.QuoteWithdrawal(c.Request.Context(), tx, id.Customer.Ownership.OperatorID, in.Amount)
		if e != nil {
			return nil, e
		}
		encoded, _ := json.Marshal(quote)
		row := withdrawalPreview{ID: uuid.NewString(), CustomerID: id.Customer.Customer.ID, OperatorID: id.Customer.Ownership.OperatorID, Quote: encoded, ExpiresAt: time.Now().Add(15 * time.Minute), CreatedAt: time.Now()}
		if e = tx.Create(&row).Error; e != nil {
			return nil, e
		}
		return gin.H{"id": row.ID, "status": "quoted", "message": "总额 " + quote.AmountUSD.String() + " USD，手续费 " + quote.FeeUSD.String() + " USD，实付 " + quote.NetUSD.String() + " USD。请刷新列表核对后，在此报价行确认收款资料。"}, nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) confirmWithdrawal(c *gin.Context) {
	var in struct {
		Recipient string `json:"recipient_reference"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	key, _ := key(c)
	raw, e := a.command(c, "withdrawal-confirm:"+id.Customer.Customer.ID+":"+c.Param("id"), in, func(tx *gorm.DB) (any, error) {
		var row withdrawalPreview
		if e := tx.Where("id=? AND customer_id=? AND operator_id=? AND expires_at>?", c.Param("id"), id.Customer.Customer.ID, id.Customer.Ownership.OperatorID, time.Now()).Take(&row).Error; e != nil {
			return nil, e
		}
		var q incentives.WithdrawalQuote
		if e := json.Unmarshal(row.Quote, &q); e != nil {
			return nil, e
		}
		out, e := a.Incentives.RequestWithdrawal(c.Request.Context(), tx, incentives.WithdrawalInput{OperatorID: row.OperatorID, CustomerID: row.CustomerID, BusinessKey: key, RecipientReference: in.Recipient, PolicyID: q.Policy.ID, AmountUSD: q.AmountUSD})
		if e != nil {
			return nil, e
		}
		if e = tx.Delete(&row).Error; e != nil {
			return nil, e
		}
		return commandResult(out.ID, out.Status), nil
	})
	jsonResponse(c, raw, e)
}
func (a *Application) cancelWithdrawal(c *gin.Context) {
	id := subject(c)
	e := a.Incentives.CancelWithdrawal(c.Request.Context(), nil, id.Customer.Customer.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(c.Param("id"), "cancelled"))
}
func (a *Application) withdrawalStaffResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	p["search_supported"], p["status_supported"] = true, true
	var policy incentives.WithdrawalPolicy
	if e := a.DB.WithContext(c.Request.Context()).Table("incentives_withdrawal_policies").Where("operator_id=?", op.ID).Order("version DESC").Limit(1).Find(&policy).Error; e != nil {
		handleError(c, e)
		return
	}
	rows, next, e := a.Incentives.WithdrawalsForOperator(c.Request.Context(), nil, actor, op.ID, c.Query("cursor"), 50, c.Query("q"), c.Query("status"))
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, w := range rows {
		row := entity(w.ID, w.Status, 1, w)
		actions := withdrawalAllowedActions(w.Status)
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "提现办理"
	p["items"] = items
	p["next_cursor"] = next
	p["columns"] = []gin.H{col("customer_id", "客户", "text"), col("amount_usd", "总额", "usd"), col("fee_usd", "手续费", "usd"), col("net_usd", "实付", "usd"), col("recipient_reference", "收款资料", "text"), col("status", "状态", "status")}
	minimum := f("configuration.minimum_usd", "最低提现 USD", "decimal")
	method := f("configuration.method_reference", "对客户公示付款方式规则", "text")
	if policy.Version > 0 {
		minimum.Default = policy.MinimumUSD
		method.Default = policy.MethodReference
	}
	// Policy CAS belongs to the policy itself, independently of the first
	// withdrawal row. Explicit nested fields retain their real defaults through
	// the generic resource presenter and decodeConfiguration reads this object.
	p["operations"] = []operation{{Name: "policy", Label: "发布提现规则", Method: "POST", Path: base, Fields: []field{
		minimum,
		defaultField("configuration.flat_fee_usd", "固定手续费 USD", "decimal", policy.FlatFeeUSD),
		defaultField("configuration.fee_rate", "手续费比例（0到1）", "decimal", policy.FeeRate),
		defaultField("configuration.requires_dual_approval", "必须双人审批付款", "boolean", policy.RequiresDualApproval),
		method,
		defaultField("configuration.expected_version", "当前规则版本", "hidden", policy.Version),
	}}}
	p["row_operations"] = withdrawalRowOperations(base)
	a.resourceResponse(c, p)
}
func (a *Application) withdrawalStaffCommand(c *gin.Context, op organization.Operator, actor string) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	var in struct {
		incentives.WithdrawalPolicy
		Expected int64 `json:"expected_version"`
	}
	if json.Unmarshal(raw, &in) != nil {
		handleError(c, incentives.ErrInvalidInput)
		return
	}
	result, e := a.command(c, "withdrawal-policy:"+op.ID, raw, func(tx *gorm.DB) (any, error) {
		out, e := a.Incentives.SetWithdrawalPolicy(c.Request.Context(), tx, actor, op.ID, in.WithdrawalPolicy, in.Expected)
		return gin.H{"id": out.ID, "status": "published", "version": out.Version}, e
	})
	jsonResponse(c, result, e)
}
func (a *Application) withdrawalStaffAction(c *gin.Context, op organization.Operator, actor, action string) {
	ctx := c.Request.Context()
	var out incentives.Withdrawal
	var e error
	switch action {
	case "approve", "begin_payment":
		var empty struct{}
		if !body(c, &empty) {
			return
		}
		if action == "approve" {
			out, e = a.Incentives.ApproveWithdrawal(ctx, nil, actor, op.ID, c.Param("id"))
		} else {
			out, e = a.Incentives.BeginWithdrawalPayment(ctx, nil, actor, op.ID, c.Param("id"))
		}
	case "unknown":
		var in struct{ Evidence string }
		if !body(c, &in) {
			return
		}
		e = a.Incentives.MarkWithdrawalUnknown(ctx, nil, actor, op.ID, c.Param("id"), in.Evidence)
	case "verify_paid":
		raw, ok := decodeConfiguration(c)
		if !ok {
			return
		}
		var in incentives.WithdrawalPaymentProof
		if json.Unmarshal(raw, &in) != nil {
			handleError(c, incentives.ErrInvalidInput)
			return
		}
		out, e = a.Incentives.VerifyWithdrawalPaid(ctx, nil, actor, op.ID, c.Param("id"), in)
	case "fail":
		var in struct {
			Evidence       string
			VerifiedUnpaid bool `json:"verified_unpaid"`
		}
		if !body(c, &in) {
			return
		}
		e = a.Incentives.FailWithdrawal(ctx, nil, actor, op.ID, c.Param("id"), in.Evidence, in.VerifiedUnpaid)
	default:
		fail(c, 404, "not_found", "提现命令不存在", false)
		return
	}
	if e != nil {
		handleError(c, e)
		return
	}
	status := out.Status
	if status == "" {
		status = "updated"
	}
	c.JSON(200, commandResult(c.Param("id"), status))
}

func withdrawalRowOperations(base string) []operation {
	return []operation{{Name: "approve", Label: "审批", Method: "POST", Path: base + "/{id}/approve", Fields: []field{}, Confirmation: "确认原申请费用、收款资料及权限。启用双人复核时付款人必须与审批人不同。"}, {Name: "begin_payment", Label: "登记开始真实付款", Method: "POST", Path: base + "/{id}/begin_payment", Fields: []field{}, Confirmation: "将保存付款意图，未知结果不得再次盲目付款。"}, {Name: "unknown", Label: "标记付款结果未知", Method: "POST", Path: base + "/{id}/unknown", Fields: []field{f("evidence", "查询与不确定性依据", "textarea")}}, {Name: "verify_paid", Label: "登记已核实实际付款", Method: "POST", Path: base + "/{id}/verify_paid", Fields: []field{jsonField("payment_reference、evidence、currency、original_amount、valuation_evidence、paid_usd；必须是本次真实付款事实。")}, Confirmation: "确认真实付款凭证与USD净额一致后释放占用并完成账务。"}, {Name: "fail", Label: "确认未付并终止", Method: "POST", Path: base + "/{id}/fail", Fields: []field{f("evidence", "已核实未付款依据", "textarea"), f("verified_unpaid", "已核实未付", "boolean")}, Confirmation: "未知结果不能终止并再次付款。"}}
}

func withdrawalAllowedActions(status string) []string {
	actions := []string{}
	switch status {
	case "requested":
		actions = []string{"approve", "fail"}
	case "approved":
		actions = []string{"begin_payment", "fail"}
	case "paying", "unknown":
		actions = []string{"unknown", "verify_paid", "fail"}
	}
	return actions
}
