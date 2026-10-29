package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Stripe struct{ HTTP *Transport }

func (Stripe) Name() string { return "stripe" }
func (Stripe) Validate(m Merchant) []string {
	missing := required(m, "secret_key", "publishable_key", "webhook_secret", "currency")
	if _, ok := stripeMinorDigits(m.Credentials["currency"]); !ok {
		missing = append(missing, "supported_presentment_currency")
	}
	return missing
}
func (d Stripe) call(ctx context.Context, m Merchant, method, path string, form url.Values, key string) ([]byte, error) {
	base, e := d.HTTP.endpoint(m, "https://api.stripe.com")
	if e != nil {
		return nil, e
	}
	headers := http.Header{"Authorization": []string{"Bearer " + m.Credentials["secret_key"]}, "Stripe-Version": []string{"2025-06-30.basil"}}
	var body []byte
	if form != nil {
		body = []byte(form.Encode())
		headers.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if key != "" {
		headers.Set("Idempotency-Key", key)
	}
	raw, _, e := d.HTTP.do(ctx, method, base+path, body, headers)
	return raw, e
}
func (d Stripe) Create(ctx context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, d); e != nil {
		return Checkout{}, e
	}
	digits, supported := stripeMinorDigits(o.Money.Currency)
	if o.Validate(m) != nil || !supported || o.Money.Currency != m.Credentials["currency"] {
		return Checkout{}, ErrInvalid
	}
	minor, e := o.Money.Minor(digits)
	if e != nil {
		return Checkout{}, e
	}
	form := url.Values{"amount": {strconv.FormatInt(minor, 10)}, "currency": {strings.ToLower(o.Money.Currency)}, "metadata[order_id]": {o.ID}, "metadata[operator_id]": {o.OperatorID}, "automatic_payment_methods[enabled]": {"true"}}
	if o.PaymentMethod != "" {
		if o.CustomerRef == "" || o.AuthorizationID == "" {
			return Checkout{}, ErrForbidden
		}
		form.Set("payment_method", o.PaymentMethod)
		form.Set("customer", o.CustomerRef)
		form.Set("confirm", "true")
		form.Set("off_session", "true")
		form.Set("metadata[authorization_id]", o.AuthorizationID)
	}
	raw, e := d.call(ctx, m, http.MethodPost, "/v1/payment_intents", form, "payment:"+m.ID+":"+o.ID)
	if e != nil {
		return Checkout{}, e
	}
	var pi struct {
		ID, Status   string
		ClientSecret string `json:"client_secret"`
	}
	if json.Unmarshal(raw, &pi) != nil || pi.ID == "" {
		return Checkout{}, ErrOutcomeUnknown
	}
	return Checkout{ProviderID: pi.ID, State: pi.Status, ClientSecret: pi.ClientSecret, PublishableKey: m.Credentials["publishable_key"]}, nil
}
func (d Stripe) Lookup(ctx context.Context, m Merchant, o Order) (Fact, error) {
	if e := ready(m, d); e != nil {
		return Fact{}, e
	}
	if o.Validate(m) != nil || o.ProviderID == "" {
		return Fact{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, http.MethodGet, "/v1/payment_intents/"+url.PathEscape(o.ProviderID), nil, "")
	if e != nil {
		return Fact{}, e
	}
	var pi struct {
		ID, Status, Currency string
		AmountReceived       int64 `json:"amount_received"`
		Metadata             struct {
			OrderID    string `json:"order_id"`
			OperatorID string `json:"operator_id"`
		}
	}
	if json.Unmarshal(raw, &pi) != nil || pi.ID != o.ProviderID || pi.Metadata.OrderID != o.ID || (pi.Metadata.OperatorID != "" && pi.Metadata.OperatorID != m.OperatorID) {
		return Fact{}, ErrConflict
	}
	if pi.Status != "succeeded" {
		return Fact{}, ErrNotFinal
	}
	digits, ok := stripeMinorDigits(strings.ToUpper(pi.Currency))
	if !ok {
		return Fact{}, ErrUnsupported
	}
	money, e := fromMinor(pi.Currency, pi.AmountReceived, digits)
	if e != nil {
		return Fact{}, e
	}
	return receiptFact(m, o, pi.ID, money, nil, map[string]any{"payment_intent": pi.ID, "status": pi.Status, "amount_received": pi.AmountReceived, "currency": pi.Currency})
}
func (d Stripe) Refund(ctx context.Context, m Merchant, r Refund) (RefundStatus, error) {
	if e := ready(m, d); e != nil {
		return RefundStatus{}, e
	}
	if r.Validate(m) != nil || r.Order.ProviderID == "" {
		return RefundStatus{}, ErrInvalid
	}
	digits, ok := stripeMinorDigits(r.Money.Currency)
	if !ok {
		return RefundStatus{}, ErrUnsupported
	}
	minor, e := r.Money.Minor(digits)
	if e != nil {
		return RefundStatus{}, e
	}
	form := url.Values{"payment_intent": {r.Order.ProviderID}, "amount": {strconv.FormatInt(minor, 10)}, "metadata[order_id]": {r.Order.ID}, "metadata[refund_id]": {r.ID}}
	raw, e := d.call(ctx, m, http.MethodPost, "/v1/refunds", form, "refund:"+m.ID+":"+r.ID)
	if e != nil {
		return RefundStatus{State: "outcome_unknown"}, e
	}
	return stripeRefundResult(m, r, raw)
}
func (d Stripe) LookupRefund(ctx context.Context, m Merchant, r Refund) (RefundStatus, error) {
	if e := ready(m, d); e != nil {
		return RefundStatus{}, e
	}
	if r.Validate(m) != nil || r.ProviderID == "" {
		return RefundStatus{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, http.MethodGet, "/v1/refunds/"+url.PathEscape(r.ProviderID), nil, "")
	if e != nil {
		return RefundStatus{}, e
	}
	return stripeRefundResult(m, r, raw)
}
func stripeRefundResult(m Merchant, r Refund, raw []byte) (RefundStatus, error) {
	var response struct {
		ID, Status, Currency string
		Amount               int64
		PaymentIntent        string `json:"payment_intent"`
		Metadata             struct {
			RefundID string `json:"refund_id"`
			OrderID  string `json:"order_id"`
		}
	}
	if json.Unmarshal(raw, &response) != nil || response.ID == "" || response.PaymentIntent != r.Order.ProviderID || (response.Metadata.RefundID != "" && response.Metadata.RefundID != r.ID) || (response.Metadata.OrderID != "" && response.Metadata.OrderID != r.Order.ID) {
		return RefundStatus{}, ErrConflict
	}
	out := RefundStatus{ProviderID: response.ID, State: response.Status}
	if response.Status == "succeeded" {
		digits, ok := stripeMinorDigits(strings.ToUpper(response.Currency))
		if !ok {
			return out, ErrUnsupported
		}
		money, e := fromMinor(response.Currency, response.Amount, digits)
		if e != nil || money.Amount.Cmp(r.Money.Amount) != 0 || money.Currency != r.Money.Currency {
			return out, ErrConflict
		}
		fact, e := refundFact(m, r, response.ID, money, nil, map[string]any{"refund": response.ID, "payment_intent": response.PaymentIntent, "amount": response.Amount, "currency": response.Currency, "status": response.Status})
		if e != nil {
			return out, e
		}
		out.Fact = &fact
	}
	return out, nil
}
func (d Stripe) VerifyWebhook(_ context.Context, m Merchant, w Webhook) (Hint, error) {
	if e := ready(m, d); e != nil {
		return Hint{}, e
	}
	if len(w.Body) == 0 || len(w.Body) > 1<<20 {
		return Hint{}, ErrInvalid
	}
	at := w.ReceivedAt
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var timestamp string
	signatures := []string{}
	for _, piece := range strings.Split(w.Headers.Get("Stripe-Signature"), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(piece), "=")
		if !ok {
			continue
		}
		if key == "t" {
			if timestamp != "" {
				return Hint{}, ErrSignature
			}
			timestamp = value
		}
		if key == "v1" {
			signatures = append(signatures, value)
		}
	}
	unix, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil || at.Unix()-unix > 300 || unix-at.Unix() > 30 {
		return Hint{}, ErrSignature
	}
	mac := hmac.New(sha256.New, []byte(m.Credentials["webhook_secret"]))
	_, _ = fmt.Fprintf(mac, "%s.%s", timestamp, w.Body)
	expected := mac.Sum(nil)
	valid := false
	for _, signature := range signatures {
		actual, e := hex.DecodeString(signature)
		if e == nil && hmac.Equal(actual, expected) {
			valid = true
		}
	}
	if !valid {
		return Hint{}, ErrSignature
	}
	var event struct {
		ID, Type string
		Created  int64
		Data     struct {
			Object struct {
				ID       string
				Metadata struct {
					OrderID  string `json:"order_id"`
					RefundID string `json:"refund_id"`
				}
			}
		}
	}
	if json.Unmarshal(w.Body, &event) != nil || event.ID == "" || event.Data.Object.ID == "" {
		return Hint{}, ErrInvalid
	}
	kind := ""
	switch event.Type {
	case "payment_intent.succeeded":
		kind = "receipt"
	case "refund.created", "refund.updated", "refund.failed":
		kind = "refund"
	default:
		return Hint{}, ErrUnsupported
	}
	occurred := time.Unix(event.Created, 0).UTC()
	if event.Created <= 0 {
		return Hint{}, ErrInvalid
	}
	return Hint{EventID: event.ID, Kind: kind, OrderID: event.Data.Object.Metadata.OrderID, ReferenceID: event.Data.Object.Metadata.RefundID, ProviderID: event.Data.Object.ID, OccurredAt: &occurred, SignatureVerified: true, Final: kind == "receipt"}, nil
}

// The charge/refund minor unit follows Stripe's presentment-currency rules.
// This explicit implemented set avoids guessing special-case currency behavior.
// https://docs.stripe.com/currencies#minor-units-in-api-amounts
func stripeMinorDigits(code string) (uint8, bool) {
	switch code {
	case "JPY":
		return 0, true
	case "USD", "EUR", "GBP", "CNY", "HKD", "SGD", "MYR", "AUD", "CAD", "CHF", "NZD":
		return 2, true
	default:
		return 0, false
	}
}
