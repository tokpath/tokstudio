package cooperation

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

type RefundDestinationProposal struct {
	Version                     int64           `json:"version"`
	ID                          string          `json:"id"`
	LegID                       string          `json:"leg_id"`
	BuyerID                     string          `json:"buyer_id"`
	SupplierID                  string          `json:"supplier_id"`
	Destination                 json.RawMessage `json:"destination" gorm:"type:jsonb"`
	PayerAuthorizationReference string          `json:"payer_authorization_reference"`
	ProposedBy                  string          `json:"proposed_by"`
	CreatedAt                   time.Time       `json:"created_at"`
}

func (RefundDestinationProposal) TableName() string {
	return "cooperation_refund_destination_proposals"
}

type RefundDestinationAuthorization struct {
	Version             int64           `json:"version"`
	ExecutionMerchantID string          `json:"execution_merchant_id"`
	ID                  string          `json:"id"`
	ProposalID          string          `json:"proposal_id"`
	LegID               string          `json:"leg_id"`
	BuyerID             string          `json:"buyer_id"`
	SupplierID          string          `json:"supplier_id"`
	Destination         json.RawMessage `json:"destination" gorm:"type:jsonb"`
	VerifiedBy          string          `json:"verified_by"`
	CreatedAt           time.Time       `json:"created_at"`
}

type BuyerRefundLegView struct {
	ID            string         `json:"id"`
	ExitID        string         `json:"exit_id"`
	State         string         `json:"state"`
	Driver        string         `json:"driver"`
	Currency      string         `json:"currency"`
	Network       string         `json:"network,omitempty"`
	TokenContract string         `json:"token_contract,omitempty"`
	AmountUSD     ledger.Decimal `json:"amount_usd"`
	SettledUSD    ledger.Decimal `json:"settled_usd"`
	ActualUSD     ledger.Decimal `json:"actual_usd"`
	ExcessUSD     ledger.Decimal `json:"excess_usd"`
}

func (p *ProcurementRefunds) BuyerRefundLegs(ctx context.Context, actor, buyer string) ([]BuyerRefundLegView, error) {
	out := []BuyerRefundLegView{}
	if !p.Ready() {
		return nil, ErrReplenishment
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		var legs []ProcurementExitRefund
		if e := tx.Where("buyer_id=?", buyer).Order("created_at DESC,id DESC").Limit(100).Find(&legs).Error; e != nil {
			return e
		}
		copy := *p.Payments
		copy.DB = tx
		for _, leg := range legs {
			if leg.SourceKind == "manual_receipt" {
				receipt, e := p.manualReceipt(ctx, tx, leg.SupplierID, leg.ReceiptID)
				if e != nil {
					return e
				}
				out = append(out, BuyerRefundLegView{ID: leg.ID, ExitID: leg.ExitID, State: leg.State, Driver: "manual", Currency: receipt.Currency, Network: receipt.Network, AmountUSD: leg.AmountUSD, SettledUSD: leg.SettledUSD, ActualUSD: leg.ActualUSD, ExcessUSD: leg.ExcessUSD})
				continue
			}
			source, e := copy.PaymentSourceForReceipt(ctx, leg.SupplierID, leg.ReceiptID)
			if e != nil {
				return e
			}
			m, e := copy.Merchant(ctx, leg.SupplierID, leg.MerchantID)
			if e != nil {
				return e
			}
			network, contract := m.Credentials["pay_network"], m.Credentials["pay_token_contract"]
			if m.Driver == "evm_wallet" {
				network, contract = m.Credentials["chain_id"], m.Credentials["token_contract"]
			}
			out = append(out, BuyerRefundLegView{ID: leg.ID, ExitID: leg.ExitID, State: leg.State, Driver: source.Driver, Currency: source.OriginalOrder.Money.Currency, Network: network, TokenContract: contract, AmountUSD: leg.AmountUSD, SettledUSD: leg.SettledUSD, ActualUSD: leg.ActualUSD, ExcessUSD: leg.ExcessUSD})
		}
		return nil
	})
	return out, e
}

