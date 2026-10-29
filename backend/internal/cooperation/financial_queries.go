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
	PendingRefundFees                int64          `json:"pending_refund_fees"`
	PendingRefundExecutions          int64          `json:"pending_refund_executions"`
	RefundMismatchCount              int64          `json:"refund_mismatch_count"`
	OutstandingRefundMoney           ledger.Decimal `json:"outstanding_refund_money"`
	VerifiedProcurementPaymentsMoney ledger.Decimal `json:"verified_procurement_payments_money"`
	ClosingProcurementPaymentsMoney  ledger.Decimal `json:"closing_procurement_payments_money"`
	ClosingWholesalePrepaymentMoney  ledger.Decimal `json:"closing_wholesale_prepayment_money"`
	ProcurementExitAdjustmentMoney   ledger.Decimal `json:"procurement_exit_adjustment_money"`
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
			{`SELECT COALESCE(sum(a.amount_money),0)::text FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id WHERE o.buyer_id=? AND a.created_at>=? AND a.created_at<?`, &out.VerifiedProcurementPaymentsMoney},
		} {
			if e := t.Raw(q.SQL, operator, from, to).Row().Scan(q.Target); e != nil {
				return e
			}
		}
		if e := t.Raw(`SELECT (COALESCE((SELECT sum(paid_money) FROM cooperation_procurement_lots WHERE buyer_id=? AND delivered_at<?),0)-COALESCE((SELECT sum(cost_money) FROM cooperation_procurement_settlements WHERE buyer_id=? AND created_at<?),0)+COALESCE((SELECT sum(cost_money) FROM cooperation_procurement_reversals WHERE buyer_id=? AND created_at<?),0)-COALESCE((SELECT sum(book_cost_money) FROM cooperation_procurement_removals WHERE buyer_id=? AND created_at<?),0))::text`, operator, to, operator, to, operator, to, operator, to).Row().Scan(&out.ClosingInventoryCostMoney); e != nil {
			return e
		}
		if e := t.Raw(`SELECT COALESCE(sum(a.amount_money),0)::text FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id WHERE o.buyer_id=? AND a.created_at<?`, operator, to).Row().Scan(&out.ClosingProcurementPaymentsMoney); e != nil {
			return e
		}
		if e := t.Raw(`SELECT (COALESCE((SELECT sum(l.paid_money) FROM cooperation_procurement_lots l JOIN cooperation_procurement_positions p ON p.buyer_id=l.buyer_id WHERE p.supplier_id=? AND l.delivered_at<?),0)-COALESCE((SELECT sum(c.cost_money) FROM cooperation_procurement_settlements c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0)+COALESCE((SELECT sum(c.cost_money) FROM cooperation_procurement_reversals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0)-COALESCE((SELECT sum(c.book_cost_money) FROM cooperation_procurement_removals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0))::text`, operator, to, operator, to, operator, to, operator, to).Row().Scan(&out.ClosingWholesalePrepaymentMoney); e != nil {
			return e
		}
		if e := t.Model(&ProcurementExit{}).Where("buyer_id=? AND created_at>=? AND created_at<?", operator, from, to).Select("COALESCE(sum(cost_difference_money),0)::text").Row().Scan(&out.ProcurementExitAdjustmentMoney); e != nil {
			return e
		}
		if e := t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND actual_money>0 AND fee_status='pending'", operator).Count(&out.PendingRefundFees).Error; e != nil {
			return e
		}
		if e := t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND state IN ('executing','unknown','manual_executing')", operator).Count(&out.PendingRefundExecutions).Error; e != nil {
			return e
		}
		if e := t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND state='exception'", operator).Count(&out.RefundMismatchCount).Error; e != nil {
			return e
		}
		return t.Model(&ProcurementExit{}).Where("supplier_id=? AND state IN ('awaiting_refund','refund_exception')", operator).Select("COALESCE(sum(contract_money-refunded_money),0)::text").Row().Scan(&out.OutstandingRefundMoney)
	})
	return out, e
}
