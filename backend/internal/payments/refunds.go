package payments

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type RefundFactSink interface {
	RecordVerifiedRefund(context.Context, *gorm.DB, Merchant, Order, Refund, Fact, USDValue) error
}

// ErrSettlementReview means a sink committed an actual payment observation and
// an explicit review state. It must survive the matching savepoint, while the
// payment intent remains pending and cannot be reported as fully settled.
var ErrSettlementReview = errors.New("actual refund execution requires settlement review")

func refundSettlement(tx *gorm.DB, fn func(*gorm.DB) error) error {
	var review error
	e := tx.Transaction(func(t *gorm.DB) error {
		e := fn(t)
		if errors.Is(e, ErrSettlementReview) {
			review = e
			return nil
		}
		return e
	})
	if e != nil {
		return e
	}
	return review
}

type refundIntentRow struct {
	MerchantID, RefundID, OperatorID, OrderID, Fingerprint, State, RefundCiphertext, ProviderID, ResultCiphertext string
	BusinessRefundID                                                                                              string
	CreatedAt, UpdatedAt                                                                                          time.Time
}

func refundExecutionKey(r Refund) string {
	if r.ExecutionID != "" {
		return r.ExecutionID
	}
	return r.ID
}

func bindRefundFact(tx *gorm.DB, m Merchant, r Refund, f Fact) error {
	if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "refund-fact:"+f.ID).Error; e != nil {
		return e
	}
	var old struct{ OperatorID, MerchantID, BusinessRefundID string }
	q := tx.Table("payments_refund_fact_bindings").Where("fact_id=?", f.ID).Limit(1).Find(&old)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		if old.OperatorID != m.OperatorID || old.MerchantID != m.ID || old.BusinessRefundID != r.ID {
			return ErrConflict
		}
		return nil
	}
	return tx.Table("payments_refund_fact_bindings").Create(map[string]any{"fact_id": f.ID, "operator_id": m.OperatorID, "merchant_id": m.ID, "business_refund_id": r.ID}).Error
}

// SubmitRefund requires the business refund approval/hold in the same database
// transaction. A finance permission alone does not authorize sending money.
func (s *Service) SubmitRefund(ctx context.Context, actor, operator, merchant string, r Refund) (RefundStatus, error) {
	if r.ExecutionID != "" {
		return RefundStatus{}, ErrUnsupported
	}
	if s.Authorize == nil || s.RefundApproval == nil {
		return RefundStatus{}, ErrNotConfigured
	}
	m, e := s.Merchant(ctx, operator, merchant)
	if e != nil {
		return RefundStatus{}, e
	}
	d := s.Drivers[m.Driver]
	if m.Driver != "stripe" && m.Driver != "alipay" && m.Driver != "wechat" {
		return RefundStatus{}, ErrUnsupported
	}
	m.Enabled = true // Collection disablement does not revoke an approved original-route refund.
	if d == nil || ready(m, d) != nil {
		return RefundStatus{}, ErrNotConfigured
	}
	if r.Validate(m) != nil {
		return RefundStatus{}, ErrInvalid
	}
	original, e := s.loadOrder(ctx, m, r.Order.ID)
	if e != nil {
		return RefundStatus{}, e
	}
	if original.Money.Currency != r.Order.Money.Currency || original.Money.Amount.Cmp(r.Order.Money.Amount) != 0 {
		return RefundStatus{}, ErrConflict
	}
	r.Order = original
	hash := fingerprint(r)
	fresh := false
	var previous refundIntentRow
	e = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := s.Authorize.RequirePermission(ctx, tx, actor, operator, "payments.manage"); e != nil {
			return e
		}
		if e := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", merchant+":refund:"+r.ID).Error; e != nil {
			return e
		}
		q := tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", merchant, r.ID).Limit(1).Find(&previous)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if previous.Fingerprint != hash {
				return ErrConflict
			}
			return nil
		}
		now, e := databaseTime(ctx, tx)
		if e != nil {
			return e
		}
		if r.Money.Currency != "USD" {
			if r.ValuedAt.After(now.Add(30*time.Second)) || now.Sub(r.ValuedAt) > 5*time.Minute || r.RefundValuation.Validate(r.Money.Currency, now) != nil {
				return ErrInvalid
			}
		}
		if e := s.RefundApproval.RequireApprovedRefund(ctx, tx, actor, r); e != nil {
			return e
		}
		cipher, e := s.seal(r)
		if e != nil {
			return e
		}
		fresh = true
		return tx.Table("payments_refund_intents").Create(map[string]any{"merchant_id": merchant, "refund_id": r.ID, "business_refund_id": r.ID, "operator_id": operator, "order_id": r.Order.ID, "fingerprint": hash, "state": "executing", "refund_ciphertext": cipher, "created_at": now, "updated_at": now}).Error
	})
	if e != nil {
		return RefundStatus{}, e
	}
	if !fresh {
		if previous.ResultCiphertext != "" && previous.State != "outcome_unknown" && previous.State != "executing" {
			var out RefundStatus
			if e = s.open(previous.ResultCiphertext, &out); e != nil {
				return out, e
			}
			if out.Fact != nil {
				out.Fact.proof = true
			}
			return out, nil
		}
		return RefundStatus{ProviderID: previous.ProviderID, State: "outcome_unknown"}, ErrOutcomeUnknown
	}
	result, e := d.Refund(ctx, m, r)
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelPersist()
	if saveErr := s.saveRefundResult(persistCtx, m, r, result, e); saveErr != nil {
		if result.Fact != nil && result.Fact.Verified() {
			result.State = "verified_pending_settlement"
			return result, saveErr
		}
		return result, ErrOutcomeUnknown
	}
	return result, e
}
func (s *Service) saveRefundResult(ctx context.Context, m Merchant, r Refund, result RefundStatus, callErr error) error {
	state := result.State
	if callErr != nil {
		state = "outcome_unknown"
		if errors.Is(callErr, ErrInvalid) || errors.Is(callErr, ErrProvider) || errors.Is(callErr, ErrForbidden) || errors.Is(callErr, ErrUnsupported) {
			state = "failed"
		}
	}
	var settlementErr error
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if result.Fact != nil {
			if !result.Fact.Verified() {
				return ErrInvalid
			}
			copy := *s
			copy.DB = tx
			if e := bindRefundFact(tx, m, r, *result.Fact); e != nil {
				return e
			}
			if e := copy.RecordFact(ctx, m, r.Order, *result.Fact); e != nil {
				return e
			}
			if result.Fact.ExecutionValue == nil {
				return ErrInvalid
			}
			// An already executed transfer is durable even when its business
			// matching/FX settlement cannot commit. Reconciliation replays this
			// stored fact without creating another external refund.
			settlementErr = refundSettlement(tx, func(t *gorm.DB) error {
				if e := validateExternalExecution(*result.Fact); e != nil {
					return e
				}
				if s.RefundSink == nil {
					return ErrNotConfigured
				}
				return s.RefundSink.RecordVerifiedRefund(ctx, t, m, r.Order, r, *result.Fact, *result.Fact.ExecutionValue)
			})
			if settlementErr != nil {
				state = "verified_pending_settlement"
			}
		}
		result.State = state
		cipher, e := s.seal(result)
		if e != nil {
			return e
		}
		updates := map[string]any{"state": state, "result_ciphertext": cipher, "updated_at": gorm.Expr("clock_timestamp()"), "query_lease_until": nil}
		if result.ProviderID != "" {
			updates["provider_id"] = result.ProviderID
		}
		return tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", m.ID, refundExecutionKey(r)).Updates(updates).Error
	})
	if e != nil {
		return e
	}
	return settlementErr
}

