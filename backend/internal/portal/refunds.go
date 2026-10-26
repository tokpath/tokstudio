package portal

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func (a *Application) refunds(c *gin.Context) {
	id := subject(c)
	rows, e := a.Remediation.CustomerCases(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		items = append(items, gin.H{"id": r.ID, "status": r.State, "amount_usd": r.AmountUSD, "reason": r.Reason, "created_at": r.CreatedAt, "paid_at": r.CompletedAt})
	}
	c.JSON(200, gin.H{"items": items})
}
func (a *Application) refundWorkspace(c *gin.Context, p gin.H) {
	id := subject(c)
	rows, e := a.Remediation.CustomerCases(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID)
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		row := entity(r.ID, r.State, 1, r)
		actions := []string{}
		if r.State == "pending_review" || r.State == "approved" {
			actions = append(actions, "cancel")
		}
		row["allowed_actions"] = actions
		items = append(items, row)
		statuses, e := a.Remediation.CustomerRefundDestinations(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID, r.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, status := range statuses {
			v := values(status)
			v["record_kind"] = "稳定币退款目的地"
			if status.Proposal != nil {
				v["destination"] = values(status.Proposal.RefundDestination)
			}
			if status.AuthorizedDestination != nil {
				for key, value := range values(*status.AuthorizedDestination) {
					v[key] = value
				}
			} else if status.Proposal != nil {
				for key, value := range values(status.Proposal.RefundDestination) {
					v[key] = value
				}
			}
			legRow := entity(status.LegID, status.State, 0, v)
			legRow["allowed_actions"] = []string{}
			if status.DestinationMutable && status.AuthorizationID == "" {
				if status.DestinationRequired {
					legRow["allowed_actions"] = []string{"propose_destination", "propose_responsible_destination"}
				} else {
					legRow["allowed_actions"] = []string{"propose_responsible_destination"}
				}
			}
			items = append(items, legRow)
		}
	}
	p["title"] = "USD 余额原路退款"
	p["description"] = "仅可退本人未使用的USD余额，按原始收款来源退回。已购ATG不能按现价或普通退订兑现；错误交易由经营方按原成交证据处理。"
	p["items"] = items
	p["columns"] = []gin.H{col("operator_id", "原责任经营方", "text"), col("amount_usd", "申请USD", "usd"), col("kind", "类型", "text"), col("state", "处理状态", "status"), col("reason", "申请原因", "text"), col("created_at", "申请时间", "datetime")}
	request := operation{Name: "request", Label: "申请USD余额原路退回", Method: "POST", Path: "/api/v1/me/refunds", Fields: []field{f("amount_usd", "申请金额 USD", "decimal"), f("reason", "退款原因", "textarea")}, Confirmation: "将占用本人未使用USD余额并进入原路退款审核；未知支付结果期间不会重复付款。"}
	if id := subject(c); id != nil && id.Customer.Customer.ID != "" {
		if channels := a.Payments.KVoucherRefundChannels(c.Request.Context(), id.Customer.Ownership.OperatorID); len(channels) > 0 {
			options := make([]string, 0, len(channels))
			labels := map[string]string{"bank": "银行卡", "alipay": "支付宝", "wechat": "微信支付", "tng": "Touch 'n Go", "duitnow": "DuitNow"}
			for _, channel := range channels {
				options = append(options, channel+"|"+labels[channel])
			}
			request.Label = "申请退款并登记收款账号"
			request.Fields = []field{f("amount_usd", "申请金额 USD", "decimal"), selectField("payout_method", "收款方式", options...), f("payout_account", "收款账号", "text"), f("reason", "退款原因", "textarea")}
			request.Confirmation = "提交后占用未使用的 USD 余额。经营方按你登记的账号线下打款，不会通过原支付通道自动退回。"
		}
	}
	p["operations"] = []operation{request}
	p["row_operations"] = []operation{{Name: "cancel", Label: "撤销未执行申请", Method: "POST", Path: "/api/v1/me/refunds/{id}/cancel", Fields: []field{}, Confirmation: "只可撤销尚未执行的原申请；已付款和结果未知的申请需继续核实。"}}

	p["columns"] = append(p["columns"].([]gin.H), col("network", "退款网络", "text"), col("asset", "准确资产", "text"), col("token_contract", "代币合约", "text"), col("address", "本人授权接收地址", "text"), col("authorization_id", "财务已核实授权", "text"), col("route_version", "剩余款授权版本", "text"), col("history", "已封存授权历史", "details"))
	p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "propose_destination", Label: "本人确认稳定币退款目的地", Method: "POST", Path: "/api/v1/me/refunds/legs/{id}/destination", Fields: append(append(refundDestinationFields(), defaultField("responsible_execution", "Responsible execution", "hidden", false)), f("authorization_reference", "本人明确授权与地址来源说明", "textarea")), Confirmation: "我已核对网络、准确资产、代币合约和接收地址，并明确授权本次退款退到该目的地。该地址不会从原转账发送方推定；经财务核实锁定后不可更改。"})
	p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "propose_responsible_destination", Label: "原路受限时明确选择责任方替代付款", Method: "POST", Path: "/api/v1/me/refunds/legs/{id}/responsible-destination", Fields: append(append(refundDestinationFields(), defaultField("responsible_execution", "本人明确选择责任方付款", "hidden", true)), f("authorization_reference", "本人明确授权与接收地址依据", "textarea")), Confirmation: "本人明确授权当前责任经营方通过其自身商户或银行向此准确目的地付款，不授予其使用原经营方商户配置的权利。"})
	a.resourceResponse(c, p)
}
func (a *Application) requestRefund(c *gin.Context) {
	var in struct {
		Amount        ledger.Decimal `json:"amount_usd"`
		Reason        string         `json:"reason"`
		PayoutMethod  string         `json:"payout_method"`
		PayoutAccount string         `json:"payout_account"`
	}
	if !body(c, &in) {
		return
	}
	id := subject(c)
	channels := a.Payments.KVoucherRefundChannels(c.Request.Context(), id.Customer.Ownership.OperatorID)
	if len(channels) > 0 {
		allowed := false
		for _, channel := range channels {
			if channel == in.PayoutMethod {
				allowed = true
			}
		}
		if !allowed || strings.TrimSpace(in.PayoutAccount) == "" {
			handleError(c, payments.ErrInvalid)
			return
		}
		labels := map[string]string{"bank": "银行卡", "alipay": "支付宝", "wechat": "微信支付", "tng": "Touch 'n Go", "duitnow": "DuitNow"}
		in.Reason = "线下退款方式：" + labels[in.PayoutMethod] + "\n收款账号：" + strings.TrimSpace(in.PayoutAccount) + "\n退款原因：" + in.Reason
	}
	key, _ := key(c)
	out, e := a.Remediation.RequestWalletRefund(c.Request.Context(), remediation.WalletRefundInput{OperatorID: id.Customer.Ownership.OperatorID, CustomerID: id.Customer.Customer.ID, BusinessKey: key, AmountUSD: in.Amount, Reason: in.Reason})
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) cancelRefund(c *gin.Context) {
	id := subject(c)
	out, e := a.Remediation.CustomerCancelRefundByBrand(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) refundStaffResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	p["search_supported"], p["status_supported"] = true, true
	policy := remediation.Policy{OperatorID: op.ID}
	if err := a.DB.WithContext(c.Request.Context()).Where("operator_id=?", op.ID).Find(&policy).Error; err != nil {
		handleError(c, err)
		return
	}
	threshold := optional("second_review_above", "需独立复核的USD阈值（留空关闭）", "decimal")
	threshold.Default = policy.SecondReviewAbove
	var rows []remediation.Case
	var e error
	if record := c.Query("record"); record != "" {
		var one remediation.Case
		one, e = a.Remediation.GetCase(c.Request.Context(), nil, op.ID, record)
		if e == nil {
			rows = []remediation.Case{one}
		}
	} else {
		rows, p["next_cursor"], e = a.refundCasesPage(c, op.ID)
	}
	if e != nil {
		handleError(c, e)
		return
	}
	items := []any{}
	for _, r := range rows {
		v := values(r)
		v["record_kind"] = "case"
		row := entity(r.ID, r.State, 1, v)
		actions := []string{}
		if r.State == "pending_review" {
			actions = []string{"review"}
		}
		if r.State == "pending_review" || r.State == "approved" {
			actions = append(actions, "cancel")
		}
		row["allowed_actions"] = actions
		items = append(items, row)
		legs, e := a.Remediation.Legs(c.Request.Context(), nil, op.ID, r.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, leg := range legs {
			v := values(leg)
			v["record_kind"] = "leg"
			remaining := leg.AmountUSD.Sub(leg.SettledUSD)
			if remaining.Sign() < 0 {
				remaining = ledger.Zero()
			}
			v["remaining_usd"] = remaining
			row := entity(leg.ID, leg.State, 1, v)
			allowed := []string{}
			if r.State == "exception" && leg.State == "exception" && leg.SettledUSD.Sign() > 0 && leg.SettledUSD.Cmp(leg.AmountUSD) < 0 {
				allowed = []string{"begin_manual"}
			}
			if (r.State == "approved" || r.State == "paying") && (leg.State == "reserved" || leg.State == "approved") {
				allowed = []string{"begin_manual"}
				if leg.SourceOperatorID == op.ID {
					allowed = append(allowed, "execute_api")
				}
			}
			if leg.State == "executing" || leg.State == "unknown" {
				allowed = []string{"unknown", "verify_manual", "query_api"}
			}

			status, err := a.Remediation.RefundDestinationStatusForStaff(c.Request.Context(), actor, op.ID, leg.ID)
			if err != nil {
				handleError(c, err)
				return
			}
			v["route_version"], v["route_history"], v["expected_execution_key"] = status.RouteVersion, status.History, leg.ExecutionKey
			if status.DestinationRequired {
				v["destination_required"] = true
				v["responsible_execution"] = status.ResponsibleExecution
				if status.Proposal != nil {
					v["proposal_id"] = status.Proposal.ID
					for k, value := range values(status.Proposal.RefundDestination) {
						v[k] = value
					}
				}
				if status.AuthorizedDestination != nil {
					for k, value := range values(*status.AuthorizedDestination) {
						v[k] = value
					}
					v["authorization_id"] = status.AuthorizationID
				}
				if status.AuthorizationID == "" {
					allowed = []string{}
					if status.Proposal != nil {
						allowed = []string{"authorize_destination"}
					} else if r.CustomerID == nil {
						allowed = []string{"authorize_unallocated_destination"}
					}
				} else if leg.State == "exception" && leg.SettledUSD.Sign() > 0 && leg.SettledUSD.Cmp(leg.AmountUSD) < 0 {
					allowed = []string{"begin_manual", "verify_evm", "verify_external"}
				} else if leg.State == "executing" || leg.State == "unknown" || leg.State == "exception" {
					allowed = []string{"unknown"}
					var source payments.PaymentSource
					var err error
					if !status.ResponsibleExecution {
						source, err = a.Payments.PaymentSourceForReceipt(c.Request.Context(), leg.SourceOperatorID, leg.ReceiptID)
					}
					if err == nil && leg.SourceOperatorID == op.ID {
						if source.Driver == "evm_wallet" {
							allowed = append(allowed, "verify_evm")
						} else if source.Driver == "nowpayments" {
							allowed = append(allowed, "verify_external")
						}
					}
				} else {
					allowed = []string{"begin_manual"}
				}
				if status.ResponsibleExecution && status.AuthorizationID != "" && (leg.State == "executing" || leg.State == "unknown") {
					allowed = []string{"unknown", "verify_responsible_evm", "verify_responsible_external", "verify_responsible_bank"}
				}
			}
			if remaining.Sign() > 0 && leg.ExecutionKey != "" {
				if leg.ExecutionMode == "manual" && (leg.State == "executing" || leg.State == "unknown" || leg.State == "exception") {
					allowed = append(allowed, "verify_manual_failure")
				}
				if leg.State == "executing" || leg.State == "exception" {
					allowed = append(allowed, "close_execution")
				}
			}
			row["allowed_actions"] = allowed
			row["values"] = v
			items = append(items, row)
			if a.Organization.RequirePermission(c.Request.Context(), nil, actor, op.ID, "ledger.read") == nil {
				facts, e := a.Remediation.RefundAccounting(c.Request.Context(), nil, actor, op.ID, leg.ID)
				if e != nil {
					handleError(c, e)
					return
				}
				for _, f := range facts {
					v := values(f)
					v["record_kind"] = "actual_refund_payment"
					v["actual_payment_usd"] = f.ActualUSD
					row := entity(f.ID, f.FeeStatus, 0, v)
					row["allowed_actions"] = []string{}
					if f.FeeStatus == "pending" {
						row["allowed_actions"] = []string{"confirm_fee"}
					}
					items = append(items, row)
				}
			}
		}
	}
	p["title"] = "退款与原路执行"
	p["items"] = items
	p["summary"] = []gin.H{{"label": "独立复核政策版本", "value": strconv.FormatInt(policy.Version, 10)}}
	if policy.SecondReviewAbove == nil {
		p["summary"] = append(p["summary"].([]gin.H), gin.H{"label": "二次复核阈值", "value": "未启用"})
	} else {
		p["summary"] = append(p["summary"].([]gin.H), gin.H{"label": "二次复核阈值", "value": policy.SecondReviewAbove.String(), "unit": "USD"})
	}
	p["columns"] = []gin.H{col("route_version", "路由版本", "text"), col("route_history", "已封存执行与授权", "details"), col("record_kind", "记录类型", "text"), col("case_id", "原申请", "text"), col("receipt_id", "原收款来源", "text"), col("amount_usd", "USD责任", "usd"), col("settled_usd", "已真实清偿USD", "usd"), col("remaining_usd", "仍占用待付USD", "usd"), col("excess_usd", "多付经营异常USD", "usd"), col("reason", "原因", "text"), col("state", "状态", "status")}
	p["operations"] = []operation{{Name: "package_exception", Label: "登记真实套餐异常", Method: "POST", Path: base, Fields: []field{jsonField("action:'package_exception'、customer_id、order_id、reason(transaction_error/unable_to_fulfill)、evidence；只按原成交剩余责任退款，撤销未用权益与赠送。")}}, {Name: "unallocated", Label: "退回未分配收款", Method: "POST", Path: base, Fields: []field{jsonField("action:'unallocated'、receipt_id、amount_usd、reason、evidence")}}, {Name: "policy", Label: "设置独立复核阈值", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "hidden", "policy"), threshold, defaultField("expected_version", "复核政策版本", "hidden", policy.Version)}}}
	p["row_operations"] = []operation{{Name: "review", Label: "审批/独立复核", Method: "POST", Path: base + "/{id}/review", Fields: []field{selectField("decision", "审核决定", "approve|批准按原责任办理", "reject|拒绝并保留审核原因"), f("evidence", "审核依据", "textarea")}}, {Name: "cancel", Label: "取消未执行申请", Method: "POST", Path: base + "/{id}/cancel", Fields: []field{f("evidence", "取消依据", "textarea")}}, {Name: "begin_manual", Label: "登记外部退款执行意图", Method: "POST", Path: base + "/{id}/begin_manual", Fields: []field{f("evidence", "银行付款计划与依据", "textarea")}, Confirmation: "将记录原退款责任下本次独立执行编号。仅实际少付后可另行补付剩余金额；结果未知时不得更换编号再次付款。"}, {Name: "verify_manual", Label: "核实实际银行退款", Method: "POST", Path: base + "/{id}/verify_manual", Fields: []field{jsonField("reference、evidence、actual_usd、fee_usd、currency、amount、occurred_at。必须填写本次实际付款证据。")}}, {Name: "unknown", Label: "结果未知，保留占用", Method: "POST", Path: base + "/{id}/unknown", Fields: []field{f("evidence", "未知结果依据", "textarea")}}, {Name: "execute_api", Label: "通过原通道执行", Permission: "payments.manage", Method: "POST", Path: base + "/{id}/execute_api", Fields: []field{jsonField("money:{currency,amount}、refund_valuation（非USD必填本次FX实际报价）、valued_at、fx_execution_reference；原merchant/order由服务器解析。")}, Confirmation: "确认本次执行汇率与费用依据；只执行已批准原退款leg，不会重提未知退款。"}, {Name: "query_api", Label: "查询原通道结果", Permission: "payments.manage", Method: "POST", Path: base + "/{id}/query_api", Fields: []field{}}}

	p["columns"] = append(p["columns"].([]gin.H), col("network", "准确网络", "text"), col("asset", "准确资产", "text"), col("token_contract", "代币合约", "text"), col("address", "明确接收地址", "text"), col("authorization_id", "不可变目的地授权", "text"))
	authorizationFields := []field{f("proposal_id", "本人最新确认", "hidden"), f("payer_authorization_reference", "原付款人明确授权证据", "textarea"), f("verification_evidence", "地址及原付款人核实依据", "textarea"), defaultField("responsible_execution", "已明确选择责任方付款", "hidden", false), optional("route_exception_reference", "同经营方原路受限的替代依据（跨经营方接管可留空）", "textarea")}
	p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "authorize_destination", Label: "核实客户已确认的退款目的地", Method: "POST", Path: base + "/{id}/authorize_destination", Fields: authorizationFields, Confirmation: "已核实当前行本人最新提案、原付款人授权和接收地址；锁定后不可改为其它地址。"}, operation{Name: "authorize_unallocated_destination", Label: "核实未分配款原付款人授权目的地", Method: "POST", Path: base + "/{id}/authorize_unallocated_destination", Fields: append(refundDestinationFields(), authorizationFields[1:]...), Confirmation: "已核实原付款人明确授权，未从原交易sender推定退款地址。"}, operation{Name: "verify_evm", Label: "只读核实已执行链上退款", Method: "POST", Path: base + "/{id}/verify_evm", Fields: append([]field{f("evm.transaction_hash", "实际退款交易哈希", "text"), f("evm.log_index", "真实ERC20 Transfer日志索引（0x）", "text"), f("verification_evidence", "本次链上查验依据", "textarea")}, executionValuationFields("asset")...), Confirmation: "本操作只读查询实际链上交易，不会签名或发送交易；准确接收地址由已锁定授权取值。"}, operation{Name: "verify_external", Label: "登记NOWPayments外部已核实退款", Method: "POST", Path: base + "/{id}/verify_external", Fields: append([]field{f("external.external_reference", "实际外部退款唯一流水", "text"), f("external.money.currency", "原准确资产代码", "text"), f("external.money.amount", "实际退回原币种金额", "decimal"), f("external.executed_at", "实际执行时刻", "datetime"), f("external.evidence_reference", "外部执行真实凭据", "textarea"), f("verification_evidence", "本次收款地址及事实查验依据", "textarea")}, executionValuationFields("external.money.currency")...), Confirmation: "已在外部渠道核实实际转账。本操作登记真实执行事实，不调用自动退款API。"})
	p["row_operations"] = append(p["row_operations"].([]operation), responsibleRefundOperations(base)...)
	p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "confirm_fee", Label: "核实本次实际退款手续费", Method: "POST", Path: base + "/{id}/confirm_fee", Fields: []field{f("leg_id", "原退款责任", "hidden"), f("fee_usd", "实际已核实经营手续费USD（确为零时填0）", "decimal"), f("reference", "实际结算流水", "text"), f("evidence", "最终费用核实证据", "textarea"), f("occurred_at", "实际费用发生时刻", "datetime")}, Confirmation: "只补本次经营费用，不更改客户实际收到金额和原清偿事实。"})
	p["columns"] = append(p["columns"].([]gin.H), col("actual_payment_usd", "本次实际支付USD", "usd"), col("fee_status", "手续费核实状态", "status"), col("fee_usd", "已核实手续费（待核实为空）", "usd"))
	p["row_operations"] = append(p["row_operations"].([]operation),
		operation{Name: "verify_manual_failure", Label: "核实外部付款已失败或取消", Method: "POST", Path: base + "/{id}/verify_manual_failure", Fields: []field{f("expected_execution_key", "当前实际付款指令", "hidden"), selectField("result", "外部已核实终态", "failed|实际失败", "cancelled|执行方已取消"), f("external_reference", "外部执行方唯一核实流水", "text"), f("no_further_payment_reference", "不会继续付款的独立核实证明", "textarea"), f("evidence", "实际银行/执行方终态凭据", "textarea"), f("verified_at", "真实核实时刻", "datetime")}, Confirmation: "仅用于已取得外部终态证明。超时、查不到或结果未知不能认定失败；本操作不释放剩余退款责任。"},
		operation{Name: "close_execution", Label: "封存已终止执行并重新授权剩余款", Method: "POST", Path: base + "/{id}/close_execution", Fields: []field{f("expected_execution_key", "本次旧执行编号", "hidden"), selectField("reason", "独立核实封存原因", "failed|已失败且不会再付", "terminated|已终止且不会再付", "settled_partial|实际少付且旧指令已结束"), f("no_further_payment_reference", "旧指令不会继续付款的独立证明", "textarea"), f("evidence", "核实与封存依据", "textarea")}, Confirmation: "保留历史授权、已付事实和剩余占用。封存后必须由客户重新指定剩余款目的地、财务重新核实；未知结果不会允许封存。"})
	a.resourceResponse(c, p)
}
func (a *Application) refundStaffCommand(c *gin.Context, op organization.Operator, actor string) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	var in struct {
		Action, CustomerID, OrderID, ReceiptID, Reason, Evidence string
		Amount                                                   ledger.Decimal  `json:"amount_usd"`
		Above                                                    *ledger.Decimal `json:"second_review_above"`
		Expected                                                 int64           `json:"expected_version"`
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		handleError(c, remediation.ErrInvalid)
		return
	}
	_ = json.Unmarshal(raw, &in)
	for key, dst := range map[string]*string{"action": &in.Action, "customer_id": &in.CustomerID, "order_id": &in.OrderID, "receipt_id": &in.ReceiptID} {
		_ = json.Unmarshal(fields[key], dst)
	}
	key, _ := key(c)
	var out remediation.Case
	var e error
	switch in.Action {
	case "package_exception":
		out, e = a.Remediation.RequestPackageException(c.Request.Context(), remediation.PackageExceptionInput{ActorStaffID: actor, OperatorID: op.ID, CustomerID: in.CustomerID, OrderID: in.OrderID, BusinessKey: key, Reason: in.Reason, Evidence: in.Evidence})
	case "unallocated":
		out, e = a.Remediation.RequestUnallocatedRefund(c.Request.Context(), actor, op.ID, in.ReceiptID, key, in.Reason, in.Evidence, in.Amount)
	case "policy":
		p, err := a.Remediation.SetPolicy(c.Request.Context(), actor, op.ID, in.Above, in.Expected)
		if err != nil {
			handleError(c, err)
			return
		}
		c.JSON(200, gin.H{"id": op.ID, "status": "updated", "version": p.Version})
		return
	default:
		e = remediation.ErrInvalid
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, out.State))
}
func (a *Application) refundStaffAction(c *gin.Context, op organization.Operator, actor, action string) {
	ctx := c.Request.Context()
	if a.externalRefundAction(c, actor, op.ID, action) {
		return
	}
	var e error
	switch action {
	case "review":
		var in struct{ Decision, Evidence string }
		if !body(c, &in) {
			return
		}
		_, e = a.Remediation.ReviewRefund(ctx, actor, op.ID, c.Param("id"), in.Decision, in.Evidence)
	case "cancel":
		var in struct{ Evidence string }
		if !body(c, &in) {
			return
		}
		_, e = a.Remediation.CancelRefund(ctx, actor, op.ID, c.Param("id"), in.Evidence)
	case "begin_manual":
		var in struct{ Evidence string }
		if !body(c, &in) {
			return
		}
		key, _ := key(c)
		_, e = a.Remediation.BeginManualRefund(ctx, actor, op.ID, c.Param("id"), key, in.Evidence)
	case "unknown":
		var in struct{ Evidence string }
		if !body(c, &in) {
			return
		}
		_, e = a.Remediation.MarkRefundUnknown(ctx, actor, op.ID, c.Param("id"), in.Evidence)
	case "verify_manual":
		raw, ok := decodeConfiguration(c)
		if !ok {
			return
		}
		var in remediation.ExecutionProof
		if json.Unmarshal(raw, &in) != nil {
			handleError(c, remediation.ErrInvalid)
			return
		}
		in.BusinessKey, _ = key(c)
		in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
		if in.Currency == "USD" {
			amount, err := ledger.ParseDecimal(in.Amount)
			if err != nil {
				handleError(c, err)
				return
			}
			in.ActualUSD = amount
		}
		_, e = a.Remediation.RecordManualRefund(ctx, actor, op.ID, c.Param("id"), in)
	case "execute_api", "query_api":
		var leg remediation.RefundLeg
		cases, err := a.Remediation.Cases(ctx, op.ID, "")
		if err != nil {
			handleError(c, err)
			return
		}
		for _, r := range cases {
			legs, err := a.Remediation.Legs(ctx, nil, op.ID, r.ID)
			if err != nil {
				handleError(c, err)
				return
			}
			for _, candidate := range legs {
				if candidate.ID == c.Param("id") {
					leg = candidate
				}
			}
		}
		if leg.ID == "" {
			handleError(c, remediation.ErrScope)
			return
		}
		source, err := a.Payments.PaymentSourceForReceipt(ctx, op.ID, leg.ReceiptID)
		if err != nil {
			handleError(c, err)
			return
		}
		if action == "query_api" {
			var empty struct{}
			if !body(c, &empty) {
				return
			}
			_, e = a.Payments.QueryRefund(ctx, op.ID, source.MerchantID, leg.ID)
		} else {
			raw, ok := decodeConfiguration(c)
			if !ok {
				return
			}
			var in payments.Refund
			if json.Unmarshal(raw, &in) != nil {
				handleError(c, payments.ErrInvalid)
				return
			}
			in.RefundValuation = payments.EnsureValuationIdentity(in.RefundValuation, "retail-api:"+op.ID+":"+leg.ID, in.Money.Currency)
			in.ID = leg.ID
			in.Order = source.OriginalOrder
			in.USDAmount = leg.AmountUSD
			if in.Money.Currency == "USD" && in.Money.Amount.IsZero() {
				in.Money.Amount = leg.AmountUSD
			}
			_, e = a.Payments.SubmitRefund(ctx, actor, op.ID, source.MerchantID, in)
		}
	default:
		fail(c, 404, "not_found", "退款命令不存在", false)
		return
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(c.Param("id"), "recorded"))
}
