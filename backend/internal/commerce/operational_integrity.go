package commerce

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type IntegrityFact struct {
	Reference, Kind, Unit string
	Expected, Actual      ledger.Decimal
}

// Orders and their allocations remain linked when service responsibility moves.
func (s *Service) OperationalIntegrity(ctx context.Context, tx *gorm.DB, operator string) ([]IntegrityFact, error) {
	if !validID(operator) {
		return nil, ErrInvalidInput
	}
	out := []IntegrityFact{}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(`WITH orders AS (
 SELECT o.*, COALESCE(a.amount,0) AS verified_money, d.id AS delivery_id,
   CASE WHEN g.customer_id=o.customer_id AND g.order_id=o.id::text THEN g.total ELSE 0 END AS delivered_atg
 FROM commerce_orders o
 LEFT JOIN LATERAL (
   SELECT sum(a.amount_money) AS amount FROM commerce_receipt_allocations a
   JOIN commerce_receipts r ON r.id=a.receipt_id AND r.operator_id=a.operator_id
   WHERE a.order_id=o.id AND a.operator_id=o.original_operator_id
 ) a ON true
 LEFT JOIN commerce_deliveries d ON d.order_id=o.id
 LEFT JOIN ledger_grants g ON g.id=d.grant_id
 WHERE o.original_operator_id=?
 )
 SELECT id::text AS reference,'retail_receipt_allocation' AS kind,'MONEY' AS unit,verified_money::text AS expected,external_allocated_money::text AS actual
 FROM orders WHERE external_allocated_money<>verified_money
 UNION ALL SELECT id::text,'retail_order_payment','MONEY',due_money::text,(internal_money+commission_money+external_allocated_money)::text
 FROM orders WHERE status IN ('paid','completed') AND due_money<>internal_money+commission_money+external_allocated_money AND external_allocated_money=verified_money
 UNION ALL SELECT id::text,'retail_delivery','笔',CASE WHEN status='completed' THEN '1' ELSE '0' END,CASE WHEN delivery_id IS NULL THEN '0' ELSE '1' END
 FROM orders WHERE (status='completed')<>(delivery_id IS NOT NULL)
 UNION ALL SELECT id::text,'retail_delivered_credits','ATG',(paid_atg+package_gift_atg+merchant_gift_atg+kol_gift_atg)::text,COALESCE(delivered_atg,0)::text
 FROM orders WHERE kind IN ('package','renewal') AND delivery_id IS NOT NULL AND
 COALESCE(delivered_atg,0)<>paid_atg+package_gift_atg+merchant_gift_atg+kol_gift_atg`, operator).Scan(&out).Error
	})
	return out, err
}

func (s *Service) HasOrderReconciliationEvidence(ctx context.Context, tx *gorm.DB, operator, order, kind, evidence string) (bool, error) {
	var query string
	switch kind {
	case "retail_receipt_allocation", "retail_order_payment":
		query = `SELECT EXISTS(SELECT 1 FROM commerce_receipt_allocations a
 JOIN commerce_orders o ON o.id=a.order_id AND o.original_operator_id=a.operator_id
 WHERE o.original_operator_id=? AND o.id=? AND (a.id=? OR a.receipt_id=?))`
	case "retail_delivery", "retail_delivered_credits":
		query = `SELECT EXISTS(SELECT 1 FROM commerce_deliveries d JOIN commerce_orders o ON o.id=d.order_id
 WHERE o.original_operator_id=? AND o.id=? AND (d.id=? OR d.grant_id=?))`
	default:
		return false, nil
	}
	var exists bool
	err := s.dbFor(ctx, tx).Raw(query, operator, order, evidence, evidence).Row().Scan(&exists)
	return exists, err
}
