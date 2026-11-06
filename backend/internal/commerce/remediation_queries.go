package commerce

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type ReceiptWalletExposure struct {
	OperatorID, CustomerID, ReceiptID string
	CreditedMoney                     ledger.Decimal
}
type ReceiptOrderExposure struct {
	OperatorID, CustomerID, OrderID string
	DueMoney, FundingMoney          ledger.Decimal
}
type ReceiptExposure struct {
	Wallets []ReceiptWalletExposure
	Orders  []ReceiptOrderExposure
}

// ReceiptExposure links a payment only to the transaction it actually paid.
// Top-ups identify the credited wallet; later purchases are independent sales.
func (s *Service) ReceiptExposure(ctx context.Context, tx *gorm.DB, sourceOperator, receipt string) (ReceiptExposure, error) {
	var out ReceiptExposure
	t := s.dbFor(ctx, tx)
	if _, e := s.GetReceipt(ctx, tx, sourceOperator, receipt); e != nil {
		return out, e
	}
	e := t.Raw(`SELECT o.operator_id,o.customer_id,a.receipt_id,sum(a.amount_money)::text AS credited_money
 FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id
 WHERE a.operator_id=? AND a.receipt_id=? AND o.kind='topup' AND o.status='completed'
 GROUP BY o.operator_id,o.customer_id,a.receipt_id`, sourceOperator, receipt).Scan(&out.Wallets).Error
	if e != nil {
		return out, e
	}
	e = t.Raw(`SELECT o.operator_id,o.customer_id,o.id AS order_id,o.due_money,sum(a.amount_money)::text AS funding_money
 FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id
 WHERE a.operator_id=? AND a.receipt_id=? AND o.kind IN ('package','renewal') AND o.status='completed'
 GROUP BY o.operator_id,o.customer_id,o.id,o.due_money ORDER BY o.id`, sourceOperator, receipt).Scan(&out.Orders).Error
	return out, e
}
