package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func ValidAdapter(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case AdapterStripe, AdapterAlipay, AdapterWechat, AdapterManual:
		return true
	}
	return false
}

// SupportsAutoRenew：只有 Stripe 沙箱声明具备支付方式引用和代扣。
// 支付宝/微信在 P0 没有代扣资质，不得伪造自动续费成功。
func SupportsAutoRenew(adapter string) bool {
	return strings.EqualFold(adapter, AdapterStripe)
}

func SignWebhook(secret, eventID, orderID, status string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(eventID + "|" + orderID + "|" + status))
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyWebhook(secret, eventID, orderID, status, sig string) bool {
	want := SignWebhook(secret, eventID, orderID, status)
	return hmac.Equal([]byte(want), []byte(strings.TrimSpace(sig)))
}
