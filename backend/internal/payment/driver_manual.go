package payment

import "context"

// manualDriver 人工入账。不出现在用户收银台按钮里，渠道/财务在订单页确认。
type manualDriver struct{ sandboxDriver }

func (manualDriver) Spec() AdapterSpec {
	return AdapterSpec{
		ID: AdapterManual, DisplayName: "人工入账", Kind: KindManual,
		PayCurrency: "USD", CheckoutMode: CheckoutManual, UserFacing: false,
	}
}

func (manualDriver) Test(_ context.Context, _ TestInput) error { return nil }

func (d manualDriver) CreateCheckout(_ context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	return d.sandboxCheckout(d.Spec(), in), nil
}
