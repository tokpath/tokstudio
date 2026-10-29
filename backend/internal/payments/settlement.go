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
		_, supported = stripeMinorDigits(currency)
		supported = supported && m.Credentials["currency"] == currency
	case "alipay", "wechat":
		supported = currency == "CNY"
	case "kvoucher":
		supported = currency == "MYR"
	case "nowpayments":
		// These price currencies are covered by the implemented fiat invoice flow.
		supported = currency == "USD" || currency == "EUR" || currency == "GBP" || currency == "JPY"
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
