package operations

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"gorm.io/gorm"
)

type ProcurementInvoiceSource interface {
	ProcurementInvoiceBasis(context.Context, *gorm.DB, string, string, string) (InvoiceBasis, error)
}
type DomainProcurementInvoiceSource struct {
	Cooperation *cooperation.Service
	Commerce    *commerce.Service
	Adjustments OrderInvoiceAdjustments
}

func (d DomainProcurementInvoiceSource) ProcurementInvoiceBasis(ctx context.Context, tx *gorm.DB, supplier, buyer, order string) (InvoiceBasis, error) {
	out := InvoiceBasis{OrderID: order}
	if d.Cooperation == nil || d.Adjustments == nil {
		return out, ErrNotReady
	}
	purchase, e := d.Cooperation.InvoicePurchase(ctx, tx, supplier, buyer, order)
	if e != nil {
		return out, e
	}
	out.EligibleMoney = purchase.DueMoney
	out.ReviewRequired = purchase.Status != "completed"
	review, e := d.Adjustments.InvoiceOrderReview(ctx, tx, supplier, "procurement", order)
	out.ReviewRequired = out.ReviewRequired || review
	return out, e
}
func (s *Service) invoiceBasis(ctx context.Context, tx *gorm.DB, i Invoice, receipt string) (InvoiceBasis, error) {
	if i.PartyKind == "procurement" {
		if s.ProcurementInvoices == nil || i.BuyerOperatorID == nil {
			return InvoiceBasis{}, ErrNotReady
		}
		return s.ProcurementInvoices.ProcurementInvoiceBasis(ctx, tx, i.OperatorID, *i.BuyerOperatorID, receipt)
	}
	if s.Invoices == nil {
		return InvoiceBasis{}, ErrNotReady
	}
	return s.Invoices.InvoiceBasis(ctx, tx, i.OperatorID, i.CustomerID, receipt)
}
func (s *Service) RequestProcurementInvoice(ctx context.Context, actor, buyer, supplier, key string, lines []InvoiceLine, details InvoiceDetails) (Invoice, error) {
	if !validID(buyer) || !validID(supplier) || buyer == supplier {
		return Invoice{}, ErrInvalid
	}
	return s.requestInvoice(ctx, "procurement", buyer, supplier, key, lines, details, func(tx *gorm.DB) error { return s.require(ctx, tx, actor, buyer, "cooperation.manage") })
}
func (s *Service) ListProcurementInvoices(ctx context.Context, actor, buyer string) ([]Invoice, error) {
	out := []Invoice{}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		return tx.Where("party_kind='procurement' AND buyer_operator_id=?", buyer).Order("created_at DESC,id").Limit(100).Find(&out).Error
	})
	return out, e
}