func (RefundDestinationAuthorization) TableName() string {
	return "cooperation_refund_destination_authorizations"
}

// The buyer proposes a purpose-specific return address with real original-payer
// authorization. A separate supplier finance actor verifies it before execution.
// The original blockchain sender is never used as a default destination.
func (p *ProcurementRefunds) ProposeRefundDestination(ctx context.Context, actor, buyer, legID string, d payments.ExternalRefundDestination, payerAuthorization string) (RefundDestinationProposal, error) {
	var out RefundDestinationProposal
	if !p.Ready() || payerAuthorization == "" {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		var leg ProcurementExitRefund
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=? AND buyer_id=?", legID, buyer).Error; e != nil {
			return e
		}
		if leg.State != "prepared" {
			return ErrConflict
		}
		version, e := p.refundRouteVersion(ctx, tx, leg.ID)
		if e != nil {
			return e
		}
		out.ID = uuid.NewString()
		d.AuthorizationReference = out.ID
		d.VerificationEvidence = "pending independent recipient verification"
		d, e = payments.NormalizeExternalRefundDestination(d)
		if e != nil {
			return e
		}
		if version == 1 {
			if leg.SourceKind == "manual_receipt" {
				return payments.ErrUnsupported
			}
			copy := *p.Payments
			copy.DB = tx
			source, e := copy.PaymentSourceForReceipt(ctx, leg.SupplierID, leg.ReceiptID)
			if e != nil {
				return e
			}
			m, e := copy.Merchant(ctx, leg.SupplierID, leg.MerchantID)
			if e != nil {
				return e
			}
			if e = payments.ValidateExternalRefundDestinationForSource(m, source, d); e != nil {
				return e
			}
		}
		raw, _ := json.Marshal(d)
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		out = RefundDestinationProposal{Version: version, ID: out.ID, LegID: leg.ID, BuyerID: buyer, SupplierID: leg.SupplierID, Destination: raw, PayerAuthorizationReference: payerAuthorization, ProposedBy: actor, CreatedAt: now}
		return tx.Create(&out).Error
	})
	return out, e
}
func (p *ProcurementRefunds) VerifyRefundDestination(ctx context.Context, actor, supplier, proposalID, evidence string) (RefundDestinationAuthorization, error) {
	return p.verifyRefundDestination(ctx, actor, supplier, proposalID, "", evidence)
}
func (p *ProcurementRefunds) VerifyReplacementRefundDestination(ctx context.Context, actor, supplier, proposalID, merchantID, evidence string) (RefundDestinationAuthorization, error) {
	if merchantID == "" {
		return RefundDestinationAuthorization{}, ErrConflict
	}
	return p.verifyRefundDestination(ctx, actor, supplier, proposalID, merchantID, evidence)
}
func (p *ProcurementRefunds) verifyRefundDestination(ctx context.Context, actor, supplier, proposalID, merchantID, evidence string) (RefundDestinationAuthorization, error) {
	var out RefundDestinationAuthorization
	if !p.Ready() || evidence == "" {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, supplier, "payments.manage"); e != nil {
			return e
		}
		var proposal RefundDestinationProposal
		if e := tx.First(&proposal, "id=? AND supplier_id=?", proposalID, supplier).Error; e != nil {
			return e
		}
		var leg ProcurementExitRefund
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=?", proposal.LegID).Error; e != nil {
			return e
		}
		if leg.State != "prepared" || proposal.ProposedBy == actor {
			return ErrConflict
		}
		version, e := p.refundRouteVersion(ctx, tx, leg.ID)
		if e != nil {
			return e
		}
		if proposal.Version != version {
			return ErrConflict
		}
		q := tx.Where("leg_id=? AND version=?", leg.ID, version).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.ProposalID != proposalID {
				return ErrConflict
			}
			return nil
		}
		var d payments.ExternalRefundDestination
		if json.Unmarshal(proposal.Destination, &d) != nil {
			return ErrConflict
		}
		out.ID = uuid.NewString()
		d.AuthorizationReference = out.ID
		d.VerificationEvidence = evidence
		d, e = payments.NormalizeExternalRefundDestination(d)
		if e != nil {
			return e
		}
		if version == 1 {
			merchantID = leg.MerchantID
		} else {
			service := *p.Payments
			service.DB = tx
			m, e := service.Merchant(ctx, supplier, merchantID)
			if e != nil {
				return e
			}
			if !m.Enabled {
				return payments.ErrNotConfigured
			}
			if m.Driver == "manual" {
				if d.Network != "bank" || d.TokenContract != "" || d.Asset != m.Credentials["currency"] {
					return ErrConflict
				}
			} else {
				if e = payments.ValidateExternalRefundDestinationForSource(m, payments.PaymentSource{MerchantID: m.ID, Driver: m.Driver, OriginalOrder: payments.Order{OperatorID: m.OperatorID, Money: payments.Money{Currency: d.Asset}}}, d); e != nil {
					return e
				}
			}
		}
		raw, _ := json.Marshal(d)
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		out = RefundDestinationAuthorization{Version: version, ExecutionMerchantID: merchantID, ID: out.ID, ProposalID: proposalID, LegID: leg.ID, BuyerID: leg.BuyerID, SupplierID: supplier, Destination: raw, VerifiedBy: actor, CreatedAt: now}
		return tx.Create(&out).Error
	})
	return out, e
}

