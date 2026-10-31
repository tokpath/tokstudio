package payments

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestForeignPaymentQuoteIsCachedPinnedAndNeverReplacesActualMoney(t *testing.T) {
	for _, tc := range []struct {
		name, actual string
		pending      bool
	}{{"exact", "70", false}, {"underpaid", "0.01", true}, {"overpaid", "71", true}} {
		t.Run(tc.name, func(t *testing.T) {
			db := paymentDB(t)
			sink := &sqlReceiptSink{}
			s := NewService(db, "quote-test", testPaymentAuth{}, NewTransport(true), sink)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v2/rate/usd/cny" {
					t.Error("unexpected currency pair", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"base": "USD", "quote": "CNY", "date": time.Now().UTC().Format("2006-01-02"), "rate": 7})
			}))
			defer server.Close()
			s.FXBaseURL = server.URL
			m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "CNY", "recipient_reference": "cash desk", "instructions": "test payment"}}
			if _, err := s.SaveMerchant(t.Context(), "finance", m, 0); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "CNY"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: now, ExpiresAt: now.Add(20 * time.Minute)}
			ch, err := s.Create(t.Context(), m.OperatorID, m.ID, o)
			if err != nil || ch.PaymentQuote == nil || ch.PayMoney.Currency != "CNY" || ch.PayMoney.Amount.String() != "70" || ch.PaymentQuote.Source != "Frankfurter" {
				t.Fatal("missing real quoted payable", ch, err)
			}
			if _, err = s.SetFXSettings(t.Context(), "finance", m.OperatorID, FXSettings{Source: "frankfurter", Enabled: false}, 1); err != nil {
				t.Fatal(err)
			}
			replayed, err := s.Create(t.Context(), m.OperatorID, m.ID, o)
			if err != nil || replayed.PaymentQuote.ID != ch.PaymentQuote.ID || calls.Load() != 1 {
				t.Fatal("retry repriced an accepted payment", err)
			}
			fact, err := s.RecordManual(t.Context(), "finance", m.OperatorID, m.ID, o.ID, ManualVerification{ExternalReference: "actual-transfer", Money: Money{Currency: "CNY", Amount: ledger.MustDecimal(tc.actual)}, PaidAt: now, EvidenceReference: "verified actual cash receipt", PayerReference: "test payer"})
			if (tc.pending && !errors.Is(err, ErrAwaitingSettlement)) || (!tc.pending && err != nil) || fact.Money.Currency != "CNY" || fact.Money.Amount.String() != tc.actual {
				t.Fatal("actual original receipt was lost or invented", fact.Money, err)
			}
			var receipts int64
			if err = db.Table("payment_test_receipts").Count(&receipts).Error; err != nil {
				t.Fatal(err)
			}
			if tc.pending && receipts != 0 || !tc.pending && receipts != 1 {
				t.Fatal("receipt credited without matching the quote", receipts)
			}
			if !tc.pending {
				var amount string
				db.Table("payment_test_receipts").Select("amount").Scan(&amount)
				if amount != "10.000000000000000000" && amount != "10" {
					t.Fatal("wrong contractual amount", amount)
				}
			}
		})
	}
}

func TestFrankfurterStaleFailedOrMismatchedDataDoesNotProduceAQuote(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "quote-failure", testPaymentAuth{}, NewTransport(true), nil)
	var broken atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if broken.Load() {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"base": "USD", "quote": "CNY", "date": time.Now().UTC().Format("2006-01-02"), "rate": 7})
	}))
	defer server.Close()
	s.FXBaseURL = server.URL
	if _, err := s.referenceRate(t.Context(), "USD", "CNY", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.referenceRate(t.Context(), "USD", "CNY", false); err != nil || calls.Load() != 1 {
		t.Fatal("rates were fetched per payment", err, calls.Load())
	}
	broken.Store(true)
	if err := db.Exec("UPDATE payments_fx_rates SET fetched_at=clock_timestamp()-interval '25 hours',attempted_at=clock_timestamp()-interval '2 hours'").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.referenceRate(t.Context(), "USD", "CNY", false); !errors.Is(err, ErrQuoteUnavailable) {
		t.Fatal("stale data was silently reused", err)
	}
	if settings, err := s.FXSettings(t.Context()); err != nil || settings.Status != "unavailable" {
		t.Fatal("source failure invisible to operators", settings.Status, err)
	}
	broken.Store(false)
	if _, err := s.referenceRate(t.Context(), "EUR", "JPY", false); !errors.Is(err, ErrQuoteUnavailable) {
		t.Fatal("mismatched rate response accepted", err)
	}
}

