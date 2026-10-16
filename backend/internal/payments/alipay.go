package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

// Alipay implements the official OpenAPI 2.0 RSA2 protocol. API responses are
// verified over the exact raw *_response bytes; reconstructing JSON is unsafe.
// https://github.com/alipay/alipay-sdk-nodejs-all/blob/master/src/util.ts
type Alipay struct{ HTTP *Transport }

func (Alipay) Name() string { return "alipay" }
func (Alipay) Validate(m Merchant) []string {
	missing := required(m, "app_id", "app_private_key", "alipay_public_key", "seller_id")
	if _, e := privateRSA(m.Credentials["app_private_key"]); e != nil {
		missing = append(missing, "valid_app_private_key")
	}
	if _, e := publicRSA(m.Credentials["alipay_public_key"]); e != nil {
		missing = append(missing, "valid_alipay_public_key")
	}
	return missing
}
func alipayCanonical(values url.Values, notification bool) string {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if len(value) != 1 {
			return ""
		}
		if key == "sign" || (notification && key == "sign_type") || value[0] == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	return strings.Join(parts, "&")
}
func (d Alipay) call(ctx context.Context, m Merchant, method string, biz any, notify string) (json.RawMessage, error) {
	base, e := d.HTTP.endpoint(m, "https://openapi.alipay.com")
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(biz)
	if e != nil {
		return nil, e
	}
	values := url.Values{"app_id": {m.Credentials["app_id"]}, "method": {method}, "format": {"JSON"}, "charset": {"utf-8"}, "sign_type": {"RSA2"}, "timestamp": {time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02 15:04:05")}, "version": {"1.0"}, "biz_content": {string(raw)}}
	if notify != "" {
		values.Set("notify_url", notify)
	}
	signature, e := signRSA(m.Credentials["app_private_key"], alipayCanonical(values, false))
	if e != nil {
		return nil, e
	}
	values.Set("sign", signature)
	response, _, e := d.HTTP.do(ctx, http.MethodPost, base+"/gateway.do", []byte(values.Encode()), http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}})
	if e != nil {
		return nil, e
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(response, &envelope) != nil {
		return nil, ErrOutcomeUnknown
	}
	key := strings.ReplaceAll(method, ".", "_") + "_response"
	body, exists := envelope[key]
	if !exists {
		body = envelope["error_response"]
	}
	var signed string
	if json.Unmarshal(envelope["sign"], &signed) != nil || !verifyRSA(m.Credentials["alipay_public_key"], string(body), signed) {
		return nil, ErrSignature
	}
	var result struct{ Code string }
	if json.Unmarshal(body, &result) != nil {
		return nil, ErrOutcomeUnknown
	}
	if result.Code != "10000" {
		return nil, ErrProvider
	}
	return body, nil
}
func (d Alipay) Create(ctx context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, d); e != nil {
		return Checkout{}, e
	}
	if o.Validate(m) != nil || o.Money.Currency != "CNY" {
		return Checkout{}, ErrInvalid
	}
	if _, e := o.Money.Minor(2); e != nil {
		return Checkout{}, e
	}
	raw, e := d.call(ctx, m, "alipay.trade.precreate", map[string]string{"out_trade_no": o.ID, "total_amount": o.Money.Amount.Rat().FloatString(2), "subject": o.Description}, o.NotifyURL)
	if e != nil {
		return Checkout{}, e
	}
	var body struct {
		OutTradeNo string `json:"out_trade_no"`
		QRCode     string `json:"qr_code"`
	}
	if json.Unmarshal(raw, &body) != nil || body.OutTradeNo != o.ID || body.QRCode == "" {
		return Checkout{}, ErrOutcomeUnknown
	}
	return Checkout{ProviderID: o.ID, State: "pending", URL: body.QRCode}, nil
}
func (d Alipay) Lookup(ctx context.Context, m Merchant, o Order) (Fact, error) {
	if e := ready(m, d); e != nil {
		return Fact{}, e
	}
	if o.Validate(m) != nil {
		return Fact{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, "alipay.trade.query", map[string]string{"out_trade_no": o.ID}, "")
	if e != nil {
		return Fact{}, e
	}
	var body struct {
		OutTradeNo  string `json:"out_trade_no"`
		TradeNo     string `json:"trade_no"`
		TradeStatus string `json:"trade_status"`
		TotalAmount string `json:"total_amount"`
		SendPayDate string `json:"send_pay_date"`
	}
	if json.Unmarshal(raw, &body) != nil || body.OutTradeNo != o.ID || body.TradeNo == "" {
		return Fact{}, ErrConflict
	}
	if body.TradeStatus != "TRADE_SUCCESS" && body.TradeStatus != "TRADE_FINISHED" {
		return Fact{}, ErrNotFinal
	}
	amount, e := ledger.ParseDecimal(body.TotalAmount)
	if e != nil {
		return Fact{}, ErrInvalid
	}
	var at *time.Time
	if parsed, e := time.ParseInLocation("2006-01-02 15:04:05", body.SendPayDate, time.FixedZone("Asia/Shanghai", 8*3600)); e == nil {
		at = &parsed
	}
	return receiptFact(m, o, body.TradeNo, Money{Currency: "CNY", Amount: amount}, at, map[string]any{"out_trade_no": body.OutTradeNo, "trade_no": body.TradeNo, "trade_status": body.TradeStatus, "total_amount": body.TotalAmount, "send_pay_date": body.SendPayDate, "response_signature_verified": true})
}
func (d Alipay) Refund(ctx context.Context, m Merchant, r Refund) (RefundStatus, error) {
	if e := ready(m, d); e != nil {
		return RefundStatus{}, e
	}
	if r.Validate(m) != nil || r.Money.Currency != "CNY" {
		return RefundStatus{}, ErrInvalid
	}
	if _, e := r.Money.Minor(2); e != nil {
		return RefundStatus{}, e
	}
	raw, e := d.call(ctx, m, "alipay.trade.refund", map[string]string{"out_trade_no": r.Order.ID, "out_request_no": r.ID, "refund_amount": r.Money.Amount.Rat().FloatString(2)}, "")
	if e != nil {
		return RefundStatus{State: "outcome_unknown"}, e
	}
	return alipayRefundResult(m, r, raw, false)
}
func (d Alipay) LookupRefund(ctx context.Context, m Merchant, r Refund) (RefundStatus, error) {
	if e := ready(m, d); e != nil {
		return RefundStatus{}, e
	}
	if r.Validate(m) != nil {
		return RefundStatus{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, "alipay.trade.fastpay.refund.query", map[string]string{"out_trade_no": r.Order.ID, "out_request_no": r.ID}, "")
	if e != nil {
		return RefundStatus{}, e
	}
	return alipayRefundResult(m, r, raw, true)
}
func alipayRefundResult(m Merchant, r Refund, raw []byte, query bool) (RefundStatus, error) {
	var body struct {
		OutTradeNo   string `json:"out_trade_no"`
		OutRequestNo string `json:"out_request_no"`
		TradeNo      string `json:"trade_no"`
		RefundFee    string `json:"refund_fee"`
		RefundAmount string `json:"refund_amount"`
		FundChange   string `json:"fund_change"`
		RefundStatus string `json:"refund_status"`
	}
	if json.Unmarshal(raw, &body) != nil || body.OutTradeNo != r.Order.ID || body.TradeNo == "" || (query && body.OutRequestNo != r.ID) {
		return RefundStatus{}, ErrConflict
	}
	out := RefundStatus{ProviderID: r.ID, State: "processing"}
	final := (query && body.RefundStatus == "REFUND_SUCCESS") || (!query && body.FundChange == "Y")
	if !final {
		return out, nil
	}
	amountText := body.RefundAmount
	if amountText == "" {
		amountText = body.RefundFee
	}
	amount, e := ledger.ParseDecimal(amountText)
	if e != nil || amount.Cmp(r.Money.Amount) != 0 {
		return out, ErrConflict
	}
	fact, e := refundFact(m, r, r.ID, Money{Currency: "CNY", Amount: amount}, nil, map[string]any{"trade_no": body.TradeNo, "out_request_no": r.ID, "refund_amount": amountText, "response_signature_verified": true})
	if e != nil {
		return out, e
	}
	out.State = "succeeded"
	out.Fact = &fact
	return out, nil
}
func (d Alipay) VerifyWebhook(_ context.Context, m Merchant, w Webhook) (Hint, error) {
	if e := ready(m, d); e != nil {
		return Hint{}, e
	}
	if len(w.Body) > 1<<20 {
		return Hint{}, ErrInvalid
	}
	values, e := url.ParseQuery(string(w.Body))
	if e != nil || values.Get("sign_type") != "RSA2" || values.Get("app_id") != m.Credentials["app_id"] || values.Get("seller_id") != m.Credentials["seller_id"] || !verifyRSA(m.Credentials["alipay_public_key"], alipayCanonical(values, true), values.Get("sign")) {
		return Hint{}, ErrSignature
	}
	if values.Get("notify_id") == "" || values.Get("out_trade_no") == "" || values.Get("trade_no") == "" {
		return Hint{}, ErrInvalid
	}
	if values.Get("trade_status") != "TRADE_SUCCESS" && values.Get("trade_status") != "TRADE_FINISHED" {
		return Hint{}, ErrNotFinal
	}
	var at *time.Time
	if parsed, e := time.ParseInLocation("2006-01-02 15:04:05", values.Get("gmt_payment"), time.FixedZone("Asia/Shanghai", 8*3600)); e == nil {
		at = &parsed
	}
	return Hint{EventID: values.Get("notify_id"), Kind: "receipt", OrderID: values.Get("out_trade_no"), ProviderID: values.Get("trade_no"), OccurredAt: at, SignatureVerified: true, Final: true}, nil
}
