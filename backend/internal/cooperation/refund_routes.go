package cooperation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProcurementRefundRouteClosure struct {
	ID           string          `json:"id"`
	LegID        string          `json:"leg_id"`
	Version      int64           `json:"version"`
	RemainingUSD ledger.Decimal  `json:"remaining_usd"`
	Reason       string          `json:"reason"`
	Evidence     string          `json:"evidence"`
	PaymentFacts json.RawMessage `json:"payment_facts" gorm:"type:jsonb"`
	ClosedBy     string          `json:"closed_by"`
	CreatedAt    time.Time       `json:"created_at"`
}

type ProcurementRefundRouteStatus struct {
	LegID        string                          `json:"leg_id"`
	Version      int64                           `json:"version"`
	RemainingUSD ledger.Decimal                  `json:"remaining_usd"`
	State        string                          `json:"state"`
	Closures     []ProcurementRefundRouteClosure `json:"closures"`
}

func (p *ProcurementRefunds) RefundRouteStatus(ctx context.Context, actor, operator, legID string) (ProcurementRefundRouteStatus, error) {
	out := ProcurementRefundRouteStatus{LegID: legID, Closures: []ProcurementRefundRouteClosure{}}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		var leg ProcurementExitRefund
		if e := tx.First(&leg, "id=? AND (buyer_id=? OR supplier_id=?)", legID, operator, operator).Error; e != nil {
			return e
		}
		permission := "payments.manage"
		if operator == leg.BuyerID {
			permission = "cooperation.manage"
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, operator, permission); e != nil {
			return e
		}
		version, e := p.refundRouteVersion(ctx, tx, legID)
		if e != nil {
			return e
		}
		out.Version = version
		out.State = leg.State
		out.RemainingUSD = leg.AmountUSD.Sub(leg.SettledUSD)
		if e = tx.Where("leg_id=?", legID).Order("version").Find(&out.Closures).Error; e != nil {
			return e
		}
		if operator == leg.BuyerID {
			for i := range out.Closures {
				out.Closures[i].Reason = "previous_refund_route_closed"
				out.Closures[i].Evidence = ""
				out.Closures[i].PaymentFacts = nil
				out.Closures[i].ClosedBy = ""
			}
		}
		return nil
	})
	return out, e
}

func (ProcurementRefundRouteClosure) TableName() string { return "cooperation_refund_route_closures" }

type ManualRefundFailureInput struct {
	BusinessKey               string    `json:"business_key"`
	ExpectedExecutionKey      string    `json:"expected_execution_key"`
	Result                    string    `json:"result"`
	ExternalReference         string    `json:"external_reference"`
	NoFurtherPaymentReference string    `json:"no_further_payment_reference"`
	Evidence                  string    `json:"evidence"`
	VerifiedAt                time.Time `json:"verified_at"`
}
type ProcurementRefundManualFailure struct {
	ID, LegID, BusinessKey, ExecutionKey, Result, ExternalReference, NoFurtherPaymentReference, Evidence, VerifiedBy string
	VerifiedAt, CreatedAt                                                                                            time.Time
}

func (ProcurementRefundManualFailure) TableName() string { return "cooperation_refund_manual_failures" }

func (p *ProcurementRefunds) refundRouteVersion(ctx context.Context, tx *gorm.DB, leg string) (int64, error) {
	var version int64
	e := tx.WithContext(ctx).Table("cooperation_refund_route_closures").Where("leg_id=?", leg).Select("COALESCE(MAX(version),0)+1").Scan(&version).Error
	return version, e
}

func (p *ProcurementRefunds) destinationByID(ctx context.Context, tx *gorm.DB, leg ProcurementExitRefund, id string) (payments.ExternalRefundDestination, error) {
	var d payments.ExternalRefundDestination
	var a RefundDestinationAuthorization
	if e := tx.WithContext(ctx).First(&a, "id=? AND leg_id=? AND buyer_id=? AND supplier_id=?", id, leg.ID, leg.BuyerID, leg.SupplierID).Error; e != nil {
		return d, e
	}
	if json.Unmarshal(a.Destination, &d) != nil || d.AuthorizationReference != a.ID {
		return d, ErrConflict
	}
	return payments.NormalizeExternalRefundDestination(d)
}

