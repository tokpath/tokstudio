package payment

import (
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/billing"
)

// FenPerUSD 是沙箱默认汇率：7.15 CNY = 1 USD，用分避免浮点。
// 真实牌价以后由平台配置；渠道不能改。
const FenPerUSD int64 = 715

func PayCurrency(adapter string) string {
	if a, ok := DefaultRegistry().Get(adapter); ok && a.Spec().PayCurrency != "" {
		return a.Spec().PayCurrency
	}
	return "USD"
}

func QuotePay(adapter string, payMajor, feeBPS, issueBPS int64) (*QuoteView, error) {
	if payMajor <= 0 {
		return nil, ErrInvalidAmount
	}
	if issueBPS <= 0 {
		issueBPS = billing.DefaultIssueRatioBPS
	}
	if feeBPS < 0 {
		feeBPS = 0
	}
	adapter = strings.ToLower(strings.TrimSpace(adapter))
	if adapter == "" {
		adapter = AdapterStripe
	}
	spec, _ := specOf(adapter)
	currency := spec.PayCurrency
	if currency == "" {
		currency = PayCurrency(adapter)
	}
	view := &QuoteView{
		Adapter:       adapter,
		PayMajor:      payMajor,
		PayCurrency:   currency,
		FeeBPS:        feeBPS,
		IssueRatioBPS: issueBPS,
		FenPerUSD:     FenPerUSD,
		AutoRenew:     spec.AutoRenew,
	}
	if currency == "CNY" {
		payFen := payMajor * 100
		feeFen := payFen * feeBPS / 10000
		netFen := payFen - feeFen
		if netFen <= 0 {
			return nil, ErrInvalidAmount
		}
		wallet := netFen * billing.MinorPerUSD / FenPerUSD
		if wallet <= 0 {
			return nil, ErrInvalidAmount
		}
		grant, err := billing.ConvertQuota(wallet, issueBPS)
		if err != nil {
			return nil, err
		}
		view.PayCurrency = "CNY"
		view.PayMinor = payFen
		view.FeeMinor = feeFen
		view.WalletMinor = wallet
		view.CreditMinor = grant
		return view, nil
	}
	payUSD := payMajor * billing.MinorPerUSD
	feeUSD := payUSD * feeBPS / 10000
	net := payUSD - feeUSD
	if net <= 0 {
		return nil, ErrInvalidAmount
	}
	grant, err := billing.ConvertQuota(net, issueBPS)
	if err != nil {
		return nil, err
	}
	view.PayMinor = payUSD
	view.FeeMinor = feeUSD
	view.WalletMinor = net
	view.CreditMinor = grant
	return view, nil
}
