package cooperation

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"time"
)

type IntegrityFact struct {
	Reference string         `json:"reference"`
	Kind      string         `json:"kind"`
	Unit      string         `json:"unit"`
	Expected  ledger.Decimal `json:"expected"`
	Actual    ledger.Decimal `json:"actual"`
}

func (s *Service) OperationalIntegrity(ctx context.Context, tx *gorm.DB, operator string) ([]IntegrityFact, error) {
	if _, e := uuid.Parse(operator); e != nil {
		return nil, e
	}
	out := []IntegrityFact{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(`WITH orders AS (
 SELECT o.*, COALESCE(a.amount,0) AS verified_money, l.id AS lot_id, l.paid_atg+l.gift_atg AS delivered_atg
 FROM cooperation_purchase_orders o
 LEFT JOIN LATERAL (
   SELECT sum(a.amount_money) AS amount FROM cooperation_purchase_allocations a
   JOIN commerce_receipt_funds_reservations f ON f.id=a.receipt_reservation_id AND f.destination_id=o.id
      AND f.receipt_id=a.receipt_id AND f.amount_money=a.amount_money AND f.operator_id=o.supplier_id
   JOIN commerce_receipts r ON r.id=f.receipt_id AND r.operator_id=f.operator_id
   WHERE a.order_id=o.id
 ) a ON true
 LEFT JOIN cooperation_procurement_lots l ON l.order_id=o.id::text AND l.buyer_id=o.buyer_id
 WHERE o.buyer_id=? OR o.supplier_id=?
 )
 SELECT p.buyer_id::text AS reference,'procurement_lots' AS kind,'ATG' AS unit,COALESCE(l.remaining,0)::text AS expected,p.quantity::text AS actual
 FROM cooperation_procurement_positions p LEFT JOIN(SELECT buyer_id,sum(remaining)remaining FROM cooperation_procurement_lots GROUP BY buyer_id)l ON l.buyer_id=p.buyer_id WHERE p.buyer_id=? AND p.quantity<>COALESCE(l.remaining,0)
 UNION ALL SELECT p.buyer_id::text,'procurement_account','ATG',a.balance::text,p.quantity::text FROM cooperation_procurement_positions p JOIN ledger_accounts a ON a.id=p.account_id WHERE p.buyer_id=? AND p.quantity<>a.balance
 UNION ALL SELECT id::text,'purchase_receipt_allocation','MONEY',verified_money::text,allocated_money::text FROM orders WHERE allocated_money<>verified_money
 UNION ALL SELECT id::text,'purchase_order_payment','MONEY',due_money::text,allocated_money::text FROM orders WHERE status IN ('paid','completed') AND due_money<>allocated_money AND allocated_money=verified_money
 UNION ALL SELECT id::text,'purchase_delivery','ATG',
   (CASE WHEN status='completed' THEN paid_atg+gift_atg ELSE 0 END)::text,COALESCE(delivered_atg,0)::text
 FROM orders WHERE (status='completed' AND (lot_id IS NULL OR delivered_atg<>paid_atg+gift_atg)) OR (status<>'completed' AND lot_id IS NOT NULL)`, operator, operator, operator, operator).Scan(&out).Error
	})
	return out, e
}

func (s *Service) HasPurchaseReconciliationEvidence(ctx context.Context, tx *gorm.DB, operator, order, kind, evidence string) (bool, error) {
	var query string
	switch kind {
	case "purchase_receipt_allocation", "purchase_order_payment":
		query = `SELECT EXISTS(SELECT 1 FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id
 WHERE (o.buyer_id=? OR o.supplier_id=?) AND o.id=? AND (a.id=? OR a.receipt_id=?))`
	case "purchase_delivery":
		query = `SELECT EXISTS(SELECT 1 FROM cooperation_procurement_lots l
 JOIN cooperation_purchase_orders o ON o.id::text=l.order_id AND o.buyer_id=l.buyer_id
 WHERE (o.buyer_id=? OR o.supplier_id=?) AND o.id=? AND l.id=?)`
	default:
		return false, nil
	}
	args := []any{operator, operator, order, evidence}
	if kind != "purchase_delivery" {
		args = append(args, evidence)
	}
	var exists bool
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw(query, args...).Row().Scan(&exists)
	})
	return exists, err
}
func (s *Service) OperationalLots(ctx context.Context, tx *gorm.DB, operator string, limit int) ([]ProcurementLot, error) {
	if _, e := uuid.Parse(operator); e != nil || limit < 1 || limit > 10001 {
		return nil, ErrConflict
	}
	out := []ProcurementLot{}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("buyer_id=?", operator).Order("delivered_at,id").Limit(limit).Find(&out).Error
	})
	return out, e
}

// Export only purchases in which this operator is an actual contracting party.
// A parent relationship alone never exposes another operator's purchases.
func (s *Service) OperationalPurchaseOrders(ctx context.Context, tx *gorm.DB, operator, party string, from, to time.Time, limit int) ([]PurchaseOrder, error) {
	if _, err := uuid.Parse(operator); err != nil {
		return nil, ErrConflict
	}
	if party != "" {
		if _, err := uuid.Parse(party); err != nil {
			return nil, ErrConflict
		}
	}
	if !to.After(from) || to.Sub(from) > 366*24*time.Hour || limit < 1 || limit > 100001 {
		return nil, ErrConflict
	}
	out := []PurchaseOrder{}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		query := t.Where("(supplier_id=? OR buyer_id=?) AND created_at>=? AND created_at<?", operator, operator, from, to)
		if party != "" {
			query = query.Where("(supplier_id=? OR buyer_id=?)", party, party)
		}
		return query.Order("created_at,id").Limit(limit).Find(&out).Error
	})
	return out, err
}
