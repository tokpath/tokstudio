package payments

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAwaitingValuation = errors.New("verified original-currency payment awaits execution-time USD valuation")

type ReceiptAcceptanceFence interface {
	LockReceiptAcceptance(context.Context, *gorm.DB, string, string) error
}

func (s *Service) lockReceiptAcceptance(ctx context.Context, tx *gorm.DB, operator, orderID string) error {
	fence, ok := s.Authorize.(interface {
		LockOperatorResponsibility(context.Context, *gorm.DB, string) error
	})
	if !ok {
		return ErrNotConfigured
	}
	if e := fence.LockOperatorResponsibility(ctx, tx, operator); e != nil {
		return e
	}
	if domain, ok := s.Sink.(ReceiptAcceptanceFence); ok {
		return domain.LockReceiptAcceptance(ctx, tx, operator, orderID)
	}
	return nil
}

type receiptFactRow struct {
	ID, MerchantID, OperatorID, OrderID, Kind, Fingerprint, ReceiptID, State, ValuationReason string
	Fact, Valuation                                                                           []byte
	ValuationVersion                                                                          int64
	ObservedAt                                                                                time.Time
}
type ReceiptValuationView struct {
	FactID     string     `json:"fact_id"`
	MerchantID string     `json:"merchant_id"`
	OrderID    string     `json:"order_id"`
	Driver     string     `json:"driver"`
	Money      Money      `json:"money"`
	PaidAt     *time.Time `json:"paid_at,omitempty"`
	ObservedAt time.Time  `json:"observed_at"`
	State      string     `json:"state"`
	Reason     string     `json:"reason"`
}
type ReceiptValuationConfirmation struct {
	BusinessKey         string     `json:"business_key"`
	Valuation           Valuation  `json:"valuation"`
	PaidAt              *time.Time `json:"paid_at,omitempty"`
	PaymentTimeEvidence string     `json:"payment_time_evidence"`
	EvidenceReference   string     `json:"evidence_reference"`
}
type ReceiptValuationResult struct {
	FactID    string    `json:"fact_id"`
	ReceiptID string    `json:"receipt_id,omitempty"`
	State     string    `json:"state"`
	Version   int64     `json:"version"`
	Value     *USDValue `json:"value,omitempty"`
}

func receiptExecutionValue(o Order, f Fact) (USDValue, string) {
	if f.Money.Currency == "USD" {
		at := f.ObservedAt
		if f.OccurredAt != nil {
			at = *f.OccurredAt
		}
		if at.IsZero() {
			at = o.AcceptedAt
		}
		v, e := (Valuation{ID: "native-usd-receipt:" + f.ID, Currency: "USD", USDPerUnit: ledger.FromInt64(1), Source: "native_usd_identity", EffectiveAt: at.Add(-time.Second), ExpiresAt: at.Add(time.Second)}).Convert(f.Money, at)
		if e != nil {
			return USDValue{}, "invalid_original_amount"
		}
		return v, ""
	}
	if f.OccurredAt == nil || f.OccurredAt.IsZero() {
		return USDValue{}, "payment_time_unverified"
	}
	v, e := o.Valuation.Convert(f.Money, *f.OccurredAt)
	if e != nil {
		return USDValue{}, "execution_time_valuation_unavailable"
	}
	return v, ""
}

