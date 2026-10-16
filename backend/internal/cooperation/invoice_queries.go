package cooperation

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"math/big"
)

type InvoiceReceiptBasis struct {
	ReceiptID, BuyerID                                     string
	AllocatedUSD, PurchaseUSD, ConsumptionUSD, RefundedUSD ledger.Decimal
}

// InvoiceReceiptBasis proves a buyer's real completed purchase allocations.
// Procurement gifts create no cash invoice value; consumed paid inventory is
// attributed proportionally back to the original actual payment sources.
func (s *Service) InvoiceReceiptBasis(ctx context.Context, tx *gorm.DB, supplier, buyer, receipt string) (InvoiceReceiptBasis, error) {
	out := InvoiceReceiptBasis{ReceiptID: receipt, BuyerID: buyer}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		var rows []struct{ Amount, Due, PaidUSD, PaidATG, PaidConsumed ledger.Decimal }
		if e := t.Raw(`SELECT a.amount_usd::text AS amount,o.due_usd::text AS due,l.paid_usd::text,l.paid_atg::text,l.paid_consumed::text FROM cooperation_purchase_allocations a JOIN cooperation_purchase_orders o ON o.id=a.order_id JOIN cooperation_procurement_lots l ON l.order_id=o.id::text WHERE o.supplier_id=? AND o.buyer_id=? AND a.receipt_id=? AND o.status='completed'`, supplier, buyer, receipt).Scan(&rows).Error; e != nil {
			return e
		}
		if len(rows) == 0 || len(rows) > 10000 {
			return ErrConflict
		}
		consumed := new(big.Rat)
		for _, r := range rows {
			if r.Due.Sign() <= 0 || r.PaidATG.Sign() <= 0 {
				return ErrConflict
			}
			out.AllocatedUSD = out.AllocatedUSD.Add(r.Amount)
			part := new(big.Rat).Mul(r.PaidUSD.Rat(), new(big.Rat).Quo(r.PaidConsumed.Rat(), r.PaidATG.Rat()))
			part.Mul(part, new(big.Rat).Quo(r.Amount.Rat(), r.Due.Rat()))
			consumed.Add(consumed, part)
		}
		out.PurchaseUSD = out.AllocatedUSD
		out.ConsumptionUSD, _ = ledger.Quantize(consumed)
		return t.Model(&ProcurementExitRefund{}).Where("supplier_id=? AND buyer_id=? AND receipt_id=?", supplier, buyer, receipt).Select("COALESCE(sum(settled_usd),0)::text").Row().Scan(&out.RefundedUSD)
	})
	return out, e
}
