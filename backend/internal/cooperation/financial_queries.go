package cooperation

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type FinancialSnapshot struct {
	PurchaseOrders                 int64          `json:"purchase_orders"`
	PurchasesUSD                   ledger.Decimal `json:"purchases_usd"`
	WholesaleOrders                int64          `json:"wholesale_orders"`
	WholesaleSalesUSD              ledger.Decimal `json:"wholesale_sales_usd"`
	ProcurementCostUSD             ledger.Decimal `json:"procurement_cost_usd"`
	ProcurementCorrectionUSD       ledger.Decimal `json:"procurement_correction_usd"`
	WholesaleEarnedUSD             ledger.Decimal `json:"wholesale_earned_usd"`
	WholesaleCorrectionUSD         ledger.Decimal `json:"wholesale_correction_usd"`
	ClosingInventoryCostUSD        ledger.Decimal `json:"closing_inventory_cost_usd"`
	PendingRefundFees              int64          `json:"pending_refund_fees"`
	PendingRefundExecutions        int64          `json:"pending_refund_executions"`
	RefundMismatchCount            int64          `json:"refund_mismatch_count"`
	OutstandingRefundUSD           ledger.Decimal `json:"outstanding_refund_usd"`
	VerifiedProcurementPaymentsUSD ledger.Decimal `json:"verified_procurement_payments_usd"`
	ClosingProcurementPaymentsUSD  ledger.Decimal `json:"closing_procurement_payments_usd"`
	ClosingWholesalePrepaymentUSD  ledger.Decimal `json:"closing_wholesale_prepayment_usd"`
	ProcurementExitAdjustmentUSD   ledger.Decimal `json:"procurement_exit_adjustment_usd"`
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
		}{{"buyer_id=?", &out.PurchaseOrders, &out.PurchasesUSD}, {"supplier_id=?", &out.WholesaleOrders, &out.WholesaleSalesUSD}} {
			var r struct {
				Count  int64
				Amount ledger.Decimal
			}
			if e := t.Model(&PurchaseOrder{}).Where(q.Where, operator).Where("status='completed' AND completed_at>=? AND completed_at<?", from, to).Select("count(*) AS count,COALESCE(sum(due_usd),0)::text AS amount").Scan(&r).Error; e != nil {
				return e
			}
			*q.Count = r.Count
			*q.Amount = r.Amount
		}
		for _, q := range []struct {
			SQL    string
			Target *ledger.Decimal
		}{
			{`SELECT COALESCE(sum(cost_usd),0)::text FROM cooperation_procurement_settlements WHERE buyer_id=? AND created_at>=? AND created_at<?`, &out.ProcurementCostUSD},
			{`SELECT COALESCE(sum(cost_usd),0)::text FROM cooperation_procurement_reversals WHERE buyer_id=? AND created_at>=? AND created_at<?`, &out.ProcurementCorrectionUSD},
			{`SELECT COALESCE(sum(c.cost_usd),0)::text FROM cooperation_procurement_settlements c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at>=? AND c.created_at<?`, &out.WholesaleEarnedUSD},
			{`SELECT COALESCE(sum(c.cost_usd),0)::text FROM cooperation_procurement_reversals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at>=? AND c.created_at<?`, &out.WholesaleCorrectionUSD},
			{`SELECT COALESCE(sum(a.amount_usd),0)::text FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id WHERE o.buyer_id=? AND a.created_at>=? AND a.created_at<?`, &out.VerifiedProcurementPaymentsUSD},
		} {
			if e := t.Raw(q.SQL, operator, from, to).Row().Scan(q.Target); e != nil {
				return e
			}
		}
		if e := t.Raw(`SELECT (COALESCE((SELECT sum(paid_usd) FROM cooperation_procurement_lots WHERE buyer_id=? AND delivered_at<?),0)-COALESCE((SELECT sum(cost_usd) FROM cooperation_procurement_settlements WHERE buyer_id=? AND created_at<?),0)+COALESCE((SELECT sum(cost_usd) FROM cooperation_procurement_reversals WHERE buyer_id=? AND created_at<?),0)-COALESCE((SELECT sum(book_cost_usd) FROM cooperation_procurement_removals WHERE buyer_id=? AND created_at<?),0))::text`, operator, to, operator, to, operator, to, operator, to).Row().Scan(&out.ClosingInventoryCostUSD); e != nil {
			return e
		}
		if e := t.Raw(`SELECT COALESCE(sum(a.amount_usd),0)::text FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id WHERE o.buyer_id=? AND a.created_at<?`, operator, to).Row().Scan(&out.ClosingProcurementPaymentsUSD); e != nil {
			return e
		}
		if e := t.Raw(`SELECT (COALESCE((SELECT sum(l.paid_usd) FROM cooperation_procurement_lots l JOIN cooperation_procurement_positions p ON p.buyer_id=l.buyer_id WHERE p.supplier_id=? AND l.delivered_at<?),0)-COALESCE((SELECT sum(c.cost_usd) FROM cooperation_procurement_settlements c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0)+COALESCE((SELECT sum(c.cost_usd) FROM cooperation_procurement_reversals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0)-COALESCE((SELECT sum(c.book_cost_usd) FROM cooperation_procurement_removals c JOIN cooperation_procurement_positions p ON p.buyer_id=c.buyer_id WHERE p.supplier_id=? AND c.created_at<?),0))::text`, operator, to, operator, to, operator, to, operator, to).Row().Scan(&out.ClosingWholesalePrepaymentUSD); e != nil {
			return e
		}
		if e := t.Model(&ProcurementExit{}).Where("buyer_id=? AND created_at>=? AND created_at<?", operator, from, to).Select("COALESCE(sum(cost_difference_usd),0)::text").Row().Scan(&out.ProcurementExitAdjustmentUSD); e != nil {
			return e
		}
		if e := t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND actual_usd>0 AND fee_status='pending'", operator).Count(&out.PendingRefundFees).Error; e != nil {
			return e
		}
		if e := t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND state IN ('executing','unknown','manual_executing')", operator).Count(&out.PendingRefundExecutions).Error; e != nil {
			return e
		}
		if e := t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND state='exception'", operator).Count(&out.RefundMismatchCount).Error; e != nil {
			return e
		}
		return t.Model(&ProcurementExit{}).Where("supplier_id=? AND state IN ('awaiting_refund','refund_exception')", operator).Select("COALESCE(sum(contract_usd-refunded_usd),0)::text").Row().Scan(&out.OutstandingRefundUSD)
	})
	return out, e
}
