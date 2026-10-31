package payments

import (
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"net/http"
	"os"
	"testing"
	"time"
)

// Run against integration-lab/payment-quote-mock as a separate process. The
// fixture verifies KVoucher request signatures and keeps real checkout state.
func TestExternalQuoteSourceAndKVoucherActualReceipt(t *testing.T) {
	base := os.Getenv("TOKENHUB_QUOTE_MOCK_URL")
	if base == "" {
		t.Skip("independently deployed payment quote lab required")
	}
	db := paymentDB(t)
	s := NewService(db, "external-quotes", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	s.FXBaseURL = base
	rate, err := s.referenceRate(t.Context(), "USD", "CNY", false)
	if err != nil || rate.Rate.String() != "7" {
		t.Fatal("external Frankfurter protocol failed", rate.Rate, err)
	}
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "kvoucher", Enabled: true, APIBaseURL: base + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "fixture", "kvoucher_api_secret": "fixture"}}
	if _, err = s.SaveMerchant(t.Context(), "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "MYR"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	ch, err := s.Create(t.Context(), m.OperatorID, m.ID, o)
	if err != nil || ch.PayMoney == nil || ch.PayMoney.Amount.String() != "40" || ch.PaymentQuote.Source != "KVoucher" {
		t.Fatal("external signed checkout failed", ch, err)
	}
	if _, err = s.Reconcile(t.Context(), m.OperatorID, m.ID, o.ID); !errors.Is(err, ErrNotFinal) {
		t.Fatal("unpaid external checkout was accepted", err)
	}
	response, err := http.Post(base+"/__fixture/settle?id="+ch.ProviderID+"&amount=0.01", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	fact, err := s.Reconcile(t.Context(), m.OperatorID, m.ID, o.ID)
	if !errors.Is(err, ErrAwaitingSettlement) || fact.Money.Currency != "MYR" || fact.Money.Amount.String() != "0.01" {
		t.Fatal("incorrect external payment fulfilled the order", fact.Money, err)
	}
	var n int64
	db.Table("payment_test_receipts").Count(&n)
	if n != 0 {
		t.Fatal("external underpayment created an account credit")
	}
}
