package commerce

import (
	"errors"
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresReceiptSeparatesCustomerCreditFromActualSettlement(t *testing.T) {
	for _, tc := range []struct{ name, payerCurrency, payerAmount, netCurrency, net string }{
		{"provider adds a payer fee", "USD", "103", "USD", "100.02"},
		{"provider settles in another currency", "USD", "102", "MYR", "400.02"},
		{"voucher asset", "MYR", "408", "MYR", "408"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t)
			o, err := f.s.CreateTopUpOrder(f.ctx, nil, CreateTopUpInput{CustomerID: f.customer, OperatorID: f.operator, BusinessKey: "topup", ExpectedOwnershipVersion: 1, AmountMoney: d("100")})
			if err != nil {
				t.Fatal(err)
			}
			fact := f.fact("provider-payment", "102", "0", o.CreatedAt)
			fact.PaymentFeeMoney = d("2")
			fact.FeeStatus, fact.FeeMoney, fact.NetMoney = "pending", nil, nil
			fact.Currency, fact.OriginalAmount = tc.payerCurrency, tc.payerAmount
			r, err := f.s.RecordReceipt(f.ctx, nil, fact)
			if err != nil {
				t.Fatal(err)
			}
			if r.SettlementNetAmount != nil {
				t.Fatal("unknown net was invented")
			}
			f.allocate(t, "topup", r, o, "100")
			if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); err != nil {
				t.Fatal(err)
			}
			if got := f.balance(t, "customer:money").Balance.String(); got != "100" {
				t.Fatal("wrong customer credit", got)
			}
			for i := 0; i < 2; i++ {
				if err = f.s.RecordReceiptSettlement(f.ctx, nil, r.ID, tc.netCurrency, d(tc.net), "adapter:test"); err != nil {
					t.Fatal(err)
				}
			}
			var cash ledger.Account
			if err = f.db.Where("operator_id=? AND purpose=? AND unit=?", f.operator, "control:external_cash", tc.netCurrency).First(&cash).Error; err != nil || cash.Balance.Cmp(d(tc.net).Neg()) != 0 {
				t.Fatal("wrong actual cash", cash, err)
			}
			var invented int64
			if err = f.db.Table("ledger_accounts").Where("operator_id=? AND purpose IN ? AND unit=? AND balance<>0", f.operator, []string{"control:external_cash", "control:provider_receivable"}, ledger.MoneyUnit).Count(&invented).Error; err != nil || invented != 0 {
				t.Fatal("invented order-currency cash", invented, err)
			}
			if err = f.s.RecordReceiptSettlement(f.ctx, nil, r.ID, tc.netCurrency, d(tc.net).Add(d("1")), "adapter:test"); !errors.Is(err, ErrConflict) {
				t.Fatal("conflicting settlement accepted", err)
			}
			if tc.payerCurrency == "USD" && tc.netCurrency == "USD" {
				fee := d("2.98")
				if _, err = f.s.RecordReceiptAccounting(f.ctx, nil, ReceiptAccounting{ReceiptID: r.ID, OriginalFeeAmount: &fee, VerifiedBy: "adapter:test"}); err != nil {
					t.Fatal("actual fees confused with order fee", err)
				}
			}
		})
	}
}
