package incentives

import (
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"golang.org/x/text/currency"
)

// Payout profiles are the payment methods an operator can offer. Each profile
// fixes the recipient materials a customer must supply. The operator still
// chooses the fee; later edits do not rewrite an application already stored.

type PayoutChoice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type PayoutField struct {
	Optional bool           `json:"optional,omitempty"`
	Name     string         `json:"name"`
	Label    string         `json:"label"`
	Help     string         `json:"help"`
	Kind     string         `json:"kind"`
	Options  []PayoutChoice `json:"options,omitempty"`
}

type PayoutProfile struct {
	Kind        string        `json:"kind"`
	Label       string        `json:"label"`
	OptionLabel string        `json:"option_label"`
	Summary     string        `json:"summary"`
	Fields      []PayoutField `json:"fields"`
}

func PayoutProfiles() []PayoutProfile {
	return []PayoutProfile{
		{Kind: "offline", Label: "线下支付", OptionLabel: "线下支付 · 收款人、办理说明", Summary: "与经营方联系并约定现金等线下收款安排，经营方付款后登记完成。", Fields: []PayoutField{{Name: "payee_name", Label: "收款人姓名", Kind: "text", Help: "填写实际收款人。"}, {Name: "instructions", Label: "办理说明", Kind: "text", Help: "可填写已约定的联系或交付安排。", Optional: true}}},
		{
			Kind: "bank_cn", Label: "银行转账", OptionLabel: "境内银行卡 · 户名、开户银行、卡号、支行",
			Summary: "填写户名、银行和账号。开户支行仅在实际转账需要时补充。",
			Fields: []PayoutField{
				{Name: "payee_name", Label: "收款人姓名", Kind: "text", Help: "须与银行卡户名一致。"},
				{Name: "bank_name", Label: "开户银行", Kind: "text", Help: "填写开户银行名称，例如中国工商银行。"},
				{Name: "account_number", Label: "银行卡号", Kind: "text", Help: "填写银行卡号，不要包含空格。"},
				{Name: "branch_name", Label: "开户支行", Kind: "text", Help: "实际转账需要时填写。", Optional: true},
			},
		},
		{
			Kind: "alipay", Label: "支付宝", OptionLabel: "支付宝 · 实名、登录账号",
			Summary: "支付宝用登录账号收款时，必须同时提供与该账号实名一致的姓名。",
			Fields: []PayoutField{
				{Name: "payee_name", Label: "支付宝实名", Kind: "text", Help: "须与支付宝账号的实名一致。用登录账号收款时，支付宝要求填写真实姓名。"},
				{Name: "account", Label: "支付宝账号", Kind: "text", Help: "填写可用于人工转账的支付宝登录手机号或邮箱。"},
			},
		},
		{
			Kind: "wechat", Label: "微信支付", OptionLabel: "微信支付 · 实名、微信号或手机号",
			Summary: "经营方通过微信联系并核对实际收款方式后人工付款。填写联系方式不代表已完成微信账户验证。",
			Fields: []PayoutField{
				{Name: "payee_name", Label: "微信实名", Kind: "text", Help: "供经营方联系时核对收款人。"},
				{Name: "account", Label: "微信号或绑定手机号", Kind: "text", Help: "填写可联系到你的微信号或手机号，实际转账方式与经营方确认。"},
			},
		},
		{
			Kind: "paypal", Label: "PayPal", OptionLabel: "PayPal · 邮箱、账户姓名",
			Summary: "PayPal 按邮箱定位收款账户。姓名供经营方核对，邮箱错误会使款项进入错误账户。",
			Fields: []PayoutField{
				{Name: "email", Label: "PayPal 邮箱", Kind: "text", Help: "填写接收本次付款的 PayPal 邮箱。"},
				{Name: "payee_name", Label: "账户姓名", Kind: "text", Help: "供经营方核对。PayPal 发款以邮箱为准。", Optional: true},
			},
		},
		{
			Kind: "wire", Label: "国际电汇", OptionLabel: "国际电汇 · 英文姓名、银行、账号或 IBAN、SWIFT、国家、地址",
			Summary: "国际电汇需要收款人英文姓名、银行、账号或 IBAN、SWIFT/BIC、银行所在地和收款人地址。",
			Fields: []PayoutField{
				{Name: "payee_name", Label: "收款人英文姓名", Kind: "text", Help: "与收款银行账户上的英文姓名一致。"},
				{Name: "bank_name", Label: "收款银行", Kind: "text", Help: "填写收款银行的英文名称。"},
				{Name: "account_number", Label: "账号或 IBAN", Kind: "text", Help: "填写收款账号或 IBAN，不要包含空格。"},
				{Name: "swift", Label: "SWIFT/BIC", Kind: "text", Help: "填写 8 位或 11 位 SWIFT/BIC。"},
				{Name: "country", Label: "银行所在国家或地区", Kind: "text", Help: "填写收款银行所在的国家或地区。"},
				{Name: "address", Label: "收款人地址", Kind: "text", Help: "填写收款人地址，电汇通常需要它才能入账。"},
				{Name: "bank_address", Label: "收款银行地址", Kind: "text", Help: "汇出银行要求时填写。", Optional: true},
				{Name: "instructions", Label: "补充汇款资料", Kind: "text", Help: "可补充银行要求的当地清算代码或中转行信息。", Optional: true},
			},
		},
		{
			Kind: "usdt", Label: "USDT", OptionLabel: "USDT · 网络、钱包地址",
			Summary: "客户需选择 USDT 网络并填写该网络上的钱包地址。网络和地址不一致可能导致无法到账。",
			Fields: []PayoutField{
				{Name: "network", Label: "网络", Kind: "select", Help: "必须与钱包地址的网络一致。", Options: []PayoutChoice{{Value: "TRC20", Label: "TRC20"}, {Value: "ERC20", Label: "ERC20"}}},
				{Name: "address", Label: "钱包地址", Kind: "text", Help: "填写所选网络上的 USDT 地址。选错网络可能导致无法到账。"},
			},
		},
	}
}

