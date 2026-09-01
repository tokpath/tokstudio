package payment

import "context"

// alipayDriver 支付宝官方直连。凭证字段对照官方开放平台（AppID / 应用私钥 / 支付宝公钥）。
// 后续把 CreateCheckout 换成当面付/电脑网站支付即可，注册表不用动。
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
	return d.sandboxDriver.Test(ctx, in)
}

func (d alipayDriver) CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	return d.sandboxCheckout(d.Spec(), in), nil
}
