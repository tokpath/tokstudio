package operations

import (
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestReceiptStageDoesNotInvoiceTopUp(t *testing.T) {
	eligible := ledger.FromInt64(100)
	topUp := ledger.FromInt64(60)
	receipt := eligible.Sub(topUp)
	basis := InvoiceBasis{EligibleMoney: eligible, TopUpMoney: topUp, ReceiptMoney: &receipt, PurchaseMoney: ledger.FromInt64(40)}
	got, err := eligibleInvoiceAmount(basis, "receipt", ledger.FromInt64(1))
	if err != nil || got.Cmp(ledger.FromInt64(40)) != 0 {
		t.Fatal(got, err)
	}
	pure := ledger.Zero()
	topUpOnly := InvoiceBasis{EligibleMoney: eligible, TopUpMoney: eligible, ReceiptMoney: &pure, PurchaseMoney: ledger.Zero()}
	got, err = eligibleInvoiceAmount(topUpOnly, "receipt", ledger.FromInt64(1))
	if err != nil || !got.IsZero() {
		t.Fatal(got, err)
	}
	got, err = eligibleInvoiceAmount(basis, "purchase", ledger.FromInt64(1))
	if err != nil || got.Cmp(ledger.FromInt64(40)) != 0 {
		t.Fatal(got, err)
	}
}