func (p *ProcurementRefunds) RefundDestinations(ctx context.Context, actor, operator, legID string) ([]RefundDestinationProposal, []RefundDestinationAuthorization, error) {
	proposals := []RefundDestinationProposal{}
	authorizations := []RefundDestinationAuthorization{}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		var leg ProcurementExitRefund
		if e := tx.First(&leg, "id=? AND (supplier_id=? OR buyer_id=?)", legID, operator, operator).Error; e != nil {
			return e
		}
		permission := "payments.manage"
		if operator == leg.BuyerID {
			permission = "cooperation.manage"
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, operator, permission); e != nil {
			return e
		}
		if e := tx.Where("leg_id=?", legID).Order("created_at,id").Find(&proposals).Error; e != nil {
			return e
		}
		return tx.Where("leg_id=?", legID).Find(&authorizations).Error
	})
	return proposals, authorizations, e
}
func (p *ProcurementRefunds) destination(ctx context.Context, tx *gorm.DB, leg ProcurementExitRefund) (payments.ExternalRefundDestination, error) {
	var d payments.ExternalRefundDestination
	var a RefundDestinationAuthorization
	version, e := p.refundRouteVersion(ctx, tx, leg.ID)
	if e != nil {
		return d, e
	}
	if e := tx.First(&a, "leg_id=? AND buyer_id=? AND supplier_id=? AND version=?", leg.ID, leg.BuyerID, leg.SupplierID, version).Error; e != nil {
		return d, e
	}
	if json.Unmarshal(a.Destination, &d) != nil || d.AuthorizationReference != a.ID {
		return d, ErrConflict
	}
	return payments.NormalizeExternalRefundDestination(d)
}
func (p *ProcurementRefunds) RequireExternalRefundFact(ctx context.Context, tx *gorm.DB, actor string, r payments.Refund, d payments.ExternalRefundDestination) error {
	if tx == nil || !p.Ready() {
		return ErrReplenishment
	}
	var leg ProcurementExitRefund
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", r.ID).Limit(1).Find(&leg)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected == 0 {
		return ErrNotProcurementRefund
	}
	if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, leg.SupplierID, "payments.manage"); e != nil {
		return e
	}
	if r.USDAmount.Cmp(leg.AmountUSD) != 0 || r.Order.ID != leg.OriginOrderID || r.Order.MerchantID != leg.MerchantID || r.Order.OperatorID != leg.SupplierID {
		return ErrConflict
	}
	expected, e := p.destinationByID(ctx, tx, leg, d.AuthorizationReference)
	if e != nil {
		return e
	}
	actual, e := payments.NormalizeExternalRefundDestination(d)
	if e != nil {
		return e
	}
	if expected != actual {
		return ErrConflict
	}
	var authorization RefundDestinationAuthorization
	if e = tx.First(&authorization, "id=? AND leg_id=?", d.AuthorizationReference, leg.ID).Error; e != nil {
		return e
	}
	if authorization.Version != 1 {
		return ErrConflict
	}
	var count int64
	if e = tx.Table("cooperation_procurement_refund_instructions").Where("leg_id=? AND (authorization_id=? OR authorization_id IS NULL)", leg.ID, authorization.ID).Count(&count).Error; e != nil {
		return e
	}
	if count == 0 {
		return ErrConflict
	}
	return nil
}

