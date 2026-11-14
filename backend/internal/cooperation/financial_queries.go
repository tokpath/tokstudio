package cooperation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type FinancialSnapshot struct {
	PurchaseOrders                   int64          `json:"purchase_orders"`
	PurchasesMoney                   ledger.Decimal `json:"purchases_money"`
	WholesaleOrders                  int64          `json:"wholesale_orders"`
	WholesaleSalesMoney              ledger.Decimal `json:"wholesale_sales_money"`
	ProcurementCostMoney             ledger.Decimal `json:"procurement_cost_money"`
	ProcurementCorrectionMoney       ledger.Decimal `json:"procurement_correction_money"`
	WholesaleEarnedMoney             ledger.Decimal `json:"wholesale_earned_money"`
	WholesaleCorrectionMoney         ledger.Decimal `json:"wholesale_correction_money"`
	ClosingInventoryCostMoney        ledger.Decimal `json:"closing_inventory_cost_money"`
	VerifiedProcurementPaymentsMoney ledger.Decimal `json:"verified_procurement_payments_money"`
	ClosingProcurementPaymentsMoney  ledger.Decimal `json:"closing_procurement_payments_money"`
	ClosingWholesalePrepaymentMoney  ledger.Decimal `json:"closing_wholesale_prepayment_money"`
}

// FinancialFacts reads real lots, consumption and reversals. Cash purchases
// increase inventory; only consumption releases its weighted historical cost.
// A supplier earns its direct buyers' consumed purchase value, independently
// from the price those buyers charge their own customers.
func (s *Service) FinancialFacts(ctx context.Context, tx *gorm.DB, operator string, from, to time.Time) (FinancialSnapshot, error) {
	var out FinancialSnapshot
	if _, e := uuid.Parse(operator); e != nil || from.IsZero() || !to.After(from) || to.Sub(from) > 366*24*time.Hour {
		return out, ErrConflict
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		for _, q := range []struct {
			Where  string
			Count  *int64
			Amount *ledger.Decimal
		}{{"buyer_id=?", &out.PurchaseOrders, &out.PurchasesMoney}, {"supplier_id=?", &out.WholesaleOrders, &out.WholesaleSalesMoney}} {
			var r struct {
				Count  int64
				Amount ledger.Decimal
			}
			if e := t.Model(&PurchaseOrder{}).Where(q.Where, operator).Where("status='completed' AND completed_at>=? AND completed_at<?", from, to).Select("count(*) AS count,COALESCE(sum(due_money),0)::text AS amount").Scan(&r).Error; e != nil {
				return e
			}
			*q.Count = r.Count
			*q.Amount = r.Amount
		}
		for _, q := range []struct {
			SQL    string
			Target *ledger.Decimal
		}{
			{`SELECT COALESCE(sum(cost_money),0)::text FROM cooperation_procurement_settlements WHERE buyer_id=? AND created_at>=? AND created_at<?`, &out.ProcurementCostMoney},
			{`SELECT COALESCE(sum(cost_money),0)::text FROM cooperation_procurement_reversals WHERE buyer_id=? AND created_at>=? AND created_at<?`, &out.ProcurementCorrectionMoney},
			{`SELECT COALESCE(sum(c.cost_money),0)::text FROM cooperation_procurement_settlements c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at>=? AND c.created_at<?`, &out.WholesaleEarnedMoney},
			{`SELECT COALESCE(sum(c.cost_money),0)::text FROM cooperation_procurement_reversals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at>=? AND c.created_at<?`, &out.WholesaleCorrectionMoney},
		} {
			if e := t.Raw(q.SQL, operator, from, to).Row().Scan(q.Target); e != nil {
				return e
			}
		}
		if e := t.Raw(`SELECT (COALESCE((SELECT sum(paid_money) FROM cooperation_procurement_lots WHERE buyer_id=? AND delivered_at<?),0)-COALESCE((SELECT sum(cost_money) FROM cooperation_procurement_settlements WHERE buyer_id=? AND created_at<?),0)+COALESCE((SELECT sum(cost_money) FROM cooperation_procurement_reversals WHERE buyer_id=? AND created_at<?),0)-COALESCE((SELECT sum(book_cost_money) FROM cooperation_procurement_removals WHERE buyer_id=? AND created_at<?),0))::text`, operator, to, operator, to, operator, to, operator, to).Row().Scan(&out.ClosingInventoryCostMoney); e != nil {
			return e
		}
		// Procurement allocations cover the principal. The buyer actually pays the
		// whole receipt, including any fee charged to them at checkout.
		if e := t.Raw(`SELECT
 COALESCE(sum(a.amount_money+r.payment_fee_money) FILTER (WHERE COALESCE(r.paid_at,r.observed_at)>=?),0)::text,
 COALESCE(sum(a.amount_money+r.payment_fee_money),0)::text
 FROM cooperation_purchase_allocations a
 JOIN cooperation_purchase_orders o ON o.id=a.order_id
 JOIN commerce_receipts r ON r.id=a.receipt_id AND r.operator_id=o.supplier_id
 WHERE o.buyer_id=? AND COALESCE(r.paid_at,r.observed_at)<?`, from, operator, to).Row().Scan(&out.VerifiedProcurementPaymentsMoney, &out.ClosingProcurementPaymentsMoney); e != nil {
			return e
		}
		if e := t.Raw(`SELECT (COALESCE((SELECT sum(l.paid_money) FROM cooperation_procurement_lots l JOIN cooperation_procurement_positions p ON p.buyer_id=l.buyer_id WHERE p.supplier_id=? AND l.delivered_at<?),0)-COALESCE((SELECT sum(c.cost_money) FROM cooperation_procurement_settlements c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0)+COALESCE((SELECT sum(c.cost_money) FROM cooperation_procurement_reversals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0)-COALESCE((SELECT sum(c.book_cost_money) FROM cooperation_procurement_removals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0))::text`, operator, to, operator, to, operator, to, operator, to).Row().Scan(&out.ClosingWholesalePrepaymentMoney); e != nil {
			return e
		}
		return nil
	})
	return out, e
}