func TestKVoucherUnderpaymentCannotDischargeTheOrderObligation(t *testing.T) {
	db := paymentDB(t)
	sink := &sqlReceiptSink{}
	s := NewService(db, "kvoucher-original", testPaymentAuth{}, NewTransport(true), sink)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/exchange-rates":
			w.Write([]byte(`{"base":"MYR","rates":{"USD":0.25}}`))
		case "/api/kv/merchant/v1/orders":
			w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"status":"pending_payment"}}}`))
		case "/api/kv/merchant/v1/orders/30/gkash-pay":
			w.Write([]byte(`{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_amount":"40","v_currency":"MYR"}}}}`))
		case "/api/kv/merchant/v1/orders/30":
			w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"status":"pins_loaded","items":[{"denomination":0.01,"qty":1}]}}}`))
		case "/api/kv/merchant/v1/stock/redeemed":
			w.Write([]byte(`{"ok":true,"data":{"items":[]}}`))
		case "/api/kv/merchant/v1/stock/retrieve":
			w.Write([]byte(`{"ok":true,"data":{"retrieved":[{"pin_code":"TEST-PIN","denomination":0.01}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "fixture", "kvoucher_api_secret": "fixture"}}
	if _, err := s.SaveMerchant(t.Context(), "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "MYR"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	ch, err := s.Create(t.Context(), m.OperatorID, m.ID, o)
	if err != nil || ch.PaymentQuote == nil || ch.PaymentQuote.Source != "KVoucher" {
		t.Fatal("channel quote was not preferred", err)
	}
	fact, err := s.Reconcile(t.Context(), m.OperatorID, m.ID, o.ID)
	if !errors.Is(err, ErrAwaitingSettlement) || fact.Money.Currency != "MYR" || fact.Money.Amount.String() != "0.01" {
		t.Fatal("small MYR receipt became the USD amount due", fact.Money, err)
	}
	var n int64
	db.Table("payment_test_receipts").Count(&n)
	if n != 0 {
		t.Fatal("underpaid receipt fulfilled the order")
	}
}

func TestQuoteExpiryAndNativeCurrencyMinorUnits(t *testing.T) {
	for code, digits := range map[string]uint8{"JPY": 0, "KRW": 0, "KWD": 3, "RUB": 2, "CNY": 2} {
		if got, ok := fiatMinorDigits(code); !ok || got != digits {
			t.Fatal(code, got)
		}
	}
	now := time.Now().UTC()
	o := Order{Money: Money{Currency: "CNY"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, ExpiresAt: now.Add(time.Minute)}
	q, err := quoteMoney(o, ledger.FromInt64(7), "Frankfurter", "https://frankfurter.dev/", now)
	if err != nil {
		t.Fatal(err)
	}
	o.Money = q.PayMoney
	o.PaymentQuote = &q
	at := q.ExpiresAt.Add(time.Second)
	if _, reason := receiptExecutionValue(o, Fact{Money: q.PayMoney, OccurredAt: &at}); !strings.Contains(reason, "expiry") {
		t.Fatal("late payment reused an expired quote", reason)
	}
}

func TestStablecoinPaymentPinsBothReferenceInvoiceAndProviderQuote(t *testing.T) {
	db := paymentDB(t, "CNY")
	s := NewService(db, "stablecoin-quote", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	orderID := uuid.NewString()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v2/rate/cny/usd":
			_ = json.NewEncoder(w).Encode(map[string]any{"base": "CNY", "quote": "USD", "date": time.Now().UTC().Format("2006-01-02"), "rate": 0.14})
		case "/v1/payment":
			var in struct {
				Amount   json.Number `json:"price_amount"`
				Currency string      `json:"price_currency"`
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil || in.Amount.String() != "14" || in.Currency != "usd" {
				t.Error("provider invoice did not use the reference quote", in)
			}
			_, _ = w.Write([]byte(`{"payment_id":123,"pay_address":"synthetic-wallet","pay_amount":14.2,"pay_currency":"usdt"}`))
		case "/v1/payment/123":
			_ = json.NewEncoder(w).Encode(map[string]any{"payment_id": 123, "order_id": orderID, "payment_status": "finished", "pay_currency": "usdt", "actually_paid": 14.2, "price_currency": "usd", "price_amount": 14, "pay_amount": 14.2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	s.FXBaseURL = server.URL
	now := time.Now().UTC()
	asset := AssetNetwork{ID: uuid.NewString(), VersionID: uuid.NewString(), Version: 1, Driver: "nowpayments", Asset: "USDT", Network: "eth", TokenContract: "synthetic-token", TokenDecimals: 6, MinimumConfirmations: 12, Enabled: true, EffectiveAt: now.Add(-time.Hour), EvidenceReference: "explicit synthetic asset fixture", PublishedBy: "test:fixture", PublishedAt: now.Add(-time.Hour)}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "nowpayments", Enabled: true, AssetNetworkVersionID: asset.VersionID, APIBaseURL: server.URL, Credentials: map[string]string{"api_key": "fixture", "ipn_secret": "fixture"}}
	if _, err := s.SaveMerchant(t.Context(), "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	o := Order{ID: orderID, OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "USDT"}, SettlementMoney: Money{Currency: "CNY", Amount: ledger.FromInt64(100)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour), NotifyURL: "https://example.test/webhook"}
	ch, err := s.Create(t.Context(), m.OperatorID, m.ID, o)
	if err != nil || ch.PaymentQuote == nil {
		t.Fatal("stablecoin checkout failed", ch, err)
	}
	q := ch.PaymentQuote
	if q.Source != "NOWPayments + Frankfurter" || q.InvoiceMoney == nil || q.InvoiceMoney.Currency != "USD" || q.InvoiceMoney.Amount.String() != "14" || q.PayMoney.Currency != "USDT" || q.PayMoney.Amount.String() != "14.2" || q.Rate.String() != "0.142" || q.RateNumerator != "71" || q.RateDenominator != "500" {
		t.Fatal("invoice, payable token and quote relation must remain distinct", q)
	}
	fact, err := s.Reconcile(t.Context(), m.OperatorID, m.ID, o.ID)
	if err != nil || fact.Money.Currency != "USDT" || fact.Money.Amount.String() != "14.2" {
		t.Fatal("actual token payment was lost", fact.Money, err)
	}
	var amount ledger.Decimal
	if err = db.Table("payment_test_receipts").Select("amount").Scan(&amount).Error; err != nil || amount.String() != "100" {
		t.Fatal("quote did not discharge the original CNY obligation", amount, err)
	}
}

func TestUnavailableQuoteDoesNotLeaveAnUnresolvedOnlinePayment(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "quote-failure-proof", testPaymentAuth{}, NewTransport(true), nil)
	var paymentCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v2/rate/") {
			paymentCalls.Add(1)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	s.FXBaseURL = server.URL
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "stripe", Enabled: true, APIBaseURL: server.URL, Credentials: map[string]string{"currency": "CNY", "secret_key": "sk_test_fixture", "publishable_key": "pk_test_fixture", "webhook_secret": "whsec_fixture"}}
	if _, err := s.SaveMerchant(t.Context(), "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "CNY"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	if _, err := s.Create(t.Context(), m.OperatorID, m.ID, o); !errors.Is(err, ErrQuoteUnavailable) {
		t.Fatal("unavailable reference quote was accepted", err)
	}
	intent, err := s.PaymentIntent(t.Context(), m.OperatorID, m.ID, o.ID)
	if err != nil || intent.State != "failed" || paymentCalls.Load() != 0 {
		t.Fatal("failure before provider creation must permit cancellation", intent.State, paymentCalls.Load(), err)
	}
}
