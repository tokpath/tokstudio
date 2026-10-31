package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPaymentQuoteRoundsActualPaymentWithoutAnImplicitFee(t *testing.T) {
	now := time.Now().UTC()
	o := Order{Money: Money{Currency: "MYR"}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, ExpiresAt: now.Add(time.Hour)}
	q, err := quoteMoneyRat(o, new(big.Rat).Inv(ledger.MustDecimal("0.245").Rat()), "KVoucher", "https://kvoucher.com", now)
	if err != nil || q.PayMoney.Amount.String() != "40.82" {
		t.Fatal(q.PayMoney, err)
	}
}

func TestKVoucherRejectsUnquotedForeignMoney(t *testing.T) {
	m := Merchant{ID: "m", OperatorID: "op", Driver: "kvoucher", Enabled: true, Credentials: map[string]string{"kvoucher_api_key": "key", "kvoucher_api_secret": "secret"}}
	o := paymentTestOrder(m, "EUR")
	if _, e := (KVoucher{}).Create(context.Background(), m, o); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}

func TestKVoucherConvertsUSDDueIntoMYRAndBooksTheUSDObligation(t *testing.T) {
	var face string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/exchange-rates":
			_, _ = w.Write([]byte(`{"base":"MYR","rates":{"USD":0.245}}`))
		case "/api/kv/merchant/v1/orders":
			var body struct {
				Items []struct {
					Denomination json.Number `json:"denomination"`
				} `json:"custom_items"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Items) == 1 {
				face = body.Items[0].Denomination.String()
			}
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"status":"pending_payment"}}}`))
		case "/api/kv/merchant/v1/orders/30/gkash-pay":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_amount":"40.82","v_currency":"MYR","CID":"M"}}}}`))
		case "/api/kv/merchant/v1/orders/30":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"order_no":"B2B-1","status":"pins_loaded","items":[{"denomination":40.82,"qty":1}]}}}`))
		case "/api/kv/merchant/v1/stock/redeemed":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"items":[]}}`))
		case "/api/kv/merchant/v1/stock/retrieve":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"retrieved":[{"pin_code":"0000-0000-TEST-0001","serial":"SN","denomination":40.82}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	now := time.Now().UTC()
	merchant := Merchant{ID: "merchant", OperatorID: "operator", Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "key", "kvoucher_api_secret": "secret"}}
	driver := KVoucher{HTTP: &Transport{Client: server.Client(), AllowTestLoopback: true}}
	order := Order{ID: "order-1", OperatorID: merchant.OperatorID, MerchantID: merchant.ID, Money: Money{Currency: "MYR", Amount: ledger.MustDecimal("40.82")}, SettlementMoney: Money{Currency: "USD", Amount: ledger.MustDecimal("10")}, AcceptedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	quoted, err := driver.QuotePayment(context.Background(), merchant, order)
	if err != nil {
		t.Fatal(err)
	}
	order.PaymentQuote = &quoted
	order.Money = quoted.PayMoney
	checkout, err := driver.Create(context.Background(), merchant, order)
	if err != nil || face != "40.82" || checkout.PayMoney == nil || checkout.PayMoney.Currency != "MYR" || checkout.PayMoney.Amount.String() != "40.82" || checkout.SettlementMoney == nil || checkout.SettlementMoney.Amount.String() != "10" {
		t.Fatal(face, checkout, err)
	}
	order.ProviderID = checkout.ProviderID
	fact, err := driver.Lookup(context.Background(), merchant, order)
	if err != nil || fact.Money.Currency != "MYR" || fact.Money.Amount.String() != "40.82" {
		t.Fatal(fact, err)
	}
}

