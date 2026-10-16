package cooperation

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

type PaymentObligationReader interface {
	PendingReceiptObligations(context.Context, *gorm.DB, string, []string) ([]payments.PendingReceiptObligation, error)
	RecordedReceiptLinks(context.Context, *gorm.DB, string, []string) ([]payments.ReceiptLink, error)
	UnsettledReceiptFunds(context.Context, *gorm.DB, string, []string) ([]commerce.ReceiptFundObligation, error)
}

// ProcurementBuyerForPayment resolves the immutable supplier/order boundary,
// including automatically replenished and closed historical purchases.
func (s *Service) ProcurementBuyerForPayment(ctx context.Context, tx *gorm.DB, supplier, orderID string) (string, error) {
	if tx == nil || supplier == "" || orderID == "" {
		return "", ErrConflict
	}
	var row PurchaseOrder
	if e := tx.WithContext(ctx).Select("buyer_id").Where("supplier_id=? AND id=?", supplier, orderID).Take(&row).Error; e != nil {
		return "", e
	}
	return row.BuyerID, nil
}

// RequireNoPendingProcurementReceipts checks both money collected by the buyer
// and its own procurement payments held by suppliers. Closed purchase orders
// remain included because actual collected money cannot disappear with status.
func (s *Service) RequireNoPendingProcurementReceipts(ctx context.Context, tx *gorm.DB, buyer string, source PaymentObligationReader) error {
	if tx == nil || source == nil {
		return ErrLiability
	}
	own, e := source.PendingReceiptObligations(ctx, tx, buyer, nil)
	if e != nil {
		return e
	}
	if len(own) > 0 {
		return ErrLiability
	}
	ownFunds, e := source.UnsettledReceiptFunds(ctx, tx, buyer, nil)
	if e != nil {
		return e
	}
	if len(ownFunds) > 0 {
		return ErrLiability
	}
	var orders []struct{ ID, SupplierID string }
	if e := tx.WithContext(ctx).Model(&PurchaseOrder{}).Select("id,supplier_id").Where("buyer_id=?", buyer).Order("supplier_id,id").Find(&orders).Error; e != nil {
		return e
	}
	groups := map[string][]string{}
	for _, o := range orders {
		groups[o.SupplierID] = append(groups[o.SupplierID], o.ID)
	}
	for supplier, ids := range groups {
		for len(ids) > 0 {
			n := len(ids)
			if n > 1000 {
				n = 1000
			}
			facts, e := source.PendingReceiptObligations(ctx, tx, supplier, ids[:n])
			if e != nil {
				return e
			}
			if len(facts) > 0 {
				return ErrLiability
			}
			links, e := source.RecordedReceiptLinks(ctx, tx, supplier, ids[:n])
			if e != nil {
				return e
			}
			receipts := []string{}
			for _, link := range links {
				receipts = append(receipts, link.ReceiptID)
			}
			unsettled, e := source.UnsettledReceiptFunds(ctx, tx, supplier, receipts)
			if e != nil {
				return e
			}
			if len(unsettled) > 0 {
				return ErrLiability
			}
			ids = ids[n:]
		}
	}
	return nil
}
