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
	if _, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MethodID: uuid.NewString(), MethodName: "Missing kind", Enabled: true, MinimumMoney: d("1"), FlatFeeMoney: d("1")}, 0); err != ErrInvalidInput {
		t.Fatal("a payout method without a payment profile was accepted", err)
	}
	fixed, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MethodID: uuid.NewString(), MethodName: "Bank transfer", PayoutKind: "bank_cn", Enabled: true, MinimumMoney: d("1"), FlatFeeMoney: d("1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	percent, err := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MethodID: uuid.NewString(), MethodName: "Wallet payout", PayoutKind: "alipay", Enabled: true, MinimumMoney: d("1"), FeeRate: d("0.02")}, 0)
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
	paid, err := f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, WithdrawalPaymentProof{Currency: "USD", OriginalAmount: "9.01", ValuationEvidence: "actual USD transfer", PaidMoney: d("9.01"), PaidAt: time.Now().UTC()})
	if err != nil || paid.FeeMoney.String() != "1" || paid.NetMoney.String() != "9.01" {
		t.Fatal("method change altered the accepted payout", paid, err)
	}
}

func TestWithdrawalFreezesForeignPayoutBeforeApplication(t *testing.T) {
	f := setup(t)
	c := f.register(t, "foreign-payout@example.test", "")
	f.seed(t, c.Customer.ID, "customer:money", "100")
	p, e := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MethodID: uuid.NewString(), PayoutKind: "paypal", MethodName: "PayPal", Enabled: true, MinimumMoney: d("1"), FlatFeeMoney: d("1"), PayoutCurrency: "EUR", PayoutRate: d("0.9")}, 0)
	if e != nil {
		t.Fatal(e)
	}
	in := WithdrawalInput{Wallet: "balance", OperatorID: f.operator, CustomerID: c.Customer.ID, BusinessKey: "foreign-payout", PolicyID: p.ID, AmountMoney: d("40"), RecipientReference: "PayPal: recipient@example.test"}
	w, e := f.s.RequestWithdrawal(f.ctx, nil, in)
	if e != nil || w.PayoutCurrency != "EUR" || w.PayoutAmount.String() != "35.1" {
		t.Fatal(w, e)
	}
	p.PayoutRate = d("0.8")
	if _, e = f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, p, p.Version); e != nil {
		t.Fatal(e)
	}
	replay, e := f.s.RequestWithdrawal(f.ctx, nil, in)
	if e != nil || replay.ID != w.ID || replay.PayoutAmount.String() != "35.1" {
		t.Fatal(replay, e)
	}
	if _, e = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID); e != nil {
		t.Fatal(e)
	}
	proof := WithdrawalPaymentProof{Currency: "EUR", OriginalAmount: "31.2", PaidAt: time.Now().UTC()}
	if _, e = f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, proof); e == nil {
		t.Fatal("changed-rate payment accepted")
	}
	proof.Currency = "USD"
	proof.OriginalAmount = "35.1"
	if _, e = f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, proof); e == nil {
		t.Fatal("wrong currency accepted")
	}
	proof.Currency = ""
	proof.OriginalAmount = ""
	paid, e := f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, proof)
	if e != nil || paid.Status != "paid" || paid.PayoutAmount.String() != "35.1" {
		t.Fatal(paid, e)
	}
	balance := f.balance(t, c.Customer.ID, "customer:money")
	if balance.Balance.String() != "60" || !balance.Held.IsZero() {
		t.Fatal(balance)
	}
}
