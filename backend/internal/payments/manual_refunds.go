package payments

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ManualRefundVerification records an already completed offline payment.
// RecipientReference must match the original verified payer; a transfer request
// or an approval document is not a completed-transfer fact.
type ManualRefundVerification struct {
	ReceiptID          string    `json:"receipt_id"`
	ExternalReference  string    `json:"external_reference"`
	Money              Money     `json:"money"`
	ExecutedAt         time.Time `json:"executed_at"`
	EvidenceReference  string    `json:"evidence_reference"`
	RecipientReference string    `json:"recipient_reference"`
}

func manualBankFactID(kind, account, reference string) string {
	// Merchant aliases for the same receiving/paying bank account cannot turn
	// one real transfer into several receipts or refunds.
	return "manual:bank:" + kind + ":" + fingerprint([]string{strings.TrimSpace(account), strings.TrimSpace(reference)})
}

// ClaimManualReceiptRefund gives a finance-verified, unlinked bank receipt the
// same global transfer identity as merchant-linked refunds. It only reserves
// ownership; the caller must independently verify the receipt and refund proof.
func ClaimManualReceiptRefund(tx *gorm.DB, operator, refundID, account, reference string) (string, error) {
	if tx == nil || operator == "" || refundID == "" || strings.TrimSpace(account) == "" || strings.TrimSpace(reference) == "" {
		return "", ErrInvalid
	}
	id := manualBankFactID("refund", account, reference)
	return id, bindRefundFact(tx, Merchant{OperatorID: operator}, Refund{ID: refundID}, Fact{ID: id})
}

// RecordManualRefund verifies scope, original-route identity and actual bank
// evidence. Verified cash facts survive a settlement mismatch and remain
// pending; the original obligation is preserved. Callers must durably
// begin the approved manual execution before a human sends the transfer.
func (s *Service) RecordManualRefund(ctx context.Context, actor, operator, merchant string, r Refund, v ManualRefundVerification) (RefundStatus, error) {
	out := RefundStatus{}
	if s.Authorize == nil || s.RefundApproval == nil || s.RefundSink == nil {
		return out, ErrNotConfigured
	}
	manualApproval, ok := s.RefundApproval.(interface {
		RequireVerifiedManualRefund(context.Context, *gorm.DB, string, Refund) error
	})
	if !ok {
		return out, ErrNotConfigured
	}
	var settlementErr error
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.Authorize.RequirePermission(ctx, tx, actor, operator, "payments.manage"); e != nil {
			return e
		}
		copy := *s
		copy.DB = tx
		m, e := copy.Merchant(ctx, operator, merchant)
		if e != nil {
			return e
		}
		m.Enabled = true // Existing executed returns remain verifiable after collection is disabled.
		if m.Driver != "manual" || ready(m, Manual{}) != nil {
			return ErrNotConfigured
		}
		original, e := copy.loadOrder(ctx, m, r.Order.ID)
		if e != nil {
			return e
		}
		r.Order = original
		r.Money = v.Money
		if strings.TrimSpace(v.ExternalReference) == "" {
			v.ExternalReference = ManualReference("refund", operator, r.ID)
		}
		account := ManualAccountScope(operator, m.Credentials["recipient_reference"])
		r.ExecutionID = "external:" + fingerprint([]string{r.ID, "manual", account, v.ExternalReference})
		now, e := databaseTime(ctx, tx)
		if e != nil {
			return e
		}
		validation := r
		if validation.Money.Currency == validation.Order.Settlement().Currency {
			validation.MoneyAmount = validation.Money.Amount
		}
		if v.ReceiptID == "" || v.ExecutedAt.IsZero() || v.ExecutedAt.After(now.Add(5*time.Minute)) || v.EvidenceReference == "" || validation.Validate(m) != nil {
			return ErrInvalid
		}
		source, e := copy.PaymentSourceForReceipt(ctx, operator, v.ReceiptID)
		if e != nil {
			return e
		}
		proof := manualEvidence(source.Fact)
		if source.MerchantID != merchant || source.OriginalOrder.ID != original.ID || source.Driver != "manual" || (proof["payer_reference"] != "" && proof["payer_reference"] != v.RecipientReference) || proof["recipient_reference"] != m.Credentials["recipient_reference"] {
			return ErrConflict
		}
		if e = tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", merchant+":refund:"+r.ID).Error; e != nil {
			return e
		}
		fp := fingerprint(struct {
			Refund       Refund
			Verification ManualRefundVerification
		}{r, v})
		var previous refundIntentRow
		q := tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", merchant, refundExecutionKey(r)).Limit(1).Find(&previous)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if previous.Fingerprint != fp {
				return ErrConflict
			}
			if e = copy.open(previous.ResultCiphertext, &out); e != nil {
				return e
			}
			if out.Fact == nil || out.Fact.ExecutionValue == nil {
				return ErrInvalid
			}
			out.Fact.proof = true
		} else {
			f, e := refundFact(m, r, v.ExternalReference, v.Money, &v.ExecutedAt, map[string]string{"verified_by": "staff:" + actor, "evidence_reference": v.EvidenceReference, "original_receipt_id": v.ReceiptID, "payer_reference": m.Credentials["recipient_reference"], "recipient_reference": v.RecipientReference})
			if e != nil {
				return e
			}
			f.ID = manualBankFactID("refund", account, v.ExternalReference)
			f.ObservedAt = now
			if e = bindRefundFact(tx, m, r, f); e != nil {
				return e
			}
			if e = copy.RecordFact(ctx, m, original, f); e != nil {
				return e
			}
			out = RefundStatus{ProviderID: v.ExternalReference, State: "succeeded", Fact: &f}
			refundCipher, e := copy.seal(r)
			if e != nil {
				return e
			}
			resultCipher, e := copy.seal(out)
			if e != nil {
				return e
			}
			if e = tx.Table("payments_refund_intents").Create(map[string]any{"merchant_id": merchant, "refund_id": refundExecutionKey(r), "business_refund_id": r.ID, "operator_id": operator, "order_id": original.ID, "fingerprint": fp, "state": "verified_pending_settlement", "refund_ciphertext": refundCipher, "provider_id": v.ExternalReference, "result_ciphertext": resultCipher, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		if previous.State != "succeeded" {
			// A savepoint retains the verified bank fact when FX or business
			// settlement requires review. The original receipt stays reserved.
			settlementErr = refundSettlement(tx, func(t *gorm.DB) error {
				if e := manualApproval.RequireVerifiedManualRefund(ctx, t, actor, r); e != nil {
					return e
				}
				return s.RefundSink.RecordVerifiedRefund(ctx, t, m, original, r, *out.Fact, *out.Fact.ExecutionValue)
			})
		}
		state := "succeeded"
		if settlementErr != nil {
			state = "verified_pending_settlement"
		}
		out.State = state
		encoded, e := copy.seal(out)
		if e != nil {
			return e
		}
		return tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", merchant, refundExecutionKey(r)).Updates(map[string]any{"state": state, "result_ciphertext": encoded, "updated_at": now}).Error
	})
	if e != nil {
		return out, e
	}
	return out, settlementErr
}
