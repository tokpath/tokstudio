package payment

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	alipayGateway = "https://openapi.alipay.com/gateway.do"
	alipayHTTP    = &http.Client{Timeout: 15 * time.Second}
)

// alipayDriver 支付宝官方直连。live 走预下单；sandbox 仍用 HMAC webhook。
type alipayDriver struct{ sandboxDriver }

func (alipayDriver) Spec() AdapterSpec {
	return AdapterSpec{
		ID: AdapterAlipay, DisplayName: "支付宝", Kind: KindOfficial,
		PayCurrency: "CNY", CheckoutMode: CheckoutQR, BrandColor: "#02A9F1",
		UserFacing: true, MerchantKeys: []string{"app_id"},
		Credentials: []CredentialFieldView{
			{Key: "app_id", Label: "AppID", Required: true},
			{Key: "app_private_key", Label: "应用私钥", Secret: true, Required: true},
			{Key: "alipay_public_key", Label: "支付宝公钥", Secret: true, Required: true},
		},
	}
}

func (d alipayDriver) Test(ctx context.Context, in TestInput) error {
	if missing := missingFromSpec(d.Spec(), in.Credentials); len(missing) > 0 {
		return ErrInstanceIncomplete
	}
	if !officialLive(in.Mode, in.Credentials, "app_id", "app_private_key") {
		return d.sandboxDriver.Test(ctx, in)
	}
	_, err := alipayCall(ctx, in.Credentials, "alipay.trade.query", map[string]string{"out_trade_no": "probe"})
	if err != nil {
		return ErrProviderFailed
	}
	return nil
}

func (d alipayDriver) CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	if !officialLive(in.Mode, in.Credentials, "app_id", "app_private_key") {
		return d.sandboxCheckout(d.Spec(), in), nil
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	notify := WebhookURL(strings.TrimRight(in.PublicBase, "/"), AdapterAlipay)
	biz, _ := json.Marshal(map[string]any{
		"out_trade_no": in.Order.ID,
		"total_amount": fenToYuan(in.Order.AmountMinor),
		"subject":      "TokenHub " + in.Order.ID,
	})
	resp, err := alipayCall(ctx, in.Credentials, "alipay.trade.precreate", map[string]string{
		"biz_content": string(biz),
		"notify_url":  notify,
	})
	if err != nil || !alipayOK(resp) {
		return nil, ErrProviderFailed
	}
	qr := asString(resp["qr_code"])
	if qr == "" {
		return nil, ErrProviderFailed
	}
	return &CheckoutSession{
		Mode: CheckoutQR, Sandbox: false,
		WebhookURL: notify,
		QRCode:     qr,
	}, nil
}

func (d alipayDriver) ParseWebhook(_ context.Context, in WebhookRequest) (*WebhookEvent, error) {
	vals, err := url.ParseQuery(string(in.Body))
	if err != nil || vals.Get("out_trade_no") == "" || vals.Get("sign") == "" {
		return d.sandboxDriver.ParseWebhook(context.Background(), in)
	}
	status := StatusPending
	switch vals.Get("trade_status") {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		status = StatusPaid
	case "TRADE_CLOSED":
		status = StatusFailed
	}
	valid := false
	if pub := cred(in.Credentials, "alipay_public_key"); pub != "" {
		valid = alipayVerify(pub, vals)
	}
	eventID := vals.Get("notify_id")
	if eventID == "" {
		eventID = vals.Get("trade_no")
	}
	return &WebhookEvent{
		ExternalEventID: eventID, OrderID: vals.Get("out_trade_no"), Status: status,
		TradeID: vals.Get("trade_no"), SignatureValid: valid,
	}, nil
}

func (d alipayDriver) QueryOrder(ctx context.Context, in QueryRequest) (*QueryResult, error) {
	if !officialLive(in.Mode, in.Credentials, "app_id", "app_private_key") {
		return d.sandboxDriver.QueryOrder(ctx, in)
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	resp, err := alipayCall(ctx, in.Credentials, "alipay.trade.query", map[string]string{"out_trade_no": in.Order.ID})
	if err != nil || !alipayOK(resp) {
		return nil, ErrProviderFailed
	}
	st := asString(resp["trade_status"])
	out := StatusPending
	if st == "TRADE_SUCCESS" || st == "TRADE_FINISHED" {
		out = StatusPaid
	}
	return &QueryResult{Status: out, TradeID: asString(resp["trade_no"])}, nil
}

func (d alipayDriver) Refund(ctx context.Context, in RefundRequest) (*RefundResult, error) {
	if !officialLive(in.Mode, in.Credentials, "app_id", "app_private_key") {
		return d.sandboxDriver.Refund(ctx, in)
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	biz, _ := json.Marshal(map[string]any{
		"out_trade_no":   in.Order.ID,
		"refund_amount":  fenToYuan(in.Order.AmountMinor),
		"out_request_no": in.Order.ID + "-rf",
	})
	resp, err := alipayCall(ctx, in.Credentials, "alipay.trade.refund", map[string]string{"biz_content": string(biz)})
	if err != nil || !alipayOK(resp) {
		return nil, ErrProviderFailed
	}
	return &RefundResult{Status: StatusRefunded, TradeID: asString(resp["trade_no"])}, nil
}

func fenToYuan(fen int64) string {
	return strconv.FormatFloat(float64(fen)/100, 'f', 2, 64)
}

func alipayOK(resp map[string]any) bool {
	code := asString(resp["code"])
	return code == "" || code == "10000"
}

func alipayBeijingNow() string {
	return time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05")
}

func alipayCall(ctx context.Context, creds map[string]string, method string, extra map[string]string) (map[string]any, error) {
	vals := url.Values{}
	vals.Set("app_id", cred(creds, "app_id"))
	vals.Set("method", method)
	vals.Set("format", "JSON")
	vals.Set("charset", "utf-8")
	vals.Set("sign_type", "RSA2")
	vals.Set("timestamp", alipayBeijingNow())
	vals.Set("version", "1.0")
	for k, v := range extra {
		vals.Set(k, v)
	}
	sign, err := alipaySign(cred(creds, "app_private_key"), alipaySignContent(vals))
	if err != nil {
		return nil, err
	}
	vals.Set("sign", sign)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, alipayGateway, strings.NewReader(vals.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := alipayHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxProviderBody))
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	for k, v := range envelope {
		if strings.HasSuffix(k, "_response") {
			if m, ok := v.(map[string]any); ok {
				return m, nil
			}
		}
	}
	return envelope, nil
}

func alipaySignContent(vals url.Values) string {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		if k == "sign" || k == "sign_type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+vals.Get(k))
	}
	return strings.Join(parts, "&")
}

func alipaySign(pemRaw, content string) (string, error) {
	key, err := parseRSAPrivateKey(pemRaw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func alipayVerify(pemRaw string, vals url.Values) bool {
	pub, err := parseRSAPublicKey(pemRaw)
	if err != nil {
		return false
	}
	sigRaw, err := base64.StdEncoding.DecodeString(vals.Get("sign"))
	if err != nil {
		return false
	}
	sum := sha256.Sum256([]byte(alipaySignContent(vals)))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sigRaw) == nil
}
