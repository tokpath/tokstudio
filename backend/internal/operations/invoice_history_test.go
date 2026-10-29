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
	return InvoiceBasis{ReceiptID: receipt, EligibleMoney: value, GlobalEligibleMoney: &value, ReceiptMoney: &value}, nil
}
func TestInvoiceHistoryAndPolicyVersionsSurviveCurrentRelationshipChange(t *testing.T) {
	s, a, _, op, customer := opsFixture(t)
	ctx := context.Background()
	receipt := uuid.NewString()
	s.Invoices = scopedInvoiceSource{customer, op, receipt}
	p := InvoicePolicy{OperatorID: op, Enabled: true, Stage: "receipt", MaximumRatio: ledger.FromInt64(1), RulesReference: "original-public-rules"}
	if _, e := s.SaveInvoicePolicy(ctx, a.actor, p, 0); e != nil {
		t.Fatal(e)
	}
	i, e := s.RequestInvoice(ctx, customer, op, "initial", []InvoiceLine{{receipt, ledger.FromInt64(6)}}, InvoiceDetails{Title: "Original customer"})
	if e != nil {
		t.Fatal(e)
	}
	p.RulesReference = "new-public-rules"
	if _, e = s.SaveInvoicePolicy(ctx, a.actor, p, 1); e != nil {
		t.Fatal(e)
	}
	versions, e := s.ListInvoicePolicyVersions(ctx, a.actor, op, "customer")
	if e != nil || len(versions) != 2 || versions[1].RulesReference != "original-public-rules" || versions[1].Version != i.PolicyVersion {
		t.Fatalf("applied policy evidence overwritten: %+v %v", versions, e)
	}
	if e = s.DB.Table("operations_invoice_policy_versions").Where("operator_id=?", op).Delete(nil).Error; e == nil {
		t.Fatal("immutable published invoice policy removed")
	}
	s.Recipients = operationRecipients{operator: uuid.NewString(), customer: customer}
	if _, e = s.ListInvoices(ctx, customer, op, true); !errors.Is(e, ErrForbidden) {
		t.Fatalf("fixture did not remove current relationship: %v", e)
	}
	rows, _, e := s.ListCustomerInvoices(ctx, customer, "", 20)
	if e != nil || len(rows) != 1 || rows[0].ID != i.ID || rows[0].OperatorID != op {
		t.Fatalf("historical original issuer hidden: %+v %v", rows, e)
	}
	if _, e = s.RequestHistoricalInvoice(ctx, customer, op, "after-transfer", []InvoiceLine{{receipt, ledger.FromInt64(4)}}, InvoiceDetails{Title: "Original customer"}); e != nil {
		t.Fatal("original paid relationship lost on current reassignment", e)
	}
	other := uuid.NewString()
	if rows, _, e = s.ListCustomerInvoices(ctx, other, "", 20); e != nil || len(rows) != 0 {
		t.Fatal("another identity read original invoices", e)
	}
	if _, e = s.RequestHistoricalInvoice(ctx, other, op, "forged", []InvoiceLine{{receipt, ledger.FromInt64(1)}}, InvoiceDetails{Title: "Outsider"}); !errors.Is(e, ErrForbidden) {
		t.Fatalf("historical source skipped exact customer proof: %v", e)
	}
}
