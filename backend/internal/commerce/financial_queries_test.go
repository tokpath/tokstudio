package commerce

import (
	"testing"
	"time"
)

func TestFinancialSalesSeparateWalletFundingFromExternalReceipts(t *testing.T) {
	f := setup(t)
	f.separatelyFundedTopups(t)
	o := f.order(t, "financial-wallet-package", "60", "0")
	r := f.receipt(t, "financial-direct", "20", "0", o.CreatedAt)
	f.allocate(t, "financial-direct", r, o, "20")
	if _, e := f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); e != nil {
		t.Fatal(e)
	}
	facts, e := f.s.FinancialFacts(f.ctx, nil, f.operator, time.Now().Add(-time.Hour), time.Now().Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if facts.CompletedSales != 1 || facts.SalesMoney.String() != "80" || facts.WalletTopUps != 2 || facts.WalletTopUpMoney.String() != "100" || facts.VerifiedReceiptGrossMoney.String() != "120" {
		t.Fatalf("wallet funding was counted as a second external receipt: %+v", facts)
	}
}
