package commerce

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type ReceiptWalletExposure struct {
	OperatorID, CustomerID, ReceiptID string
	AvailableUSD                      ledger.Decimal
}
type ReceiptOrderExposure struct {
	OperatorID, CustomerID, OrderID string
	DueUSD, FundingUSD              ledger.Decimal
}
type ReceiptExposure struct {
	Wallets []ReceiptWalletExposure
	Orders  []ReceiptOrderExposure
}

// ReceiptExposure follows only proven receipt allocations, including the
// original topup lots spent by later orders. It never infers funding from a
// customer's unrelated balance or from a matching email/name.
func (s *Service) ReceiptExposure(ctx context.Context, tx *gorm.DB, sourceOperator, receipt string) (ReceiptExposure, error) {
	var out ReceiptExposure
	t := s.dbFor(ctx, tx)
	if _, e := s.GetReceipt(ctx, tx, sourceOperator, receipt); e != nil {
		return out, e
	}
	e := t.Raw(`SELECT operator_id,customer_id,receipt_id,sum(amount_usd-reserved_usd-spent_usd)::text AS available_usd FROM commerce_wallet_funding_lots WHERE source_operator_id=? AND receipt_id=? GROUP BY operator_id,customer_id,receipt_id`, sourceOperator, receipt).Scan(&out.Wallets).Error
	if e != nil {
		return out, e
	}
	e = t.Raw(`SELECT o.operator_id,o.customer_id,o.id AS order_id,o.due_usd,sum(x.amount_usd)::text AS funding_usd FROM (
 SELECT a.order_id,a.amount_usd FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.operator_id=? AND a.receipt_id=? AND o.kind<>'topup'
 UNION ALL SELECT c.reference_id AS order_id,a.amount_usd-a.restored_usd AS amount_usd FROM commerce_wallet_funding_allocations a JOIN commerce_wallet_funding_commands c ON c.id=a.command_id JOIN commerce_wallet_funding_lots l ON l.id=a.lot_id WHERE l.source_operator_id=? AND l.receipt_id=? AND c.kind='order' AND a.status='consumed'
 ) x JOIN commerce_orders o ON o.id=x.order_id WHERE o.status='completed' GROUP BY o.operator_id,o.customer_id,o.id,o.due_usd ORDER BY o.id`, sourceOperator, receipt, sourceOperator, receipt).Scan(&out.Orders).Error
	return out, e
}
