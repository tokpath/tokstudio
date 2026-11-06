package operations

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"testing"
)

type scopedInvoiceSource struct{ Customer, Operator, Receipt string }

func (b scopedInvoiceSource) InvoiceBasis(_ context.Context, _ *gorm.DB, operator, customer, receipt string) (InvoiceBasis, error) {
	if customer != b.Customer || operator != b.Operator || receipt != b.Receipt {
		return InvoiceBasis{}, ErrForbidden
	}
	value := ledger.FromInt64(10)
	return InvoiceBasis{OrderID: receipt, EligibleMoney: value}, nil
}
func TestInvoiceHistoryAndInstructionsSurviveCurrentRelationshipChange(t *testing.T) {
	s, a, _, op, customer := opsFixture(t)
	ctx := context.Background()
	receipt := uuid.NewString()
	s.Invoices = scopedInvoiceSource{customer, op, receipt}
	if _, e := s.SaveInvoiceInstructions(ctx, a.actor, op, "Original instructions", 0); e != nil {
		t.Fatal(e)
	}
	i, e := s.RequestInvoice(ctx, customer, op, "initial", []InvoiceLine{{receipt, ledger.FromInt64(6)}}, InvoiceDetails{Title: "Original customer"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveInvoiceInstructions(ctx, a.actor, op, "New instructions", 1); e != nil {
		t.Fatal(e)
	}
	var saved Invoice
	if e = s.DB.First(&saved, "id=?", i.ID).Error; e != nil || saved.Instructions != "Original instructions" {
		t.Fatal("application instructions changed", saved, e)
	}
	s.Recipients = operationRecipients{operator: uuid.NewString(), customer: customer}
	if _, e = s.ListInvoices(ctx, customer, op, true); !errors.Is(e, ErrForbidden) {
		t.Fatalf("fixture did not remove current relationship: %v", e)
	}
	rows, _, e := s.ListCustomerInvoices(ctx, customer, "", 20)
	if e != nil || len(rows) != 1 || rows[0].ID != i.ID || rows[0].OperatorID != op {
		t.Fatalf("historical original issuer hidden: %+v %v", rows, e)
	}
	if _, e = s.RequestHistoricalInvoice(ctx, customer, op, "initial", []InvoiceLine{{receipt, ledger.FromInt64(6)}}, InvoiceDetails{Title: "Original customer"}); e != nil {
		t.Fatal("original paid relationship lost on current reassignment", e)
	}
	// A different uninvoiced purchase from the former operator remains eligible.
	secondOrder := uuid.NewString()
	s.Invoices = scopedInvoiceSource{customer, op, secondOrder}
	if _, err := s.RequestHistoricalInvoice(ctx, customer, op, "after-transfer", []InvoiceLine{{OrderID: secondOrder}}, InvoiceDetails{Title: "Original customer"}); err != nil {
		t.Fatal("historical purchase unavailable", err)
	}
	s.Invoices = scopedInvoiceSource{customer, op, receipt}
	other := uuid.NewString()
	if rows, _, e = s.ListCustomerInvoices(ctx, other, "", 20); e != nil || len(rows) != 0 {
		t.Fatal("another identity read original invoices", e)
	}
	if _, e = s.RequestHistoricalInvoice(ctx, other, op, "forged", []InvoiceLine{{receipt, ledger.FromInt64(1)}}, InvoiceDetails{Title: "Outsider"}); !errors.Is(e, ErrForbidden) {
		t.Fatalf("historical source skipped exact customer proof: %v", e)
	}
}
