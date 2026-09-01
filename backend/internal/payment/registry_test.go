package payment

import (
	"context"
	"testing"
)

type localPayDriver struct{ sandboxDriver }

func (localPayDriver) Spec() AdapterSpec {
	return AdapterSpec{
		ID: "local_paynow", DisplayName: "PayNow", Kind: KindLocal,
		PayCurrency: "SGD", CheckoutMode: CheckoutQR, BrandColor: "#111111",
		UserFacing: true,
		Credentials: []CredentialFieldView{
			{Key: "merchant_id", Label: "Merchant ID", Required: true},
			{Key: "api_secret", Label: "API Secret", Secret: true, Required: true},
		},
	}
}

func (d localPayDriver) Test(ctx context.Context, in TestInput) error {
	if missing := missingFromSpec(d.Spec(), in.Credentials); len(missing) > 0 {
		return ErrInstanceIncomplete
	}
	return nil
}

func (d localPayDriver) CreateCheckout(_ context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	return d.sandboxCheckout(d.Spec(), in), nil
}

func TestRegistryPluggable(t *testing.T) {
	reg := NewRegistry()
	reg.MustRegister(alipayDriver{})
	reg.MustRegister(localPayDriver{})
	if !reg.Has("alipay") || !reg.Has("local_paynow") {
		t.Fatal("expected builtin and custom adapters")
	}
	if DefaultRegistry().Has("local_paynow") {
		t.Fatal("custom adapter must not leak into the builtin registry")
	}
	paynow, ok := reg.Get("local_paynow")
	if !ok || paynow.Spec().Kind != KindLocal || paynow.Spec().PayCurrency != "SGD" {
		t.Fatalf("local plugin spec: %+v", paynow)
	}
	facing := reg.UserFacing()
	if len(facing) != 2 {
		t.Fatalf("user facing: %d", len(facing))
	}
	if err := paynow.Test(context.Background(), TestInput{Credentials: map[string]string{"merchant_id": "m1"}}); err != ErrInstanceIncomplete {
		t.Fatalf("incomplete test: %v", err)
	}
	if err := paynow.Test(context.Background(), TestInput{Credentials: map[string]string{"merchant_id": "m1", "api_secret": "s"}}); err != nil {
		t.Fatal(err)
	}
	sess, err := paynow.CreateCheckout(context.Background(), CheckoutRequest{PublicBase: "https://oem.example"})
	if err != nil || sess == nil || sess.Mode != CheckoutQR {
		t.Fatalf("local plugin checkout: %+v %v", sess, err)
	}
}

func TestQuoteUsesAdapterCurrency(t *testing.T) {
	q, err := QuotePay(AdapterAlipay, 100, 0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if q.PayCurrency != "CNY" || q.PayMinor != 10000 || q.WalletMinor <= 0 {
		t.Fatalf("alipay quote: %+v", q)
	}
	usd, err := QuotePay(AdapterStripe, 10, 0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if usd.PayCurrency != "USD" || usd.WalletMinor != 10_000_000 {
		t.Fatalf("stripe quote: %+v", usd)
	}
}