// RecordManualRefundFailure records an independently verified terminal result,
// rather than interpreting a timeout or a missing transaction as failure.
func (p *ProcurementRefunds) RecordManualRefundFailure(ctx context.Context, actor, supplier, legID string, in ManualRefundFailureInput) (ProcurementRefundManualFailure, error) {
	var out ProcurementRefundManualFailure
	if !p.Ready() || in.BusinessKey == "" || in.ExpectedExecutionKey == "" || in.Result != "failed" && in.Result != "cancelled" || in.ExternalReference == "" || in.NoFurtherPaymentReference == "" || in.Evidence == "" || in.VerifiedAt.IsZero() {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, supplier, "payments.manage"); e != nil {
			return e
		}
		var leg ProcurementExitRefund
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=? AND supplier_id=?", legID, supplier).Error; e != nil {
			return e
		}
		q := tx.Where("leg_id=? AND business_key=?", legID, in.BusinessKey).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if out.ExecutionKey != in.ExpectedExecutionKey || out.Result != in.Result || out.ExternalReference != in.ExternalReference || out.NoFurtherPaymentReference != in.NoFurtherPaymentReference || out.Evidence != in.Evidence || !out.VerifiedAt.Equal(in.VerifiedAt) {
				return ErrConflict
			}
			return nil
		}
		if leg.ExecutionMode != "manual" || leg.ExecutionKey != in.ExpectedExecutionKey || leg.State != "manual_executing" || leg.AmountUSD.Cmp(leg.SettledUSD) <= 0 {
			return ErrConflict
		}
		facts, e := p.Payments.RefundExecutionClosureFacts(ctx, tx, supplier, legID)
		if e != nil {
			return e
		}
		for _, f := range facts {
			if f.ClosureID == "" && (f.State == "executing" || f.State == "querying" || f.State == "outcome_unknown") {
				return payments.ErrOutcomeUnknown
			}
		}
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		if in.VerifiedAt.After(now.Add(5 * time.Minute)) {
			return ErrConflict
		}
		out = ProcurementRefundManualFailure{ID: uuid.NewString(), LegID: legID, BusinessKey: in.BusinessKey, ExecutionKey: in.ExpectedExecutionKey, Result: in.Result, ExternalReference: in.ExternalReference, NoFurtherPaymentReference: in.NoFurtherPaymentReference, Evidence: in.Evidence, VerifiedBy: actor, VerifiedAt: in.VerifiedAt, CreatedAt: now}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Model(&leg).Update("state", "exception").Error
	})
	return out, e
}

// CloseRefundRoute preserves the outstanding payable and original receipt
// reservation. A new route needs a new buyer proposal and independent approval.
func (p *ProcurementRefunds) CloseRefundRoute(ctx context.Context, actor, supplier, legID, reason, evidence string) (ProcurementRefundRouteClosure, error) {
	var out ProcurementRefundRouteClosure
	if !p.Ready() || reason == "" || evidence == "" {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, supplier, "payments.manage"); e != nil {
			return e
		}
		var leg ProcurementExitRefund
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=? AND supplier_id=?", legID, supplier).Error; e != nil {
			return e
		}
		q := tx.Where("leg_id=? AND reason=? AND evidence=?", legID, reason, evidence).Limit(1).Find(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			return nil
		}
		if leg.AmountUSD.Cmp(leg.SettledUSD) <= 0 || leg.State != "prepared" && leg.State != "exception" {
			return ErrConflict
		}
		version, e := p.refundRouteVersion(ctx, tx, leg.ID)
		if e != nil {
			return e
		}
		facts, e := p.Payments.RefundExecutionClosureFacts(ctx, tx, supplier, legID)
		if e != nil {
			return e
		}
		matched := false
		for _, f := range facts {
			if f.ClosureID != "" {
				continue
			}
			switch f.State {
			case "failed", "cancelled":
				matched = true
			case "succeeded", "verified_pending_settlement":
				var n int64
				if e := tx.Model(&ProcurementRefundObservation{}).Where("leg_id=? AND fact_id=?", legID, f.FactID).Count(&n).Error; e != nil {
					return e
				}
				if f.FactID == "" || n != 1 {
					return ErrConflict
				}
				matched = true
			default:
				return payments.ErrOutcomeUnknown
			}
		}
		if leg.ExecutionKey != "" && !matched {
			var n int64
			if e = tx.Model(&ProcurementRefundManualFailure{}).Where("leg_id=? AND execution_key=?", legID, leg.ExecutionKey).Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return payments.ErrOutcomeUnknown
			}
		}
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		raw, _ := json.Marshal(facts)
		out = ProcurementRefundRouteClosure{ID: uuid.NewString(), LegID: legID, Version: version, RemainingUSD: leg.AmountUSD.Sub(leg.SettledUSD), Reason: reason, Evidence: evidence, PaymentFacts: raw, ClosedBy: actor, CreatedAt: now}
		if e = p.Payments.SealRefundExecutions(ctx, tx, supplier, legID, out.ID); e != nil {
			return e
		}
		if e = tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Model(&leg).Updates(map[string]any{"state": "prepared", "execution_mode": "", "execution_key": "", "execution_evidence": ""}).Error
	})
	return out, e
}

