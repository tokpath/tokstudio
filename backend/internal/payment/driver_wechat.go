package payment

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// wechatDriver 微信支付官方直连。live 走 APIv3 Native；sandbox 仍用 HMAC webhook。
type wechatDriver struct{ sandboxDriver }

func (wechatDriver) Spec() AdapterSpec {
	return AdapterSpec{
		ID: AdapterWechat, DisplayName: "微信支付", Kind: KindOfficial,
		PayCurrency: "CNY", CheckoutMode: CheckoutQR, BrandColor: "#09BB07",
		UserFacing: true, MerchantKeys: []string{"mch_id", "app_id"},
		Credentials: []CredentialFieldView{
			{Key: "app_id", Label: "AppID", Required: true},
			{Key: "mch_id", Label: "商户号", Required: true},
			{Key: "merchant_api_private_key", Label: "商户 API 私钥", Secret: true, Required: true},
			{Key: "api_v3_key", Label: "APIv3 密钥", Secret: true, Required: true},
			{Key: "wechat_public_key", Label: "微信支付公钥", Secret: true, Required: true},
			{Key: "public_key_id", Label: "公钥 ID", Required: true},
			{Key: "cert_serial", Label: "证书序列号", Required: true},
		},
	}
}

func (d wechatDriver) Test(ctx context.Context, in TestInput) error {
	if missing := missingFromSpec(d.Spec(), in.Credentials); len(missing) > 0 {
		return ErrInstanceIncomplete
	}
	if !officialLive(in.Mode, in.Credentials, "mch_id", "merchant_api_private_key", "cert_serial") {
		return d.sandboxDriver.Test(ctx, in)
	}
	_, code, err := wechatDo(ctx, in.Credentials, http.MethodGet, "/v3/certificates", nil)
	if err != nil || code >= 300 {
		return ErrProviderFailed
	}
	return nil
}

func (d wechatDriver) CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	if !officialLive(in.Mode, in.Credentials, "mch_id", "app_id", "merchant_api_private_key", "cert_serial") {
		return d.sandboxCheckout(d.Spec(), in), nil
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	notify := WebhookURL(strings.TrimRight(in.PublicBase, "/"), AdapterWechat)
	body := encodeJSON(map[string]any{
		"appid":        cred(in.Credentials, "app_id"),
		"mchid":        cred(in.Credentials, "mch_id"),
		"description":  "TokenHub " + in.Order.ID,
		"out_trade_no": in.Order.ID,
		"notify_url":   notify,
		"amount":       map[string]any{"total": in.Order.AmountMinor, "currency": "CNY"},
	})
	raw, code, err := wechatDo(ctx, in.Credentials, http.MethodPost, "/v3/pay/transactions/native", body)
	if err != nil || code >= 300 {
		return nil, ErrProviderFailed
	}
	obj := decodeJSONMap(raw)
	qr := asString(obj["code_url"])
	if qr == "" {
		return nil, ErrProviderFailed
	}
	return &CheckoutSession{
		Mode: CheckoutQR, Sandbox: false, WebhookURL: notify, QRCode: qr,
	}, nil
}

func (d wechatDriver) ParseWebhook(_ context.Context, in WebhookRequest) (*WebhookEvent, error) {
	sig := ""
	if in.Headers != nil {
		sig = in.Headers.Get("Wechatpay-Signature")
	}
	if sig == "" {
		return d.sandboxDriver.ParseWebhook(context.Background(), in)
	}
	outer, resource := wechatNotifyResource(in.Body)
	if outer == nil || resource == nil {
		return nil, ErrInvalidEvent
	}
	plain, err := wechatDecryptResource(
		cred(in.Credentials, "api_v3_key"),
		asString(resource["ciphertext"]),
		asString(resource["nonce"]),
		asString(resource["associated_data"]),
	)
	if err != nil {
		return nil, err
	}
	inner := decodeJSONMap(plain)
	if inner == nil {
		return nil, ErrInvalidEvent
	}
	valid := wechatVerify(
		cred(in.Credentials, "wechat_public_key"),
		in.Headers.Get("Wechatpay-Timestamp"),
		in.Headers.Get("Wechatpay-Nonce"),
		sig,
		in.Body,
	)
	status, orderID, tradeID, eventID := wechatEventStatus(outer, inner)
	if eventID == "" {
		return nil, ErrInvalidEvent
	}
	return &WebhookEvent{
		ExternalEventID: eventID, OrderID: orderID, Status: status, TradeID: tradeID,
		MerchantID: asString(inner["mchid"]), SignatureValid: valid,
	}, nil
}

func (d wechatDriver) QueryOrder(ctx context.Context, in QueryRequest) (*QueryResult, error) {
	if !officialLive(in.Mode, in.Credentials, "mch_id", "merchant_api_private_key", "cert_serial") {
		return d.sandboxDriver.QueryOrder(ctx, in)
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(in.Order.ID) + "?mchid=" + url.QueryEscape(cred(in.Credentials, "mch_id"))
	raw, code, err := wechatDo(ctx, in.Credentials, http.MethodGet, path, nil)
	if err != nil || code >= 300 {
		return nil, ErrProviderFailed
	}
	obj := decodeJSONMap(raw)
	st := asString(obj["trade_state"])
	out := StatusPending
	if st == "SUCCESS" {
		out = StatusPaid
	}
	if st == "CLOSED" || st == "PAYERROR" {
		out = StatusFailed
	}
	return &QueryResult{Status: out, TradeID: asString(obj["transaction_id"])}, nil
}

func (d wechatDriver) Refund(ctx context.Context, in RefundRequest) (*RefundResult, error) {
	if !officialLive(in.Mode, in.Credentials, "mch_id", "merchant_api_private_key", "cert_serial") {
		return d.sandboxDriver.Refund(ctx, in)
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	body := encodeJSON(map[string]any{
		"out_trade_no":  in.Order.ID,
		"out_refund_no": in.Order.ID + "-rf",
		"amount": map[string]any{
			"refund": in.Order.AmountMinor, "total": in.Order.AmountMinor, "currency": "CNY",
		},
	})
	raw, code, err := wechatDo(ctx, in.Credentials, http.MethodPost, "/v3/refund/domestic/refunds", body)
	if err != nil || code >= 300 {
		return nil, ErrProviderFailed
	}
	obj := decodeJSONMap(raw)
	return &RefundResult{Status: StatusRefunded, TradeID: asString(obj["refund_id"])}, nil
}
