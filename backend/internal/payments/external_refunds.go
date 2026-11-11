package payments

import (
	"context"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

type ExternalRefundDestination struct {
	Network                string `json:"network"`
	Asset                  string `json:"asset"`
	TokenContract          string `json:"token_contract"`
	Address                string `json:"address"`
	AuthorizationReference string `json:"authorization_reference"`
	VerificationEvidence   string `json:"verification_evidence"`
}
type ExternalRefundExecution struct {
	Destination        ExternalRefundDestination `json:"destination"`
	SenderAddress      string                    `json:"sender_address,omitempty"`
	RecipientAddress   string                    `json:"recipient_address"`
	VerificationMethod string                    `json:"verification_method"`
	VerifiedBy         string                    `json:"verified_by"`
}
type ExternalRefundVerification struct {
	ReceiptID         string                    `json:"receipt_id"`
	ExternalReference string                    `json:"external_reference"`
	Money             Money                     `json:"money"`
	ExecutedAt        time.Time                 `json:"executed_at"`
	Destination       ExternalRefundDestination `json:"destination"`
	EvidenceReference string                    `json:"evidence_reference"`
}
type ExternalRefundApproval interface {
	RequireExternalRefundFact(context.Context, *gorm.DB, string, Refund, ExternalRefundDestination) error
}

type ResponsibleRefundBasis struct {
	RefundID, OperatorID, SourceOperatorID, ReceiptID, Currency string
	ExecutionMerchantID                                         string
	AmountMoney, RemainingMoney                                 ledger.Decimal
	AuthorizedAt, CreatedAt                                     time.Time
}
type RefundSource struct {
	SourceOperatorID        string `json:"source_operator_id"`
	ReceiptID               string `json:"receipt_id"`
	ResponsibilityReference string `json:"responsibility_reference"`
}
type ResponsibleRefundAuthority interface {
	RequireResponsibleRefundExecution(context.Context, *gorm.DB, string, string, string, ExternalRefundDestination) (ResponsibleRefundBasis, error)
}

func NormalizeExternalRefundDestination(d ExternalRefundDestination) (ExternalRefundDestination, error) {
	d.Network = strings.ToLower(strings.TrimSpace(d.Network))
	d.Asset = strings.ToUpper(strings.TrimSpace(d.Asset))
	d.Address = strings.TrimSpace(d.Address)
	d.TokenContract = strings.TrimSpace(d.TokenContract)
	if d.Network == "" || len(d.Network) > 64 || d.Asset == "" || len(d.Asset) > 32 || d.Address == "" || len(d.Address) > 256 || len(d.TokenContract) > 256 || strings.TrimSpace(d.AuthorizationReference) == "" || len(d.AuthorizationReference) > 1024 || strings.TrimSpace(d.VerificationEvidence) == "" || len(d.VerificationEvidence) > 2048 {
		return d, ErrInvalid
	}
	for _, v := range []string{d.Network, d.Asset, d.Address, d.TokenContract} {
		for _, r := range v {
			if unicode.IsSpace(r) || unicode.IsControl(r) {
				return d, ErrInvalid
			}
		}
	}
	if _, ok := fiatMinorDigits(d.Asset); !ok || d.Network != "bank" || d.TokenContract != "" {
		return d, ErrInvalid
	}
	return d, nil
}

func externalValuationAt(r Refund, at *time.Time) error {
	if r.Money.Validate() != nil || at == nil || at.IsZero() {
		return ErrInvalid
	}
	if r.Money.Currency == r.Order.Settlement().Currency {
		return nil
	}
	if r.RefundValuation.Validate("", time.Time{}) != nil || r.RefundValuation.Currency != r.Order.Settlement().Currency {
		return ErrInvalid
	}
	return nil
}

func validateExternalExecution(f Fact) error {
	if f.ExternalRefund == nil {
		return nil
	}
	x := f.ExternalRefund
	_, e := NormalizeExternalRefundDestination(x.Destination)
	if e != nil {
		return e
	}
	if x.VerifiedBy == "" || x.RecipientAddress == "" {
		return ErrInvalid
	}
	return nil
}

func (s *Service) recordExternalRefund(ctx context.Context, actor string, m Merchant, r Refund, f Fact, d ExternalRefundDestination, verificationHash string, mismatch error) (RefundStatus, error) {
	var out RefundStatus
	var settlementErr error
	fp := fingerprint([]string{fingerprint(r), verificationHash})
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.Authorize.RequirePermission(ctx, tx, actor, m.OperatorID, "payments.manage"); e != nil {
			return e
		}
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", m.ID+":refund:"+r.ID).Error; e != nil {
			return e
		}
		var old refundIntentRow
		q := tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", m.ID, refundExecutionKey(r)).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			if e := s.open(old.ResultCiphertext, &out); e != nil {
				return e
			}
			if out.Fact == nil || out.Fact.ExecutionValue == nil {
				return ErrInvalid
			}
			out.Fact.proof = true
		} else {
			copy := *s
			copy.DB = tx
			if e := bindRefundFact(tx, m, r, f); e != nil {
				return e
			}
			if e := copy.RecordFact(ctx, m, r.Order, f); e != nil {
				return e
			}
			out = RefundStatus{ProviderID: f.ProviderID, State: "verified_pending_settlement", Fact: &f}
			command, e := s.seal(r)
			if e != nil {
				return e
			}
			result, e := s.seal(out)
			if e != nil {
				return e
			}
			now, e := databaseTime(ctx, tx)
			if e != nil {
				return e
			}
			if e = tx.Table("payments_refund_intents").Create(map[string]any{"merchant_id": m.ID, "refund_id": refundExecutionKey(r), "business_refund_id": r.ID, "operator_id": m.OperatorID, "order_id": r.Order.ID, "fingerprint": fp, "state": "verified_pending_settlement", "refund_ciphertext": command, "provider_id": f.ProviderID, "result_ciphertext": result, "created_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		if old.State != "succeeded" {
			settlementErr = refundSettlement(tx, func(t *gorm.DB) error {
				if e := validateExternalExecution(*out.Fact); e != nil {
					return e
				}
				if mismatch != nil {
					return fmt.Errorf("external transfer recipient differs from approved destination: %w", mismatch)
				}
				if f.RefundSource != nil {
					authority, ok := s.RefundApproval.(ResponsibleRefundAuthority)
					if !ok {
						return ErrNotConfigured
					}
					basis, e := authority.RequireResponsibleRefundExecution(ctx, t, actor, m.OperatorID, r.ID, d)
					if e != nil {
						return e
					}
					if basis.SourceOperatorID != f.RefundSource.SourceOperatorID || basis.ReceiptID != f.RefundSource.ReceiptID || basis.AmountMoney.Cmp(r.MoneyAmount) != 0 {
						return ErrConflict
					}
				} else if e := s.RefundApproval.(ExternalRefundApproval).RequireExternalRefundFact(ctx, t, actor, r, d); e != nil {
					return e
				}
				return s.RefundSink.RecordVerifiedRefund(ctx, t, m, r.Order, r, *out.Fact, *out.Fact.ExecutionValue)
			})
		}
		out.State = "succeeded"
		if settlementErr != nil {
			out.State = "verified_pending_settlement"
		}
		encoded, e := s.seal(out)
		if e != nil {
			return e
		}
		return tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", m.ID, refundExecutionKey(r)).Updates(map[string]any{"state": out.State, "result_ciphertext": encoded, "updated_at": gorm.Expr("clock_timestamp()")}).Error
	})
	if e != nil {
		return out, e
	}
	return out, settlementErr
}
