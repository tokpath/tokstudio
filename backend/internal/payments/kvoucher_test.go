package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestMYRFaceCoversUSDDueAfterKVoucherFee(t *testing.T) {
	face, err := myrFaceForUSD(ledger.MustDecimal("10"), ledger.MustDecimal("0.245"))
	if err != nil || face != "40.02" {
		t.Fatal(face, err)
	}
}

func TestKVoucherWebhookSignatureAndPaidRetrieval(t *testing.T) {
	secret := "webhook-secret"
	var retrieved bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/exchange-rates":
			_, _ = w.Write([]byte(`{"base":"MYR","rates":{"USD":0.245}}`))
		case "/api/kv/merchant/v1/orders":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"status":"pending_payment"}}}`))
		case "/api/kv/merchant/v1/orders/30/gkash-pay":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"payment":{"action":"https://pay.example.test/form","method":"POST","fields":{"v_amount":"1.05","v_currency":"MYR","CID":"M"}}}}`))
		case "/api/kv/merchant/v1/orders/30":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"order":{"id":30,"order_no":"B2B-1","status":"pins_loaded","items":[{"denomination":40.02,"qty":1}]}}}`))
		case "/api/kv/merchant/v1/stock/redeemed":
			_, _ = w.Write([]byte(`{"ok":true,"data":{"items":[]}}`))
		case "/api/kv/merchant/v1/stock/retrieve":
			retrieved = true
			_, _ = w.Write([]byte(`{"ok":true,"data":{"retrieved":[{"pin_code":"0000-0000-TEST-0001","serial":"SN","expires_at":"2027-01-01","denomination":40.02}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	now := time.Now().UTC()
	merchant := Merchant{ID: "merchant", OperatorID: "operator", Driver: "kvoucher", Enabled: true, APIBaseURL: server.URL + "/api/kv/merchant/v1", Credentials: map[string]string{"kvoucher_api_key": "key", "kvoucher_api_secret": "secret", "webhook_secret": secret}}
	driver := KVoucher{HTTP: &Transport{Client: server.Client(), AllowTestLoopback: true}}
	order := Order{ID: "order-1", OperatorID: merchant.OperatorID, MerchantID: merchant.ID, Money: Money{Currency: "USD", Amount: ledger.MustDecimal("10")}, Valuation: Valuation{ID: "usd", Currency: "USD", USDPerUnit: ledger.FromInt64(1), Source: "native", EffectiveAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Hour)}, AcceptedAt: now, ExpiresAt: now.Add(30 * time.Minute)}
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
}
