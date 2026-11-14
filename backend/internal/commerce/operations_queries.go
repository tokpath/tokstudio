package commerce

import (
	"context"
	"gorm.io/gorm"
)

// EventAudience resolves only customer-facing aggregate ownership. It never
// exposes receipt verification evidence or grants the supplier customer access.
func (s *Service) EventAudience(ctx context.Context, tx *gorm.DB, e Event) (customer, operator, reference string, err error) {
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
			if e := t.First(&o, "id=? AND (operator_id=? OR original_operator_id=?)", e.AggregateID, e.OperatorID, e.OperatorID).Error; e != nil {
				return e
			}
			customer = o.CustomerID
			reference = "order:" + o.ID
		case "renewal.authorized", "renewal.cancelled", "renewal.reconfirmed", "renewal.confirmation_required", "renewal.insufficient_funds":
			var r subscriptionRow
			if e := t.First(&r, "id=? AND (operator_id=? OR original_operator_id=?)", e.AggregateID, e.OperatorID, e.OperatorID).Error; e != nil {
				return e
			}
			customer = r.CustomerID
			reference = "subscription:" + r.ID
		}
		if customer == "" {
			return nil
		}
		account, err := s.org.GetCustomer(ctx, t, customer)
		if err != nil {
			return err
		}
		if account.Status == "closed" || account.Status == "anonymized" {
			customer = ""
			return nil
		}
		// A delayed payment prompt may arrive after the order has finished.
		// Keep the order link, but avoid giving a stale payment instruction.
		own, e := s.org.CurrentOwnership(ctx, t, customer)
		if e != nil {
			return e
		}
		operator = own.OperatorID
		return nil
	})
	return
}
