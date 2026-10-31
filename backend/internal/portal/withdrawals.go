package portal

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (a *Application) applyWithdrawal(c *gin.Context) {
	var in struct {
		MethodID  string            `json:"method_id"`
		Amount    ledger.Decimal    `json:"amount_money"`
		Recipient map[string]string `json:"recipient"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	idem, ok := key(c)
	if !ok {
		return
	}
	operatorID := id.Customer.Ownership.OperatorID
	raw, e := a.command(c, "withdrawal-apply:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		methods, err := a.Incentives.WithdrawalMethods(c.Request.Context(), tx, operatorID, true)
		if err != nil {
			return nil, err
		}
		var method incentives.WithdrawalPolicy
		found := false
		for _, candidate := range methods {
			if candidate.MethodID == in.MethodID {
				method, found = candidate, true
			}
		}
		if !found {
			return nil, incentives.ErrNotReady
		}
		if _, ok := incentives.FindPayoutProfile(method.PayoutKind); !ok {
			return nil, incentives.ErrInvalidInput
		}
		reference, err := incentives.FormatPayoutRecipient(method.PayoutKind, in.Recipient)
		if err != nil {
			return nil, err
		}
		quote, err := a.Incentives.QuoteWithdrawal(c.Request.Context(), tx, operatorID, in.Amount, method.MethodID)
		if err != nil {
			return nil, err
		}
		out, err := a.Incentives.RequestWithdrawal(c.Request.Context(), tx, incentives.WithdrawalInput{OperatorID: operatorID, CustomerID: id.Customer.Customer.ID, BusinessKey: idem, RecipientReference: reference, PolicyID: quote.Policy.ID, AmountMoney: quote.AmountMoney})
		if err != nil {
			return nil, err
		}
		return gin.H{"id": out.ID, "status": out.Status, "message": "已提交提现申请。总额 " + quote.AmountMoney.String() + " " + quote.Policy.Currency + "，手续费 " + quote.FeeMoney.String() + " " + quote.Policy.Currency + "，实付 " + quote.NetMoney.String() + " " + quote.Policy.Currency + "。"}, nil
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
	p["views"] = []gin.H{{"value": "requests", "label": "提现申请"}, {"value": "methods", "label": "提现方式"}}
	if c.Query("view") == "methods" {
		methods, err := a.Incentives.WithdrawalMethods(c.Request.Context(), nil, op.ID, false)
		if err != nil {
			handleError(c, err)
			return
		}
		items := []any{}
		for _, m := range methods {
			v := values(m)
			v["fee_mode"] = "fixed"
			if m.FeeRate.Sign() > 0 {
				v["fee_mode"] = "percent"
			}
			v["fee_percent"] = ratePercentage(m.FeeRate)
			status := "disabled"
			if m.Enabled {
				status = "enabled"
			}
			if profile, ok := incentives.FindPayoutProfile(m.PayoutKind); ok {
				v["payout_kind_label"] = profile.Label
			} else {
				v["payout_kind_label"] = "尚未选择收款材料"
			}
			row := entity(m.MethodID, status, m.Version, v)
			row["allowed_actions"] = []string{"edit_method"}
			items = append(items, row)
		}
		p["title"] = "提现方式"
		p["description"] = "设置可用提现方式及固定手续费或手续费率。申请时固定本次费用和实付金额，线下按实付金额付款，无需逐笔登记实际手续费。"
		p["items"] = items
		p["columns"] = []gin.H{col("method_name", "提现方式", "text"), col("payout_kind_label", "收款材料", "text"), col("flat_fee_money", "固定手续费", "money"), col("fee_percent", "手续费率 %", "decimal"), col("enabled", "开放申请", "boolean")}
		fields := withdrawalMethodFields()
		p["operations"] = []operation{{Name: "method", Label: "新增提现方式", Method: "POST", Path: base, Fields: fields}}
		p["row_operations"] = []operation{{Name: "edit_method", Label: "修改提现方式", Method: "POST", Path: base + "/{id}/edit_method", Fields: fields}}
		a.resourceResponse(c, p)
		return
	}
	p["search_supported"], p["status_supported"] = true, true
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
	p["title"] = "佣金提现申请"
	p["description"] = "在邀请与分佣中审批并登记线下付款。最低提现金额在分佣规则中设置，手续费在提现方式中设置。"
	p["items"] = items
	p["next_cursor"] = next
	p["columns"] = []gin.H{col("customer_id", "客户", "text"), col("method_name", "提现方式", "text"), col("amount_money", "总额", "money"), col("fee_money", "手续费", "money"), col("net_money", "实付", "money"), col("recipient_reference", "收款资料", "text"), col("status", "状态", "status")}
	p["operations"] = []operation{}
	p["row_operations"] = withdrawalRowOperations(base)
	a.resourceResponse(c, p)
}
func customerPayoutMethods(methods []incentives.WithdrawalPolicy) []gin.H {
	out := []gin.H{}
	for _, method := range methods {
		profile, ok := incentives.FindPayoutProfile(method.PayoutKind)
		if !ok {
			continue
		}
		out = append(out, gin.H{"id": method.MethodID, "name": method.MethodName, "kind": method.PayoutKind, "kind_label": profile.Label, "summary": profile.Summary, "flat_fee_money": method.FlatFeeMoney.String(), "fee_rate": method.FeeRate.String(), "minimum_money": method.MinimumMoney.String(), "fields": profile.Fields})
	}
	return out
}
func withdrawalMethodFields() []field {
	mode := f("fee_mode", "手续费方式", "select")
	mode.Options = []gin.H{{"value": "fixed", "label": "固定金额"}, {"value": "percent", "label": "按比例"}}
	mode.Default = "fixed"
	fixed := f("flat_fee_money", "固定手续费 结算币种", "decimal")
	fixed.Default = "0"
	fixed.VisibleWhen = &fieldVisibility{Field: "fee_mode", Values: []string{"fixed"}}
	rate := f("fee_percent", "手续费率 %", "decimal")
	rate.Default = "0"
	rate.VisibleWhen = &fieldVisibility{Field: "fee_mode", Values: []string{"percent"}}
	rate.Help = "填写 2 表示按提现总额收取 2%。手续费按清算币种最小支付单位四舍五入，提交后固定。"
	enabled := f("enabled", "开放申请", "boolean")
	enabled.Default = true
	kind := f("payout_kind", "支付方式", "select")
	kind.Help = "选定支付方式后，客户必须按该方式配套的收款材料填写。已提交的申请保留当时的资料，不会因后来修改方式而改变。"
	for _, profile := range incentives.PayoutProfiles() {
		kind.Options = append(kind.Options, gin.H{"value": profile.Kind, "label": profile.OptionLabel})
	}
	return []field{kind, f("method_name", "提现方式名称", "text"), mode, fixed, rate, enabled, defaultField("expected_version", "当前版本", "hidden", 0)}
}
func (a *Application) saveWithdrawalMethod(c *gin.Context, op organization.Operator, actor, methodID string) {
	var in struct {
		Kind     string         `json:"payout_kind"`
		Name     string         `json:"method_name"`
		Mode     string         `json:"fee_mode"`
		Fixed    ledger.Decimal `json:"flat_fee_money"`
		Percent  ledger.Decimal `json:"fee_percent"`
		Enabled  bool           `json:"enabled"`
		Expected int64          `json:"expected_version"`
	}
	if !body(c, &in) {
		return
	}
	if _, ok := incentives.FindPayoutProfile(in.Kind); !ok {
		fail(c, 400, "invalid_input", "请选择支付方式，客户将按该方式需要的收款材料填写。", false)
		return
	}
	p := incentives.WithdrawalPolicy{MethodID: methodID, MethodName: in.Name, PayoutKind: in.Kind, Enabled: in.Enabled, MinimumMoney: ledger.FromInt64(1)}
	if in.Name == "" {
		handleError(c, incentives.ErrInvalidInput)
		return
	}
	if p.MethodID == "" {
		p.MethodID = uuid.NewString()
	}
	switch in.Mode {
	case "fixed":
		p.FlatFeeMoney = in.Fixed
	case "percent":
		var err error
		p.FeeRate, err = percentageRate(in.Percent)
		if err != nil {
			handleError(c, err)
			return
		}
	default:
		handleError(c, incentives.ErrInvalidInput)
		return
	}
	raw, err := a.command(c, "withdrawal-method:"+methodID, in, func(tx *gorm.DB) (any, error) {
		saved, err := a.Incentives.SetWithdrawalPolicy(c.Request.Context(), tx, actor, op.ID, p, in.Expected)
		return commandResult(saved.MethodID, "saved"), err
	})
	jsonResponse(c, raw, err)
}
func (a *Application) withdrawalStaffCommand(c *gin.Context, op organization.Operator, actor string) {
	a.saveWithdrawalMethod(c, op, actor, "")
}
func (a *Application) withdrawalStaffAction(c *gin.Context, op organization.Operator, actor, action string) {
	if action == "edit_method" {
		a.saveWithdrawalMethod(c, op, actor, c.Param("id"))
		return
	}
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
		settings, err := ledger.GetMonetary(ctx, a.DB)
		if err != nil {
			handleError(c, err)
			return
		}
		in, e = normalizeWithdrawalPaymentProof(in, settings.Currency)
		if e != nil {
			handleError(c, e)
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
	return []operation{{Name: "approve", Label: "审批", Method: "POST", Path: base + "/{id}/approve", Fields: []field{}, Confirmation: "确认原申请费用、收款资料及权限。启用双人复核时付款人必须与审批人不同。"}, {Name: "begin_payment", Label: "登记开始真实付款", Method: "POST", Path: base + "/{id}/begin_payment", Fields: []field{}, Confirmation: "将保存付款意图，未知结果不得再次盲目付款。"}, {Name: "unknown", Label: "标记付款结果未知", Method: "POST", Path: base + "/{id}/unknown", Fields: []field{f("evidence", "查询与不确定性依据", "textarea")}}, {Name: "verify_paid", Label: "登记已核实实际付款", Method: "POST", Path: base + "/{id}/verify_paid", Fields: []field{jsonField("payment_reference、evidence、currency、original_amount、valuation_evidence、paid_money；必须是本次真实付款事实。")}, Confirmation: "确认真实付款凭证与结算币种净额一致后释放占用并完成账务。"}, {Name: "fail", Label: "确认未付并终止", Method: "POST", Path: base + "/{id}/fail", Fields: []field{f("evidence", "已核实未付款依据", "textarea"), f("verified_unpaid", "已核实未付", "boolean")}, Confirmation: "未知结果不能终止并再次付款。"}}
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
