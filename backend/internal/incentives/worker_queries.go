package incentives

import (
	"context"

	"gorm.io/gorm"
)

func (s *Service) PendingRegistrationRewards(ctx context.Context, tx *gorm.DB, limit int) ([]string, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrInvalidInput
	}
	ids := []string{}
	e := s.dbFor(ctx, tx).Model(&promiseRow{}).Where("status='pending_delivery'").Order("created_at,intent_id").Limit(limit).Pluck("intent_id", &ids).Error
	return ids, e
}
func (s *Service) PendingRegistrationRewardsPage(ctx context.Context, tx *gorm.DB, cursor string, limit int) ([]string, error) {
	if limit < 1 || limit > 200 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	q := s.dbFor(ctx, tx).Model(&promiseRow{}).Where("status='pending_delivery'")
	if cursor != "" {
		q = q.Where("intent_id>?", cursor)
	}
	ids := []string{}
	e := q.Order("intent_id").Limit(limit).Pluck("intent_id", &ids).Error
	return ids, e
}

type DueQualification struct{ CustomerID, OperatorID string }

// Read candidates from payable facts and qualification balances. The worker
// rechecks ownership and all release/qualification conditions before posting.
func (s *Service) DueQualificationPage(ctx context.Context, cursor string, limit int) ([]DueQualification, error) {
	if limit < 1 || limit > 100 || (cursor != "" && !validID(cursor)) {
		return nil, ErrInvalidInput
	}
	if cursor == "" {
		cursor = "00000000-0000-0000-0000-000000000000"
	}
	rows := []DueQualification{}
	err := s.dbFor(ctx, nil).Raw(`WITH due AS (
 SELECT e.operator_id,e.customer_id FROM incentives_confirmation_events e
 WHERE e.status='pending' AND e.confirm_at<=now() AND e.customer_id>?
 UNION
 SELECT e.operator_id,e.beneficiary_id FROM incentives_commission_entries e
 JOIN incentives_confirmation_events source ON source.id=e.event_id AND source.status='confirmed'
 LEFT JOIN LATERAL (
   SELECT release_blocked FROM incentives_function_states f
   WHERE f.operator_id=e.operator_id AND f.customer_id=e.beneficiary_id AND f.effective_at<=now()
   ORDER BY f.effective_at DESC,f.business_sequence DESC LIMIT 1
 ) state ON true
 WHERE e.status='frozen' AND e.confirm_at<=now() AND e.beneficiary_id>?
   AND NOT COALESCE(state.release_blocked,false)
 UNION
 SELECT q.operator_id,q.customer_id FROM incentives_qualification_accounts q
 JOIN LATERAL (
   SELECT p.threshold_money,p.version FROM incentives_policies p
   WHERE p.operator_id=q.operator_id AND p.effective_at<=now()
   ORDER BY p.effective_at DESC,p.business_sequence DESC LIMIT 1
 ) policy ON policy.version>0 AND policy.threshold_money<=q.confirmed_money
 WHERE q.customer_id>? AND q.acquired_at IS NULL AND (q.confirmed_money>0 OR policy.threshold_money=0)
 UNION
 SELECT q.operator_id,q.customer_id FROM incentives_qualification_accounts q
 JOIN ledger_accounts a ON a.operator_id=q.operator_id AND a.owner_id=q.customer_id AND a.purpose='commission:available' AND a.unit='MONEY'
 WHERE q.customer_id>? AND q.pending_offset_money>0 AND a.balance>a.held
 )
 SELECT due.operator_id,due.customer_id FROM due
 JOIN organization_service_ownerships own ON own.customer_id=due.customer_id AND own.operator_id=due.operator_id AND own.ended_at IS NULL
 JOIN organization_customers c ON c.id=due.customer_id AND c.status NOT IN ('closed','anonymized')
 ORDER BY due.customer_id LIMIT ?`, cursor, cursor, cursor, cursor, limit).Scan(&rows).Error
	return rows, err
}
