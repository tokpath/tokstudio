package payment

import "testing"

func TestSandboxAdapters(t *testing.T) {
	if !ValidAdapter("stripe") || !ValidAdapter("alipay") || !ValidAdapter("wechat") || !ValidAdapter("manual") {
		t.Fatal("expected four P0 adapters")
	}
	if !SupportsAutoRenew(AdapterStripe) {
		t.Fatal("stripe must support auto renew")
	}
	if SupportsAutoRenew(AdapterAlipay) || SupportsAutoRenew(AdapterWechat) {
		t.Fatal("domestic adapters must not fake auto renew")
	}
	sig := SignWebhook("secret", "evt_1", "ord_1", "paid")
	if !VerifyWebhook("secret", "evt_1", "ord_1", "paid", sig) {
		t.Fatal("signature should verify")
	}
	if VerifyWebhook("secret", "evt_1", "ord_1", "paid", "nope") {
		t.Fatal("bad signature accepted")
	}
}
