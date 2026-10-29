package operations

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresInvoiceSearchPrecedesLimitAndRetainsIssuerAndCustomerScope(t *testing.T) {
	s, auth, _, operator, customer := opsFixture(t)
	ctx := context.Background()
	makeRow := func(op, owner, reference string, age time.Duration) Invoice {
		return Invoice{ID: uuid.NewString(), OperatorID: op, CustomerID: owner, PartyKind: "customer", BusinessKey: uuid.NewString(), Fingerprint: "synthetic search fixture", AmountMoney: ledger.FromInt64(1), State: "issued", DetailsCiphertext: "synthetic encrypted details", CreatedAt: time.Now().Add(-age), InvoiceReference: reference, EvidenceReference: "verified-search-proof", PolicyStage: "receipt", PolicyVersion: 1, PolicyRatio: ledger.FromInt64(1)}
	}
	target := makeRow(operator, customer, "Doc_100%", 24*time.Hour)
	rows := []Invoice{target, makeRow(uuid.NewString(), uuid.NewString(), "Doc_100%", 0), makeRow(operator, uuid.NewString(), "Doc_100%", 0)}
	for i := 0; i < 110; i++ {
		rows = append(rows, makeRow(operator, customer, fmt.Sprintf("ordinary-document-%d", i), time.Duration(i)*time.Minute))
	}
	if err := s.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	receipt := uuid.NewString()
	if err := s.DB.Table("operations_invoice_sources").Create(map[string]any{"invoice_id": target.ID, "receipt_id": receipt, "amount_money": "1"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, search := range []string{target.ID, strings.ToUpper(target.ID), receipt, "DOC_100%"} {
		found, err := s.ListInvoices(ctx, auth.actor, operator, false, search)
		if err != nil {
			t.Fatal(err)
		}
		want := 1
		if search == "DOC_100%" {
			want = 2
		}
		if len(found) != want {
			t.Fatalf("staff search %q failed before page limit: got %d", search, len(found))
		}
		for _, row := range found {
			if row.OperatorID != operator {
				t.Fatal("search escaped original issuer scope")
			}
		}

		paged, cursor, pageErr := s.ListOperatorInvoicesPage(ctx, auth.actor, operator, "", search, "issued", 1)
		if pageErr != nil || len(paged) != 1 || paged[0].OperatorID != operator {
			t.Fatalf("issuer page search escaped scope or applied limit first: %+v %q %v", paged, cursor, pageErr)
		}
		if search != "DOC_100%" && (paged[0].ID != target.ID || cursor != "") {
			t.Fatal("exact invoice search returned the wrong page", paged, cursor)
		}
		rejected, _, pageErr := s.ListOperatorInvoicesPage(ctx, auth.actor, operator, "", search, "rejected", 1)
		if pageErr != nil || len(rejected) != 0 {
			t.Fatal("invoice status was not applied before limit", rejected, pageErr)
		}
		own, next, err := s.ListCustomerInvoices(ctx, customer, "", 1, search)
		if err != nil || len(own) != 1 || own[0].ID != target.ID || next != "" {
			t.Fatalf("customer search %q lost source or identity scope: %+v %q %v", search, own, next, err)
		}
	}
	if found, _, err := s.ListCustomerInvoices(ctx, uuid.NewString(), "", 10, target.ID); err != nil || len(found) != 0 {
		t.Fatal("another customer searched a private invoice", found, err)
	}
}
