package payment

import "context"

// stripeDriver 国际卡 / Link。P0 唯一声明自动续费的通道。后续换成 Payment Element。
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
	return d.sandboxDriver.Test(ctx, in)
}

func (d stripeDriver) CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	sess := d.sandboxCheckout(d.Spec(), in)
	if in.Credentials != nil {
		sess.PublishableKey = in.Credentials["publishable_key"]
	}
	return sess, nil
}
