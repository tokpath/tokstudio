package payments

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// A durable short lease fences route closure against a network request that
// has been authorized but has not yet returned. No database lock spans HTTP.
func (s *Service) claimRefundQuery(ctx context.Context, operator, merchant, execution string) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct {
			State           string
			QueryLeaseUntil *time.Time
		}
		if e := tx.Table("payments_refund_intents").Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND merchant_id=? AND refund_id=?", operator, merchant, execution).Take(&row).Error; e != nil {
			return e
		}
		var n int64
		if e := tx.Table("payments_refund_execution_closures").Where("merchant_id=? AND execution_id=?", merchant, execution).Count(&n).Error; e != nil {
			return e
		}
		if n > 0 {
			return ErrConflict
		}
		now, e := databaseTime(ctx, tx)
		if e != nil {
			return e
		}
		if row.QueryLeaseUntil != nil && row.QueryLeaseUntil.After(now) {
			return ErrOutcomeUnknown
		}
		return tx.Table("payments_refund_intents").Where("merchant_id=? AND refund_id=?", merchant, execution).Updates(map[string]any{"state": "querying", "query_lease_until": now.Add(time.Minute), "updated_at": now}).Error
	})
}

type RefundExecutionClosureFact struct {
	ExecutionID string `json:"execution_id"`
	MerchantID  string `json:"merchant_id"`
	State       string `json:"state"`
	FactID      string `json:"fact_id,omitempty"`
	ClosureID   string `json:"closure_id,omitempty"`
}

// RefundExecutionClosureFacts is a trusted, transaction-scoped inspection of
// stored executions. It never opens merchant credentials or contacts a provider.
func (s *Service) RefundExecutionClosureFacts(ctx context.Context, tx *gorm.DB, operator, legID string) ([]RefundExecutionClosureFact, error) {
	out := []RefundExecutionClosureFact{}
	if tx == nil || operator == "" || legID == "" {
		return out, ErrInvalid
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return out, ErrInvalid
	}
	var rows []refundIntentRow
	if e := tx.WithContext(ctx).Table("payments_refund_intents").Where("operator_id=? AND business_refund_id=?", operator, legID).Order("merchant_id,refund_id").Find(&rows).Error; e != nil {
		return out, e
	}
	for _, row := range rows {
		v := RefundExecutionClosureFact{ExecutionID: row.RefundID, MerchantID: row.MerchantID, State: row.State}
		if row.ResultCiphertext != "" {
			var result RefundStatus
			if e := s.open(row.ResultCiphertext, &result); e != nil {
				return out, e
			}
			if result.Fact != nil {
				v.FactID = result.Fact.ID
			}
		}
		var c struct{ ClosureID string }
		if e := tx.Table("payments_refund_execution_closures").Where("merchant_id=? AND execution_id=?", row.MerchantID, row.RefundID).Limit(1).Find(&c).Error; e != nil {
			return out, e
		}
		v.ClosureID = c.ClosureID
		out = append(out, v)
	}
	return out, nil
}

// SealRefundExecutions is called only after the domain has verified every
// stored payment fact and independently closed the old route. Unknown or
// running executions cannot be sealed. No mutation is retried after sealing.
func (s *Service) SealRefundExecutions(ctx context.Context, tx *gorm.DB, operator, legID, closureID string) error {
	if closureID == "" {
		return ErrInvalid
	}
	if tx == nil {
		return ErrInvalid
	}
	var locked []refundIntentRow
	if e := tx.WithContext(ctx).Table("payments_refund_intents").Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND business_refund_id=?", operator, legID).Order("merchant_id,refund_id").Find(&locked).Error; e != nil {
		return e
	}
	facts, e := s.RefundExecutionClosureFacts(ctx, tx, operator, legID)
	if e != nil {
		return e
	}
	for _, v := range facts {
		if v.ClosureID != "" {
			continue
		}
		if v.State != "failed" && v.State != "cancelled" && !((v.State == "succeeded" || v.State == "verified_pending_settlement") && v.FactID != "") {
			return ErrOutcomeUnknown
		}
		if e := tx.Exec(`INSERT INTO payments_refund_execution_closures(operator_id,merchant_id,execution_id,business_refund_id,closure_id) VALUES(?,?,?,?,?) ON CONFLICT(merchant_id,execution_id) DO NOTHING`, operator, v.MerchantID, v.ExecutionID, legID, closureID).Error; e != nil {
			return e
		}
	}
	return nil
}

func (s *Service) refundExecutionSealed(ctx context.Context, operator, merchant, execution string) (bool, error) {
	var count int64
	e := s.DB.WithContext(ctx).Table("payments_refund_execution_closures").Where("operator_id=? AND merchant_id=? AND execution_id=?", operator, merchant, execution).Count(&count).Error
	return count > 0, e
}
