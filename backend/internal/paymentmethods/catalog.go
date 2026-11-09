package paymentmethods

// Method identities belong to business payment ways, independent of the
// connection used to collect a particular transaction.
type Method struct {
	Kind       string   `json:"kind"`
	Label      string   `json:"label"`
	PayoutKind string   `json:"payout_kind"`
	Drivers    []string `json:"drivers"`
}

func Catalog() []Method {
	return []Method{
		{"offline", "线下支付", "offline", []string{"manual"}},
		{"bank_transfer", "银行转账", "bank_cn", []string{"manual"}},
		{"stripe", "Stripe", "", []string{"stripe"}},
		{"alipay", "支付宝", "alipay", []string{"manual", "alipay"}},
		{"wechat", "微信支付", "wechat", []string{"manual", "wechat"}},
		{"paypal", "PayPal", "paypal", []string{"manual"}},
		{"wire", "国际电汇", "wire", []string{"manual"}},
		{"usdt", "USDT", "usdt", []string{"manual", "nowpayments", "evm_wallet"}},
		{"kvoucher", "KVoucher", "", []string{"kvoucher"}},
	}
}
func Find(kind string) (Method, bool) {
	for _, m := range Catalog() {
		if m.Kind == kind {
			return m, true
		}
	}
	return Method{}, false
}
func (m Method) Supports(driver string) bool {
	for _, d := range m.Drivers {
		if d == driver {
			return true
		}
	}
	return false
}

type Identity struct {
	OperatorID string
	Kind       string
	MethodID   string
	MerchantID string
}

func (Identity) TableName() string { return "payment_method_identities" }
