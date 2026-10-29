package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func paymentTestOrder(m Merchant, currency string) Order {
	at := time.Now().UTC()
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: currency, Amount: ledger.MustDecimal("10")}, AcceptedAt: at, ExpiresAt: at.Add(15 * time.Minute), NotifyURL: "https://example.com/payments/webhook", Description: "TokenHub protocol contract"}
	if m.Driver == "nowpayments" {
		o.SettlementMoney = Money{Currency: "USD", Amount: ledger.FromInt64(10)}
	}
	return o
}
func TestMoneyAndSettlementAmountsStayExact(t *testing.T) {
	m := Money{Currency: "USD", Amount: ledger.MustDecimal("0.001")}
	if _, e := m.Minor(2); e == nil {
		t.Fatal("sub-cent actual debit silently rounded")
	}
	original := Money{Currency: "USDT", Amount: ledger.MustDecimal("17.123456789012345678")}
	confirmation := Valuation{Currency: "EUR", Amount: ledger.MustDecimal("14.123456789012345678"), Source: "verified individual transaction"}
	value, e := confirmation.Convert(original, time.Time{})
	if e != nil || value.Amount.Cmp(confirmation.Amount) != 0 || value.OriginalMoney.Amount.Cmp(original.Amount) != 0 || value.Currency != "EUR" {
		t.Fatal(value, e)
	}
	confirmation.Source = ""
	if _, e = confirmation.Convert(original, time.Time{}); e == nil {
		t.Fatal("cross-currency settlement lacked evidence")
	}
	fact := Fact{ID: "user-provided", proof: false}
	raw, _ := json.Marshal(fact)
	var untrusted Fact
	_ = json.Unmarshal(raw, &untrusted)
	if untrusted.Verified() {
		t.Fatal("JSON marked its own money as verified")
	}
}
func TestStripeAuthenticatesRawBodyAndRejectsReplay(t *testing.T) {
	m := Merchant{ID: "merchant", OperatorID: "operator", Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_synthetic", "publishable_key": "pk_test_synthetic", "webhook_secret": "whsec_test", "currency": "USD"}}
	body := []byte(`{"id":"evt_1","type":"payment_intent.succeeded","created":1700000000,"data":{"object":{"id":"pi_1","metadata":{"order_id":"order"}}}}`)
	at := time.Unix(1700000000, 0)
	timestamp := fmt.Sprint(at.Unix())
	mac := hmac.New(sha256.New, []byte("whsec_test"))
	_, _ = mac.Write([]byte(timestamp + "." + string(body)))
	header := http.Header{"Stripe-Signature": []string{"t=" + timestamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))}}
	hint, e := (Stripe{}).VerifyWebhook(context.Background(), m, Webhook{Headers: header, Body: body, ReceivedAt: at})
	if e != nil || !hint.SignatureVerified || !hint.Final {
		t.Fatal(e)
	}
	body = append(body, ' ')
	if _, e = (Stripe{}).VerifyWebhook(context.Background(), m, Webhook{Headers: header, Body: body, ReceivedAt: at}); !errors.Is(e, ErrSignature) {
		t.Fatal("modified body authenticated")
	}
	if _, e = (Stripe{}).VerifyWebhook(context.Background(), m, Webhook{Headers: header, Body: bytes.TrimSpace(body), ReceivedAt: at.Add(6 * time.Minute)}); !errors.Is(e, ErrSignature) {
		t.Fatal("expired callback authenticated")
	}
}

