package payments

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Wechat implements APIv3 Native payments. Both HTTP replies and encrypted
// callbacks require the configured platform serial and RSA signature.
type Wechat struct{ HTTP *Transport }

func (Wechat) Name() string { return "wechat" }
func (Wechat) Validate(m Merchant) []string {
	missing := required(m, "app_id", "mch_id", "merchant_api_private_key", "cert_serial", "api_v3_key", "wechat_public_key", "public_key_id")
	if _, e := privateRSA(m.Credentials["merchant_api_private_key"]); e != nil {
		missing = append(missing, "valid_merchant_api_private_key")
	}
	if _, e := publicRSA(m.Credentials["wechat_public_key"]); e != nil {
		missing = append(missing, "valid_wechat_public_key")
	}
	if len(m.Credentials["api_v3_key"]) != 32 {
		missing = append(missing, "api_v3_key_32_bytes")
	}
	return missing
}
func wechatPlatformKey(m Merchant, serial string) string {
	if serial != "" && serial == m.Credentials["public_key_id"] {
		return m.Credentials["wechat_public_key"]
	}
	var previous map[string]string
	if json.Unmarshal([]byte(m.Credentials["previous_platform_public_keys"]), &previous) == nil {
		return previous[serial]
	}
	return ""
}
func wechatVerifyHeaders(m Merchant, headers http.Header, body []byte, at time.Time) bool {
	timestamp := headers.Get("Wechatpay-Timestamp")
	unix, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil || at.Unix()-unix > 300 || unix-at.Unix() > 30 {
		return false
	}
	nonce := headers.Get("Wechatpay-Nonce")
	if nonce == "" {
		return false
	}
	key := wechatPlatformKey(m, headers.Get("Wechatpay-Serial"))
	return key != "" && verifyRSA(key, timestamp+"\n"+nonce+"\n"+string(body)+"\n", headers.Get("Wechatpay-Signature"))
}
func (d Wechat) call(ctx context.Context, m Merchant, method, path string, payload any) ([]byte, error) {
	base, e := d.HTTP.endpoint(m, "https://api.mch.weixin.qq.com")
	if e != nil {
		return nil, e
	}
	var body []byte
	if payload != nil {
		body, e = json.Marshal(payload)
		if e != nil {
			return nil, e
		}
	}
	buffer := make([]byte, 16)
	if _, e = rand.Read(buffer); e != nil {
		return nil, e
	}
	nonce := hex.EncodeToString(buffer)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature, e := signRSA(m.Credentials["merchant_api_private_key"], method+"\n"+path+"\n"+timestamp+"\n"+nonce+"\n"+string(body)+"\n")
	if e != nil {
		return nil, e
	}
	auth := fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",timestamp="%s",serial_no="%s",signature="%s"`, m.Credentials["mch_id"], nonce, timestamp, m.Credentials["cert_serial"], signature)
	headers := http.Header{"Authorization": []string{auth}, "Content-Type": []string{"application/json"}, "Accept": []string{"application/json"}, "Wechatpay-Serial": []string{m.Credentials["public_key_id"]}}
	raw, responseHeaders, e := d.HTTP.do(ctx, method, base+path, body, headers)
	if e != nil {
		return nil, e
	}
	if !wechatVerifyHeaders(m, responseHeaders, raw, time.Now().UTC()) {
		return nil, ErrSignature
	}
	return raw, nil
}
func (d Wechat) Create(ctx context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, d); e != nil {
		return Checkout{}, e
	}
	if o.Validate(m) != nil || o.Money.Currency != "CNY" || o.NotifyURL == "" {
		return Checkout{}, ErrInvalid
	}
	minor, e := o.Money.Minor(2)
	if e != nil {
		return Checkout{}, e
	}
	raw, e := d.call(ctx, m, http.MethodPost, "/v3/pay/transactions/native", map[string]any{"appid": m.Credentials["app_id"], "mchid": m.Credentials["mch_id"], "description": o.Description, "out_trade_no": o.ID, "time_expire": o.ExpiresAt.Format(time.RFC3339), "notify_url": o.NotifyURL, "amount": map[string]any{"total": minor, "currency": "CNY"}})
	if e != nil {
		return Checkout{}, e
	}
	var response struct {
		CodeURL string `json:"code_url"`
	}
	if json.Unmarshal(raw, &response) != nil || response.CodeURL == "" {
		return Checkout{}, ErrOutcomeUnknown
	}
	return Checkout{ProviderID: o.ID, State: "pending", URL: response.CodeURL}, nil
}

