package incentives

import (
	"errors"
	"strings"
	"testing"
)

func TestPayoutProfilesKeepRecipientMaterialsWithThePaymentMethod(t *testing.T) {
	alipay, ok := FindPayoutProfile("alipay")
	if !ok || len(alipay.Fields) != 2 || alipay.Fields[0].Name != "payee_name" || alipay.Fields[1].Name != "account" {
		t.Fatal(alipay, ok)
	}
	text, err := FormatPayoutRecipient("alipay", map[string]string{"payee_name": "张三", "account": "13800138000"})
	if err != nil || !strings.Contains(text, "支付宝实名：张三") || !strings.Contains(text, "支付宝账号：13800138000") {
		t.Fatal(text, err)
	}
	if _, err = FormatPayoutRecipient("alipay", map[string]string{"payee_name": "张三"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("missing alipay account was accepted", err)
	}
	if _, err = FormatPayoutRecipient("usdt", map[string]string{"network": "SOL", "address": "TXYZ1234567890abcd"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("unknown network was accepted", err)
	}
	wire, err := FormatPayoutRecipient("wire", map[string]string{"payee_name": "Ada Lovelace", "bank_name": "Example Bank", "account_number": "GB29 NWBK 6016 1331 9268 19", "swift": "nwbk gb2l", "country": "United Kingdom", "address": "1 Example Street"})
	if err != nil || !strings.Contains(wire, "SWIFT/BIC：NWBKGB2L") || !strings.Contains(wire, "GB29NWBK60161331926819") {
		t.Fatal(wire, err)
	}
}

func TestWithdrawalPreviewUsesPayableCurrencyUnits(t *testing.T) {
	amount, fee, net, err := QuoteDigits("USD", d("10.0134"), d("0"), d("0.02"), d("1"))
	if err != nil || amount.String() != "10.01" || fee.String() != "0.2" || net.String() != "9.81" {
		t.Fatal(amount, fee, net, err)
	}
	if _, _, _, err = QuoteDigits("USD", d("0.009"), d("0"), d("0"), d("0.001")); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("sub-cent amount was payable", err)
	}
}

func TestPayoutMaterialsAreManualAndNetworkSpecific(t *testing.T) {
	for kind, values := range map[string]map[string]string{
		"offline": {"payee_name": "张三"},
		"bank_cn": {"payee_name": "张三", "bank_name": "Example Bank", "account_number": "1234567890"},
		"paypal":  {"email": "payee@example.test"},
		"usdt":    {"network": "TRC20", "address": "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7"},
	} {
		if _, e := FormatPayoutRecipient(kind, values); e != nil {
			t.Fatalf("%s: %v", kind, e)
		}
	}
	for _, tc := range []struct {
		network, address string
		valid            bool
	}{
		{"ERC20", "0x1234567890123456789012345678901234567890", true},
		{"ERC20", "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU7", false},
		{"TRC20", "TLa2f6VPqDgRE67v1736s7bJ8Ray5wYjU8", false},
		{"ERC20", "0x123456789012345678901234567890123456789z", false},
	} {
		if got := validPayoutAddress(tc.network, tc.address); got != tc.valid {
			t.Fatal(tc, got)
		}
	}
	if _, e := FormatPayoutRecipient("paypal", map[string]string{"email": "Name <payee@example.test>"}); e == nil {
		t.Fatal("display name accepted as an email")
	}
	for _, tc := range []struct{ net, rate, code, want string }{{"9.81", "7.123", "CNY", "69.88"}, {"9.81", "150.5", "JPY", "1476"}, {"9.81", "1.001", "USDT", "9.81981"}} {
		got, e := PayoutAmount(d(tc.net), d(tc.rate), tc.code)
		if e != nil || got.String() != tc.want {
			t.Fatal(tc, got, e)
		}
	}
}