func externalPaymentLab(t *testing.T) (string, map[string]string) {
	t.Helper()
	base := strings.TrimRight(os.Getenv("TOKENHUB_PAYMENT_MOCK_URL"), "/")
	if base == "" {
		t.Skip("TOKENHUB_PAYMENT_MOCK_URL required for independent protocol service")
	}
	if !strings.HasPrefix(base, "http://127.0.0.1:") {
		t.Fatal("external protocol tests require explicit loopback mock")
	}
	r, e := http.Get(base + "/__control/protocol/config")
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	var object map[string]any
	if json.NewDecoder(r.Body).Decode(&object) != nil || object["synthetic_only"] != true {
		t.Fatal("independent protocol fixture unavailable")
	}
	out := map[string]string{}
	for key, value := range object {
		if text, ok := value.(string); ok {
			out[key] = text
		}
	}
	return base, out
}
func paymentControl(t *testing.T, base, path string, body any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	r, e := http.Post(base+path, "application/json", bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	defer r.Body.Close()
	var result map[string]any
	if e = json.NewDecoder(r.Body).Decode(&result); e != nil {
		t.Fatal(e)
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		t.Fatalf("mock control %s: %d", path, r.StatusCode)
	}
	return result
}
func merchantFixture(driver, base string, c map[string]string) Merchant {
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: driver, Enabled: true, APIBaseURL: base, Credentials: map[string]string{}}
	switch driver {
	case "stripe":
		m.Credentials = map[string]string{"secret_key": "sk_test_synthetic", "publishable_key": "pk_test_synthetic", "webhook_secret": "whsec_integration_test", "currency": "USD"}
	case "alipay":
		m.Credentials = map[string]string{"app_id": c["app_id"], "app_private_key": c["merchant_private_key"], "alipay_public_key": c["provider_public_key"], "seller_id": c["seller_id"]}
	case "wechat":
		m.Credentials = map[string]string{"app_id": c["app_id"], "mch_id": c["mch_id"], "merchant_api_private_key": c["merchant_private_key"], "cert_serial": c["cert_serial"], "wechat_public_key": c["provider_public_key"], "public_key_id": c["public_key_id"], "api_v3_key": c["api_v3_key"]}
	case "nowpayments":
		m.Credentials = map[string]string{"api_key": c["now_api_key"], "ipn_secret": c["now_ipn_secret"], "pay_currency": "usdterc20"}
	case "evm_wallet":
		m.Credentials = map[string]string{"rpc_url": base + "/rpc", "chain_id": c["chain_id"], "token_contract": c["token_contract"], "token_decimals": c["token_decimals"], "wallet_address": c["wallet_address"], "asset": c["asset"], "min_confirmations": "12"}
	}
	return m
}
func TestExternalPaymentDriversAndSignedCallbacks(t *testing.T) {
	base, config := externalPaymentLab(t)
	transport := NewTransport(true)
	defer transport.Client.CloseIdleConnections()
	drivers := []Driver{Stripe{transport}, Alipay{transport}, Wechat{transport}, NOWPayments{transport}}
	for _, d := range drivers {
		t.Run(d.Name(), func(t *testing.T) {
			m := merchantFixture(d.Name(), base, config)
			currency := "CNY"
			if d.Name() == "stripe" {
				currency = "USD"
			}
			if d.Name() == "nowpayments" {
				currency = "USDTERC20"
			}
			o := paymentTestOrder(m, currency)
			checkout, e := d.Create(context.Background(), m, o)
			if e != nil || checkout.ProviderID == "" {
				t.Fatalf("create %#v %v", checkout, e)
			}
			o.ProviderID = checkout.ProviderID
			if _, e = d.Lookup(context.Background(), m, o); !errors.Is(e, ErrNotFinal) {
				t.Fatalf("unpaid intent became money: %v", e)
			}
			var eventID string
			if d.Name() == "stripe" {
				settled := paymentControl(t, base, "/__control/payments/"+o.ProviderID+"/settle", map[string]any{})
				eventID, _ = settled["event_id"].(string)
			} else {
				paymentControl(t, base, "/__control/protocol/settle", map[string]any{"driver": d.Name(), "order_id": o.ID, "provider_id": o.ProviderID})
			}
			fact, e := d.Lookup(context.Background(), m, o)
			if e != nil || !fact.Verified() || fact.Money.Amount.String() != "10" {
				t.Fatalf("verified receipt %#v %v", fact, e)
			}
			if d.Name() == "stripe" && fact.OccurredAt != nil {
				t.Fatal("Stripe PI creation time masqueraded as payment time")
			}
			errorsSeen := make(chan error, 5)
			hints := make(chan Hint, 5)
			server := externalCallbackServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
				hint, e := d.VerifyWebhook(r.Context(), m, Webhook{Headers: r.Header, Body: body, ReceivedAt: time.Now().UTC()})
				errorsSeen <- e
				if e != nil {
					w.WriteHeader(400)
					return
				}
				hints <- hint
				w.WriteHeader(200)
			}))
			defer server.Close()
			control := map[string]any{"driver": d.Name(), "order_id": o.ID, "provider_id": o.ProviderID, "url": server.URL, "repeat": 2}
			path := "/__control/protocol/webhook"
			if d.Name() == "stripe" {
				path = "/__control/webhooks"
				control["event_id"] = eventID
			}
			paymentControl(t, base, path, control)
			for i := 0; i < 2; i++ {
				if e := <-errorsSeen; e != nil {
					t.Fatal(e)
				}
				hint := <-hints
				if !hint.SignatureVerified || hint.OrderID != o.ID || hint.ProviderID != fact.ProviderID {
					t.Fatalf("callback hint %#v", hint)
				}
			}
			control["repeat"] = 1
			control["invalid_signature"] = true
			paymentControl(t, base, path, control)
			if e := <-errorsSeen; !errors.Is(e, ErrSignature) {
				t.Fatalf("invalid provider signature: %v", e)
			}
			if d.Name() == "alipay" || d.Name() == "wechat" {
				paymentControl(t, base, "/__control/protocol/tamper", map[string]any{"count": 1})
				if _, e = d.Lookup(context.Background(), m, o); !errors.Is(e, ErrSignature) {
					t.Fatalf("unsigned/tampered API response became fact: %v", e)
				}
			}
			if d.Name() != "nowpayments" {
				refund := Refund{ID: uuid.NewString(), Order: o, Money: Money{Currency: currency, Amount: ledger.MustDecimal("3")}, MoneyAmount: ledger.MustDecimal("3"), RefundValuation: o.Valuation, ValuedAt: o.AcceptedAt, FXExecutionReference: "synthetic-current-refund-fx"}
				result, e := d.Refund(context.Background(), m, refund)
				if e != nil || result.State != "succeeded" || result.Fact == nil || !result.Fact.Verified() {
					t.Fatalf("partial refund %#v %v", result, e)
				}
				refund.ProviderID = result.ProviderID
				queried, e := d.LookupRefund(context.Background(), m, refund)
				if e != nil || queried.Fact == nil || queried.Fact.ID != result.Fact.ID {
					t.Fatalf("refund query %#v %v", queried, e)
				}
			}
		})
	}
}
func TestExternalEVMWalletConfirmationsReorgAndReadOnly(t *testing.T) {
	base, config := externalPaymentLab(t)
	transport := NewTransport(true)
	defer transport.Client.CloseIdleConnections()
	d := EVMWallet{transport}
	m := merchantFixture(d.Name(), base, config)
	o := paymentTestOrder(m, "USDC")
	tx := "0x" + strings.Repeat("dd", 32)
	paymentControl(t, base, "/__control/protocol/evm", map[string]any{"transaction_hash": tx, "raw_units": "10000000", "head": 120, "chain_id": "0x1", "reorg": false})
	o.ProviderID = tx + "#0x0"
	fact, e := d.Lookup(context.Background(), m, o)
	if e != nil || !fact.Verified() || fact.Money.Amount.String() != "10" || !strings.HasPrefix(fact.ID, "evm:1:") {
		t.Fatalf("chain receipt %#v %v", fact, e)
	}
	second := m
	second.ID = uuid.NewString()
	otherOrder := o
	otherOrder.MerchantID = second.ID
	other, e := d.Lookup(context.Background(), second, otherOrder)
	if e != nil || other.ID != fact.ID {
		t.Fatal("same on-chain log escaped global dedup by changing merchant")
	}
	paymentControl(t, base, "/__control/protocol/evm", map[string]any{"head": 100})
	if _, e = d.Lookup(context.Background(), m, o); !errors.Is(e, ErrNotFinal) {
		t.Fatal("insufficient confirmations accepted")
	}
	paymentControl(t, base, "/__control/protocol/evm", map[string]any{"head": 120, "reorg": true})
	if _, e = d.Lookup(context.Background(), m, o); !errors.Is(e, ErrConflict) {
		t.Fatal("reorganized block accepted")
	}
	if _, e = d.Refund(context.Background(), m, Refund{}); !errors.Is(e, ErrUnsupported) {
		t.Fatal("wallet can execute external transfers")
	}
}