func (s *Service) recordValuationResidual(tx *gorm.DB, m Merchant, o Order, f Fact, value USDValue) error {
	if value.RemainderNumerator == "0" {
		return nil
	}
	return tx.Table("payments_valuation_residuals").Clauses(clause.OnConflict{DoNothing: true}).Create(map[string]any{"fact_id": f.ID, "operator_id": m.OperatorID, "order_id": o.ID, "numerator": value.RemainderNumerator, "denominator": value.RemainderDenominator, "valuation_reference": value.Valuation.ID}).Error
}
func (s *Service) recordReceiptValuation(ctx context.Context, tx *gorm.DB, m Merchant, o Order, f Fact, value USDValue, key, hash, timeEvidence, evidence, actor string) error {
	now, e := databaseTime(ctx, tx)
	if e != nil {
		return e
	}
	raw, _ := json.Marshal(value)
	v, _ := json.Marshal(value.Valuation)
	if e = tx.Table("payments_receipt_valuations").Create(map[string]any{"id": uuid.NewString(), "fact_id": f.ID, "operator_id": m.OperatorID, "version": 1, "business_key": key, "fingerprint": hash, "paid_at": f.OccurredAt, "payment_time_evidence": timeEvidence, "valuation": string(v), "value": string(raw), "evidence_reference": evidence, "verified_by": actor, "created_at": now}).Error; e != nil {
		return e
	}
	state := "verified"
	if value.Amount.Sign() <= 0 {
		state = "awaiting_precision"
	}
	if e = tx.Table("payments_verified_facts").Where("id=?", f.ID).Updates(map[string]any{"valuation": string(raw), "valuation_version": 1, "valuation_reason": "", "state": state}).Error; e != nil {
		return e
	}
	return s.recordValuationResidual(tx, m, o, f, value)
}

// The verified money and valuation commit even if later receipt matching fails.
// Recovery retries this stored fact; it never asks the payer to pay again.
func (s *Service) attachReceiptDurably(ctx context.Context, tx *gorm.DB, m Merchant, o Order, f Fact, value USDValue) error {
	e := tx.Transaction(func(t *gorm.DB) error { return s.attachReceipt(ctx, t, m, o, f, value) })
	if e == nil {
		return nil
	}
	if update := tx.Table("payments_verified_facts").Where("id=?", f.ID).Update("state", "verified_pending_receipt").Error; update != nil {
		return update
	}
	if update := tx.Table("payments_intents").Where("merchant_id=? AND order_id=?", m.ID, o.ID).Update("state", "verified_pending_receipt").Error; update != nil {
		return update
	}
	return e
}

