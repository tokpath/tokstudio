package paymentmethods

import (
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"testing"
)

func TestCollectionFeesUseExternalPrincipalAndCurrencyRounding(t *testing.T) {
	for _, tc := range []struct{ amount, rate, fixed, currency, fee, total string }{
		{"40", "0.02", "0", "USD", "0.8", "40.8"},
		{"100", "0", "3", "USD", "3", "103"},
		{"0.25", "0.02", "0", "USD", "0.01", "0.26"},
		{"25", "0.02", "0", "JPY", "1", "26"},
		{"0.025", "0.02", "0", "KWD", "0.001", "0.026"},
		{"100", "0", "0", "EUR", "0", "100"},
	} {
		r := Rule{Collection: true, FeeRate: ledger.MustDecimal(tc.rate), FlatFeeMoney: ledger.MustDecimal(tc.fixed)}
		q, e := CollectionQuote(r, ledger.MustDecimal(tc.amount), tc.currency)
		if e != nil || q.FeeMoney.String() != tc.fee || q.TotalMoney.String() != tc.total {
			t.Fatalf("%+v: %+v %v", tc, q, e)
		}
	}
}
