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
