package payments

import (
	"fmt"
	"strings"
)

// SettlementSupport describes the implemented provider capability. Cross-currency
// manual/wallet payments need an independently confirmed amount after receipt.
func SettlementSupport(m Merchant, currency string) (bool, string) {
	supported := false
	switch m.Driver {
	case "manual", "evm_wallet":
		supported = true
	case "stripe":
		_, supported = fiatMinorDigits(currency)
		_, paySupported := stripeMinorDigits(m.Credentials["currency"])
		supported = supported && paySupported
	case "alipay", "wechat":
		_, supported = fiatMinorDigits(currency)
	case "kvoucher":
		// Gkash collects MYR; foreign settlement obligations use a pinned
		// per-payment quote without changing the verified receipt currency.
		_, supported = fiatMinorDigits(currency)
	case "nowpayments":
		// Other fiat obligations use a reference-priced provider invoice.
		_, supported = fiatMinorDigits(currency)
	}
	if supported {
		return true, ""
	}
	return false, fmt.Sprintf("该付款通道当前不支持 %s 订单清算，请选择其他付款方式。", currency)
}

func merchantCurrency(m Merchant) string {
	switch m.Driver {
	case "stripe", "manual":
		return m.Credentials["currency"]
	case "evm_wallet":
		return m.Credentials["asset"]
	case "nowpayments":
		return strings.ToUpper(m.Credentials["pay_currency"])
	case "kvoucher":
		return "MYR"
	case "alipay", "wechat":
		return "CNY"
	}
	return ""
}
