package portal

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"gorm.io/gorm"
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
		items = append(items, customerRefundRecord(r))
	}
	c.JSON(200, gin.H{"items": items})
}

func customerRefundRecord(r remediation.Case) gin.H {
	return gin.H{"id": r.ID, "status": r.State, "amount_money": r.AmountMoney, "reason": r.Reason, "created_at": r.CreatedAt, "paid_at": r.CompletedAt}
}
func (a *Application) requestRefund(c *gin.Context) {
	fail(c, 409, "use_balance_withdrawal", "支付余额请通过余额提现申请办理，不再按原充值逐笔退回。", false)
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
func refundStaffActions(actions []string, destinationRequired bool) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, name := range actions {
		switch name {
		case "begin_manual":
			if !destinationRequired {
				name = "verify_manual"
			}
		case "unknown":
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}
func (a *Application) refundStaffResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	ctx := c.Request.Context()
	canSend := a.Organization.RequirePermission(ctx, nil, actor, op.ID, "payments.manage") == nil
	p["search_supported"], p["status_supported"] = true, true
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
			remaining := leg.AmountMoney.Sub(leg.SettledMoney)
			if remaining.Sign() < 0 {
				remaining = ledger.Zero()
			}
			v["remaining_money"] = remaining
			row := entity(leg.ID, leg.State, 1, v)
			allowed := []string{}

			ready := (r.State == "approved" || r.State == "paying") && leg.State == "approved"
			executing := leg.State == "executing" || leg.State == "unknown" || leg.State == "exception"
			if ready {
				allowed = []string{"begin_manual"}
			}
			if executing && leg.ExecutionMode == "manual" && leg.State != "exception" {
				allowed = []string{"verify_manual"}
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

				} else if status.ResponsibleExecution {
					switch leg.State {
					case "approved":
						allowed = []string{"begin_manual"}
					case "executing", "unknown":
						allowed = []string{"verify_responsible_bank"}
					}
				}
			}

			if leg.State == "completed" || leg.State == "cancelled" || leg.State == "exception" {
				allowed = []string{}
			}
			if !status.DestinationRequired && leg.SourceOperatorID == op.ID && (ready || (executing && leg.ExecutionMode == "provider")) {
				source, sourceErr := a.Payments.PaymentSourceForReceipt(ctx, op.ID, leg.ReceiptID)
				if sourceErr != nil && !errors.Is(sourceErr, gorm.ErrRecordNotFound) {
					handleError(c, sourceErr)
					return
				}
				if sourceErr == nil && payments.SupportsOriginalRefund(source.Driver) {
					allowed = []string{}
					v["original_refund_currency"] = source.Fact.Money.Currency
					v["refund_cross_currency"] = "same"
					if source.Fact.Money.Currency != source.OriginalOrder.Settlement().Currency {
						v["refund_cross_currency"] = "different"
					}
					if ready && canSend {
						allowed = []string{"execute_api"}
					}
					if executing && leg.ExecutionMode == "provider" {
						allowed = []string{"query_api"}
						executions, err := a.Payments.RefundExecutions(ctx, op.ID, source.MerchantID, leg.ID)
						if err != nil {
							handleError(c, err)
							return
						}
						row["status"] = "pending_verification"
						if len(executions) > 0 && contains([]string{"failed", "cancelled", "canceled"}, executions[len(executions)-1].State) {
							row["status"] = "exception"
						}
					}
				}
			}
			row["allowed_actions"] = refundStaffActions(allowed, status.DestinationRequired)
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
					v["actual_payment_money"] = f.ActualMoney
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
	p["columns"] = []gin.H{col("record_kind", "记录类型", "text"), col("case_id", "原申请", "text"), col("receipt_id", "原收款来源", "text"), col("amount_money", "清算责任", "money"), col("settled_money", "已真实清偿结算币种", "money"), col("remaining_money", "仍占用待付结算币种", "money"), col("reason", "原因", "text"), col("state", "状态", "status")}
	p["operations"] = []operation{{Name: "package_exception", Label: "退回未交付套餐", Method: "POST", Path: base, Fields: []field{jsonField("已付款但尚未交付 ATG")}}, {Name: "procurement_nondelivery", Label: "退回未交付采购", Method: "POST", Path: base, Fields: []field{jsonField("已付款但尚未交付 ATG")}}, {Name: "unallocated", Label: "退回未分配收款", Method: "POST", Path: base, Fields: []field{jsonField("action:'unallocated'、receipt_id、amount_money、reason、evidence")}}}
	p["row_operations"] = []operation{{Name: "review", Label: "审批", Method: "POST", Path: base + "/{id}/review", Fields: []field{selectField("decision", "审核决定", "approve|批准按原责任办理", "reject|拒绝并保留审核原因"), refundReviewNoteField()}}, {Name: "cancel", Label: "取消未执行申请", Method: "POST", Path: base + "/{id}/cancel", Fields: []field{f("evidence", "取消原因", "textarea")}}, {Name: "verify_manual", Label: "登记付款", Method: "POST", Path: base + "/{id}/verify_manual", Fields: []field{jsonField("reference、currency、amount、fee_money、occurred_at、evidence。手续费随本次付款入账，没有则填 0。")}, Confirmation: "确认已按应退金额完成付款，并填写本次手续费。提交后申请结束。没有手续费填 0。"}, {Name: "begin_manual", Label: "登记外部退款执行意图", Method: "POST", Path: base + "/{id}/begin_manual", Fields: []field{optional("evidence", "付款备注（可选）", "textarea")}, Confirmation: "用于已授权的外部退款，确认付款后登记完成。"}}

	p["columns"] = append(p["columns"].([]gin.H), col("asset", "付款币种", "text"), col("address", "收款信息", "text"), col("authorization_id", "不可变目的地授权", "text"))
	authorizationFields := []field{f("proposal_id", "本人最新确认", "hidden"), f("payer_authorization_reference", "原付款人明确授权证据", "textarea"), f("verification_evidence", "地址及原付款人核实依据", "textarea"), defaultField("responsible_execution", "已明确选择责任方付款", "hidden", false), optional("route_exception_reference", "同经营方原路受限的替代依据（跨经营方接管可留空）", "textarea")}
	p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "authorize_destination", Label: "核实客户已确认的退款目的地", Method: "POST", Path: base + "/{id}/authorize_destination", Fields: authorizationFields, Confirmation: "已核实当前行本人最新提案、原付款人授权和接收地址；锁定后不可改为其它地址。"}, operation{Name: "authorize_unallocated_destination", Label: "核实未分配款原付款人授权目的地", Method: "POST", Path: base + "/{id}/authorize_unallocated_destination", Fields: append(refundDestinationFields(), authorizationFields[1:]...), Confirmation: "已核实原付款人明确授权，未从原交易sender推定退款地址。"})
	p["row_operations"] = append(p["row_operations"].([]operation), responsibleRefundOperations(base)...)
	mode := f("refund_cross_currency", "原付款币种类型", "hidden")
	originalAmount := f("money.amount", "原币退款金额", "decimal")
	originalAmount.VisibleWhen = &fieldVisibility{Field: mode.Name, Values: []string{"different"}}
	originalEvidence := f("settlement_evidence", "原币退款金额的核实说明", "textarea")
	originalEvidence.VisibleWhen = originalAmount.VisibleWhen
	p["row_operations"] = append(p["row_operations"].([]operation),
		operation{Name: "execute_api", Label: "发起原路退款", Method: "POST", Path: base + "/{id}/execute_api", Permission: "payments.manage", Fields: []field{mode, originalAmount, originalEvidence}, Confirmation: "确认将批准的金额通过原收款通道退给原付款人。"},
		operation{Name: "query_api", Label: "核实原路退款结果", Method: "POST", Path: base + "/{id}/query_api", Fields: []field{}},
	)
	p["columns"] = append(p["columns"].([]gin.H), col("original_refund_currency", "原付款币种", "text"))
	p["columns"] = append(p["columns"].([]gin.H), col("actual_payment_money", "本次实际支付结算币种", "money"), col("fee_status", "手续费核实状态", "status"), col("fee_money", "已核实手续费（待核实为空）", "money"))
	p["row_operations"] = append(p["row_operations"].([]operation), operation{Name: "confirm_fee", Label: "登记退款手续费", Method: "POST", Path: base + "/{id}/confirm_fee", Fields: []field{f("leg_id", "原退款明细", "hidden"), f("fee_money", "已核实退款手续费（结算币种，没有则填 0）", "decimal"), optional("reference", "费用流水或收据号（可选）", "text"), f("occurred_at", "费用发生时间", "datetime"), optional("evidence", "费用说明（可选）", "textarea")}, Confirmation: "只登记经营方承担的退款费用，不改变客户应退金额。"})
	a.resourceResponse(c, p)
}
func (a *Application) refundStaffCommand(c *gin.Context, op organization.Operator, actor string) {
	raw, ok := decodeConfiguration(c)
	if !ok {
		return
	}
	var in struct {
		Action, CustomerID, OrderID, ReceiptID, Reason, Evidence string
		Amount                                                   ledger.Decimal `json:"amount_money"`
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
	case "procurement_nondelivery":
		out, e = a.Remediation.RequestProcurementNonDelivery(c.Request.Context(), remediation.ProcurementNonDeliveryInput{ActorStaffID: actor, SupplierID: op.ID, OrderID: in.OrderID, BusinessKey: key, Reason: in.Reason, Evidence: in.Evidence})
	case "unallocated":
		out, e = a.Remediation.RequestUnallocatedRefund(c.Request.Context(), actor, op.ID, in.ReceiptID, key, in.Reason, in.Evidence, in.Amount)
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
		if in.FeeStatus == "" {
			in.FeeStatus = "confirmed"
		}

		_, e = a.Remediation.RecordManualRefund(ctx, actor, op.ID, c.Param("id"), in)
	case "execute_api", "query_api":
		if !validContextID(c) {
			return
		}
		leg, err := a.refundLegForStaff(ctx, op.ID, c.Param("id"))
		if err != nil {
			handleError(c, err)
			return
		}
		if leg.SourceOperatorID != op.ID {
			handleError(c, remediation.ErrScope)
			return
		}
		source, err := a.Payments.PaymentSourceForReceipt(ctx, op.ID, leg.ReceiptID)
		if err != nil {
			handleError(c, err)
			return
		}
		var result payments.RefundStatus
		if action == "query_api" {
			var empty struct{}
			if !body(c, &empty) {
				return
			}
			result, e = a.Payments.QueryRefund(ctx, op.ID, source.MerchantID, leg.ID)
		} else {
			raw, ok := decodeConfiguration(c)
			if !ok {
				return
			}
			var input struct {
				Money    payments.Money `json:"money"`
				Evidence string         `json:"settlement_evidence"`
			}
			if json.Unmarshal(raw, &input) != nil {
				handleError(c, payments.ErrInvalid)
				return
			}
			in := payments.Refund{ID: leg.ID, Order: source.OriginalOrder, MoneyAmount: leg.AmountMoney, Money: input.Money}
			if in.Money.Currency != "" && in.Money.Currency != source.Fact.Money.Currency {
				handleError(c, payments.ErrInvalid)
				return
			}
			in.Money.Currency = source.Fact.Money.Currency
			if in.Money.Currency == in.Order.Settlement().Currency {
				if !in.Money.Amount.IsZero() && in.Money.Amount.Cmp(leg.AmountMoney) != 0 {
					handleError(c, remediation.ErrRefundFullPayment)
					return
				}
				in.Money.Amount = leg.AmountMoney
			} else {
				if in.Money.Amount.Sign() <= 0 || strings.TrimSpace(input.Evidence) == "" {
					handleError(c, payments.ErrInvalid)
					return
				}
				in.RefundValuation, e = a.refundSettlementConfirmation(ctx, "retail-api:"+op.ID+":"+leg.ID, leg.AmountMoney, strings.TrimSpace(input.Evidence))
				if e != nil {
					handleError(c, e)
					return
				}
			}
			result, e = a.Payments.SubmitRefund(ctx, actor, op.ID, source.MerchantID, in)
		}
		if e == nil && result.ProviderID != "" && result.Fact == nil && contains([]string{"failed", "cancelled", "canceled"}, result.State) {
			record, err := a.Remediation.GetCase(ctx, nil, op.ID, leg.CaseID)
			if err != nil {
				handleError(c, err)
				return
			}
			if record.Kind == "package_exception" || record.Kind == "unallocated" {
				// Require stored channel evidence before asking for an alternate destination.
				_, e = a.Remediation.CloseRefundExecution(ctx, actor, op.ID, leg.ID, remediation.RefundExecutionClosureInput{
					BusinessKey:               "original-refund-failed:" + leg.ID,
					ExpectedExecutionKey:      leg.ID,
					Reason:                    "failed",
					NoFurtherPaymentReference: source.Driver + ":" + result.ProviderID,
					Evidence:                  "Original payment provider confirmed a terminal refund failure",
				})
			}
		}
		if errors.Is(e, payments.ErrOutcomeUnknown) || (e == nil && result.State != "succeeded" && result.State != "closed" && !contains([]string{"failed", "cancelled", "canceled"}, result.State)) {
			c.JSON(200, gin.H{"id": leg.ID, "status": "pending_verification", "message": "原路退款结果待核实，请在原退款记录继续查询。"})
			return
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

// An ordinary approval is already an authenticated recorded decision.
func refundReviewNoteField() field {
	note := f("evidence", "拒绝原因", "textarea")
	note.VisibleWhen = &fieldVisibility{Field: "decision", Values: []string{"reject"}}

	return note
}
