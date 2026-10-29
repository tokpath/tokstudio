package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

// NOWPayments implements direct stablecoin payments and authenticated IPN. Its
// payout endpoints are deliberately not used for refunds or commission payout.
// https://github.com/NowPaymentsIO/nowpayments-api-js
type NOWPayments struct{ HTTP *Transport }

func (NOWPayments) Name() string { return "nowpayments" }
func (NOWPayments) Validate(m Merchant) []string {
	return required(m, "api_key", "ipn_secret", "pay_currency")
}
func (d NOWPayments) call(ctx context.Context, m Merchant, method, path string, payload any) ([]byte, error) {
	base, e := d.HTTP.endpoint(m, "https://api.nowpayments.io")
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
	raw, _, e := d.HTTP.do(ctx, method, base+path, body, http.Header{"X-Api-Key": []string{m.Credentials["api_key"]}, "Content-Type": []string{"application/json"}})
	return raw, e
}
func (d NOWPayments) Create(ctx context.Context, m Merchant, o Order) (Checkout, error) {
	if e := ready(m, d); e != nil {
		return Checkout{}, e
	}
	if o.Validate(m) != nil || strings.ToLower(o.Money.Currency) != m.Credentials["pay_currency"] || o.NotifyURL == "" {
		return Checkout{}, ErrInvalid
	}
	settlement := o.Settlement()
	if supported, _ := SettlementSupport(m, settlement.Currency); !supported {
		return Checkout{}, ErrUnsupported
	}
	raw, e := d.call(ctx, m, http.MethodPost, "/v1/payment", map[string]any{"price_amount": json.Number(settlement.Amount.String()), "price_currency": strings.ToLower(settlement.Currency), "pay_currency": m.Credentials["pay_currency"], "order_id": o.ID, "order_description": o.Description, "ipn_callback_url": o.NotifyURL, "is_fixed_rate": true, "is_fee_paid_by_user": false})
	if e != nil {
		return Checkout{}, e
	}
	var response struct {
		PaymentID   json.Number `json:"payment_id"`
		PayAddress  string      `json:"pay_address"`
		PayAmount   json.Number `json:"pay_amount"`
		PayCurrency string      `json:"pay_currency"`
	}
	if json.Unmarshal(raw, &response) != nil || response.PaymentID == "" || response.PayAddress == "" || strings.ToUpper(response.PayCurrency) != o.Money.Currency {
		return Checkout{}, ErrOutcomeUnknown
	}
	amount, e := ledger.ParseDecimal(string(response.PayAmount))
	if e != nil || amount.Sign() <= 0 {
		return Checkout{}, ErrInvalid
	}
	money := Money{Currency: o.Money.Currency, Amount: amount}
	return Checkout{ProviderID: string(response.PaymentID), State: "pending", PaymentAddress: response.PayAddress, PayMoney: &money, SettlementMoney: &settlement}, nil
}
func (d NOWPayments) Lookup(ctx context.Context, m Merchant, o Order) (Fact, error) {
	if e := ready(m, d); e != nil {
		return Fact{}, e
	}
	if o.Validate(m) != nil || o.ProviderID == "" {
		return Fact{}, ErrInvalid
	}
	raw, e := d.call(ctx, m, http.MethodGet, "/v1/payment/"+url.PathEscape(o.ProviderID), nil)
	if e != nil {
		return Fact{}, e
	}
	var response struct {
		PaymentID          json.Number `json:"payment_id"`
		OrderID            string      `json:"order_id"`
		Status             string      `json:"payment_status"`
		PayCurrency        string      `json:"pay_currency"`
		ActuallyPaid       json.Number `json:"actually_paid"`
		PayAmount          json.Number `json:"pay_amount"`
		PriceAmount        json.Number `json:"price_amount"`
		PriceCurrency      string      `json:"price_currency"`
		ActuallyPaidAtFiat json.Number `json:"actually_paid_at_fiat"`
		PayinHash          string      `json:"payin_hash"`
		UpdatedAt          string      `json:"updated_at"`
	}
	if json.Unmarshal(raw, &response) != nil || string(response.PaymentID) != o.ProviderID || response.OrderID != o.ID || strings.ToUpper(response.PayCurrency) != o.Money.Currency {
		return Fact{}, ErrConflict
	}
	if response.Status != "finished" {
		return Fact{}, ErrNotFinal
	}
	amount, e := ledger.ParseDecimal(string(response.ActuallyPaid))
	if e != nil || amount.Sign() <= 0 {
		return Fact{}, ErrInvalid
	}
	fact, e := receiptFact(m, o, o.ProviderID, Money{Currency: o.Money.Currency, Amount: amount}, nil, json.RawMessage(raw))
	if e != nil {
		return fact, e
	}
	// The provider's authenticated transaction carries the settlement relation.
	// Missing or partial relations remain pending for finance confirmation.
	if strings.ToUpper(response.PriceCurrency) == o.Settlement().Currency {
		settled, parseErr := ledger.ParseDecimal(string(response.ActuallyPaidAtFiat))
		if parseErr != nil || settled.Sign() <= 0 {
			quoted, quoteErr := ledger.ParseDecimal(string(response.PayAmount))
			price, priceErr := ledger.ParseDecimal(string(response.PriceAmount))
			if quoteErr == nil && priceErr == nil && quoted.Sign() > 0 && amount.Cmp(quoted) >= 0 && price.Cmp(o.Settlement().Amount) == 0 {
				settled = price
				parseErr = nil
			}
		}
		if parseErr == nil && settled.Sign() > 0 {
			value := MoneyValue{Amount: settled, Currency: o.Settlement().Currency, OriginalMoney: fact.Money, EvidenceReference: "NOWPayments verified payment " + o.ProviderID, VerifiedBy: "adapter:nowpayments"}
			fact.ExecutionValue = &value
		}
	}
	return fact, nil
}
func (NOWPayments) Refund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (NOWPayments) LookupRefund(context.Context, Merchant, Refund) (RefundStatus, error) {
	return RefundStatus{}, ErrUnsupported
}
func (d NOWPayments) VerifyWebhook(_ context.Context, m Merchant, w Webhook) (Hint, error) {
	if e := ready(m, d); e != nil {
		return Hint{}, e
	}
	if len(w.Body) > 1<<20 {
		return Hint{}, ErrInvalid
	}
	canonical, e := canonicalIPN(w.Body)
	if e != nil {
		return Hint{}, e
	}
	mac := hmac.New(sha512.New, []byte(m.Credentials["ipn_secret"]))
	_, _ = mac.Write(canonical)
	signature, e := hex.DecodeString(w.Headers.Get("X-Nowpayments-Sig"))
	if e != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return Hint{}, ErrSignature
	}
	var payload struct {
		PaymentID json.Number `json:"payment_id"`
		OrderID   string      `json:"order_id"`
		Status    string      `json:"payment_status"`
		UpdatedAt string      `json:"updated_at"`
	}
	if json.Unmarshal(w.Body, &payload) != nil || payload.PaymentID == "" || payload.OrderID == "" {
		return Hint{}, ErrInvalid
	}
	sum := sha512.Sum512(canonical)
	return Hint{EventID: string(payload.PaymentID) + ":" + hex.EncodeToString(sum[:16]), Kind: "receipt", OrderID: payload.OrderID, ProviderID: string(payload.PaymentID), SignatureVerified: true, Final: payload.Status == "finished"}, nil
}
func canonicalIPN(raw []byte) ([]byte, error) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		return nil, ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, ErrInvalid
	}
	var normalize func(any) (any, error)
	normalize = func(v any) (any, error) {
		switch value := v.(type) {
		case map[string]any:
			for key, item := range value {
				normalized, e := normalize(item)
				if e != nil {
					return nil, e
				}
				value[key] = normalized
			}
			return value, nil
		case []any:
			for i, item := range value {
				normalized, e := normalize(item)
				if e != nil {
					return nil, e
				}
				value[i] = normalized
			}
			return value, nil
		case json.Number:
			// The provider's official IPN protocol signs JSON.stringify output.
			// IEEE conversion is confined to signature canonicalization. Financial
			// quantities are parsed separately from the original JSON as Decimal.
			n, e := strconv.ParseFloat(string(value), 64)
			if e != nil || math.IsInf(n, 0) || math.IsNaN(n) {
				return nil, ErrInvalid
			}
			if n == 0 {
				return json.Number("0"), nil
			}
			form := byte('f')
			if math.Abs(n) < 1e-6 || math.Abs(n) >= 1e21 {
				form = 'e'
			}
			encoded := strconv.FormatFloat(n, form, -1, 64)
			if i := strings.IndexByte(encoded, 'e'); i >= 0 {
				exponent, _ := strconv.Atoi(encoded[i+1:])
				sign := ""
				if exponent >= 0 {
					sign = "+"
				}
				encoded = encoded[:i] + "e" + sign + strconv.Itoa(exponent)
			}
			return json.Number(encoded), nil
		default:
			return v, nil
		}
	}
	normalized, e := normalize(payload)
	if e != nil {
		return nil, e
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(normalized) != nil {
		return nil, ErrInvalid
	}
	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}
