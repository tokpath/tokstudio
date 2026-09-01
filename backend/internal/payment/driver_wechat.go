package payment

import "context"

// wechatDriver 微信支付官方直连（APIv3）。后续换成 Native / JSAPI / H5。
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
	return d.sandboxDriver.Test(ctx, in)
}

func (d wechatDriver) CreateCheckout(ctx context.Context, in CheckoutRequest) (*CheckoutSession, error) {
	return d.sandboxCheckout(d.Spec(), in), nil
}
