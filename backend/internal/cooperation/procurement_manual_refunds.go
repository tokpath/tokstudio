package cooperation

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// BeginManualRefund records the one approved bank-transfer instruction. A crash
// or an unknown manual outcome retains both the payable hold and shared source
// capacity. Automatic execution cannot take over this instruction.
func (p *ProcurementRefunds) BeginManualRefund(ctx context.Context, actor, id, key, evidence string) (ProcurementExitRefund, error) {
	var out ProcurementExitRefund
	if !p.Ready() || key == "" || evidence == "" {
		return out, ErrConflict
	}
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&out, "id=?", id).Error; e != nil {
			return e
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, out.SupplierID, "payments.manage"); e != nil {
			return e
		}
		if out.ExecutionMode == "manual" && out.ExecutionKey == key {
			if out.ExecutionEvidence != evidence {
				return ErrConflict
			}
			return nil
		}
		if out.State != "prepared" && !(out.State == "exception" && out.SettledUSD.Cmp(out.AmountUSD) < 0) {
			return ErrConflict
		}
		service := *p.Payments
		service.DB = tx
		version, e := p.refundRouteVersion(ctx, tx, out.ID)
		if e != nil {
			return e
		}
		merchantID := out.MerchantID
		var authorizationID *string
		var authorization RefundDestinationAuthorization
		q := tx.Where("leg_id=? AND version=?", out.ID, version).Limit(1).Find(&authorization)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			authorizationID = &authorization.ID
			if authorization.ExecutionMerchantID != "" {
				merchantID = authorization.ExecutionMerchantID
			}
		} else if version > 1 {
			return ErrConflict
		}
		m, e := service.Merchant(ctx, out.SupplierID, merchantID)
		if e != nil {
			return e
		}
		if m.Driver != "manual" && m.Driver != "evm_wallet" && m.Driver != "nowpayments" || !m.Enabled {
			return payments.ErrUnsupported
		}
		if m.Driver != "manual" {
			if _, e = p.destination(ctx, tx, out); e != nil {
				return e
			}
		}
		if e = p.Capacity.RequireReceiptRefundExecution(ctx, tx, out.SupplierID, out.ReceiptID, out.ID); e != nil {
			return e
		}
		out.State = "manual_executing"
		out.ExecutionMode = "manual"
		out.ExecutionKey = key
		out.ExecutionEvidence = evidence
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		if e = tx.Table("cooperation_procurement_refund_instructions").Create(map[string]any{"leg_id": out.ID, "execution_key": key, "remaining_usd": out.AmountUSD.Sub(out.SettledUSD), "evidence": evidence, "authorized_by": actor, "created_at": now, "authorization_id": authorizationID}).Error; e != nil {
			return e
		}
		return tx.Model(&out).Updates(map[string]any{"state": out.State, "execution_mode": out.ExecutionMode, "execution_key": key, "execution_evidence": evidence}).Error
	})
	return out, e
}

// RecordManualRefund records only completed, finance-verified original-route
// transfers. Actual FX/amount mismatches retain a durable verified payment fact
// but do not discharge the USD obligation or release the source reservation.
func (p *ProcurementRefunds) RecordManualRefund(ctx context.Context, actor, id string, proof payments.ManualRefundVerification, valuation payments.Valuation, valuedAt time.Time, fxReference string) (payments.RefundStatus, error) {
	if !p.Ready() {
		return payments.RefundStatus{}, ErrReplenishment
	}
	var leg ProcurementExitRefund
	if e := p.Cooperation.DB.WithContext(ctx).First(&leg, "id=?", id).Error; e != nil {
		return payments.RefundStatus{}, e
	}
	if leg.ExecutionMode != "manual" || leg.ExecutionKey == "" || leg.State != "manual_executing" && leg.State != "completed" && leg.State != "executing" || proof.ReceiptID != leg.ReceiptID {
		return payments.RefundStatus{}, ErrConflict
	}
	source, e := p.Payments.PaymentSourceForReceipt(ctx, leg.SupplierID, leg.ReceiptID)
	if e != nil {
		return payments.RefundStatus{}, e
	}
	service := *p.Payments
	service.RefundApproval = p
	service.RefundSink = p
	return service.RecordManualRefund(ctx, actor, leg.SupplierID, leg.MerchantID, payments.Refund{ID: leg.ID, Order: source.OriginalOrder, Money: proof.Money, USDAmount: leg.AmountUSD, RefundValuation: valuation, ValuedAt: valuedAt, FXExecutionReference: fxReference}, proof)
}
