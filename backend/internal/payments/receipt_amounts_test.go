package payments

import (
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestReceiptProviderSurchargeDoesNotIncreaseOrderFunding(t *testing.T) {
	usd := func(amount string) Money { return Money{Currency: "USD", Amount: ledger.MustDecimal(amount)} }
	o := Order{Money: usd("102"), SettlementMoney: usd("102")}
	f := Fact{Driver: "stripe", Money: usd("103"), MerchantNet: func() *Money { m := usd("100.02"); return &m }()}
	v, reason := receiptExecutionValue(o, f)
	if reason != "" || v.Amount.String() != "102" || v.OriginalMoney.Amount.String() != "103" {
		t.Fatal(v, reason)
	}
	o.Money = Money{Currency: "MYR", Amount: ledger.FromInt64(408)}
	o.PaymentQuote = &PaymentQuote{ID: "quote", Source: "test", PayMoney: o.Money, SettlementMoney: o.SettlementMoney, ExpiresAt: time.Now().Add(time.Hour)}
	f.Money = Money{Currency: "MYR", Amount: ledger.MustDecimal("426.77")}
	v, reason = receiptExecutionValue(o, f)
	if reason != "" || v.Amount.String() != "102" || v.OriginalMoney.Amount.String() != "426.77" {
		t.Fatal(v, reason)
	}
	f.Money.Amount = ledger.FromInt64(1)
	if _, reason = receiptExecutionValue(o, f); reason == "" {
		t.Fatal("underpayment funded whole order")
	}
}
