package operations

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"gorm.io/gorm"
)

// ListCustomerInvoices is an authenticated self-query. The caller supplies the
// stable customer identity from its verified session, never from request data.
// A change of service merchant does not transfer the original legal issuer.
func (s *Service) ListCustomerInvoices(ctx context.Context, customer, cursor string, limit int) ([]Invoice, string, error) {
	if !validID(customer) || limit < 1 || limit > 100 {
		return nil, "", ErrInvalid
	}
	var c notificationCursor
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || json.Unmarshal(raw, &c) != nil || c.At.IsZero() || !validID(c.ID) {
			return nil, "", ErrInvalid
		}
	}
	rows := []Invoice{}
	e := s.within(ctx, nil, func(tx *gorm.DB) error {
		q := tx.Where("party_kind='customer' AND customer_id=?", customer)
		if cursor != "" {
			q = q.Where("(created_at,id)<(?,?)", c.At, c.ID)
		}
		return q.Order("created_at DESC,id DESC").Limit(limit + 1).Find(&rows).Error
	})
	if e != nil {
		return nil, "", e
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		raw, _ := json.Marshal(notificationCursor{At: last.CreatedAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	return rows, next, nil
}

// RequestHistoricalInvoice proves the original paid customer/issuer relationship
// using immutable source allocations. It does not grant access through a new
// merchant's directory or expose another customer's original receipt details.
// The customer identity must come from the authenticated session.
func (s *Service) RequestHistoricalInvoice(ctx context.Context, customer, issuer, key string, lines []InvoiceLine, details InvoiceDetails) (Invoice, error) {
	if !validID(customer) || !validID(issuer) {
		return Invoice{}, ErrInvalid
	}
	return s.requestInvoice(ctx, "customer", customer, issuer, key, lines, details, func(tx *gorm.DB) error {
		for _, line := range lines {
			basis, e := s.Invoices.InvoiceBasis(ctx, tx, issuer, customer, line.ReceiptID)
			if e != nil {
				return e
			}
			if basis.ReceiptID != line.ReceiptID || basis.EligibleUSD.Sign() <= 0 {
				return ErrForbidden
			}
		}
		return nil
	})
}
