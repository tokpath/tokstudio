package operations

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type ProcurementInvoiceSource interface {
	ProcurementInvoiceBasis(context.Context, *gorm.DB, string, string, string) (InvoiceBasis, error)
}
type DomainProcurementInvoiceSource struct {
	Cooperation *cooperation.Service
	Commerce    *commerce.Service
	Adjustments ReceiptAdjustments
}

func (d DomainProcurementInvoiceSource) ProcurementInvoiceBasis(ctx context.Context, tx *gorm.DB, supplier, buyer, receipt string) (InvoiceBasis, error) {
	out := InvoiceBasis{ReceiptID: receipt}
	if d.Cooperation == nil || d.Commerce == nil || d.Adjustments == nil {
		return out, ErrNotReady
	}
	b, e := d.Cooperation.InvoiceReceiptBasis(ctx, tx, supplier, buyer, receipt)
	if e != nil {
		return out, e
	}
	r, e := d.Commerce.GetReceipt(ctx, tx, supplier, receipt)
	if e != nil {
		return out, e
	}
	adjustments, e := d.Adjustments.InvoiceReceiptAdjustments(ctx, tx, supplier, []string{receipt})
	if e != nil {
		return out, e
	}
	if len(adjustments) != 1 || adjustments[0].ReceiptID != receipt {
		return out, ErrNotReady
	}
	a := adjustments[0]
	global := r.GrossUSD.Sub(a.RemovedUSD)
	if global.Sign() < 0 {
		global = ledger.Zero()
	}
	out.GlobalEligibleUSD = &global
	out.EligibleUSD = b.AllocatedUSD.Sub(b.RefundedUSD)
	if out.EligibleUSD.Sign() < 0 {
		out.EligibleUSD = ledger.Zero()
	}
	out.ReceiptUSD = &out.EligibleUSD
	out.PurchaseUSD = b.PurchaseUSD.Sub(b.RefundedUSD)
	if out.PurchaseUSD.Sign() < 0 {
		out.PurchaseUSD = ledger.Zero()
	}
	out.ConsumptionUSD = b.ConsumptionUSD
	if out.ConsumptionUSD.Cmp(out.EligibleUSD) > 0 {
		out.ConsumptionUSD = out.EligibleUSD
	}
	out.ReviewRequired = a.ReviewRequired
	out.AdjustmentReference = a.Reference
	raw, _ := json.Marshal(map[string]any{"procurement": b, "adjustment": a})
	out.Evidence = string(raw)
	return out, nil
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

func (s *Service) GetProcurementInvoicePolicy(ctx context.Context, actor, buyer, supplier string) (InvoicePolicy, error) {
	out := InvoicePolicy{OperatorID: supplier, PartyKind: "procurement"}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		if e := s.require(ctx, tx, actor, buyer, "cooperation.manage"); e != nil {
			return e
		}
		return tx.Where("operator_id=? AND party_kind='procurement'", supplier).Limit(1).Find(&out).Error
	})
	return out, e
}
