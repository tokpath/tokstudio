package operations

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// Receipt adjustments remain a projection of external payment facts. They do
// not determine invoice capacity for later purchases funded from a wallet.
type ReceiptAdjustment struct {
	ReceiptID      string         `json:"receipt_id"`
	RemovedMoney   ledger.Decimal `json:"removed_money"`
	ReviewRequired bool           `json:"review_required"`
	Reference      string         `json:"reference"`
}
type ReceiptAdjustments interface {
	InvoiceReceiptAdjustments(context.Context, *gorm.DB, string, []string) ([]ReceiptAdjustment, error)
}
type PartyReceiptAdjustments interface {
	InvoicePartyReceiptAdjustments(context.Context, *gorm.DB, string, string, []string) ([]ReceiptAdjustment, error)
}
type OrderInvoiceAdjustments interface {
	InvoiceOrderReview(context.Context, *gorm.DB, string, string, string) (bool, error)
}
type CommerceInvoiceSource struct {
	Commerce    *commerce.Service
	Adjustments OrderInvoiceAdjustments
}

func (d CommerceInvoiceSource) InvoiceBasis(ctx context.Context, tx *gorm.DB, operator, customer, order string) (InvoiceBasis, error) {
	out := InvoiceBasis{OrderID: order}
	if d.Commerce == nil || d.Adjustments == nil {
		return out, ErrNotReady
	}
	purchase, e := d.Commerce.InvoicePurchase(ctx, tx, operator, customer, order)
	if e != nil {
		return out, e
	}
	out.EligibleMoney = purchase.DueMoney
	out.ReviewRequired = purchase.Status != "completed"
	review, e := d.Adjustments.InvoiceOrderReview(ctx, tx, operator, "customer", order)
	out.ReviewRequired = out.ReviewRequired || review
	return out, e
}
