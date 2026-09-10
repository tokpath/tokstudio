package payment

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// stripeDriver 国际卡 / Link。live 走 PaymentIntent；sandbox 仍用 HMAC webhook。
type stripeDriver struct{ sandboxDriver }

func (stripeDriver) Spec() AdapterSpec {
	return AdapterSpec{
		ID: AdapterStripe, DisplayName: "Stripe", Kind: KindOfficial,
		PayCurrency: "USD", CheckoutMode: CheckoutElement, BrandColor: "#635BFF",
		AutoRenew: true, UserFacing: true, MerchantKeys: []string{"publishable_key"},
		Credentials: []CredentialFieldView{
			{Key: "secret_key", Label: "Secret Key", Secret: true, Required: true},
			{Key: "publishable_key", Label: "Publishable Key", Required: true},
			{Key: "webhook_secret", Label: "Webhook Secret", Secret: true, Required: true},
			{Key: "currency", Label: "结算币种", Required: true},
		},
	}
}

func (d stripeDriver) Test(ctx context.Context, in TestInput) error {
	if missing := missingFromSpec(d.Spec(), in.Credentials); len(missing) > 0 {
		return ErrInstanceIncomplete
	}
	if !officialLive(in.Mode, in.Credentials, "secret_key") {
		return d.sandboxDriver.Test(ctx, in)
	}
	_, code, err := stripeDo(ctx, cred(in.Credentials, "secret_key"), "GET", "/v1/account", nil)
	if err != nil || code >= 300 {
		return ErrProviderFailed
	}
	return nil
}

func (d stripeDriver) CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	if !officialLive(in.Mode, in.Credentials, "secret_key") {
		sess := d.sandboxCheckout(d.Spec(), in)
		if in.Credentials != nil {
			sess.PublishableKey = in.Credentials["publishable_key"]
		}
		return sess, nil
	}
	if in.Order == nil {
		return nil, ErrNotFound
	}
	currency := strings.ToLower(cred(in.Credentials, "currency"))
	if currency == "" {
		currency = "usd"
	}
	form := url.Values{}
	form.Set("amount", strconv.FormatInt(stripeCents(in.Order.AmountMinor), 10))
	form.Set("currency", currency)
	form.Set("metadata[order_id]", in.Order.ID)
	form.Set("automatic_payment_methods[enabled]", "true")
	raw, code, err := stripeDo(ctx, cred(in.Credentials, "secret_key"), "POST", "/v1/payment_intents", form)
	if err != nil || code >= 300 {
		return nil, ErrProviderFailed
	}
	obj := decodeJSONMap(raw)
	secret := asString(obj["client_secret"])
	if secret == "" {
		return nil, ErrProviderFailed
	}
	return &CheckoutSession{
		Mode: CheckoutElement, Sandbox: false,
		WebhookURL:     WebhookURL(strings.TrimRight(in.PublicBase, "/"), AdapterStripe),
		AutoRenew:      true,
		ClientSecret:   secret,
		PublishableKey: cred(in.Credentials, "publishable_key"),
		Payload:        map[string]any{"payment_intent": asString(obj["id"])},
	}, nil
}

func (d stripeDriver) ParseWebhook(_ context.Context, in WebhookRequest) (*WebhookEvent, error) {
	sig := ""
	if in.Headers != nil {
		sig = in.Headers.Get("Stripe-Signature")
	}
	if sig != "" {
		secret := cred(in.Credentials, "webhook_secret")
		valid := verifyStripeSignature(secret, sig, in.Body)
		payload := decodeJSONMap(in.Body)
		if payload == nil {
			return nil, ErrInvalidEvent
		}
		status, orderID, tradeID, eventID := stripeEventStatus(payload)
		if eventID == "" {
			return nil, ErrInvalidEvent
		}
		return &WebhookEvent{
			ExternalEventID: eventID, OrderID: orderID, Status: status, TradeID: tradeID,
			SignatureValid: valid,
		}, nil
	}
	return d.sandboxDriver.ParseWebhook(context.Background(), in)
}

func (d stripeDriver) QueryOrder(ctx context.Context, in QueryRequest) (*QueryResult, error) {
	if !officialLive(in.Mode, in.Credentials, "secret_key") {
		return d.sandboxDriver.QueryOrder(ctx, in)
	}
	if in.Order == nil || in.Order.TradeID == "" {
		return nil, ErrNotFound
	}
	raw, code, err := stripeDo(ctx, cred(in.Credentials, "secret_key"), "GET", "/v1/payment_intents/"+url.PathEscape(in.Order.TradeID), nil)
	if err != nil || code >= 300 {
		return nil, ErrProviderFailed
	}
	obj := decodeJSONMap(raw)
	st := asString(obj["status"])
	out := StatusPending
	switch st {
	case "succeeded":
		out = StatusPaid
	case "canceled":
		out = StatusFailed
	}
	return &QueryResult{Status: out, TradeID: asString(obj["id"])}, nil
}

func (d stripeDriver) Refund(ctx context.Context, in RefundRequest) (*RefundResult, error) {
	if !officialLive(in.Mode, in.Credentials, "secret_key") {
		return d.sandboxDriver.Refund(ctx, in)
	}
	if in.Order == nil || in.Order.TradeID == "" {
		return nil, ErrNotFound
	}
	form := url.Values{}
	form.Set("payment_intent", in.Order.TradeID)
	raw, code, err := stripeDo(ctx, cred(in.Credentials, "secret_key"), "POST", "/v1/refunds", form)
	if err != nil || code >= 300 {
		return nil, ErrProviderFailed
	}
	obj := decodeJSONMap(raw)
	return &RefundResult{Status: StatusRefunded, TradeID: asString(obj["id"])}, nil
}
