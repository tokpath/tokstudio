package payment

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStripeWebhookActualTerminalStates(t *testing.T) {
	for _, test := range []struct{ name, typ, body, want string }{
		{"complete unpaid", "checkout.session.completed", `"payment_status":"unpaid"`, ""},
		{"complete no cash", "checkout.session.completed", `"payment_status":"no_payment_required"`, ""},
		{"complete paid", "checkout.session.completed", `"payment_status":"paid"`, StatusPaid},
		{"async paid", "checkout.session.async_payment_succeeded", `"payment_status":"paid"`, StatusPaid},
		{"refund pending", "refund.created", `"status":"pending","amount":100,"currency":"usd"`, StatusRefunding},
		{"refund action", "refund.updated", `"status":"requires_action","amount":100,"currency":"usd"`, StatusRefunding},
		{"refund failed", "refund.failed", `"status":"failed","amount":100,"currency":"usd"`, StatusRefundFailed},
		{"refund succeeded", "refund.updated", `"status":"succeeded","amount":100,"currency":"usd"`, StatusRefunded},
		{"charge partial", "charge.refunded", `"refunded":false,"amount":100,"amount_refunded":20,"currency":"usd"`, StatusRefundPartial},
		{"charge full", "charge.refunded", `"refunded":true,"amount":100,"amount_refunded":100,"currency":"usd"`, StatusRefunded},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"id":"evt_test","type":%q,"data":{"object":{"id":"obj_test","payment_intent":"pi_original","metadata":{"order_id":"pay_original"},%s}}}`, test.typ, test.body))
			ts := fmt.Sprint(time.Now().Unix())
			mac := hmac.New(sha256.New, []byte("local-whsec"))
			_, _ = fmt.Fprintf(mac, "%s.%s", ts, body)
			ev, err := (stripeDriver{}).ParseWebhook(context.Background(), WebhookRequest{Body: body, Headers: http.Header{"Stripe-Signature": {"t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))}}, Credentials: map[string]string{"webhook_secret": "local-whsec"}})
			if err != nil || !ev.SignatureValid || ev.Status != test.want || ev.TradeID != "pi_original" || ev.OrderID != "pay_original" {
				t.Fatalf("event %+v error %v", ev, err)
			}
		})
	}
}

func TestWechatRefundEncryptedActualStates(t *testing.T) {
	priv, pub := mustRSA(t)
	key := strings.Repeat("k", 32)
	for _, test := range []struct{ state, want string }{{"SUCCESS", StatusRefunded}, {"PROCESSING", StatusRefunding}, {"CLOSED", StatusRefundFailed}, {"ABNORMAL", StatusRefundFailed}} {
		t.Run(test.state, func(t *testing.T) {
			plain := []byte(fmt.Sprintf(`{"out_trade_no":"pay_original","transaction_id":"wx_original","refund_status":%q,"amount":{"refund":100,"total":100,"currency":"CNY"}}`, test.state))
			block, err := aes.NewCipher([]byte(key))
			if err != nil {
				t.Fatal(err)
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				t.Fatal(err)
			}
			nonce := "123456789012"
			ct := gcm.Seal(nil, []byte(nonce), plain, []byte("refund"))
			body, _ := json.Marshal(map[string]any{"id": "evt_refund", "event_type": "REFUND." + test.state, "resource": map[string]any{"nonce": nonce, "associated_data": "refund", "ciphertext": base64.StdEncoding.EncodeToString(ct)}})
			ts := fmt.Sprint(time.Now().Unix())
			sig, err := wechatSign(priv, ts+"\nnotify\n"+string(body)+"\n")
			if err != nil {
				t.Fatal(err)
			}
			ev, err := (wechatDriver{}).ParseWebhook(context.Background(), WebhookRequest{Body: body, Headers: http.Header{"Wechatpay-Signature": {sig}, "Wechatpay-Timestamp": {ts}, "Wechatpay-Nonce": {"notify"}}, Credentials: map[string]string{"api_v3_key": key, "wechat_public_key": pub}})
			if err != nil || !ev.SignatureValid || ev.Status != test.want || ev.RefundAmountMinor == nil || *ev.RefundAmountMinor != 100 || ev.TradeID != "wx_original" {
				t.Fatalf("event %+v error %v", ev, err)
			}
		})
	}
}
