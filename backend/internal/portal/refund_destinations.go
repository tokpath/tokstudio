package portal

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"time"
)

func (a *Application) customerRefundDestinations(c *gin.Context) {
	id := subject(c)
	rows, e := a.Remediation.CustomerRefundDestinations(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, gin.H{"items": rows})
}
func (a *Application) proposeRefundDestination(c *gin.Context) {
	var in remediation.RefundDestinationProposalInput
	if !body(c, &in) {
		return
	}
	if c.GetBool("responsible_refund_destination") {
		in.ResponsibleExecution = true
	}
	id := subject(c)
	key, _ := key(c)
	out, e := a.Remediation.ProposeRefundDestination(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID, c.Param("id"), key, in)
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, commandResult(out.ID, "awaiting_finance_verification"))
}
func (a *Application) proposeResponsibleRefundDestination(c *gin.Context) {
	c.Set("responsible_refund_destination", true)
	a.proposeRefundDestination(c)
}
func (a *Application) staffRefundDestination(c *gin.Context) {
	op, actor, ok := a.staffScope(c, "refunds")
	if !ok {
		return
	}
	out, e := a.Remediation.RefundDestinationStatusForStaff(c.Request.Context(), actor, op.ID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	c.JSON(200, out)
}
func refundDestinationFields() []field {
	return []field{f("destination.network", "准确退款网络（银行填bank，EVM填十进制Chain ID）", "text"), f("destination.asset", "原收款准确币种或资产代码", "text"), optional("destination.token_contract", "代币合约（银行留空）", "text"), f("destination.address", "本人明确指定接收地址或银行账户凭证", "text")}
}
func (a *Application) refundLegForStaff(ctx context.Context, op, id string) (remediation.RefundLeg, error) {
	cases, e := a.Remediation.Cases(ctx, op, "")
	if e != nil {
		return remediation.RefundLeg{}, e
	}
	for _, c := range cases {
		legs, e := a.Remediation.Legs(ctx, nil, op, c.ID)
		if e != nil {
			return remediation.RefundLeg{}, e
		}
		for _, l := range legs {
			if l.ID == id {
				return l, nil
			}
		}
	}
	return remediation.RefundLeg{}, remediation.ErrScope
}
func (a *Application) externalRefundAction(c *gin.Context, actor, operator, action string) bool {
	switch action {
	case "confirm_fee":
		var in struct {
			LegID      string         `json:"leg_id"`
			FeeUSD     ledger.Decimal `json:"fee_usd"`
			Reference  string         `json:"reference"`
			Evidence   string         `json:"evidence"`
			OccurredAt time.Time      `json:"occurred_at"`
		}
		if !body(c, &in) {
			return true
		}
		rows, e := a.Remediation.RefundAccounting(c.Request.Context(), nil, actor, operator, in.LegID)
		if e != nil {
			handleError(c, e)
			return true
		}
		business := ""
		for _, r := range rows {
			if r.ID == c.Param("id") {
				business = r.BusinessKey
			}
		}
		if business == "" {
			handleError(c, remediation.ErrScope)
			return true
		}
		k, _ := key(c)
		out, e := a.Remediation.ConfirmRefundFee(c.Request.Context(), nil, remediation.RefundFeeInput{OperatorID: operator, LegID: in.LegID, PaymentBusinessKey: business, BusinessKey: k, ActorID: actor, FeeUSD: in.FeeUSD, Reference: in.Reference, Evidence: in.Evidence, OccurredAt: in.OccurredAt})
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "confirmed"))
		return true
	case "close_execution":
		var in remediation.RefundExecutionClosureInput
		if !body(c, &in) {
			return true
		}
		in.BusinessKey, _ = key(c)
		out, e := a.Remediation.CloseRefundExecution(c.Request.Context(), actor, operator, c.Param("id"), in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "awaiting_new_destination"))
		return true
	case "verify_manual_failure":
		var in remediation.ManualRefundFailureInput
		if !body(c, &in) {
			return true
		}
		in.BusinessKey, _ = key(c)
		out, e := a.Remediation.RecordManualRefundFailure(c.Request.Context(), actor, operator, c.Param("id"), in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "exception"))
		return true
	case "authorize_destination", "authorize_unallocated_destination":
		var in remediation.RefundDestinationAuthorizationInput
		if !body(c, &in) {
			return true
		}
		key, _ := key(c)
		out, e := a.Remediation.AuthorizeRefundDestination(c.Request.Context(), actor, operator, c.Param("id"), key, in)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "destination_authorized"))
		return true
	case "verify_evm", "verify_external", "verify_responsible_evm", "verify_responsible_external", "verify_responsible_bank":
		var in struct {
			MerchantID           string                              `json:"merchant_id"`
			EVM                  payments.EVMRefundVerification      `json:"evm"`
			External             payments.ExternalRefundVerification `json:"external"`
			Valuation            payments.Valuation                  `json:"valuation"`
			ValuedAt             time.Time                           `json:"valued_at"`
			FX                   string                              `json:"fx_execution_reference"`
			VerificationEvidence string                              `json:"verification_evidence"`
		}
		if !body(c, &in) {
			return true
		}
		ctx := c.Request.Context()
		leg, e := a.refundLegForStaff(ctx, operator, c.Param("id"))
		if e != nil {
			handleError(c, e)
			return true
		}
		status, e := a.Remediation.RefundDestinationStatusForStaff(ctx, actor, operator, leg.ID)
		if e != nil {
			handleError(c, e)
			return true
		}
		if status.AuthorizedDestination == nil || status.AuthorizationID == "" {
			handleError(c, remediation.ErrReview)
			return true
		}
		d := status.AuthorizedDestination
		destination := payments.ExternalRefundDestination{Network: d.Network, Asset: d.Asset, TokenContract: d.TokenContract, Address: d.Address, AuthorizationReference: status.AuthorizationID, VerificationEvidence: in.VerificationEvidence}
		in.Valuation = payments.EnsureValuationIdentity(in.Valuation, "retail-external:"+operator+":"+leg.ID+":"+in.External.ExternalReference+":"+in.EVM.TransactionHash+":"+in.EVM.LogIndex, d.Asset)
		refund := payments.Refund{ID: leg.ID, USDAmount: leg.AmountUSD, RefundValuation: in.Valuation, ValuedAt: in.ValuedAt, FXExecutionReference: in.FX}
		in.EVM.ReceiptID, in.EVM.Destination = leg.ReceiptID, destination
		in.External.ReceiptID, in.External.Destination = leg.ReceiptID, destination
		var out payments.RefundStatus
		if status.ResponsibleExecution {
			if in.MerchantID == "" {
				handleError(c, payments.ErrInvalid)
				return true
			}
			switch action {
			case "verify_responsible_evm":
				out, e = a.Payments.RecordResponsibleEVMRefund(ctx, actor, operator, in.MerchantID, refund, in.EVM)
			case "verify_responsible_external":
				out, e = a.Payments.RecordResponsibleExternalRefund(ctx, actor, operator, in.MerchantID, refund, in.External)
			case "verify_responsible_bank":
				out, e = a.Payments.RecordResponsibleBankRefund(ctx, actor, operator, in.MerchantID, refund, in.External)
			default:
				handleError(c, remediation.ErrState)
				return true
			}
		} else {
			if leg.SourceOperatorID != operator {
				handleError(c, remediation.ErrScope)
				return true
			}
			source, err := a.Payments.PaymentSourceForReceipt(ctx, operator, leg.ReceiptID)
			if err != nil {
				handleError(c, err)
				return true
			}
			refund.Order = source.OriginalOrder
			switch action {
			case "verify_evm":
				out, e = a.Payments.RecordEVMRefund(ctx, actor, operator, source.MerchantID, refund, in.EVM)
			case "verify_external":
				out, e = a.Payments.RecordExternalRefund(ctx, actor, operator, source.MerchantID, refund, in.External)
			default:
				handleError(c, remediation.ErrState)
				return true
			}
		}
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(leg.ID, out.State))
		return true
	}
	return false
}
