package commerce

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"math/big"
)

// InvoiceReceiptBasis identifies the original external receipt behind completed
// customer orders. It does not create invoice capacity for an internal wallet
// debit; downstream funding provenance must resolve back to the same receipt.
type InvoiceReceiptBasis struct {
	ReceiptID        string          `json:"receipt_id"`
	CustomerID       string          `json:"customer_id"`
	AllocatedMoney   ledger.Decimal  `json:"allocated_money"`
	GrossMoney       ledger.Decimal  `json:"gross_money"`
	TopUpMoney       ledger.Decimal  `json:"top_up_money"`
	PurchaseMoney    ledger.Decimal  `json:"purchase_money"`
	ConsumptionMoney ledger.Decimal  `json:"consumption_money"`
	Evidence         json.RawMessage `json:"evidence"`
}

func (s *Service) InvoiceReceiptBasis(ctx context.Context, tx *gorm.DB, operator, customer, receipt string) (InvoiceReceiptBasis, error) {
	out := InvoiceReceiptBasis{ReceiptID: receipt, CustomerID: customer}
	if !validID(operator) || !validID(customer) || !validID(receipt) {
		return out, ErrInvalidInput
	}
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		var r receiptRow
		if e := t.First(&r, "id=? AND operator_id=?", receipt, operator).Error; e != nil {
			return e
		}
		out.GrossMoney = r.GrossMoney
		if e := t.Raw(`SELECT COALESCE(sum(a.amount_money),0)::text FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.operator_id=? AND a.receipt_id=? AND o.customer_id=? AND o.status='completed'`, operator, receipt, customer).Row().Scan(&out.AllocatedMoney); e != nil {
			return e
		}
		if out.AllocatedMoney.Sign() <= 0 {
			return ErrInvalidInput
		}
		var topup ledger.Decimal
		if e := t.Raw(`SELECT COALESCE(sum(a.amount_money),0)::text FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.operator_id=? AND a.receipt_id=? AND o.customer_id=? AND o.kind='topup' AND o.status='completed'`, operator, receipt, customer).Row().Scan(&topup); e != nil {
			return e
		}
		out.TopUpMoney = topup
		var funded []struct {
			OrderID     string
			Amount, Due ledger.Decimal
		}
		if e := t.Raw(`SELECT o.id::text AS order_id,a.amount_money::text AS amount,o.due_money::text AS due FROM commerce_receipt_allocations a JOIN commerce_orders o ON o.id=a.order_id WHERE a.operator_id=? AND a.receipt_id=? AND o.customer_id=? AND o.status='completed' AND o.kind IN ('package','renewal')
 UNION ALL SELECT o.id::text,(a.amount_money-a.restored_money)::text,o.due_money::text FROM commerce_wallet_funding_allocations a JOIN commerce_wallet_funding_commands c ON c.id=a.command_id JOIN commerce_wallet_funding_lots l ON l.id=a.lot_id JOIN commerce_orders o ON o.id=c.reference_id WHERE l.source_operator_id=? AND l.receipt_id=? AND l.customer_id=? AND c.kind='order' AND a.status='consumed' AND o.status='completed' AND o.kind IN ('package','renewal')`, operator, receipt, customer, operator, receipt, customer).Scan(&funded).Error; e != nil {
			return e
		}
		if len(funded) > 10000 {
			return ErrInvalidInput
		}
		funding := map[string]ledger.Decimal{}
		dues := map[string]ledger.Decimal{}
		ids := []string{}
		for _, f := range funded {
			if _, ok := funding[f.OrderID]; !ok {
				ids = append(ids, f.OrderID)
			}
			funding[f.OrderID] = funding[f.OrderID].Add(f.Amount)
			dues[f.OrderID] = f.Due
			out.PurchaseMoney = out.PurchaseMoney.Add(f.Amount)
		}
		consumed, e := s.ledger.PaidConsumptionByOrder(ctx, t, customer, ids)
		if e != nil {
			return e
		}
		exactConsumed := new(big.Rat)
		for orderID, f := range funding {
			due := dues[orderID]
			if due.Sign() <= 0 || f.Sign() < 0 || f.Cmp(due) > 0 {
				return ErrConflict
			}
			part := new(big.Rat).Mul(consumed[orderID].Rat(), new(big.Rat).Quo(f.Rat(), due.Rat()))
			exactConsumed.Add(exactConsumed, part)
		}
		out.ConsumptionMoney, _ = ledger.Quantize(exactConsumed)
		out.Evidence, _ = json.Marshal(map[string]any{"receipt_id": r.ID, "allocated_to_completed_orders_money": out.AllocatedMoney, "customer_id": customer})
		return nil
	})
	return out, e
}

// EventAudience resolves only customer-facing aggregate ownership. It never
// exposes receipt verification evidence or grants the supplier customer access.
func (s *Service) EventAudience(ctx context.Context, tx *gorm.DB, e Event) (customer, reference string, err error) {
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		switch e.Kind {
		case "manual_gift.delivered":
			var gift ManualGift
			if err := t.First(&gift, "id=? AND original_operator_id=?", e.AggregateID, e.OperatorID).Error; err != nil {
				return err
			}
			customer, reference = gift.CustomerID, "gift:"+gift.ID
		case "order.created", "order.closed", "order.receipt_allocated", "order.delivered":
			var o orderRow
			if e := t.First(&o, "id=? AND operator_id=?", e.AggregateID, e.OperatorID).Error; e != nil {
				return e
			}
			customer = o.CustomerID
			reference = "order:" + o.ID
		case "renewal.authorized", "renewal.cancelled", "renewal.reconfirmed", "renewal.confirmation_required", "renewal.insufficient_funds":
			var r subscriptionRow
			if e := t.First(&r, "id=? AND operator_id=?", e.AggregateID, e.OperatorID).Error; e != nil {
				return e
			}
			customer = r.CustomerID
			reference = "subscription:" + r.ID
		}
		return nil
	})
	return
}
