package portal

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
	"gorm.io/gorm"
)

func (a *Application) customerRefundDestinations(c *gin.Context) {
	id := subject(c)
	rows, e := a.Remediation.CustomerRefundDestinations(c.Request.Context(), id.Customer.Customer.ID, id.Customer.Customer.BrandID, c.Param("id"))
	if e != nil {
		handleError(c, e)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		var proposal gin.H
		if row.Proposal != nil {
			proposal = gin.H{"asset": row.Proposal.Asset, "address": row.Proposal.Address}
		}
		item := gin.H{"leg_id": row.LegID, "amount_money": row.AmountMoney, "destination_required": row.DestinationRequired, "destination_mutable": row.DestinationMutable, "authorization_id": row.AuthorizationID}
		if proposal != nil {
			item["proposal"] = proposal
		}
		if row.DestinationRequired && row.DestinationMutable && row.AuthorizationID == "" {
			var currency string
			e := a.DB.WithContext(c.Request.Context()).Table("remediation_refund_legs l").Select("r.currency").
				Joins("JOIN commerce_receipts r ON r.id=l.receipt_id AND r.operator_id=l.source_operator_id").Where("l.id=?", row.LegID).Row().Scan(&currency)
			if e != nil {
				handleError(c, e)
				return
			}
			item["currency"] = currency
		}
		items = append(items, item)
	}
	c.JSON(200, gin.H{"items": items})
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
	var leg remediation.RefundLeg
	if _, e := uuid.Parse(id); e != nil {
		return leg, remediation.ErrInvalid
	}
	e := a.DB.WithContext(ctx).Where("operator_id=? AND id=?", op, id).Take(&leg).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		e = remediation.ErrScope
	}
	return leg, e
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
