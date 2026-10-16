package operations

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Replacements retain the original voided legal document and source facts.
// The new request obtains fresh remaining source capacity under current policy.
func (s *Service) RequestInvoiceReplacement(ctx context.Context, customer, operator, originalID, key string, lines []InvoiceLine, details InvoiceDetails) (Invoice, error) {
	return s.invoiceReplacement(ctx, customer, operator, "", originalID, key, lines, details)
}
func (s *Service) RequestProcurementInvoiceReplacement(ctx context.Context, actor, buyer, originalID, key string, lines []InvoiceLine, details InvoiceDetails) (Invoice, error) {
	return s.invoiceReplacement(ctx, actor, "", buyer, originalID, key, lines, details)
}
func (s *Service) invoiceReplacement(ctx context.Context, actor, operator, buyer, originalID, key string, lines []InvoiceLine, details InvoiceDetails) (Invoice, error) {
	var out Invoice
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		var original Invoice
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&original, "id=?", originalID).Error; e != nil {
			return e
		}
		if original.State != "voided" {
			return ErrConflict
		}
		if buyer != "" {
			if original.PartyKind != "procurement" || original.BuyerOperatorID == nil || *original.BuyerOperatorID != buyer {
				return ErrForbidden
			}
			if e := s.require(ctx, tx, actor, buyer, "cooperation.manage"); e != nil {
				return e
			}
		} else if original.PartyKind != "customer" || original.CustomerID != actor || original.OperatorID != operator {
			return ErrForbidden
		}
		var previous []InvoiceLine
		if e := tx.Table("operations_invoice_sources").Where("invoice_id=?", originalID).Find(&previous).Error; e != nil {
			return e
		}
		allowed := map[string]bool{}
		for _, line := range previous {
			allowed[line.ReceiptID] = true
		}
		for _, line := range lines {
			if !allowed[line.ReceiptID] {
				return ErrInvalid
			}
		}
		copy := *s
		copy.DB = tx
		party := original.CustomerID
		if buyer != "" {
			party = buyer
		}
		var e error
		out, e = copy.requestInvoice(ctx, original.PartyKind, party, original.OperatorID, key, lines, details, func(*gorm.DB) error { return nil })
		if e != nil {
			return e
		}
		if out.ReplacesInvoiceID != nil && *out.ReplacesInvoiceID != originalID {
			return ErrConflict
		}
		if e = tx.Model(&out).Update("replaces_invoice_id", originalID).Error; e != nil {
			return e
		}
		out.ReplacesInvoiceID = &originalID
		return nil
	})
	return out, e
}
