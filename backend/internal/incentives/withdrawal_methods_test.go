package incentives

import (
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestWithdrawalMethodsKeepIndependentFeesAndFreezeTheChosenPayout(t *testing.T) {
	f := setup(t)
	a := f.register(t, "method-owner@example.test", "")
	b := f.register(t, "method-buyer@example.test", a.InviteCode)
	f.qualify(t, a.Customer.ID, "method-qualification")
	f.buy(t, b.Customer.ID, "method-order", "100", ActivitySelection{})
	f.consume(t, b.Customer.ID, "100")
	fixed, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MethodID: uuid.NewString(), MethodName: "Bank transfer", Enabled: true, MinimumMoney: d("1"), FlatFeeMoney: d("1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	percent, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MethodID: uuid.NewString(), MethodName: "Wallet payout", Enabled: true, MinimumMoney: d("1"), FeeRate: d("0.02")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SetCommissionWithdrawalMinimum(f.ctx, nil, f.owner, f.operator, d("5")); err != nil {
		t.Fatal(err)
	}
	q, err := f.s.QuoteWithdrawal(f.ctx, nil, f.operator, d("10"), percent.MethodID)
	if err != nil || q.FeeMoney.String() != "0.2" || q.NetMoney.String() != "9.8" || q.Policy.MinimumMoney.String() != "5" {
		t.Fatal("minimum change overwrote method fees", q, err)
	}
	precise, err := f.s.QuoteWithdrawal(f.ctx, nil, f.operator, d("10.0134"), percent.MethodID)
	if err != nil || precise.AmountMoney.String() != "10.01" || precise.FeeMoney.String() != "0.2" || precise.NetMoney.String() != "9.81" {
		t.Fatal("withdrawal quote must keep wallet tails and produce a payable amount", precise, err)
	}
	w, err := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: a.Customer.ID, BusinessKey: "fixed-request", PolicyID: fixed.ID, AmountMoney: d("10.0134"), RecipientReference: "verified recipient"})
	if err != nil || w.MethodName != "Bank transfer" || w.AmountMoney.String() != "10.01" || w.NetMoney.String() != "9.01" {
		t.Fatal(w, err)
	}
	fixed.FlatFeeMoney = d("2")
	fixed.Enabled = false
	if _, err = f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, fixed, fixed.Version); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.QuoteWithdrawal(f.ctx, nil, f.operator, d("10"), fixed.MethodID); err == nil {
		t.Fatal("disabled method still accepts applications")
	}
	if _, err = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); err != nil {
		t.Fatal(err)
	}
	paid, err := f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, WithdrawalPaymentProof{Evidence: "Verified offline payout", Currency: "USD", OriginalAmount: "9.01", ValuationEvidence: "actual USD transfer", PaidMoney: d("9.01"), PaidAt: time.Now().UTC()})
	if err != nil || paid.FeeMoney.String() != "1" || paid.NetMoney.String() != "9.01" {
		t.Fatal("method change altered the accepted payout", paid, err)
	}
}
