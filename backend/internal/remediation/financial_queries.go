package remediation

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"time"
)

type RefundFinancialFacts struct {
	PendingFees       int64          `json:"pending_fees"`
	PendingExecutions int64          `json:"pending_executions"`
	OutstandingMoney  ledger.Decimal `json:"outstanding_money"`
	MismatchCount     int64          `json:"mismatch_count"`
}

// FinancialRefundFacts is an authorized report adapter's current snapshot.
// Old unresolved liabilities remain visible even when created before its flow
// window. Actual cash movements and fees are counted only by the ledger.
func (s *Service) FinancialRefundFacts(ctx context.Context, t *gorm.DB, op string, from, to time.Time) (RefundFinancialFacts, error) {
	var out RefundFinancialFacts
	if !validID(op) || from.IsZero() || !to.After(from) {
		return out, ErrInvalid
	}
	e := s.within(ctx, t, func(t *gorm.DB) error {
		if e := t.Table("remediation_payment_observations p").Joins("JOIN remediation_refund_legs l ON l.id=p.leg_id").Where("l.operator_id=? AND p.fee_status='pending' AND NOT EXISTS(SELECT 1 FROM remediation_refund_fee_confirmations f WHERE f.observation_id=p.id)", op).Count(&out.PendingFees).Error; e != nil {
			return e
		}
		if e := t.Model(&RefundLeg{}).Where("operator_id=? AND state IN ('executing','unknown')", op).Count(&out.PendingExecutions).Error; e != nil {
			return e
		}
		if e := t.Model(&RefundLeg{}).Where("operator_id=? AND (state='exception' OR excess_money>0 OR misdirected_money>0)", op).Count(&out.MismatchCount).Error; e != nil {
			return e
		}
		return t.Raw(`SELECT COALESCE(sum(l.amount_money-l.settled_money),0)::text FROM remediation_refund_legs l JOIN remediation_cases c ON c.id=l.case_id WHERE l.operator_id=? AND c.state IN ('approved','paying','exception') AND l.state NOT IN ('completed','cancelled')`, op).Row().Scan(&out.OutstandingMoney)
	})
	return out, e
}
