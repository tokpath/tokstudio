package payments

import (
	"errors"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestCryptoCannotBeConfiguredAsAnOrdinaryPayment(t *testing.T) {
	s := NewService(nil, "test", testPaymentAuth{}, nil, nil)
	for _, driver := range []string{"nowpayments", "evm_wallet"} {
		if _, e := s.SaveMerchant(t.Context(), "finance", Merchant{ID: "test", OperatorID: "test", Driver: driver}, 0); !errors.Is(e, ErrUnsupported) {
			t.Fatal(driver, e)
		}
	}
	for _, code := range []string{"USDT", "USDC", "BTC", "ETH", "USDTTRC20", "XXX", "XTS"} {
		if (Money{Currency: code, Amount: ledger.FromInt64(1)}).Validate() == nil {
			t.Fatal("unsupported currency", code)
		}
		if len((Manual{}).Validate(Merchant{Credentials: map[string]string{"instructions": "test", "currency": code}})) == 0 {
			t.Fatal("crypto configured as offline currency", code)
		}
	}
	for _, code := range []string{"USD", "CNY", "EUR", "JPY", "MYR", "THB", "INR"} {
		if e := (Money{Currency: code, Amount: ledger.FromInt64(1)}).Validate(); e != nil {
			t.Fatal("ordinary currency rejected", code, e)
		}
	}
}

func TestRetiredCryptoIntentDoesNotOfferOldPaymentInstructions(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "test-crypto-removal", testPaymentAuth{}, nil, nil)
	m := Merchant{ID: "old-payment", OperatorID: "test-operator", Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "fixture"}}
	if _, e := s.SaveMerchant(t.Context(), "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	order, e := s.seal(Order{ID: "old-order", PaymentDriver: "manual", Money: Money{Currency: "USDT", Amount: ledger.FromInt64(15)}})
	if e != nil {
		t.Fatal(e)
	}
	checkout, e := s.seal(Checkout{PaymentAddress: "retired-wallet", Instructions: "obsolete crypto instructions"})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(`INSERT INTO payments_intents(merchant_id,operator_id,order_id,fingerprint,state,order_ciphertext,checkout_ciphertext,created_at,updated_at) VALUES(?,?,'old-order','fixture','created',?,?,now(),now())`, m.ID, m.OperatorID, order, checkout).Error; e != nil {
		t.Fatal(e)
	}
	view, e := s.PaymentIntent(t.Context(), m.OperatorID, m.ID, "old-order")
	if e != nil || view.Checkout != nil {
		t.Fatal("retired crypto checkout still instructs payment", view, e)
	}
}
