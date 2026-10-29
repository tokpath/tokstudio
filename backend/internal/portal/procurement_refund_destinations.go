package portal

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"time"
)

func (a *Application) procurementDestinationAction(c *gin.Context, actor, operator, action string) bool {
	switch action {
	case "confirm_fee":
		_, legs, scopeErr := a.ProcurementRefunds.ListForOperator(c.Request.Context(), actor, operator)
		if scopeErr != nil {
			handleError(c, scopeErr)
			return true
		}
		matched := false
		for _, leg := range legs {
			matched = matched || (leg.ID == c.Param("id") && leg.SupplierID == operator)
		}
		if !matched {
			handleError(c, payments.ErrForbidden)
			return true
		}
		var in cooperation.ProcurementRefundAccounting
		if !body(c, &in) {
			return true
		}
		in.RefundID = c.Param("id")
		in.BusinessKey, _ = key(c)
		out, e := a.ProcurementRefunds.ConfirmRefundAccounting(c.Request.Context(), actor, in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.RefundID, "confirmed"))
		return true
	case "close_route":
		var in struct {
			Reason   string `json:"reason"`
			Evidence string `json:"evidence"`
		}
		if !body(c, &in) {
			return true
		}
		out, e := a.ProcurementRefunds.CloseRefundRoute(c.Request.Context(), actor, operator, c.Param("id"), in.Reason, in.Evidence)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "awaiting_new_destination"))
		return true
	case "verify_manual_failure":
		var in cooperation.ManualRefundFailureInput
		if !body(c, &in) {
			return true
		}
		in.BusinessKey, _ = key(c)
		out, e := a.ProcurementRefunds.RecordManualRefundFailure(c.Request.Context(), actor, operator, c.Param("id"), in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "exception"))
		return true
	case "verify_replacement_destination":
		var in struct {
			ProposalID string `json:"proposal_id"`
			MerchantID string `json:"merchant_id"`
			Evidence   string `json:"evidence"`
		}
		if !body(c, &in) {
			return true
		}
		out, e := a.ProcurementRefunds.VerifyReplacementRefundDestination(c.Request.Context(), actor, operator, in.ProposalID, in.MerchantID, in.Evidence)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "destination_authorized"))
		return true
	case "propose_destination":
		var in struct {
			Destination        payments.ExternalRefundDestination `json:"destination"`
			PayerAuthorization string                             `json:"payer_authorization_reference"`
		}
		if !body(c, &in) {
			return true
		}
		out, e := a.ProcurementRefunds.ProposeRefundDestination(c.Request.Context(), actor, operator, c.Param("id"), in.Destination, in.PayerAuthorization)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "awaiting_independent_supplier_verification"))
		return true
	case "verify_destination":
		var in struct {
			ProposalID string `json:"proposal_id"`
			Evidence   string `json:"evidence"`
		}
		if !body(c, &in) {
			return true
		}
		out, e := a.ProcurementRefunds.VerifyRefundDestination(c.Request.Context(), actor, operator, in.ProposalID, in.Evidence)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "destination_authorized"))
		return true
	case "verify_evm", "verify_external", "verify_replacement":
		var in struct {
			EVM                payments.EVMRefundVerification      `json:"evm"`
			External           payments.ExternalRefundVerification `json:"external"`
			Valuation          payments.Valuation                  `json:"-"`
			SettlementAmount   ledger.Decimal                      `json:"settlement_amount"`
			SettlementEvidence string                              `json:"settlement_evidence"`
		}
		if !body(c, &in) {
			return true
		}
		confirmation, confirmationErr := a.refundSettlementConfirmation(c.Request.Context(), "procurement-external:"+operator+":"+c.Param("id")+":"+in.External.ExternalReference+":"+in.EVM.TransactionHash+":"+in.EVM.LogIndex, in.SettlementAmount, in.SettlementEvidence)
		if confirmationErr != nil {
			handleError(c, confirmationErr)
			return true
		}
		in.Valuation = confirmation
		var out payments.RefundStatus
		var e error
		if action == "verify_replacement" {
			out, e = a.ProcurementRefunds.RecordReplacementRefund(c.Request.Context(), actor, operator, c.Param("id"), in.EVM, in.External, in.Valuation, time.Time{}, "")
		} else if action == "verify_evm" {
			out, e = a.ProcurementRefunds.RecordEVMRefund(c.Request.Context(), actor, c.Param("id"), in.EVM, in.Valuation, time.Time{}, "")
		} else {
			out, e = a.ProcurementRefunds.RecordExternalRefund(c.Request.Context(), actor, c.Param("id"), in.External, in.Valuation, time.Time{}, "")
		}
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(c.Param("id"), out.State))
		return true
	}
	return false
}
func (a *Application) procurementDestinationFacts(c *gin.Context, actor, operator, id string, v gin.H) (bool, error) {
	proposals, authorizations, e := a.ProcurementRefunds.RefundDestinations(c.Request.Context(), actor, operator, id)
	if e != nil {
		return false, e
	}
	var raw json.RawMessage
	status, e := a.ProcurementRefunds.RefundRouteStatus(c.Request.Context(), actor, operator, id)
	if e != nil {
		return false, e
	}
	v["route_version"], v["route_history"] = status.Version, status.Closures
	currentProposals := proposals[:0]
	for _, p := range proposals {
		if p.Version == status.Version {
			currentProposals = append(currentProposals, p)
		}
	}
	proposals = currentProposals
	currentAuthorizations := authorizations[:0]
	for _, a := range authorizations {
		if a.Version == status.Version {
			currentAuthorizations = append(currentAuthorizations, a)
		}
	}
	authorizations = currentAuthorizations
	if len(proposals) > 0 {
		latest := proposals[len(proposals)-1]
		v["proposal_id"] = latest.ID
		raw = latest.Destination
	}
	authorized := len(authorizations) > 0
	if authorized {
		v["authorization_id"] = authorizations[0].ID
		raw = authorizations[0].Destination
	}
	if len(raw) > 0 {
		var destination map[string]any
		if e = json.Unmarshal(raw, &destination); e != nil {
			return false, e
		}
		v["destination"] = destination
		for _, name := range []string{"network", "asset", "token_contract", "address"} {
			v[name] = destination[name]
		}
	}
	return authorized, nil
}
func procurementDestinationOperations(base string) []operation {
	return []operation{
		{Name: "confirm_fee", Label: "核实采购退款实际手续费", Method: "POST", Path: base + "/{id}/confirm_fee", Fields: []field{f("fee_money", "实际已核实手续费结算币种（确为零填0）", "decimal"), f("settled_at", "真实费用结算时刻", "datetime"), f("evidence", "费用结算独立证据", "textarea")}, Confirmation: "只补经营方实际费用，不更改原买方清偿金额。"},
		{Name: "close_route", Label: "封存旧采购退款路线并重授权剩余款", Method: "POST", Path: base + "/{id}/close_route", Fields: []field{f("reason", "已核实旧路线终止原因", "text"), f("evidence", "旧执行终态与不会再付的独立证明", "textarea")}, Confirmation: "原收款容量、待付本金与历史授权全部保留。未知付款不得关闭，后续须买方重新明确目的地、供方独立核实本方新商户。"},
		{Name: "verify_manual_failure", Label: "核实原外部付款已失败或取消", Method: "POST", Path: base + "/{id}/verify_manual_failure", Fields: []field{f("expected_execution_key", "当前原执行编号", "hidden"), selectField("result", "执行方已核实终态", "failed|实际失败", "cancelled|实际取消"), f("external_reference", "外部终态核实流水", "text"), f("no_further_payment_reference", "执行方不会继续付款的独立证明", "textarea"), f("evidence", "真实外部核实凭证", "textarea"), f("verified_at", "真实核实时刻", "datetime")}},
		{Name: "verify_replacement_destination", Label: "独立核实剩余款目的地与本方新商户", Method: "POST", Path: base + "/{id}/verify_replacement_destination", Fields: []field{f("proposal_id", "本轮买方提案", "hidden"), f("merchant_id", "本供方独立配置的新付款商户", "text"), f("evidence", "原付款人授权、地址及本方付款路线核实证据", "textarea")}},
		{Name: "verify_replacement", Label: "核实本方新路线实际退款", Method: "POST", Path: base + "/{id}/verify_replacement", Fields: append([]field{optional("evm.transaction_hash", "EVM实际交易Hash（链上必填）", "text"), optional("evm.log_index", "EVM真实日志索引（0x）", "text"), optional("external.external_reference", "银行/NOW实际付款流水", "text"), optional("external.money.currency", "银行/NOW实际准确币种", "text"), optional("external.money.amount", "银行/NOW实际原币金额", "decimal"), optional("external.executed_at", "银行/NOW真实执行时刻", "datetime"), optional("external.evidence_reference", "银行/NOW核实凭证", "textarea")}, executionValuationFields("external.money.currency")...), Confirmation: "依据已锁定本轮新商户与目的地只读核实真实付款，不签名或发送交易，不更改历史实际事实。"},

		{Name: "propose_destination", Label: "买方明确授权稳定币退款地址", Method: "POST", Path: base + "/{id}/propose_destination", Fields: append(refundDestinationFields(), f("payer_authorization_reference", "原付款人明确授权依据", "textarea")), Confirmation: "买方已核对准确资产、网络、代币合约和接收地址，并取得原付款人授权；供方须另由不同人员独立核实。"},
		{Name: "verify_destination", Label: "供方独立核实买方退款地址", Method: "POST", Path: base + "/{id}/verify_destination", Fields: []field{f("proposal_id", "买方原提案", "hidden"), f("evidence", "独立核实付款人授权和接收地址证据", "textarea")}, Confirmation: "已核对当前行准确资产、网络、合约和地址。核实人员须与提案人不同，锁定后按本目的地执行。"},
		{Name: "verify_evm", Label: "只读核实已执行采购链上退款", Method: "POST", Path: base + "/{id}/verify_evm", Fields: append([]field{f("evm.transaction_hash", "实际退款交易哈希", "text"), f("evm.log_index", "真实Transfer日志索引（0x）", "text")}, executionValuationFields("asset")...), Confirmation: "只读查询真实链上执行；不签名、不发送、不按原sender猜目的地。"},
		{Name: "verify_external", Label: "登记采购NOW外部实际退款", Method: "POST", Path: base + "/{id}/verify_external", Fields: append([]field{f("external.external_reference", "外部退款唯一流水", "text"), f("external.money.currency", "原准确资产代码", "text"), f("external.money.amount", "本次实际原币金额", "decimal"), f("external.executed_at", "真实执行时刻", "datetime"), f("external.evidence_reference", "外部核实凭证", "textarea")}, executionValuationFields("external.money.currency")...), Confirmation: "已在外部核实真实退款；系统只记录实际事实，不假称自动退款。"},
	}
}
