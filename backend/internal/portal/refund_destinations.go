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
	return []field{defaultField("destination.network", "网络", "hidden", "bank"), f("destination.asset", "付款币种", "text"), f("destination.address", "收款信息", "text")}
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
			FeeMoney   ledger.Decimal `json:"fee_money"`
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
		if in.Reference == "" {
			in.Reference = "manual:refund-fee:" + c.Param("id")
		}
		out, e := a.Remediation.ConfirmRefundFee(c.Request.Context(), nil, remediation.RefundFeeInput{OperatorID: operator, LegID: in.LegID, PaymentBusinessKey: business, BusinessKey: k, ActorID: actor, FeeMoney: in.FeeMoney, Reference: in.Reference, Evidence: in.Evidence, OccurredAt: in.OccurredAt})
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(out.ID, "confirmed"))
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
	case "verify_responsible_bank":
		var in struct {
			MerchantID           string                              `json:"merchant_id"`
			External             payments.ExternalRefundVerification `json:"external"`
			Valuation            payments.Valuation                  `json:"-"`
			SettlementAmount     ledger.Decimal                      `json:"settlement_amount"`
			SettlementEvidence   string                              `json:"settlement_evidence"`
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
		in.Valuation, e = a.refundSettlementConfirmation(ctx, "retail-external:"+operator+":"+leg.ID+":"+in.External.ExternalReference, in.SettlementAmount, in.SettlementEvidence)
		if e != nil {
			handleError(c, e)
			return true
		}
		refund := payments.Refund{ID: leg.ID, MoneyAmount: leg.AmountMoney, RefundValuation: in.Valuation}
		in.External.ReceiptID, in.External.Destination = leg.ReceiptID, destination
		if !status.ResponsibleExecution || in.MerchantID == "" {
			handleError(c, remediation.ErrState)
			return true
		}
		out, e := a.Payments.RecordResponsibleBankRefund(ctx, actor, operator, in.MerchantID, refund, in.External)
		if e != nil {
			handleError(c, e)
			return true
		}
		c.JSON(200, commandResult(leg.ID, out.State))
		return true
	}
	return false
}