func (s *Service) PendingReceiptValuations(ctx context.Context, actor, operator, cursor string, limit int) ([]ReceiptValuationView, string, error) {
	out := []ReceiptValuationView{}
	if s.Authorize == nil {
		return out, "", ErrForbidden
	}
	if e := s.Authorize.RequirePermission(ctx, s.DB, actor, operator, "payments.manage"); e != nil {
		return out, "", e
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	q := s.DB.WithContext(ctx).Table("payments_verified_facts").Where("operator_id=? AND kind='receipt' AND state='awaiting_valuation'", operator)
	if cursor != "" {
		id, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || len(id) == 0 {
			return out, "", ErrInvalid
		}
		q = q.Where("id>?", string(id))
	}
	var rows []receiptFactRow
	if e := q.Order("id").Limit(limit + 1).Find(&rows).Error; e != nil {
		return out, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		next = base64.RawURLEncoding.EncodeToString([]byte(rows[len(rows)-1].ID))
	}
	for _, row := range rows {
		var f Fact
		if json.Unmarshal(row.Fact, &f) != nil {
			return out, "", ErrInvalid
		}
		out = append(out, ReceiptValuationView{FactID: f.ID, MerchantID: row.MerchantID, OrderID: row.OrderID, Driver: f.Driver, Money: f.Money, PaidAt: f.OccurredAt, ObservedAt: f.ObservedAt, State: row.State, Reason: row.ValuationReason})
	}
	return out, next, nil
}

func (s *Service) ConfirmReceiptValuation(ctx context.Context, actor, operator, factID string, in ReceiptValuationConfirmation) (ReceiptValuationResult, error) {
	out := ReceiptValuationResult{FactID: factID}
	if s.Authorize == nil || in.BusinessKey == "" || len(in.BusinessKey) > 512 || in.EvidenceReference == "" {
		return out, ErrInvalid
	}
	if in.PaidAt != nil {
		at := in.PaidAt.UTC()
		in.PaidAt = &at
	}
	hash := fingerprint(in)
	var pendingErr error
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.Authorize.RequirePermission(ctx, tx, actor, operator, "payments.manage"); e != nil {
			return e
		}
		var identity struct{ OrderID string }
		if e := tx.Table("payments_verified_facts").Where("id=? AND operator_id=? AND kind='receipt'", factID, operator).Select("order_id").Take(&identity).Error; e != nil {
			return e
		}
		if e := s.lockReceiptAcceptance(ctx, tx, operator, identity.OrderID); e != nil {
			return e
		}
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", factID).Error; e != nil {
			return e
		}
		var row receiptFactRow
		if e := tx.Table("payments_verified_facts").Where("id=? AND operator_id=? AND kind='receipt'", factID, operator).Take(&row).Error; e != nil {
			return e
		}
		var f Fact
		if json.Unmarshal(row.Fact, &f) != nil {
			return ErrInvalid
		}
		f.proof = true
		copy := *s
		copy.DB = tx
		m, e := copy.Merchant(ctx, operator, row.MerchantID)
		if e != nil {
			return e
		}
		o, e := copy.loadOrder(ctx, m, row.OrderID)
		if e != nil {
			return e
		}
		var old struct{ FactID, Fingerprint string }
		q := tx.Table("payments_receipt_valuations").Where("operator_id=? AND business_key=?", operator, in.BusinessKey).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.FactID != factID || old.Fingerprint != hash {
				return ErrConflict
			}
			var value USDValue
			if json.Unmarshal(row.Valuation, &value) != nil {
				return ErrInvalid
			}
			if row.ReceiptID == "" {
				pendingErr = s.attachReceiptDurably(ctx, tx, m, o, f, value)
			}
		} else {
			if row.ValuationVersion != 0 || row.ReceiptID != "" || row.State != "awaiting_valuation" {
				return ErrConflict
			}
			if in.PaidAt != nil {
				if f.OccurredAt != nil && !f.OccurredAt.Equal(*in.PaidAt) {
					return ErrConflict
				}
				if f.OccurredAt == nil {
					if in.PaymentTimeEvidence == "" {
						return ErrInvalid
					}
					f.OccurredAt = in.PaidAt
					evidence, _ := json.Marshal(map[string]any{"provider_evidence": json.RawMessage(f.Evidence), "payment_time_confirmation": map[string]any{"evidence_reference": in.PaymentTimeEvidence, "verified_by": "staff:" + actor, "paid_at": in.PaidAt}})
					f.Evidence = evidence
				}
			}
			if f.OccurredAt == nil || f.OccurredAt.IsZero() {
				return ErrAwaitingValuation
			}
			now, e := databaseTime(ctx, tx)
			if e != nil {
				return e
			}
			if f.OccurredAt.After(now.Add(5 * time.Minute)) {
				return ErrInvalid
			}
			value, e := in.Valuation.Convert(f.Money, *f.OccurredAt)
			if e != nil {
				return e
			}
			raw, _ := json.Marshal(f)
			if e = tx.Table("payments_verified_facts").Where("id=?", f.ID).Update("fact", string(raw)).Error; e != nil {
				return e
			}
			if e = s.recordReceiptValuation(ctx, tx, m, o, f, value, in.BusinessKey, hash, in.PaymentTimeEvidence, in.EvidenceReference, "staff:"+actor); e != nil {
				return e
			}
			pendingErr = s.attachReceiptDurably(ctx, tx, m, o, f, value)
		}
		if e = tx.Table("payments_verified_facts").Where("id=?", factID).Take(&row).Error; e != nil {
			return e
		}
		out.State, out.ReceiptID, out.Version = row.State, row.ReceiptID, row.ValuationVersion
		if string(row.Valuation) != "null" {
			var value USDValue
			if json.Unmarshal(row.Valuation, &value) != nil {
				return ErrInvalid
			}
			out.Value = &value
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	return out, pendingErr
}
