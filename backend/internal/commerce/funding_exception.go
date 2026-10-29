package commerce

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) LockReceipt(ctx context.Context, tx *gorm.DB, operator, receipt string) (Receipt, error) {
	if tx == nil || !validID(operator) || !validID(receipt) {
		return Receipt{}, ErrInvalidInput
	}
	var r receiptRow
	e := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=? AND operator_id=?", receipt, operator).Error
	return Receipt(r), classify(e)
}

// MarkFundingException records a verified loss of already allocated payment.
// The original receipt and allocation remain facts; no unpaid amount is forged.
func (s *Service) MarkFundingException(ctx context.Context, tx *gorm.DB, operator, customer, orderID, evidence string) error {
	if tx == nil || !validID(operator) || !validID(customer) || !validID(orderID) || evidence == "" {
		return ErrInvalidInput
	}
	var o orderRow
	if e := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, "id=? AND operator_id=? AND customer_id=?", orderID, operator, customer).Error; e != nil {
		return classify(e)
	}
	if o.Status == "completed" {
		return ErrOrderState
	}
	if e := tx.Model(&o).Updates(map[string]any{"status": "exception", "exception_reason": "verified allocated payment loss: " + evidence}).Error; e != nil {
		return e
	}
	return s.event(tx, operator, orderID, "order.payment_funding_exception", map[string]any{"evidence": evidence, "original_external_allocated_money": o.ExternalAllocatedMoney})
}