type wechatTransaction struct {
	AppID         string `json:"appid"`
	MerchantID    string `json:"mchid"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	State         string `json:"trade_state"`
	SuccessTime   string `json:"success_time"`
	Amount        struct {
		Total    int64
		Currency string
	}
	Payer struct {
		OpenID string `json:"openid"`
	}
}

func (d Wechat) Lookup(ctx context.Context, m Merchant, o Order) (Fact, error) {
	if e := ready(m, d); e != nil {
		return Fact{}, e
	}
	if o.Validate(m) != nil {
		return Fact{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, http.MethodGet, "/v3/pay/transactions/out-trade-no/"+url.PathEscape(o.ID)+"?mchid="+url.QueryEscape(m.Credentials["mch_id"]), nil)
	if e != nil {
		return Fact{}, e
	}
	var response wechatTransaction
	if json.Unmarshal(raw, &response) != nil || response.AppID != m.Credentials["app_id"] || response.MerchantID != m.Credentials["mch_id"] || response.OutTradeNo != o.ID || response.TransactionID == "" {
		return Fact{}, ErrConflict
	}
	if response.State != "SUCCESS" {
		return Fact{}, ErrNotFinal
	}
	money, e := fromMinor(response.Amount.Currency, response.Amount.Total, 2)
	if e != nil {
		return Fact{}, e
	}
	var at *time.Time
	if parsed, e := time.Parse(time.RFC3339, response.SuccessTime); e == nil {
		at = &parsed
	}
	return receiptFact(m, o, response.TransactionID, money, at, map[string]any{"transaction_id": response.TransactionID, "out_trade_no": response.OutTradeNo, "mchid": response.MerchantID, "appid": response.AppID, "total": response.Amount.Total, "currency": response.Amount.Currency, "success_time": response.SuccessTime, "response_signature_verified": true})
}
func (d Wechat) Refund(ctx context.Context, m Merchant, r Refund) (RefundStatus, error) {
	if e := ready(m, d); e != nil {
		return RefundStatus{}, e
	}
	if r.Validate(m) != nil || r.Money.Currency != "CNY" {
		return RefundStatus{}, ErrInvalid
	}
	refund, e := r.Money.Minor(2)
	if e != nil {
		return RefundStatus{}, e
	}
	total, e := r.Order.Money.Minor(2)
	if e != nil {
		return RefundStatus{}, e
	}
	raw, e := d.call(ctx, m, http.MethodPost, "/v3/refund/domestic/refunds", map[string]any{"out_trade_no": r.Order.ID, "out_refund_no": r.ID, "amount": map[string]any{"refund": refund, "total": total, "currency": "CNY"}})
	if e != nil {
		return RefundStatus{State: "outcome_unknown"}, e
	}
	return wechatRefundResult(m, r, raw)
}
func (d Wechat) LookupRefund(ctx context.Context, m Merchant, r Refund) (RefundStatus, error) {
	if e := ready(m, d); e != nil {
		return RefundStatus{}, e
	}
	if r.Validate(m) != nil {
		return RefundStatus{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, http.MethodGet, "/v3/refund/domestic/refunds/"+url.PathEscape(r.ID), nil)
	if e != nil {
		return RefundStatus{}, e
	}
	return wechatRefundResult(m, r, raw)
}
func wechatRefundResult(m Merchant, r Refund, raw []byte) (RefundStatus, error) {
	var response struct {
		RefundID    string `json:"refund_id"`
		OutRefundNo string `json:"out_refund_no"`
		OutTradeNo  string `json:"out_trade_no"`
		Status      string
		SuccessTime string `json:"success_time"`
		Amount      struct {
			Refund, Total int64
			Currency      string
		}
	}
	if json.Unmarshal(raw, &response) != nil || response.RefundID == "" || response.OutRefundNo != r.ID || response.OutTradeNo != r.Order.ID {
		return RefundStatus{}, ErrConflict
	}
	out := RefundStatus{ProviderID: response.RefundID, State: strings.ToLower(response.Status)}
	if response.Status == "SUCCESS" {
		money, e := fromMinor(response.Amount.Currency, response.Amount.Refund, 2)
		if e != nil || money.Currency != r.Money.Currency || money.Amount.Cmp(r.Money.Amount) != 0 {
			return out, ErrConflict
		}
		total, e := r.Order.Money.Minor(2)
		if e != nil || total != response.Amount.Total {
			return out, ErrConflict
		}
		var at *time.Time
		if parsed, e := time.Parse(time.RFC3339, response.SuccessTime); e == nil {
			at = &parsed
		}
		fact, e := refundFact(m, r, response.RefundID, money, at, map[string]any{"refund_id": response.RefundID, "out_refund_no": r.ID, "refund": response.Amount.Refund, "total": response.Amount.Total, "currency": response.Amount.Currency, "status": response.Status, "response_signature_verified": true})
		if e != nil {
			return out, e
		}
		out.State = "succeeded"
		out.Fact = &fact
	}
	return out, nil
}
func (d Wechat) VerifyWebhook(_ context.Context, m Merchant, w Webhook) (Hint, error) {
	if e := ready(m, d); e != nil {
		return Hint{}, e
	}
	at := w.ReceivedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if len(w.Body) > 1<<20 || !wechatVerifyHeaders(m, w.Headers, w.Body, at) {
		return Hint{}, ErrSignature
	}
	var envelope struct {
		ID        string
		EventType string `json:"event_type"`
		Resource  struct {
			Algorithm, Ciphertext, Nonce string
			AssociatedData               string `json:"associated_data"`
		}
	}
	if json.Unmarshal(w.Body, &envelope) != nil || envelope.ID == "" || envelope.Resource.Algorithm != "AEAD_AES_256_GCM" {
		return Hint{}, ErrInvalid
	}
	encrypted, e := base64.StdEncoding.DecodeString(envelope.Resource.Ciphertext)
	if e != nil {
		return Hint{}, ErrInvalid
	}
	block, e := aes.NewCipher([]byte(m.Credentials["api_v3_key"]))
	if e != nil {
		return Hint{}, ErrNotConfigured
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil || len(envelope.Resource.Nonce) != gcm.NonceSize() || len(encrypted) < gcm.Overhead() {
		return Hint{}, ErrInvalid
	}
	plain, e := gcm.Open(nil, []byte(envelope.Resource.Nonce), encrypted, []byte(envelope.Resource.AssociatedData))
	if e != nil {
		return Hint{}, ErrSignature
	}
	if strings.HasPrefix(envelope.EventType, "REFUND.") {
		var refund struct {
			MerchantID  string `json:"mchid"`
			OutTradeNo  string `json:"out_trade_no"`
			OutRefundNo string `json:"out_refund_no"`
			RefundID    string `json:"refund_id"`
			Status      string `json:"refund_status"`
		}
		if json.Unmarshal(plain, &refund) != nil || refund.MerchantID != m.Credentials["mch_id"] || refund.OutTradeNo == "" || refund.OutRefundNo == "" || refund.RefundID == "" {
			return Hint{}, ErrConflict
		}
		return Hint{EventID: envelope.ID, Kind: "refund", OrderID: refund.OutTradeNo, ReferenceID: refund.OutRefundNo, ProviderID: refund.RefundID, SignatureVerified: true, Final: refund.Status == "SUCCESS"}, nil
	}
	var transaction wechatTransaction
	if json.Unmarshal(plain, &transaction) != nil || transaction.MerchantID != m.Credentials["mch_id"] || transaction.AppID != m.Credentials["app_id"] || transaction.OutTradeNo == "" {
		return Hint{}, ErrConflict
	}
	if envelope.EventType != "TRANSACTION.SUCCESS" || transaction.State != "SUCCESS" || transaction.TransactionID == "" {
		return Hint{}, ErrUnsupported
	}
	var occurred *time.Time
	if parsed, e := time.Parse(time.RFC3339, transaction.SuccessTime); e == nil {
		occurred = &parsed
	}
	return Hint{EventID: envelope.ID, Kind: "receipt", OrderID: transaction.OutTradeNo, ProviderID: transaction.TransactionID, OccurredAt: occurred, SignatureVerified: true, Final: true}, nil
}