// QueryRefund only replays a Stripe mutation when its durable identical command
// is inside Stripe's guaranteed idempotency retention window. Other providers
// are queried by their original merchant refund number; they are never reissued.
func (s *Service) QueryRefund(ctx context.Context, operator, merchant, id string) (RefundStatus, error) {
	m, e := s.Merchant(ctx, operator, merchant)
	if e != nil {
		return RefundStatus{}, e
	}
	var row refundIntentRow
	q := s.DB.WithContext(ctx).Table("payments_refund_intents").Where("merchant_id=? AND refund_id=? AND operator_id=?", merchant, id, operator).Limit(1).Find(&row)
	if q.Error != nil {
		return RefundStatus{}, q.Error
	}
	if q.RowsAffected == 0 {
		if e = s.DB.WithContext(ctx).Table("payments_refund_intents").Where("merchant_id=? AND business_refund_id=? AND operator_id=?", merchant, id, operator).Order("created_at DESC,refund_id DESC").Take(&row).Error; e != nil {
			return RefundStatus{}, e
		}
	}
	sealed, e := s.refundExecutionSealed(ctx, operator, merchant, row.RefundID)
	if e != nil {
		return RefundStatus{}, e
	}
	if sealed {
		out := RefundStatus{State: "closed", ProviderID: row.ProviderID}
		if row.ResultCiphertext != "" {
			if e = s.open(row.ResultCiphertext, &out); e != nil {
				return out, e
			}
			if out.Fact != nil {
				out.Fact.proof = true
			}
			out.State = "closed"
		}
		return out, nil
	}
	var r Refund
	if e = s.open(row.RefundCiphertext, &r); e != nil {
		return RefundStatus{}, e
	}
	m, e = s.historicalMerchant(ctx, m, r.Order)
	if e != nil {
		return RefundStatus{}, e
	}
	if (row.State == "succeeded" || row.State == "verified_pending_settlement") && row.ResultCiphertext != "" {
		var final RefundStatus
		if e = s.open(row.ResultCiphertext, &final); e != nil {
			return final, e
		}
		if final.Fact != nil {
			final.Fact.proof = true
		}
		final.State = "succeeded"
		if e = s.saveRefundResult(ctx, m, r, final, nil); e != nil {
			final.State = "verified_pending_settlement"
			return final, e
		}
		return final, nil
	}
	r.ProviderID = row.ProviderID
	if e := s.claimRefundQuery(ctx, operator, merchant, row.RefundID); e != nil {
		return RefundStatus{State: "outcome_unknown", ProviderID: row.ProviderID}, e
	}
	d := s.Drivers[m.Driver]
	var result RefundStatus
	if m.Driver == "stripe" && r.ProviderID == "" {
		now, clockErr := databaseTime(ctx, s.DB)
		if clockErr != nil {
			return RefundStatus{}, clockErr
		}
		if now.Sub(row.CreatedAt) >= 23*time.Hour {
			return RefundStatus{State: "outcome_unknown"}, ErrOutcomeUnknown
		}
		result, e = d.Refund(ctx, m, r)
	} else {
		result, e = d.LookupRefund(ctx, m, r)
	}
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancelPersist()
	if saveErr := s.saveRefundResult(persistCtx, m, r, result, e); saveErr != nil {
		if result.Fact != nil && result.Fact.Verified() {
			result.State = "verified_pending_settlement"
		}
		return result, saveErr
	}
	return result, e
}
