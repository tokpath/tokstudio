package cooperation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// manualReceipt accepts only a separately verified, immutable manual collection.
// It does not invent a payment intent or merchant for an offline receipt.
func (p *ProcurementRefunds) manualReceipt(ctx context.Context, tx *gorm.DB, operator, id string) (commerce.Receipt, error) {
	var r commerce.Receipt
	e := tx.WithContext(ctx).Table("commerce_receipts").Where("id=? AND operator_id=?", id, operator).First(&r).Error
	if e != nil {
		return r, e
	}
	if r.Provider != "manual" || !strings.HasPrefix(r.VerifiedBy, "staff:") {
		return r, payments.ErrConflict
	}
	return r, nil
}

type manualReceiptRefundFact struct {
	FactID                           string `gorm:"primaryKey"`
	LegID, ExecutionKey, Fingerprint string
	Proof, Valuation                 json.RawMessage `gorm:"type:jsonb"`
	ValuedAt                         *time.Time
	FXReference, VerifiedBy          string
	CreatedAt                        time.Time
}

func (manualReceiptRefundFact) TableName() string { return "cooperation_manual_receipt_refund_facts" }

func (p *ProcurementRefunds) recordManualReceiptRefund(ctx context.Context, actor, id string, proof payments.ManualRefundVerification, valuation payments.Valuation, valuedAt time.Time, fxReference string) (payments.RefundStatus, error) {
	out := payments.RefundStatus{}
	var settlementErr error
	e := p.Cooperation.within(ctx, nil, func(tx *gorm.DB) error {
		var leg ProcurementExitRefund
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&leg, "id=?", id).Error; e != nil {
			return e
		}
		if e := p.Cooperation.Organization.RequirePermission(ctx, tx, actor, leg.SupplierID, "payments.manage"); e != nil {
			return e
		}
		version, e := p.refundRouteVersion(ctx, tx, id)
		if e != nil {
			return e
		}
		// Replacement routes have their own independently approved destination and
		// configured execution source. They must use RecordReplacementRefund.
		if leg.SourceKind != "manual_receipt" || version != 1 || leg.ExecutionMode != "manual" || leg.ExecutionKey == "" || proof.ReceiptID != leg.ReceiptID {
			return ErrConflict
		}
		receipt, e := p.manualReceipt(ctx, tx, leg.SupplierID, leg.ReceiptID)
		if e != nil {
			return e
		}
		now, e := businessTime(ctx, tx)
		if e != nil {
			return e
		}
		// A recorded chain network still needs its dedicated external verifier.
		if receipt.Network != "" && receipt.Network != "bank" && receipt.Network != "cash" {
			return payments.ErrUnsupported
		}
		if strings.TrimSpace(proof.ExternalReference) == "" {
			proof.ExternalReference = payments.ManualReference("refund", leg.SupplierID, leg.ExecutionKey)
		}
		if proof.Money.Validate() != nil || proof.Money.Currency != receipt.Currency || strings.TrimSpace(proof.ExternalReference) == "" || strings.TrimSpace(proof.EvidenceReference) == "" || proof.ExecutedAt.IsZero() || proof.ExecutedAt.After(now.Add(5*time.Minute)) {
			return payments.ErrInvalid
		}
		if receipt.PayerReference != "" && proof.RecipientReference != receipt.PayerReference {
			return payments.ErrConflict
		}
		factID, e := payments.ClaimManualReceiptRefund(tx, leg.SupplierID, leg.ID, payments.ManualAccountScope(leg.SupplierID, receipt.RecipientReference), proof.ExternalReference)
		if e != nil {
			return e
		}
		fingerprint := hash(struct {
			LegID     string
			Proof     payments.ManualRefundVerification
			Valuation payments.Valuation
			At        time.Time
			FX        string
		}{leg.ID, proof, valuation, valuedAt, fxReference})
		var old manualReceiptRefundFact
		q := tx.Where("fact_id=?", factID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		var value payments.MoneyValue
		if q.RowsAffected > 0 {
			if old.LegID != leg.ID || old.Fingerprint != fingerprint {
				return ErrConflict
			}
			if json.Unmarshal(old.Valuation, &value) != nil {
				return ErrConflict
			}
		} else {
			var existing int64
			if e := tx.Model(&manualReceiptRefundFact{}).Where("leg_id=? AND execution_key=?", leg.ID, leg.ExecutionKey).Count(&existing).Error; e != nil {
				return e
			}
			if leg.State != "manual_executing" || existing != 0 {
				return ErrConflict
			}
			base, baseErr := ledger.GetMonetary(ctx, tx)
			if baseErr != nil {
				return baseErr
			}
			if proof.Money.Currency == base.Currency {
				value = payments.MoneyValue{Currency: base.Currency, Amount: proof.Money.Amount, OriginalMoney: proof.Money, EvidenceReference: proof.EvidenceReference}
			} else {
				if valuation.Currency != base.Currency {
					return payments.ErrInvalid
				}
				value, e = valuation.Convert(proof.Money, valuedAt)
				if e != nil {
					return e
				}
			}
			rawProof, _ := json.Marshal(proof)
			rawValue, _ := json.Marshal(value)
			var at *time.Time
			if !valuedAt.IsZero() {
				at = &valuedAt
			}
			old = manualReceiptRefundFact{FactID: factID, LegID: leg.ID, ExecutionKey: leg.ExecutionKey, Fingerprint: fingerprint, Proof: rawProof, Valuation: rawValue, ValuedAt: at, FXReference: fxReference, VerifiedBy: actor, CreatedAt: now}
			if e := tx.Create(&old).Error; e != nil {
				return e
			}
		}
		// Keep the finance-verified cash fact even if matching/settlement fails. A
		// savepoint prevents a partial ledger update; retries use this exact proof.
		evidence, _ := json.Marshal(map[string]string{"verified_by": "staff:" + old.VerifiedBy, "evidence_reference": proof.EvidenceReference, "original_receipt_id": receipt.ID, "payer_reference": receipt.RecipientReference, "recipient_reference": proof.RecipientReference, "fx_execution_reference": fxReference})
		fact := payments.Fact{ID: factID, Kind: "refund", Driver: "manual", OperatorID: leg.SupplierID, Money: proof.Money, OccurredAt: &proof.ExecutedAt, Evidence: evidence, ExecutionValue: &value}
		// This private accounting entry receives verified manual evidence; it does
		// not mark the fact as an adapter-authenticated fact or call the public sink.
		var review error
		settlementErr = tx.Transaction(func(t *gorm.DB) error {
			e := p.applyRefundObservation(ctx, t, leg, fact, value)
			if errors.Is(e, payments.ErrSettlementReview) {
				review = e
				return nil
			}
			return e
		})
		if settlementErr == nil {
			settlementErr = review
		}
		out = payments.RefundStatus{ProviderID: proof.ExternalReference, State: "succeeded"}
		if settlementErr != nil {
			out.State = "verified_pending_settlement"
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	return out, settlementErr
}