func (p *ProcurementRefunds) RequireResponsibleRefundExecution(ctx context.Context, tx *gorm.DB, actor, operator, legID string, d payments.ExternalRefundDestination) (payments.ResponsibleRefundBasis, error) {
	var basis payments.ResponsibleRefundBasis
	var leg ProcurementExitRefund
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=?", legID).Limit(1).Find(&leg)
	if q.Error != nil {
		return basis, q.Error
	}
	if q.RowsAffected == 0 {
		return basis, ErrNotProcurementRefund
	}
	if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, leg.SupplierID, "payments.manage"); e != nil {
		return basis, e
	}
	if operator != leg.SupplierID {
		return basis, ErrConflict
	}
	var a RefundDestinationAuthorization
	if e := tx.First(&a, "id=? AND leg_id=? AND supplier_id=?", d.AuthorizationReference, leg.ID, operator).Error; e != nil {
		return basis, e
	}
	if a.Version <= 1 || a.ExecutionMerchantID == "" {
		return basis, ErrConflict
	}
	var expected payments.ExternalRefundDestination
	if json.Unmarshal(a.Destination, &expected) != nil || d != expected {
		return basis, ErrConflict
	}
	var n int64
	if e := tx.Table("cooperation_procurement_refund_instructions").Where("leg_id=? AND authorization_id=?", leg.ID, a.ID).Count(&n).Error; e != nil {
		return basis, e
	}
	if n == 0 {
		return basis, ErrConflict
	}
	return payments.ResponsibleRefundBasis{RefundID: leg.ID, OperatorID: operator, SourceOperatorID: leg.SupplierID, ReceiptID: leg.ReceiptID, Currency: d.Asset, ExecutionMerchantID: a.ExecutionMerchantID, AmountUSD: leg.AmountUSD, RemainingUSD: leg.AmountUSD.Sub(leg.SettledUSD), AuthorizedAt: a.CreatedAt, CreatedAt: leg.CreatedAt}, nil
}

// RecordReplacementRefund verifies an externally executed return using the
// newly approved payer and destination. It never initiates a bank or chain send.
func (p *ProcurementRefunds) RecordReplacementRefund(ctx context.Context, actor, supplier, legID string, evm payments.EVMRefundVerification, external payments.ExternalRefundVerification, valuation payments.Valuation, valuedAt time.Time, fxReference string) (payments.RefundStatus, error) {
	var leg ProcurementExitRefund
	var a RefundDestinationAuthorization
	var d payments.ExternalRefundDestination
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, supplier, "payments.manage"); e != nil {
			return e
		}
		if e := tx.First(&leg, "id=? AND supplier_id=?", legID, supplier).Error; e != nil {
			return e
		}
		version, e := p.refundRouteVersion(ctx, tx, legID)
		if e != nil {
			return e
		}
		authorizationID := evm.Destination.AuthorizationReference
		if authorizationID == "" {
			authorizationID = external.Destination.AuthorizationReference
		}
		query := tx.Where("leg_id=? AND version=?", legID, version)
		if authorizationID != "" {
			query = tx.Where("leg_id=? AND id=?", legID, authorizationID)
		}
		if e = query.First(&a).Error; e != nil {
			return e
		}
		if a.Version <= 1 || a.ExecutionMerchantID == "" {
			return ErrConflict
		}
		d, e = p.destinationByID(ctx, tx, leg, a.ID)
		return e
	})
	if e != nil {
		return payments.RefundStatus{}, e
	}
	service := *p.Payments
	service.RefundApproval = p
	service.RefundSink = p
	m, e := service.Merchant(ctx, supplier, a.ExecutionMerchantID)
	if e != nil {
		return payments.RefundStatus{}, e
	}
	r := payments.Refund{ID: leg.ID, RefundValuation: valuation, ValuedAt: valuedAt, FXExecutionReference: fxReference}
	evm.ReceiptID, evm.Destination = leg.ReceiptID, d
	external.ReceiptID, external.Destination = leg.ReceiptID, d
	switch m.Driver {
	case "evm_wallet":
		return service.RecordResponsibleEVMRefund(ctx, actor, supplier, m.ID, r, evm)
	case "nowpayments":
		return service.RecordResponsibleExternalRefund(ctx, actor, supplier, m.ID, r, external)
	case "manual":
		return service.RecordResponsibleBankRefund(ctx, actor, supplier, m.ID, r, external)
	default:
		return payments.RefundStatus{}, payments.ErrUnsupported
	}
}
