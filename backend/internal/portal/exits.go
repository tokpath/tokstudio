package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"time"
)

func (a *Application) exitsResource(c *gin.Context, op organization.Operator, actor string, p gin.H, base string) {
	ctx := c.Request.Context()
	rows, e := a.Cooperation.ListProcurementExits(ctx, nil, actor, op.ID)
	if e != nil {
		handleError(c, e)
		return
	}
	canPay := a.Organization.RequirePermission(ctx, nil, actor, op.ID, "payments.manage") == nil
	canCooperate := a.Organization.RequirePermission(ctx, nil, actor, op.ID, "cooperation.manage") == nil
	items := []any{}
	for _, r := range rows {
		v := values(r)
		v["record_kind"] = "exit"
		row := entity(r.ID, r.State, 0, v)
		row["allowed_actions"] = []string{}
		if canPay && r.SupplierID == op.ID {
			row["allowed_actions"] = []string{"prepare_refund"}
		}
		items = append(items, row)
	}

	seen := map[string]bool{}
	if a.Organization.RequirePermission(ctx, nil, actor, op.ID, "payments.manage") == nil {
		_, legs, e := a.ProcurementRefunds.ListForOperator(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
		for _, leg := range legs {
			if leg.SupplierID != op.ID {
				continue
			}
			v := values(leg)
			v["record_kind"] = "refund_leg"
			v["contract_usd"] = leg.AmountUSD
			remaining := leg.AmountUSD.Sub(leg.SettledUSD)
			if remaining.Sign() < 0 {
				remaining = ledger.Zero()
			}
			v["remaining_usd"] = remaining
			v["expected_execution_key"] = leg.ExecutionKey
			sourceDriver := "manual"
			manualOffline := true
			if leg.SourceKind == "manual_receipt" {
				receipt, err := a.Commerce.GetReceipt(ctx, nil, op.ID, leg.ReceiptID)
				if err != nil {
					handleError(c, err)
					return
				}
				manualOffline = receipt.Network == "" || receipt.Network == "bank" || receipt.Network == "cash"
				v["proof"] = gin.H{"receipt_id": receipt.ID, "recipient_reference": receipt.PayerReference, "money": gin.H{"currency": receipt.Currency}}
			} else {
				source, err := a.Payments.PaymentSourceForReceipt(ctx, op.ID, leg.ReceiptID)
				if err != nil {
					handleError(c, err)
					return
				}
				sourceDriver = source.Driver
			}
			actions := []string{}
			external := sourceDriver == "evm_wallet" || sourceDriver == "nowpayments"
			authorized := false
			if external {
				authorized, e = a.procurementDestinationFacts(c, actor, op.ID, leg.ID, v)
				if e != nil {
					handleError(c, e)
					return
				}
			}
			if !external {
				_, e = a.procurementDestinationFacts(c, actor, op.ID, leg.ID, v)
				if e != nil {
					handleError(c, e)
					return
				}
			}
			version, _ := v["route_version"].(int64)
			if version > 1 {
				external = true
				authorized = v["authorization_id"] != nil
			}
			switch leg.State {
			case "prepared":
				actions = []string{"execute_refund", "begin_manual", "release_refund"}
				if sourceDriver == "manual" {
					actions = []string{"release_refund"}
					if manualOffline {
						actions = append(actions, "begin_manual")
					}
				}
				if external {
					actions = []string{"release_refund"}
					if authorized {
						actions = append(actions, "begin_manual")
					} else if v["proposal_id"] != nil {
						actions = append(actions, "verify_destination")
					}
				}
			case "manual_executing", "executing":
				if sourceDriver == "evm_wallet" {
					actions = []string{"verify_evm"}
				} else if sourceDriver == "nowpayments" {
					actions = []string{"verify_external"}
				} else {
					actions = []string{"verify_manual"}
				}
			case "exception":
				if leg.SettledUSD.Cmp(leg.AmountUSD) < 0 {
					actions = []string{"begin_manual"}
				}
			}
			if version > 1 && leg.State == "prepared" {
				actions = []string{"release_refund"}
				if authorized {
					actions = append(actions, "begin_manual")
				} else if v["proposal_id"] != nil {
					actions = append(actions, "verify_replacement_destination")
				}
			}
			if version > 1 && (leg.State == "manual_executing" || leg.State == "executing") {
				actions = []string{"verify_replacement"}
			}
			if remaining.Sign() > 0 {
				if leg.State == "prepared" || leg.State == "exception" {
					actions = append(actions, "close_route")
				}
				if leg.ExecutionMode == "manual" && leg.State == "manual_executing" {
					actions = append(actions, "verify_manual_failure")
				}
			}
			row := entity(leg.ID, leg.State, 0, v)
			if leg.State == "completed" && leg.FeeStatus == "pending" {
				actions = append(actions, "confirm_fee")
			}
			row["allowed_actions"] = actions
			items = append(items, row)
			seen[leg.ID] = true
		}
	}
	buyerLegs := []cooperation.BuyerRefundLegView{}
	if canCooperate {
		buyerLegs, e = a.ProcurementRefunds.BuyerRefundLegs(ctx, actor, op.ID)
		if e != nil {
			handleError(c, e)
			return
		}
	}
	for _, leg := range buyerLegs {
		if seen[leg.ID] {
			continue
		}
		v := values(leg)
		v["record_kind"] = "buyer_refund_leg"
		v["contract_usd"] = leg.AmountUSD
		remaining := leg.AmountUSD.Sub(leg.SettledUSD)
		if remaining.Sign() < 0 {
			remaining = ledger.Zero()
		}
		v["remaining_usd"] = remaining
		actions := []string{}
		{
			v["destination"] = gin.H{"network": leg.Network, "asset": leg.Currency, "token_contract": leg.TokenContract}
			authorized, e := a.procurementDestinationFacts(c, actor, op.ID, leg.ID, v)
			if e != nil {
				handleError(c, e)
				return
			}
			version, _ := v["route_version"].(int64)
			if leg.State == "prepared" && !authorized && (leg.Driver == "evm_wallet" || leg.Driver == "nowpayments" || version > 1) {
				actions = []string{"propose_destination"}
			}
		}
		row := entity(leg.ID, leg.State, 0, v)
		row["allowed_actions"] = actions
		items = append(items, row)
	}
	p["title"] = "合作退出与采购退款"
	p["description"] = "先按原合同及真实责任覆盖移除库存，再逐原收款来源办理退款。客户未了责任不会因退出按钮消失。未知付款保留占用。"
	p["items"] = items
	p["columns"] = []gin.H{col("record_kind", "记录类型", "text"), col("buyer_id", "采购方", "text"), col("supplier_id", "供货方", "text"), col("contract_usd", "合同退款责任", "usd"), col("book_cost_usd", "退出账面成本", "usd"), col("cost_difference_usd", "合同与成本差额", "usd"), col("refunded_usd", "已核实退回", "usd"), col("receipt_id", "原收款", "text")}
	p["operations"] = []operation{{Name: "request", Label: "按合同申请退出库存", Permission: "cooperation.manage", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "text", "request"), selectField("mode", "退出原因", "voluntary_exit|按合同自愿退出", "supply_failure|已有供货失约批准"), f("quantity", "拟移除采购库存ATG", "decimal"), optional("approval_id", "供货失约批准编号（失约退出必填）", "text"), optional("evidence", "申请说明（可选）", "textarea")}, Confirmation: "将按原采购批次条款计算可退款并真实移除库存；仅未占用且责任已结清或已批准承接部分可以退出。"}, {Name: "approve_failure", Label: "供货方批准真实供货失约", Permission: "refunds.manage", Method: "POST", Path: base, Fields: []field{defaultField("action", "办理类型", "text", "approve_failure"), f("buyer_id", "直属采购方编号", "text"), f("evidence", "实际供货失约与承接责任依据", "textarea")}}}
	ops := p["operations"].([]operation)
	for i := range ops[0].Fields {
		if ops[0].Fields[i].Name == "approval_id" {
			ops[0].Fields[i].VisibleWhen = &fieldVisibility{Field: "mode", Values: []string{"supply_failure"}}
		}
	}
	p["row_operations"] = []operation{{Name: "prepare_refund", Label: "按原来源预留本次退款", Method: "POST", Path: base + "/{id}/prepare_refund", Fields: []field{f("receipt_id", "原采购收款编号", "text"), f("amount_usd", "本次合同退款USD", "decimal")}}, {Name: "begin_manual", Label: "记录线下退款意图", Method: "POST", Path: base + "/{id}/begin_manual", Fields: []field{f("evidence", "线下付款计划与说明", "textarea")}, Confirmation: "仅保存唯一付款意图，实际付款后再核实退款结果。"}, {Name: "verify_manual", Label: "核实原付款人实际退款", Method: "POST", Path: base + "/{id}/verify_manual", Fields: append([]field{optional("proof.external_reference", "退款流水或收据号（可选）", "text"), f("proof.money.currency", "本次付款币种", "text"), f("proof.money.amount", "本次实际付款金额", "decimal"), f("proof.executed_at", "实际退款时间", "datetime"), f("proof.evidence_reference", "付款核实说明", "textarea")}, executionValuationFields("proof.money.currency")...)}, {Name: "execute_refund", Label: "通过原通道执行退款", Method: "POST", Path: base + "/{id}/execute_refund", Fields: append([]field{f("money.currency", "本次付款币种", "text"), f("money.amount", "本次实际执行金额", "decimal")}, executionValuationFields("money.currency")...), Confirmation: "使用本次真实执行汇率，退款ID固定为原leg；未知时只查询原交易。"}, {Name: "release_refund", Label: "取消尚未执行的预留", Method: "POST", Path: base + "/{id}/release_refund", Fields: []field{f("evidence", "取消依据", "textarea")}}}
	p["row_operations"] = append(p["row_operations"].([]operation), procurementDestinationOperations(base)...)
	p["row_operations"] = tagOperationPermissions(p["row_operations"].([]operation), "payments.manage", "prepare_refund", "begin_manual", "verify_manual", "execute_refund", "release_refund", "confirm_fee", "close_route", "verify_manual_failure", "verify_replacement_destination", "verify_replacement", "verify_destination", "verify_evm", "verify_external")
	p["row_operations"] = tagOperationPermissions(p["row_operations"].([]operation), "cooperation.manage", "propose_destination")
	p["columns"] = append(p["columns"].([]gin.H), col("actual_usd", "实际累计退付USD", "usd"), col("settled_usd", "清偿责任USD", "usd"), col("remaining_usd", "待付余量USD", "usd"), col("excess_usd", "多付异常USD", "usd"), col("network", "准确网络", "text"), col("asset", "准确资产", "text"), col("token_contract", "代币合约", "text"), col("address", "明确授权地址", "text"), col("authorization_id", "独立核实授权", "text"), col("route_version", "剩余款路线版本", "text"), col("route_history", "已封存历史", "details"))
	a.resourceResponse(c, p)
}
func executionValuationFields(currencyFields ...string) []field {
	currency := "currency"
	if len(currencyFields) > 0 {
		currency = currencyFields[0]
	}
	fields := []field{f("valuation.currency", "本次估值币种", "text"), f("valuation.usd_per_unit", "本次真实每单位USD", "decimal"), f("valuation.source", "本次授权汇率/实际换汇依据", "text"), f("valuation.effective_at", "本次估值生效", "datetime"), f("valuation.expires_at", "本次估值失效", "datetime"), f("valued_at", "实际估值时刻", "datetime"), f("fx_execution_reference", "本次执行凭证/原币种结算依据", "text")}
	for i := range fields {
		fields[i].VisibleWhen = &fieldVisibility{Field: currency, Values: []string{"USD"}, Not: true, FallbackFields: []string{"asset", "currency"}}
	}
	return fields
}
func (a *Application) exitsCommand(c *gin.Context, op organization.Operator, actor string) {
	var in struct {
		Action     string         `json:"action"`
		Mode       string         `json:"mode"`
		Quantity   ledger.Decimal `json:"quantity"`
		ApprovalID string         `json:"approval_id"`
		BuyerID    string         `json:"buyer_id"`
		Evidence   string         `json:"evidence"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	key, _ := key(c)
	if in.Action == "request" && in.Evidence == "" {
		in.Evidence = "采购方确认按原成交约定申请退出"
	}
	raw, e := a.command(c, "exit:"+op.ID, in, func(tx *gorm.DB) (any, error) {
		if in.Action == "approve_failure" {
			agreement, e := a.Organization.GetSupplyAgreement(ctx, tx, in.BuyerID)
			if e != nil {
				return nil, e
			}
			if agreement.SupplierID != op.ID {
				return nil, organization.ErrForbidden
			}
			out, e := a.Cooperation.ApproveSupplyFailure(ctx, tx, actor, in.BuyerID, in.Evidence)
			return commandResult(out.ID, "approved"), e
		}
		if in.Action != "request" {
			return nil, cooperation.ErrConflict
		}
		var guard cooperation.RemovalGuard = cooperation.CoverageRemovalGuard{Cooperation: a.Cooperation, PaymentFacts: a.Payments}
		if in.Mode == "supply_failure" {
			guard = cooperation.ApprovedSupplyFailureGuard{Cooperation: a.Cooperation, PaymentFacts: a.Payments, ApprovalID: in.ApprovalID}
		}
		out, e := a.Cooperation.RequestProcurementExit(ctx, tx, actor, op.ID, key, in.Mode, in.Evidence, in.Quantity, guard)
		return commandResult(out.ID, out.State), e
	})
	jsonResponse(c, raw, e)
}
func (a *Application) exitsAction(c *gin.Context, op organization.Operator, actor, action string) {
	if a.procurementDestinationAction(c, actor, op.ID, action) {
		return
	}
	var in struct {
		ReceiptID string                            `json:"receipt_id"`
		Amount    ledger.Decimal                    `json:"amount_usd"`
		Evidence  string                            `json:"evidence"`
		Money     payments.Money                    `json:"money"`
		Valuation payments.Valuation                `json:"valuation"`
		ValuedAt  time.Time                         `json:"valued_at"`
		FX        string                            `json:"fx_execution_reference"`
		Proof     payments.ManualRefundVerification `json:"proof"`
	}
	if !body(c, &in) {
		return
	}
	ctx := c.Request.Context()
	id := c.Param("id")
	key, _ := key(c)
	var result any
	var e error
	// The domain requires the exact supplier's finance authority for each leg.
	switch action {
	case "prepare_refund":
		out, err := a.ProcurementRefunds.PrepareRefund(ctx, actor, id, in.ReceiptID, key, in.Amount)
		e = err
		result = commandResult(out.ID, out.State)
	case "execute_refund":
		in.Valuation = payments.EnsureValuationIdentity(in.Valuation, "procurement-api:"+op.ID+":"+id, in.Money.Currency)
		out, err := a.ProcurementRefunds.Execute(ctx, actor, id, in.Money, in.Valuation, in.ValuedAt, in.FX)
		e = err
		result = commandResult(id, out.State)
	case "begin_manual":
		out, err := a.ProcurementRefunds.BeginManualRefund(ctx, actor, id, key, in.Evidence)
		e = err
		result = commandResult(id, out.State)
	case "verify_manual":
		if err := a.Organization.RequirePermission(ctx, nil, actor, op.ID, "payments.manage"); err != nil {
			handleError(c, err)
			return
		}
		var leg cooperation.ProcurementExitRefund
		if err := a.DB.WithContext(ctx).Where("id=? AND supplier_id=?", id, op.ID).Take(&leg).Error; err != nil {
			handleError(c, err)
			return
		}
		receipt, err := a.Commerce.GetReceipt(ctx, nil, op.ID, leg.ReceiptID)
		if err != nil {
			handleError(c, err)
			return
		}
		in.Proof.ReceiptID = leg.ReceiptID
		in.Proof.RecipientReference = receipt.PayerReference
		in.Valuation = payments.EnsureValuationIdentity(in.Valuation, "procurement-offline:"+op.ID+":"+id+":"+in.Proof.ExternalReference, in.Proof.Money.Currency)
		out, err := a.ProcurementRefunds.RecordManualRefund(ctx, actor, id, in.Proof, in.Valuation, in.ValuedAt, in.FX)
		e = err
		result = commandResult(id, out.State)
	case "release_refund":
		e = a.ProcurementRefunds.ReleasePrepared(ctx, actor, id, in.Evidence)
		result = commandResult(id, "released_unpaid")
	default:
		e = cooperation.ErrConflict
	}
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, result)
}

var _ json.RawMessage
