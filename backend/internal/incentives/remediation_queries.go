package incentives

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type RemediationEvent struct {
	SourceEventID, CustomerID, OrderID, Status string
	PaidMoney, ReversedMoney                   ledger.Decimal
}

func (s *Service) RemediationEvents(ctx context.Context, tx *gorm.DB, operator, order string) ([]RemediationEvent, error) {
	var out []RemediationEvent
	e := s.dbFor(ctx, tx).Raw(`SELECT e.source_event_id,e.customer_id,e.order_id,e.status,e.paid_money,COALESCE((SELECT sum(c.amount_money) FROM incentives_corrections c WHERE c.original_event_id=e.id),0)::text AS reversed_money FROM incentives_confirmation_events e WHERE e.operator_id=? AND e.order_id=? AND e.kind IN('usage','expiry') ORDER BY e.id`, operator, order).Scan(&out).Error
	return out, e
}
