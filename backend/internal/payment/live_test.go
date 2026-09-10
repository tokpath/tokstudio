package payment

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStripeCents(t *testing.T) {
	if got := stripeCents(1_000_000); got != 100 {
		t.Fatalf("1 USD minor -> cents: got %d", got)
	}
	if got := stripeCents(1); got != 1 {
		t.Fatalf("tiny amount should be 1 cent, got %d", got)
	}
}

func TestPeekPaymentOrderID(t *testing.T) {
	if got := peekPaymentOrderID([]byte(`{"order_id":"pay_hmac"}`)); got != "pay_hmac" {
		t.Fatalf("json order_id: %s", got)
	}
	if got := peekPaymentOrderID([]byte(`out_trade_no=pay_ali&sign=x`)); got != "pay_ali" {
		t.Fatalf("form out_trade_no: %s", got)
	}
	raw, _ := json.Marshal(map[string]any{
		"data": map[string]any{"object": map[string]any{"metadata": map[string]any{"order_id": "pay_stripe"}}},
	})
	if got := peekPaymentOrderID(raw); got != "pay_stripe" {
		t.Fatalf("stripe metadata: %s", got)
	}
}

func TestStripeLiveCheckoutAndSignature(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment_intents" {
			t.Errorf("path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		if vals.Get("off_session") == "true" {
			if vals.Get("payment_method") != "pm_card" || vals.Get("confirm") != "true" {
				t.Errorf("off-session form %v", vals)
			}
			_, _ = w.Write([]byte(`{"id":"pi_1","status":"succeeded"}`))
			return
		}
		if vals.Get("metadata[order_id]") != "pay_1" {
			t.Errorf("order metadata %q", vals.Get("metadata[order_id]"))
		}
		if vals.Get("amount") != "100" {
			t.Errorf("amount %q", vals.Get("amount"))
		}
		_, _ = w.Write([]byte(`{"id":"pi_1","client_secret":"cs_test"}`))
	}))
	defer srv.Close()
	old := stripeAPIBase
	stripeAPIBase = srv.URL
	defer func() { stripeAPIBase = old }()

	d := stripeDriver{}
	sess, err := d.CreateCheckout(context.Background(), CheckoutRequest{
		Mode: ModeLive, PublicBase: "https://oem.example",
		Credentials: map[string]string{"secret_key": "sk_test", "publishable_key": "pk_test"},
		Order:       &OrderView{ID: "pay_1", AmountMinor: 1_000_000},
	})
	if err != nil || sess == nil || sess.Sandbox || sess.ClientSecret != "cs_test" {
		t.Fatalf("live checkout: %+v %v", sess, err)
	}

	id, err := d.ChargeOffSession(context.Background(), OffSessionCharge{
		Credentials:      map[string]string{"secret_key": "sk_test", "currency": "usd"},
		PaymentMethodRef: "pm_card",
		Order:            &OrderView{ID: "pay_renew", AmountMinor: 1_000_000},
	})
	if err != nil || id != "pi_1" {
		t.Fatalf("off-session %s %v", id, err)
	}

	body := []byte(`{"id":"evt_1","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1","metadata":{"order_id":"pay_1"}}}}`)
	ts := fmt.Sprintf("%d", time.Now().UTC().Unix())
	mac := hmac.New(sha256.New, []byte("whsec"))
	_, _ = fmt.Fprintf(mac, "%s.%s", ts, body)
	sig := "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
	ev, err := d.ParseWebhook(context.Background(), WebhookRequest{
		Body: body, Headers: http.Header{"Stripe-Signature": []string{sig}},
		Credentials: map[string]string{"webhook_secret": "whsec"},
	})
	if err != nil || !ev.SignatureValid || ev.OrderID != "pay_1" || ev.Status != StatusPaid {
		t.Fatalf("stripe webhook: %+v %v", ev, err)
	}
}

func TestAlipaySignVerifyAndFen(t *testing.T) {
	if fenToYuan(123) != "1.23" {
		t.Fatalf("fenToYuan")
	}
	priv, pub := mustRSA(t)
	vals := url.Values{}
	vals.Set("out_trade_no", "pay_ali")
	vals.Set("trade_status", "TRADE_SUCCESS")
	vals.Set("trade_no", "2026")
	vals.Set("notify_id", "ntf")
	sig, err := alipaySign(priv, alipaySignContent(vals))
	if err != nil {
		t.Fatal(err)
	}
	vals.Set("sign", sig)
	if !alipayVerify(pub, vals) {
		t.Fatal("alipay verify failed")
	}
	ev, err := alipayDriver{}.ParseWebhook(context.Background(), WebhookRequest{
		Body: []byte(vals.Encode()), Credentials: map[string]string{"alipay_public_key": pub},
	})
	if err != nil || !ev.SignatureValid || ev.OrderID != "pay_ali" || ev.Status != StatusPaid {
		t.Fatalf("alipay notify: %+v %v", ev, err)
	}
}

