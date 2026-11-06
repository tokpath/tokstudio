package cooperation

import (
	"context"
	"gorm.io/gorm"
)

// InvoicePurchase is the completed supply order's immutable paid value.
func (s *Service) InvoicePurchase(ctx context.Context, tx *gorm.DB, supplier, buyer, order string) (PurchaseOrder, error) {
	var out PurchaseOrder
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("id=? AND supplier_id=? AND buyer_id=?", order, supplier, buyer).Take(&out).Error
	})
	return out, e
}