func FindPayoutProfile(kind string) (PayoutProfile, bool) {
	for _, profile := range PayoutProfiles() {
		if profile.Kind == kind {
			return profile, true
		}
	}
	return PayoutProfile{}, false
}

func CurrencyMinorDigits(code string) int {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 2
	}
	digits, _ := currency.Standard.Rounding(unit)
	if digits < 0 || digits > 8 {
		return 2
	}
	return digits
}

// FormatPayoutRecipient checks the materials for one profile and stores them
// as labeled lines. The withdrawal keeps this text after the method changes.
func FormatPayoutRecipient(kind string, values map[string]string) (string, error) {
	profile, ok := FindPayoutProfile(kind)
	if !ok {
		return "", ErrInvalidInput
	}
	if values == nil {
		values = map[string]string{}
	}
	allowed := map[string]PayoutField{}
	for _, field := range profile.Fields {
		allowed[field.Name] = field
	}
	for key := range values {
		if _, ok := allowed[key]; !ok {
			return "", ErrInvalidInput
		}
	}
	lines := make([]string, 0, len(profile.Fields))
	for _, field := range profile.Fields {
		value := strings.TrimSpace(values[field.Name])
		if value == "" && field.Optional {
			continue
		}
		if value == "" || utf8.RuneCountInString(value) > 500 {
			return "", ErrInvalidInput
		}
		switch field.Kind {
		case "select":
			label := ""
			for _, option := range field.Options {
				if option.Value == value {
					label = option.Label
				}
			}
			if label == "" {
				return "", ErrInvalidInput
			}
			lines = append(lines, field.Label+"："+label)
		default:
			normalized, err := normalizePayoutValue(kind, field.Name, value)
			if err != nil {
				return "", err
			}
			lines = append(lines, field.Label+"："+normalized)
		}
	}
	if kind == "usdt" && !validPayoutAddress(values["network"], values["address"]) {
		return "", ErrInvalidInput
	}
	return strings.Join(lines, "\n"), nil
}

func normalizePayoutValue(kind, name, value string) (string, error) {
	switch {
	case kind == "bank_cn" && name == "account_number":
		compact := strings.ReplaceAll(value, " ", "")
		if len(compact) < 8 || len(compact) > 30 || strings.Trim(compact, "0123456789") != "" {
			return "", ErrInvalidInput
		}
		return compact, nil
	case kind == "wire" && name == "swift":
		compact := strings.ToUpper(strings.ReplaceAll(value, " ", ""))
		if len(compact) != 8 && len(compact) != 11 {
			return "", ErrInvalidInput
		}
		for _, r := range compact {
			if (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				return "", ErrInvalidInput
			}
		}
		return compact, nil
	case kind == "wire" && name == "account_number":
		compact := strings.ReplaceAll(value, " ", "")
		if len(compact) < 6 || len(compact) > 34 {
			return "", ErrInvalidInput
		}
		return compact, nil
	case kind == "paypal" && name == "email":
		address, err := mail.ParseAddress(value)
		if err != nil || address.Address != value {
			return "", ErrInvalidInput
		}
		return value, nil
	case kind == "usdt" && name == "address":
		if strings.ContainsAny(value, " \t\n") || len(value) < 16 || len(value) > 128 {
			return "", ErrInvalidInput
		}
		return value, nil
	default:
		if utf8.RuneCountInString(value) < 2 {
			return "", ErrInvalidInput
		}
		return value, nil
	}
}

// QuoteDigits exposes the payable rounding used by quoteWithdrawal so a page
// can preview every method before the customer submits once.
func QuoteDigits(code string, amount, flat, rate, minimum ledger.Decimal) (payable, fee, net ledger.Decimal, err error) {
	quote, err := quoteWithdrawal(WithdrawalPolicy{Currency: code, FlatFeeMoney: flat, FeeRate: rate, MinimumMoney: minimum}, amount)
	if err != nil {
		return ledger.Decimal{}, ledger.Decimal{}, ledger.Decimal{}, err
	}
	return quote.AmountMoney, quote.FeeMoney, quote.NetMoney, nil
}

// Validate network-specific syntax; ownership and exchange deposit support still
// require the recipient to supply their own destination.
func validPayoutAddress(network, address string) bool {
	if network == "ERC20" {
		if len(address) != 42 || !strings.HasPrefix(address, "0x") {
			return false
		}
		_, e := hex.DecodeString(address[2:])
		return e == nil
	}
	if network != "TRC20" || len(address) != 34 || !strings.HasPrefix(address, "T") {
		return false
	}
	n := new(big.Int)
	alphabet := "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	for _, c := range address {
		v := strings.IndexRune(alphabet, c)
		if v < 0 {
			return false
		}
		n.Mul(n, big.NewInt(58))
		n.Add(n, big.NewInt(int64(v)))
	}
	b := n.Bytes()
	if len(b) != 25 || b[0] != 0x41 {
		return false
	}
	a := sha256.Sum256(b[:21])
	check := sha256.Sum256(a[:])
	return string(check[:4]) == string(b[21:])
}
