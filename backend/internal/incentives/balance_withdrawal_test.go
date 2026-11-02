package incentives

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPostgresBalanceWithdrawalPaysTheWalletOnce(t *testing.T) {
	f := setup(t)
	customer := f.register(t, "balance@example.test", "")
	f.seed(t, customer.Customer.ID, "customer:money", "60")
	policy, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{Wallet: "balance", MethodID: uuid.NewString(), MethodName: "Balance bank", PayoutKind: "bank_cn", Enabled: true, MinimumMoney: d("1"), FlatFeeMoney: d("1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Wallet != "balance" {
		t.Fatal(policy.Wallet)
	}
	commissionMethods, err := f.s.WithdrawalMethods(f.ctx, nil, f.operator, false, "commission")
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range commissionMethods {
		if method.MethodID == policy.MethodID {
			t.Fatal("balance payout method appeared on the commission page")
		}
	}
	requested, err := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: customer.Customer.ID, BusinessKey: "balance-once", RecipientReference: "payee bank", PolicyID: policy.ID, AmountMoney: d("40")})
	if err != nil {
		t.Fatal(err)
	}
	if requested.Wallet != "balance" || requested.FeeMoney.Cmp(d("1")) != 0 || requested.NetMoney.Cmp(d("39")) != 0 || requested.Status != "requested" {
		t.Fatalf("%+v", requested)
	}
	held := f.balance(t, customer.Customer.ID, "customer:money")
	if held.Balance.Cmp(d("60")) != 0 || held.Held.Cmp(d("40")) != 0 {
		t.Fatalf("wallet hold %+v", held)
	}
	if _, err = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, requested.ID); err != nil {
		t.Fatal(err)
	}
	proof := WithdrawalPaymentProof{Evidence: "paid to the declared account", Currency: "USD", OriginalAmount: "39", ValuationEvidence: "USD payout", PaidMoney: d("39"), PaidAt: time.Now().UTC()}
	paid, err := f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, requested.ID, proof)
	if err != nil || paid.Status != "paid" || paid.NetMoney.Cmp(d("39")) != 0 {
		t.Fatal(paid, err)
	}
	settled := f.balance(t, customer.Customer.ID, "customer:money")
	if settled.Balance.Cmp(d("20")) != 0 || !settled.Held.IsZero() {
		t.Fatalf("wallet after payout %+v", settled)
	}
	if err = f.s.CancelWithdrawal(f.ctx, nil, customer.Customer.ID, requested.ID); !errors.Is(err, ErrWithdrawalState) {
		t.Fatal(err)
	}
}