func TestRefundUsesExplicitExecutionSettlement(t *testing.T) {
	m := Merchant{ID: "m", OperatorID: "operator", Driver: "alipay", Enabled: true}
	now := time.Now().UTC()
	o := Order{ID: "order", OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "CNY", Amount: ledger.FromInt64(100)}, SettlementMoney: Money{Currency: "EUR", Amount: ledger.FromInt64(20)}, AcceptedAt: now, ExpiresAt: now.Add(time.Hour)}
	r := Refund{ID: "refund", Order: o, Money: Money{Currency: "CNY", Amount: ledger.FromInt64(10)}, MoneyAmount: ledger.FromInt64(2), RefundValuation: Valuation{ID: "actual", Currency: "EUR", Amount: ledger.MustDecimal("1.4"), Source: "verified actual transfer relation"}}
	if e := r.Validate(m); e != nil {
		t.Fatal(e)
	}
	f, e := refundFact(m, r, "actual-refund", r.Money, nil, map[string]string{"actual": "verified"})
	if e != nil || f.ExecutionValue == nil || f.ExecutionValue.Amount.String() != "1.4" || f.ExecutionValue.Currency != "EUR" || f.Money.Amount.String() != "10" {
		t.Fatal(f, e)
	}
	r.RefundValuation = Valuation{}
	if r.Validate(m) == nil {
		t.Fatal("cross-currency refund silently used obligation amount")
	}
}