func TestAlipayLivePrecreate(t *testing.T) {
	priv, _ := mustRSA(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"alipay_trade_precreate_response":{"code":"10000","qr_code":"https://qr.alipay.com/x"}}`))
	}))
	defer srv.Close()
	old := alipayGateway
	alipayGateway = srv.URL
	defer func() { alipayGateway = old }()
	sess, err := alipayDriver{}.CreateCheckout(context.Background(), CheckoutRequest{
		Mode: ModeLive, PublicBase: "https://oem.example",
		Credentials: map[string]string{"app_id": "app", "app_private_key": priv},
		Order:       &OrderView{ID: "pay_ali", AmountMinor: 100},
	})
	if err != nil || sess == nil || sess.QRCode != "https://qr.alipay.com/x" || sess.Sandbox {
		t.Fatalf("alipay checkout %+v %v", sess, err)
	}
}

func TestWechatDecryptAndNotify(t *testing.T) {
	key := strings.Repeat("k", 32)
	plain := []byte(`{"out_trade_no":"pay_wx","trade_state":"SUCCESS","transaction_id":"wx1","mchid":"mch"}`)
	nonce := []byte("123456789012")
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	ct := gcm.Seal(nil, nonce, plain, []byte("transaction"))
	got, err := wechatDecryptResource(key, base64.StdEncoding.EncodeToString(ct), string(nonce), "transaction")
	if err != nil || string(got) != string(plain) {
		t.Fatalf("decrypt %s %v", got, err)
	}
	priv, pub := mustRSA(t)
	outer := map[string]any{
		"id": "EV1", "event_type": "TRANSACTION.SUCCESS",
		"resource": map[string]any{
			"ciphertext": base64.StdEncoding.EncodeToString(ct), "nonce": string(nonce), "associated_data": "transaction",
		},
	}
	body, _ := json.Marshal(outer)
	ts := fmt.Sprintf("%d", time.Now().Unix())
	nonceStr := "n1"
	msg := ts + "\n" + nonceStr + "\n" + string(body) + "\n"
	sig, err := wechatSign(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := wechatDriver{}.ParseWebhook(context.Background(), WebhookRequest{
		Body: body,
		Headers: http.Header{
			"Wechatpay-Signature": []string{sig},
			"Wechatpay-Timestamp": []string{ts},
			"Wechatpay-Nonce":     []string{nonceStr},
		},
		Credentials: map[string]string{"api_v3_key": key, "wechat_public_key": pub},
	})
	if err != nil || !ev.SignatureValid || ev.OrderID != "pay_wx" || ev.Status != StatusPaid {
		t.Fatalf("wechat notify %+v %v", ev, err)
	}
}

func TestWechatLiveNative(t *testing.T) {
	priv, _ := mustRSA(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/pay/transactions/native" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code_url":"weixin://wxpay/bizpayurl?pr=x"}`))
	}))
	defer srv.Close()
	old := wechatAPIBase
	wechatAPIBase = srv.URL
	defer func() { wechatAPIBase = old }()
	sess, err := wechatDriver{}.CreateCheckout(context.Background(), CheckoutRequest{
		Mode: ModeLive, PublicBase: "https://oem.example",
		Credentials: map[string]string{
			"app_id": "wx", "mch_id": "mch", "merchant_api_private_key": priv, "cert_serial": "ABC",
		},
		Order: &OrderView{ID: "pay_wx", AmountMinor: 100},
	})
	if err != nil || sess == nil || sess.QRCode == "" || sess.Sandbox {
		t.Fatalf("wechat checkout %+v %v", sess, err)
	}
}

func TestOfficialLiveRequiresMode(t *testing.T) {
	creds := map[string]string{"secret_key": "sk"}
	if officialLive(ModeSandbox, creds, "secret_key") {
		t.Fatal("sandbox must not go live")
	}
	if !officialLive(ModeLive, creds, "secret_key") {
		t.Fatal("live+key should be live")
	}
}

func mustRSA(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	priv := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	return string(priv), string(pub)
}