func TestKVoucherWebhookSignatureAndPaidRetrieval(t *testing.T) {
	secret := "webhook-secret"
	var retrieved bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/kv/merchant/v1/orders":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"status":"pending_payment"}}}`))
		case "/api/kv/merchant/v1/orders/30/gkash-pay":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_amount":"10","v_currency":"MYR","CID":"M"}}}}`))
		case "/api/kv/merchant/v1/orders/30":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"order_no":"B2B-1","status":"pins_loaded","items":[{"denomination":10,"qty":1}]}}}`))
		case "/api/kv/merchant/v1/stock/redeemed":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"items":[]}}`))
		case "/api/kv/merchant/v1/stock/retrieve":
			retrieved = true
			_, _ = w.Write([]byte(`{"ok":true,"data":{"retrieved":[{"pin_code":"0000-0000-TEST-0001","serial":"SN","expires_at":"2027-01-01","denomination":10}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	now := time.Now().UTC()
	merchant := Merchant{ID: "merchant", OperatorID: "operator", Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "key", "kvoucher_api_secret": "secret", "webhook_secret": secret}}
	driver := KVoucher{HTTP: &Transport{Client: server.Client(), AllowTestLoopback: true}}
	order := Order{ID: "order-1", OperatorID: merchant.OperatorID, MerchantID: merchant.ID, Money: Money{Currency: "MYR", Amount: ledger.MustDecimal("10")}, AcceptedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
	checkout, err := driver.Create(context.Background(), merchant, order)
	if err != nil || checkout.ProviderID != "30" || checkout.URL != "https://pay.example.test/form" || checkout.FormFields["v_currency"] != "MYR" {
		t.Fatal(checkout, err)
	}
	order.ProviderID = checkout.ProviderID
	fact, err := driver.Lookup(context.Background(), merchant, order)
	if err != nil || !fact.Verified() || fact.Money.Amount.Cmp(order.Money.Amount) != 0 || !retrieved {
		t.Fatal(fact, err, retrieved)
	}
	var evidence map[string]any
	if json.Unmarshal(fact.Evidence, &evidence) != nil || evidence["kvoucher_order_no"] != "B2B-1" {
		t.Fatal(string(fact.Evidence))
	}
	body := []byte(`{"event_id":"evt-1","event":"order.pins_loaded","data":{"id":30,"metadata":{"order_ref":"order-1"}}}`)
	sum := sha256.Sum256([]byte(secret))
	mac := hmac.New(sha256.New, []byte(hex.EncodeToString(sum[:])))
	mac.Write(body)
	header := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	hint, err := driver.VerifyWebhook(context.Background(), merchant, Webhook{Headers: http.Header{"X-Kvoucher-Signature": []string{header}}, Body: body})
	if err != nil || !hint.SignatureVerified || hint.OrderID != "order-1" || hint.ProviderID != "30" || !hint.Final {
		t.Fatal(hint, err)
	}
	if _, err = driver.VerifyWebhook(context.Background(), merchant, Webhook{Headers: http.Header{"X-Kvoucher-Signature": []string{"sha256=nope"}}, Body: body}); err == nil {
		t.Fatal("accepted a bad signature")
	}
	created := []byte(`{"event_id":"evt-created","event":"order.created","data":{"id":30,"metadata":{"order_ref":"order-1"}}}`)
	createdMAC := hmac.New(sha256.New, []byte(hex.EncodeToString(sum[:])))
	createdMAC.Write(created)
	createdHint, err := driver.VerifyWebhook(context.Background(), merchant, Webhook{Headers: http.Header{"X-Kvoucher-Signature": []string{"sha256=" + hex.EncodeToString(createdMAC.Sum(nil))}}, Body: created})
	if err != nil || createdHint.Kind != "test" || createdHint.Final {
		t.Fatal(createdHint, err)
	}
}

func TestKVoucherSandboxBooksACodeBeforeTheGatewayLeavesPending(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  string
		sandbox bool
		want    error
		booked  bool
	}{
		{name: "sandbox pending", status: "pending_payment", sandbox: true, booked: true},
		{name: "live pending", status: "pending_payment", want: ErrNotFinal},
		{name: "expired", status: "expired", sandbox: true, want: ErrProvider},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var retrieved bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/kv/merchant/v1/orders/30":
					_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"order_no":"B2B-1","status":"` + tc.status + `","items":[{"denomination":4,"qty":1}]}}}`))
				case "/api/kv/merchant/v1/account":
					mode := "false"
					if tc.sandbox {
						mode = "true"
					}
					_, _ = w.Write([]byte(`{"ok":true,"data":{"is_test_mode":` + mode + `}}`))
				case "/api/kv/merchant/v1/stock/redeemed":
					_, _ = w.Write([]byte(`{"ok":true,"data":{"items":[]}}`))
				case "/api/kv/merchant/v1/stock/retrieve":
					retrieved = true
					_, _ = w.Write([]byte(`{"ok":true,"data":{"retrieved":[{"pin_code":"0000-0000-TEST-0001","serial":"SN","denomination":4}]}}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			now := time.Now().UTC()
			merchant := Merchant{ID: "merchant", OperatorID: "operator", Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "key", "kvoucher_api_secret": "secret"}}
			driver := KVoucher{HTTP: &Transport{Client: server.Client(), AllowTestLoopback: true}}
			order := Order{ID: "order-1", OperatorID: merchant.OperatorID, MerchantID: merchant.ID, ProviderID: "30", Money: Money{Currency: "MYR", Amount: ledger.MustDecimal("4")}, AcceptedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
			fact, err := driver.Lookup(context.Background(), merchant, order)
			if tc.want != nil {
				if !errors.Is(err, tc.want) || retrieved {
					t.Fatal(err, retrieved)
				}
				return
			}
			if err != nil || !fact.Verified() || fact.Money.Amount.Cmp(order.Money.Amount) != 0 || !retrieved {
				t.Fatal(fact, err, retrieved)
			}
		})
	}
}
