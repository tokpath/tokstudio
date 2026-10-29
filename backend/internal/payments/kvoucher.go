package payments

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

const kvoucherOfficialBase = "https://kvoucher.com/api/kv/merchant/v1"

// KVoucher collects a customer payment through a merchant's own KVoucher account.
// Gkash charges MYR. Retrieving the reload code is the receipt; the recorded
// amount remains the actual MYR transaction.
type KVoucher struct{ HTTP *Transport }

func (KVoucher) Name() string { return "kvoucher" }
func (KVoucher) Validate(m Merchant) []string {
	missing := required(m, "kvoucher_api_key", "kvoucher_api_secret")
	if channels := strings.TrimSpace(m.Credentials["refund_channels"]); channels != "" && len(normalizeRefundChannels(channels)) == 0 {
		missing = append(missing, "refund_channels")
	}
	return missing
}
func (k KVoucher) Create(ctx context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, k); e != nil {
		return Checkout{}, e
	}
	if o.Settlement().Currency != "MYR" {
		return Checkout{}, ErrUnsupported
	}
	if o.Validate(m) != nil || o.Money.Currency != "MYR" {
		return Checkout{}, ErrInvalid
	}
	if _, e := o.Money.Minor(2); e != nil {
		return Checkout{}, e
	}
	face := o.Money.Amount.String()
	created, e := k.order(ctx, m, http.MethodPost, "/orders", map[string]any{
		"custom_items":    []map[string]any{{"denomination": json.Number(face), "quantity": 1, "description": "Order " + o.ID}},
		"currency":        "MYR",
		"metadata":        map[string]string{"order_ref": o.ID},
		"idempotency_key": "tokpath:" + o.ID,
	})
	if e != nil {
		return Checkout{}, e
	}
	var order struct {
		ID     json.Number `json:"id"`
		Status string      `json:"status"`
	}
	if json.Unmarshal(created, &order) != nil || order.ID == "" {
		return Checkout{}, ErrProvider
	}
	paid, e := k.order(ctx, m, http.MethodPost, "/orders/"+url.PathEscape(order.ID.String())+"/gkash-pay", nil)
	if e != nil {
		return Checkout{}, e
	}
	form, e := gkashForm(paid)
	if e != nil {
		return Checkout{}, e
	}
	if form.Money == nil || form.Money.Currency != "MYR" || form.Money.Amount.Cmp(o.Settlement().Amount) != 0 {
		return Checkout{}, ErrUnsupported
	}
	return Checkout{ProviderID: order.ID.String(), State: "pending", URL: form.Action, FormFields: form.Fields, Instructions: "请在 Gkash 完成付款。付款币种为MYR，订单按实际核实的收款总额入账。付完后回到此页并核实原通道。", PayMoney: form.Money}, nil
}
func (k KVoucher) Lookup(ctx context.Context, m Merchant, o Order) (Fact, error) {
	if o.ProviderID == "" {
		return Fact{}, ErrNotFinal
	}
	raw, e := k.order(ctx, m, http.MethodGet, "/orders/"+url.PathEscape(o.ProviderID), nil)
	if e != nil {
		return Fact{}, e
	}
	var order struct {
		ID      json.Number `json:"id"`
		OrderNo string      `json:"order_no"`
		Status  string      `json:"status"`
		Items   []struct {
			Denomination json.Number `json:"denomination"`
			Qty          int         `json:"qty"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &order) != nil || order.Status == "" {
		return Fact{}, ErrProvider
	}
	switch order.Status {
	case "rejected", "refunded", "expired":
		return Fact{}, ErrProvider
	case "pending_payment", "payment_submitted":
		// A live Gkash payment moves the order out of these states before a code exists.
		// The sandbox never does that: it issues a dummy code without a completed payment.
		sandbox, e := k.sandbox(ctx, m)
		if e != nil {
			return Fact{}, e
		}
		if !sandbox {
			return Fact{}, ErrNotFinal
		}
	}
	if len(order.Items) != 1 || order.Items[0].Qty < 1 || order.Items[0].Denomination == "" {
		return Fact{}, ErrProvider
	}
	pins, e := k.pins(ctx, m, o.ID, order.Items[0].Denomination.String(), order.Items[0].Qty)
	if e != nil {
		return Fact{}, e
	}
	if len(pins) == 0 {
		return Fact{}, ErrNotFinal
	}
	now := time.Now().UTC()
	evidence := map[string]any{"kvoucher_order_id": order.ID.String(), "kvoucher_order_no": order.OrderNo, "status": order.Status, "pins": pins}
	amount, e := ledger.ParseDecimal(order.Items[0].Denomination.String())
	if e != nil {
		return Fact{}, ErrProvider
	}
	gross := Money{Currency: "MYR", Amount: amount}
	return receiptFact(m, o, order.ID.String(), gross, &now, evidence)
}
func (KVoucher) Refund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (KVoucher) LookupRefund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (KVoucher) VerifyWebhook(_ context.Context, m Merchant, w Webhook) (Hint, error) {
	secret := strings.TrimSpace(m.Credentials["webhook_secret"])
	if secret == "" || !kvoucherSignature(secret, w.Body, w.Headers.Get("X-Kvoucher-Signature")) {
		return Hint{}, ErrSignature
	}
	var event struct {
		EventID string `json:"event_id"`
		Event   string `json:"event"`
		Data    struct {
			ID       json.Number       `json:"id"`
			Metadata map[string]string `json:"metadata"`
		} `json:"data"`
	}
	if json.Unmarshal(w.Body, &event) != nil || event.EventID == "" || event.Event == "" {
		return Hint{}, ErrInvalid
	}
	hint := Hint{EventID: event.EventID, Kind: "receipt", ProviderID: event.Data.ID.String(), OrderID: event.Data.Metadata["order_ref"], SignatureVerified: true, Final: event.Event != "order.created" && event.Event != "test"}
	// order.created only means the order exists. Sandbox can already return a
	// code then, which would mark our order paid before the payer sees the amount.
	if event.Event == "test" || event.Event == "order.created" {
		hint.Kind = "test"
	}
	return hint, nil
}

func (s *Service) EnsureKVoucherWebhook(ctx context.Context, m Merchant, publicBase string) (Merchant, error) {
	if m.Driver != "kvoucher" || !m.Enabled {
		return m, nil
	}
	if strings.TrimSpace(m.Credentials["webhook_secret"]) == "" {
		buf := make([]byte, 32)
		if _, e := rand.Read(buf); e != nil {
			return m, e
		}
		m.Credentials["webhook_secret"] = hex.EncodeToString(buf)
	}
	base := strings.TrimRight(publicBase, "/")
	if base == "" {
		return m, ErrNotConfigured
	}
	target := base + "/v1/payments/webhooks/kvoucher/" + m.OperatorID
	body := map[string]any{"url": target, "secret": m.Credentials["webhook_secret"], "events": []string{"order.created", "order.payment_verified", "order.pins_loaded", "order.completed"}, "description": "Tokpath " + m.OperatorID}
	k := KVoucher{HTTP: s.driverTransport()}
	var raw []byte
	var e error
	if id := strings.TrimSpace(m.Credentials["webhook_id"]); id != "" {
		body["is_active"] = true
		raw, e = k.order(ctx, m, http.MethodPut, "/webhooks/"+url.PathEscape(id), body)
	} else {
		raw, e = k.order(ctx, m, http.MethodPost, "/webhooks", body)
	}
	if e != nil {
		return m, e
	}
	var saved struct {
		ID json.Number `json:"id"`
	}
	if json.Unmarshal(raw, &saved) != nil || saved.ID == "" {
		return m, ErrProvider
	}
	m.Credentials["webhook_id"] = saved.ID.String()
	return m, nil
}
func (s *Service) driverTransport() *Transport {
	for _, d := range s.Drivers {
		if k, ok := d.(KVoucher); ok {
			return k.HTTP
		}
	}
	return nil
}
func (s *Service) KVoucherRefundChannels(ctx context.Context, operator string) []string {
	rows, e := s.PublicMethods(ctx, operator)
	if e != nil {
		return nil
	}
	for _, row := range rows {
		if row.Driver == "kvoucher" && len(row.RefundChannels) > 0 {
			return row.RefundChannels
		}
	}
	return nil
}

func (k KVoucher) sandbox(ctx context.Context, m Merchant) (bool, error) {
	raw, e := k.order(ctx, m, http.MethodGet, "/account", nil)
	if e != nil {
		return false, e
	}
	var account struct {
		Test bool `json:"is_test_mode"`
	}
	if json.Unmarshal(raw, &account) != nil {
		return false, ErrProvider
	}
	return account.Test, nil
}
func (k KVoucher) pins(ctx context.Context, m Merchant, reference, denomination string, quantity int) ([]map[string]any, error) {
	if found, e := k.redeemed(ctx, m, reference); e != nil || len(found) > 0 {
		return found, e
	}
	raw, e := k.order(ctx, m, http.MethodPost, "/stock/retrieve", map[string]any{"denomination": json.Number(denomination), "quantity": quantity, "reference": reference})
	if e != nil {
		if e == ErrProvider {
			return nil, ErrNotFinal
		}
		return nil, e
	}
	var out struct {
		Retrieved []map[string]any `json:"retrieved"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return nil, ErrProvider
	}
	return out.Retrieved, nil
}
func (k KVoucher) redeemed(ctx context.Context, m Merchant, reference string) ([]map[string]any, error) {
	raw, e := k.order(ctx, m, http.MethodGet, "/stock/redeemed?page=1&per_page=100", nil)
	if e != nil {
		return nil, nil
	}
	var out struct {
		Items []map[string]any `json:"items"`
		Rows  []map[string]any `json:"redeemed"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return nil, nil
	}
	rows := append(out.Items, out.Rows...)
	matched := []map[string]any{}
	for _, row := range rows {
		if ref, _ := row["reference"].(string); ref == reference {
			matched = append(matched, row)
		}
	}
	return matched, nil
}
func (k KVoucher) order(ctx context.Context, m Merchant, method, rel string, payload any) ([]byte, error) {
	base, e := k.HTTP.endpoint(m, kvoucherOfficialBase)
	if e != nil {
		return nil, e
	}
	var body []byte
	if payload != nil {
		body, e = json.Marshal(payload)
		if e != nil {
			return nil, ErrInvalid
		}
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sum := sha256.Sum256([]byte(m.Credentials["kvoucher_api_secret"]))
	secretHash := hex.EncodeToString(sum[:])
	path := "/api/kv/merchant/v1" + rel
	mac := hmac.New(sha256.New, []byte(secretHash))
	mac.Write([]byte(ts + method + path + string(body)))
	headers := http.Header{"Accept": []string{"application/json"}, "Content-Type": []string{"application/json"}, "User-Agent": []string{"Tokpath"}, "X-API-Key": []string{m.Credentials["kvoucher_api_key"]}, "X-Timestamp": []string{ts}, "X-Signature": []string{hex.EncodeToString(mac.Sum(nil))}}
	raw, _, e := k.HTTP.do(ctx, method, base+rel, body, headers)
	if e != nil {
		return nil, e
	}
	var wrapped struct {
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &wrapped) != nil || !wrapped.OK {
		return nil, ErrProvider
	}
	if len(wrapped.Data) == 0 || string(wrapped.Data) == "null" {
		return raw, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(wrapped.Data, &object) != nil {
		return wrapped.Data, nil
	}
	for _, key := range []string{"order", "webhook", "payment"} {
		if object[key] != nil {
			return object[key], nil
		}
	}
	return wrapped.Data, nil
}
func (k KVoucher) origin(m Merchant) (string, error) {
	base, e := k.HTTP.endpoint(m, kvoucherOfficialBase)
	if e != nil {
		return "", e
	}
	u, e := url.Parse(base)
	if e != nil {
		return "", ErrInvalid
	}
	return u.Scheme + "://" + u.Host, nil
}

type gkashCheckout struct {
	Action string
	Fields map[string]string
	Money  *Money
}

func gkashForm(raw []byte) (gkashCheckout, error) {
	var payment struct {
		Action string                     `json:"action"`
		Method string                     `json:"method"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if json.Unmarshal(raw, &payment) != nil || payment.Action == "" || payment.Method != "POST" || len(payment.Fields) == 0 {
		return gkashCheckout{}, ErrProvider
	}
	fields := map[string]string{}
	for key, value := range payment.Fields {
		text := strings.Trim(string(value), `"`)
		if text == "" || text == "null" {
			return gkashCheckout{}, ErrProvider
		}
		fields[key] = text
	}
	out := gkashCheckout{Action: payment.Action, Fields: fields}
	if fields["v_amount"] != "" && fields["v_currency"] != "" {
		amount, e := ledger.ParseDecimal(fields["v_amount"])
		if e != nil {
			return gkashCheckout{}, ErrProvider
		}
		out.Money = &Money{Currency: fields["v_currency"], Amount: amount}
	}
	return out, nil
}
func kvoucherSignature(secret string, body []byte, header string) bool {
	sum := sha256.Sum256([]byte(secret))
	mac := hmac.New(sha256.New, []byte(hex.EncodeToString(sum[:])))
	mac.Write(body)
	expect := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(expect), []byte(strings.TrimSpace(header))) == 1
}
func normalizeRefundChannels(value string) []string {
	allowed := map[string]string{"bank": "银行卡", "alipay": "支付宝", "wechat": "微信支付", "tng": "Touch 'n Go", "duitnow": "DuitNow"}
	seen := map[string]bool{}
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		label := allowed[key]
		if label == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
