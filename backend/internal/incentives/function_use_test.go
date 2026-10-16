package incentives

import (
	"errors"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
)

func TestPostgresReviewBlockKeepsAccrualButStopsCommissionPurchaseAndNewPayoutExecution(t *testing.T) {
	f := setup(t)
	a := f.register(t, "review-inviter@example.test", "")
	b := f.register(t, "review-child@example.test", a.InviteCode)
	f.qualify(t, a.Customer.ID, "review-own-paid100")
	f.buy(t, b.Customer.ID, "review-child-first100", "100", ActivitySelection{})
	f.consume(t, b.Customer.ID, "100")
	policy, e := f.s.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, WithdrawalPolicy{MinimumUSD: d("1"), MethodReference: "verified bank payout"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	w, e := f.s.RequestWithdrawal(f.ctx, nil, WithdrawalInput{OperatorID: f.operator, CustomerID: a.Customer.ID, BusinessKey: "approved-before-review", RecipientReference: "bank:own-recipient", PolicyID: policy.ID, AmountUSD: d("5")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID); e != nil {
		t.Fatal(e)
	}
	f.seed(t, a.Customer.ID, "customer:usd", "95")
	o, e := f.commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: a.Customer.ID, QuoteID: f.quote.ID, BusinessKey: "paid-before-review", ExpectedOwnershipVersion: 1, InternalUSD: d("95"), CommissionUSD: d("5")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.SetFunctionState(f.ctx, nil, f.owner, f.operator, a.Customer.ID, "active", "temporary review; preserve accrual", true); e != nil {
		t.Fatal(e)
	}
	if _, e = f.commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: a.Customer.ID, QuoteID: f.quote.ID, BusinessKey: "new-use-under-review", ExpectedOwnershipVersion: 1, CommissionUSD: d("1")}); !errors.Is(e, ErrBlocked) {
		t.Fatal("new purchase used reviewed commission", e)
	}
	if _, e = f.commerce.Fulfill(f.ctx, nil, o.ID, "worker:review-delivery"); !errors.Is(e, ErrBlocked) {
		t.Fatal("previously reserved purchase delivered under review", e)
	}
	if _, e = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); !errors.Is(e, ErrBlocked) {
		t.Fatal("approved payout began under review", e)
	}
	if spendable, e := f.s.CommissionSpendable(f.ctx, nil, f.operator, a.Customer.ID); e != nil || !spendable.IsZero() {
		t.Fatal(spendable, e)
	}
	f.buy(t, b.Customer.ID, "review-child-second100", "100", ActivitySelection{})
	f.consume(t, b.Customer.ID, "10")
	if frozen := f.balance(t, a.Customer.ID, "commission:frozen"); frozen.Balance.Cmp(d("1.5")) != 0 {
		t.Fatal("temporary review suppressed correct accrual", frozen)
	}
	if _, e = f.s.SetFunctionState(f.ctx, nil, f.owner, f.operator, a.Customer.ID, "active", "review resolved normally", false); e != nil {
		t.Fatal(e)
	}
	if _, e = f.commerce.Fulfill(f.ctx, nil, o.ID, "worker:review-delivery"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.SetFunctionState(f.ctx, nil, f.owner, f.operator, a.Customer.ID, "active", "new review after actual payout instruction", true); e != nil {
		t.Fatal(e)
	}
	proof := WithdrawalPaymentProof{PaymentReference: "actually-paid-before-new-review", Evidence: "independently verified executed payment", Currency: "USD", OriginalAmount: "5", ValuationEvidence: "actual USD", PaidUSD: d("5"), PaidAt: time.Now().UTC()}
	if _, e = f.s.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, proof); e != nil {
		t.Fatal("real executed payment rolled back due later review", e)
	}
}
