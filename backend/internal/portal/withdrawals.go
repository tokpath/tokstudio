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

func (a *Application) applyWithdrawal(c *gin.Context) { a.applyWalletWithdrawal(c, "commission") }
func (a *Application) applyBalanceWithdrawal(c *gin.Context) {
	a.applyWalletWithdrawal(c, "balance")
}
func (a *Application) applyWalletWithdrawal(c *gin.Context, wallet string) {
	var in struct {
		PolicyID  string            `json:"policy_id"`
		MethodID  string            `json:"method_id"`
		Amount    ledger.Decimal    `json:"amount_money"`
		Recipient map[string]string `json:"recipient"`
	}
	if !body(c, &in) {
		return
	}
	if _, err := uuid.Parse(in.PolicyID); err != nil {
		handleError(c, incentives.ErrInvalidInput)
		return
	}
	id := subject(c)
	idem, ok := key(c)
	if !ok {
		return
	}
	operatorID := id.Customer.Ownership.OperatorID
	raw, e := a.command(c, wallet+"-withdrawal-apply:"+id.Customer.Customer.ID, in, func(tx *gorm.DB) (any, error) {
		methods, err := a.Incentives.WithdrawalMethods(c.Request.Context(), tx, operatorID, true, wallet)
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
			return nil, incentives.ErrWithdrawalTermsChanged
		}
		if method.ID != in.PolicyID {
			return nil, incentives.ErrWithdrawalTermsChanged
		}
		if _, ok := incentives.FindPayoutProfile(method.PayoutKind); !ok {
			return nil, incentives.ErrInvalidInput
		}
		if method.PayoutKind == "usdt" && !contains(method.PayoutNetworks, in.Recipient["network"]) {
			return nil, incentives.ErrInvalidInput
		}
		reference, err := incentives.FormatPayoutRecipient(method.PayoutKind, in.Recipient)
		if err != nil {
			return nil, err
		}
		quote, err := a.Incentives.QuoteWithdrawal(c.Request.Context(), tx, operatorID, in.Amount, method.MethodID, wallet)
		if err != nil {
			return nil, err
		}
		if quote.Policy.ID != in.PolicyID {
			return nil, incentives.ErrWithdrawalTermsChanged
		}
		out, err := a.Incentives.RequestWithdrawal(c.Request.Context(), tx, incentives.WithdrawalInput{Wallet: wallet, OperatorID: operatorID, CustomerID: id.Customer.Customer.ID, BusinessKey: idem, RecipientReference: reference, PolicyID: in.PolicyID, AmountMoney: quote.AmountMoney})
		if err != nil {
			return nil, err
		}
		return gin.H{"id": out.ID, "status": out.Status, "message": "已提交提现申请。"}, nil
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
func (a *Application) customerWithdrawalWorkspace(c *gin.Context, p gin.H, operatorID, customerID, wallet string) {
	ctx := c.Request.Context()
	rows, e := a.Incentives.Withdrawals(ctx, nil, operatorID, customerID, wallet)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, w := range rows {
		v := values(w)
		if w.PayoutCurrency != "" {
			v["payout_display"] = w.PayoutAmount.String() + " " + w.PayoutCurrency
		}
		row := entity(w.ID, w.Status, 1, v)
		row["allowed_actions"] = []string{}
		if w.Status == "requested" {
			row["allowed_actions"] = []string{"cancel"}
		}
		items = append(items, row)
	}
	methods, err := a.Incentives.WithdrawalMethods(ctx, nil, operatorID, true, wallet)
	if err != nil {
		handleError(c, err)
		return
	}
	cancelPath := "/api/v1/me/withdrawals/{id}/cancel"
	held := "本笔佣金占用"
	title := "佣金提现"
	if wallet == "balance" {
		cancelPath = "/api/v1/me/balance-withdrawals/{id}/cancel"
		held = "这笔支付余额占用"
		title = "余额提现"
	}
	p["row_operations"] = []operation{{Name: "cancel", Label: "取消提现申请", Method: "POST", Path: cancelPath, Fields: []field{}, Description: "仅尚未审核的本人提现申请可以取消。", Confirmation: "确认取消这笔尚未审核的提现申请？取消成功后释放" + held + "；已进入审核或付款环节的申请须按最新状态继续办理。"}}
	p["title"] = title
	p["items"] = items
	netLabel := "实付"
	if wallet == "balance" {
		netLabel = "实得"
	}
	p["columns"] = []gin.H{col("method_name", "提现方式", "text"), col("amount_money", "总额", "money"), col("fee_money", "手续费", "money"), col("net_money", netLabel, "money"), col("recipient_reference", "收款资料", "text"), col("status", "状态", "status")}
	p["operations"] = []operation{}
	offered := customerPayoutMethods(methods)
	settings, settingsErr := ledger.GetMonetary(ctx, a.DB)
	digits := 2
	if settingsErr == nil {
		digits = incentives.CurrencyMinorDigits(settings.Currency)
	}
	p["payout_methods"] = offered
	p["currency_digits"] = digits
	if len(offered) > 0 {
		minimum := offered[0]["minimum_money"].(string)
		p["description"] = "提现须达到经营方公布的最低额度。手续费和实际到账金额在申请时固定，由你确认。"
		p["summary"] = []gin.H{{"label": "最低提现额度", "value": minimum, "unit": "MONEY", "detail": "单笔提现总额不能低于经营方公布的这个额度。"}}
	} else {
		p["description"] = "请等待经营方配置可用提现方式。"
	}
	a.resourceResponse(c, p)
}
func (a *Application) withdrawalStaffResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base, wallet string) {
	views := []gin.H{{"value": "requests", "label": "提现申请"}}
	if wallet == "balance" {
		views = append(views, gin.H{"value": "minimum", "label": "最低提现额度"})
	}
	p["views"] = views
	if wallet == "balance" && c.Query("view") == "minimum" {
		minimum := a.Incentives.WithdrawalMinimum(c.Request.Context(), nil, op.ID, wallet)
		p["title"] = "余额最低提现额度"
		p["description"] = "单笔支付余额提现不能低于这个额度，且不能小于 1 结算币种。已经提交的申请仍按申请时的额度办理。新申请按保存后的额度校验。"
		p["items"] = []any{}
		p["columns"] = []gin.H{}
		p["operations"] = []operation{balanceMinimumOperation(base, minimum)}
		p["row_operations"] = []operation{}
		a.resourceResponse(c, p)
		return
	}

	p["search_supported"], p["status_supported"] = true, true
	rows, next, e := a.Incentives.WithdrawalsForOperator(c.Request.Context(), nil, actor, op.ID, c.Query("cursor"), 50, c.Query("q"), c.Query("status"), wallet)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	hasUnquoted := false
	for _, w := range rows {
		if w.PayoutCurrency == "" {
			hasUnquoted = true
		}
		v := values(w)
		if w.PayoutCurrency != "" {
			v["payout_display"] = w.PayoutAmount.String() + " " + w.PayoutCurrency
		}
		row := entity(w.ID, w.Status, 1, v)
		actions := withdrawalAllowedActions(w.Status)
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "佣金提现申请"
	p["description"] = "在邀请与分佣中审批并登记线下付款。最低提现金额在分佣规则中设置，手续费统一在设置中的支付方式与手续费页面配置。"
	if wallet == "balance" {
		p["title"] = "余额提现申请"
		p["description"] = "审批支付余额提现，并按申请时固定的实付金额登记一次付款。最低额度在最低提现额度页设置，手续费统一在设置中的支付方式与手续费页面配置，由客户承担。"
		p["operations"] = []operation{}
	}
	p["items"] = items
	p["next_cursor"] = next
	p["columns"] = []gin.H{col("customer_id", "客户", "text"), col("method_name", "提现方式", "text"), col("amount_money", "总额", "money"), col("fee_money", "手续费", "money"), col("payout_display", "实际到账", "text"), col("recipient_reference", "收款资料", "text"), col("status", "状态", "status")}
	if hasUnquoted {
		p["columns"] = append(p["columns"].([]gin.H), col("net_money", "清算净额", "money"))
	}
	if wallet != "balance" {
		p["operations"] = []operation{}
	}
	held := "佣金占用"
	if wallet == "balance" {
		held = "支付余额占用"
	}
	p["row_operations"] = withdrawalRowOperations(base, held)
	a.resourceResponse(c, p)
}
func customerPayoutMethods(methods []incentives.WithdrawalPolicy) []gin.H {
	out := []gin.H{}
	for _, method := range methods {
		profile, ok := incentives.FindPayoutProfile(method.PayoutKind)
		if !ok {
			continue
		}
		if method.PayoutKind == "usdt" {
			for i := range profile.Fields {
				if profile.Fields[i].Name == "network" {
					options := []incentives.PayoutChoice{}
					for _, o := range profile.Fields[i].Options {
						if contains(method.PayoutNetworks, o.Value) {
							options = append(options, o)
						}
					}
					profile.Fields[i].Options = options
				}
			}
		}
		out = append(out, gin.H{"payout_currency": method.PayoutCurrency, "payout_rate": method.PayoutRate.String(), "payout_digits": incentives.PayoutDigits(method.PayoutCurrency), "payout_note": method.PayoutNote, "id": method.MethodID, "policy_id": method.ID, "name": method.MethodName, "kind": method.PayoutKind, "kind_label": profile.Label, "summary": profile.Summary, "flat_fee_money": method.FlatFeeMoney.String(), "fee_rate": method.FeeRate.String(), "minimum_money": method.MinimumMoney.String(), "fields": profile.Fields})
	}
	return out
}
func balanceMinimumOperation(base string, minimum ledger.Decimal) operation {
	amount := f("minimum_money", "余额最低提现 结算币种", "decimal")
	amount.Default = minimum.String()
	amount.Help = "单笔支付余额提现不能低于这个额度，且不能小于 1 结算币种。已经提交的申请仍按申请时的额度办理。"
	return operation{Name: "minimum", Label: "设置最低提现额度", Method: "POST", Path: base, Fields: []field{amount}}
}
func (a *Application) saveBalanceWithdrawalMinimum(c *gin.Context, op organization.Operator, actor string, minimum ledger.Decimal) {
	raw, err := a.command(c, "balance-withdrawal-minimum:"+op.ID, gin.H{"minimum_money": minimum}, func(tx *gorm.DB) (any, error) {
		saved, err := a.Incentives.SetBalanceWithdrawalMinimum(c.Request.Context(), tx, actor, op.ID, minimum)
		return commandResult(saved.OperatorID, "saved"), err
	})
	jsonResponse(c, raw, err)
}
func (a *Application) saveWithdrawalMethod(c *gin.Context, op organization.Operator, actor, methodID string) {
	var raw json.RawMessage
	if !body(c, &raw) {
		return
	}
	a.saveWithdrawalMethodBody(c, op, actor, methodID, raw)
}
func (a *Application) saveWithdrawalMethodBody(c *gin.Context, op organization.Operator, actor, methodID string, raw json.RawMessage) {
	fail(c, 409, "payment_method_settings_moved", "请到设置中的支付方式与手续费统一配置。", false)
}

func (a *Application) withdrawalStaffCommand(c *gin.Context, op organization.Operator, actor string) {
	var raw map[string]json.RawMessage
	if !body(c, &raw) {
		return
	}
	if c.Param("resource") == "balance-withdrawals" {
		if _, hasKind := raw["payout_kind"]; !hasKind {
			if minRaw, ok := raw["minimum_money"]; ok {
				var minimum ledger.Decimal
				if json.Unmarshal(minRaw, &minimum) != nil {
					fail(c, 400, "invalid_input", "请输入余额最低提现额度。", false)
					return
				}
				a.saveBalanceWithdrawalMinimum(c, op, actor, minimum)
				return
			}
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		fail(c, 400, "invalid_input", "请求内容格式错误", false)
		return
	}
	a.saveWithdrawalMethodBody(c, op, actor, "", encoded)
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
	case "approve":
		var empty struct{}
		if !body(c, &empty) {
			return
		}
		out, e = a.Incentives.ApproveWithdrawal(ctx, nil, actor, op.ID, c.Param("id"))
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
			Evidence string `json:"evidence"`
		}
		if !body(c, &in) {
			return
		}
		e = a.Incentives.FailWithdrawal(ctx, nil, actor, op.ID, c.Param("id"), in.Evidence)
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

func withdrawalPaymentFields() []field {
	currency := f("currency", "实际付款币种", "text")
	currency.Help = "默认是结算币种。用结算币种付款时不用再填金额。如果实际付出的是另一种货币，改成那种货币。"
	original := optional("original_amount", "实际原币付款金额", "decimal")
	original.Help = "填写实际付出的原币金额。账上仍按申请时已经固定的实付净额入账。"
	link := optional("valuation_evidence", "本笔原币付款与实付净额的关联凭证", "textarea")
	link.Help = "说明这笔原币付款对应申请里的实付净额。"
	evidence := optional("evidence", "付款备注（可选）", "textarea")
	evidence.Help = "可补充付款情况或凭证。系统自动记录经办人、付款金额与时间。"
	for _, v := range []*field{&currency, &original, &link} {
		v.VisibleWhen = &fieldVisibility{Field: "payout_currency", Values: []string{""}}
	}
	return []field{optional("payment_reference", "付款流水或收据号（可选）", "text"), evidence, currency, original, link, f("paid_at", "实际付款时刻", "datetime")}
}

func withdrawalRowOperations(base, held string) []operation {
	reason := optional("evidence", "拒绝理由", "textarea")
	reason.Help = "填写后会出现在发给申请人的通知里。可以不填。"
	return []operation{{Name: "approve", Label: "通过", Method: "POST", Path: base + "/{id}/approve", Fields: []field{}, Confirmation: "确认通过这笔提现申请？通过后可以登记付款。"}, {Name: "verify_paid", Label: "登记付款", Method: "POST", Path: base + "/{id}/verify_paid", Fields: withdrawalPaymentFields(), Confirmation: "确认已按申请时固定的实付净额完成付款。提交后申请结束。"}, {Name: "fail", Label: "拒绝", Method: "POST", Path: base + "/{id}/fail", Fields: []field{reason}, Confirmation: "确认拒绝这笔提现申请？提交后释放" + held + "，申请结束。"}}
}

func withdrawalAllowedActions(status string) []string {
	actions := []string{}
	switch status {
	case "requested":
		actions = []string{"approve", "fail"}
	case "approved":
		actions = []string{"verify_paid", "fail"}
	}
	return actions
}