func (p *ProcurementRefunds) externalRefund(ctx context.Context, actor, id, authorizationID string) (ProcurementExitRefund, payments.Refund, payments.ExternalRefundDestination, error) {
	var leg ProcurementExitRefund
	var r payments.Refund
	var d payments.ExternalRefundDestination
	if !p.Ready() {
		return leg, r, d, ErrReplenishment
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := tx.First(&leg, "id=?", id).Error; e != nil {
			return e
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, leg.SupplierID, "payments.manage"); e != nil {
			return e
		}
		var e error
		if authorizationID != "" {
			d, e = p.destinationByID(ctx, tx, leg, authorizationID)
		} else {
			d, e = p.destination(ctx, tx, leg)
		}
		if e != nil {
			return e
		}
		copy := *p.Payments
		copy.DB = tx
		source, e := copy.PaymentSourceForReceipt(ctx, leg.SupplierID, leg.ReceiptID)
		if e != nil {
			return e
		}
		r = payments.Refund{ID: leg.ID, Order: source.OriginalOrder, USDAmount: leg.AmountUSD}
		return nil
	})
	return leg, r, d, e
}
func (p *ProcurementRefunds) RecordEVMRefund(ctx context.Context, actor, id string, v payments.EVMRefundVerification, valuation payments.Valuation, valuedAt time.Time, fxReference string) (payments.RefundStatus, error) {
	leg, r, d, e := p.externalRefund(ctx, actor, id, v.Destination.AuthorizationReference)
	if e != nil {
		return payments.RefundStatus{}, e
	}
	if v.ReceiptID != "" && v.ReceiptID != leg.ReceiptID {
		return payments.RefundStatus{}, ErrConflict
	}
	v.ReceiptID = leg.ReceiptID
	v.Destination = d
	r.RefundValuation = valuation
	r.ValuedAt = valuedAt
	r.FXExecutionReference = fxReference
	service := *p.Payments
	service.RefundApproval = p
	service.RefundSink = p
	return service.RecordEVMRefund(ctx, actor, leg.SupplierID, leg.MerchantID, r, v)
}
func (p *ProcurementRefunds) RecordExternalRefund(ctx context.Context, actor, id string, v payments.ExternalRefundVerification, valuation payments.Valuation, valuedAt time.Time, fxReference string) (payments.RefundStatus, error) {
	leg, r, d, e := p.externalRefund(ctx, actor, id, v.Destination.AuthorizationReference)
	if e != nil {
		return payments.RefundStatus{}, e
	}
	if v.ReceiptID != "" && v.ReceiptID != leg.ReceiptID {
		return payments.RefundStatus{}, ErrConflict
	}
	v.ReceiptID = leg.ReceiptID
	v.Destination = d
	r.RefundValuation = valuation
	r.ValuedAt = valuedAt
	r.FXExecutionReference = fxReference
	service := *p.Payments
	service.RefundApproval = p
	service.RefundSink = p
	return service.RecordExternalRefund(ctx, actor, leg.SupplierID, leg.MerchantID, r, v)
}
