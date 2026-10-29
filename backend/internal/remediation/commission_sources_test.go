package remediation

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
)

type commissionSourceFixture struct {
	fixture
	source, normal commerce.Receipt
	sourceEvent    ledger.GrantEvent
}

func commissionSources(t *testing.T) commissionSourceFixture {
	t.Helper()
	f := setup(t)
	if _, e := f.s.Incentives.SetPolicy(f.ctx, nil, f.owner, f.operator, incentives.PolicyInput{ExpectedVersion: 1, ThresholdMoney: d("0"), DirectRate: d("0.15"), IndirectRate: d("0.05"), RewardValiditySeconds: 86400}); e != nil {
		t.Fatal(e)
	}
	q, e := f.s.Incentives.GetQualification(f.ctx, nil, f.operator, f.customer)
	if e != nil || q.AcquiredAt == nil {
		t.Fatal(q, e)
	}
	invite, e := f.s.Organization.CustomerInvite(f.ctx, nil, f.customer)
	if e != nil {
		t.Fatal(e)
	}
	op, e := f.s.Organization.GetOperator(f.ctx, nil, f.operator)
	if e != nil {
		t.Fatal(e)
	}
	result := commissionSourceFixture{fixture: f}
	for i, key := range []string{"disputed-commission-source", "normal-commission-source"} {
		intent, e := f.s.Organization.BeginRegistration(f.ctx, nil, organization.RegistrationIntentRequest{BrandID: op.BrandID, Email: key + "@example.test", InviteCode: invite, Password: "only-test-long-password"})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.s.Organization.VerifyEmail(f.ctx, nil, intent.Intent.ID, intent.VerificationToken); e != nil {
			t.Fatal(e)
		}
		customer, e := f.s.Organization.CompleteRegistration(f.ctx, nil, organization.CompleteRegistrationInput{IntentID: intent.Intent.ID, VerificationToken: intent.VerificationToken})
		if e != nil {
			t.Fatal(e)
		}
		child := f
		child.customer = customer.Customer.ID
		o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: child.customer, QuoteID: f.quote.ID, BusinessKey: key, ExpectedOwnershipVersion: 1})
		if e != nil {
			t.Fatal(e)
		}
		r := f.receipt(t, key, "80")
		if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: key, AmountMoney: d("80")}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
			t.Fatal(e)
		}
		events := child.consumeObserved(t, "100")
		if len(events) != 1 {
			t.Fatal(events)
		}
		if i == 0 {
			result.source = r
			result.sourceEvent = events[0]
		} else {
			result.normal = r
		}
	}
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	if e != nil || a.Available().Cmp(d("24")) != 0 {
		t.Fatalf("real two-source commissions %+v %v", a, e)
	}
	return result
}
func (f commissionSourceFixture) withdrawal(t *testing.T, key string, begin bool) incentives.Withdrawal {
	t.Helper()
	policy, e := f.s.Incentives.SetWithdrawalPolicy(f.ctx, nil, f.owner, f.operator, incentives.WithdrawalPolicy{MinimumMoney: d("1"), MethodReference: "verified manual banking"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	w, e := f.s.Incentives.RequestWithdrawal(f.ctx, nil, incentives.WithdrawalInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: key, RecipientReference: "private:payer-profile", PolicyID: policy.ID, AmountMoney: d("12")})
	if e != nil {
		t.Fatal(e)
	}
	w, e = f.s.Incentives.ApproveWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID)
	if e != nil {
		t.Fatal(e)
	}
	if begin {
		w, e = f.s.Incentives.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID)
		if e != nil {
			t.Fatal(e)
		}
	}
	return w
}
func TestPostgresCommissionPurchaseRefundRestoresOriginalEntryAndDisputeDoesNotFreezeNormalSource(t *testing.T) {
	f := commissionSources(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "commission-purchase", ExpectedOwnershipVersion: 1, CommissionMoney: d("12")})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "external-rest-of-commission-purchase", "68")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "rest-purchase", AmountMoney: d("68")}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	views, e := f.s.Incentives.OrderCommissionFunding(f.ctx, nil, f.operator, f.customer, o.ID)
	if e != nil || len(views) != 1 || views[0].SourceEventID != f.sourceEvent.ID || views[0].AmountMoney.Cmp(d("12")) != 0 || views[0].State != "consumed" {
		t.Fatal(views, e)
	}
	c, e := f.s.RequestPackageException(f.ctx, PackageExceptionInput{OperatorID: f.operator, CustomerID: f.customer, OrderID: o.ID, BusinessKey: "cannot-perform-commission-package", ActorStaffID: f.owner, Reason: "unable_to_fulfill", Evidence: "verified total fulfilment failure"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "approved actual unused rights return"); e != nil {
		t.Fatal(e)
	}
	views, e = f.s.Incentives.OrderCommissionFunding(f.ctx, nil, f.operator, f.customer, o.ID)
	if e != nil || len(views) != 1 || views[0].RestoredMoney.Cmp(d("12")) != 0 || views[0].SourceEventID != f.sourceEvent.ID {
		t.Fatal("restoration lost original source", views, e)
	}
	cb := f.chargeback(t, f.source, "dispute-restored-origin", "80")
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "restrict only original restored commission source"); e != nil {
		t.Fatal(e)
	}
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	if e != nil || a.Held.Cmp(d("12")) != 0 || a.Available().Cmp(d("12")) != 0 {
		t.Fatal("unrelated commission source frozen", a, e)
	}
	spendable, e := f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || spendable.Cmp(d("12")) != 0 {
		t.Fatal(spendable, e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "merchant_collection_risk", "valid payment verified"); e != nil {
		t.Fatal(e)
	}
	spendable, e = f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || spendable.Cmp(d("24")) != 0 {
		t.Fatal("source not restored after independent release", spendable, e)
	}
}
func TestPostgresCommissionDisputeBlocksNewExecutionBeforeRestrictionReviewAndKeepsOtherSourceUsable(t *testing.T) {
	f := commissionSources(t)
	w := f.withdrawal(t, "pending-source-withdrawal", false)
	cb := f.chargeback(t, f.source, "already-reserved-source-dispute", "80")
	if _, e := f.s.Incentives.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID); !errors.Is(e, incentives.ErrBlocked) {
		t.Fatal("issued new transfer after original source dispute", e)
	}
	if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "source is still reserved by withdrawal"); !errors.Is(e, ErrReview) {
		t.Fatal("misclassified in-flight source restriction", e)
	}
	var recorded Chargeback
	if e := f.db.First(&recorded, "id=?", cb.ID).Error; e != nil || recorded.RestrictionsAppliedAt != nil {
		t.Fatal(recorded, e)
	}
	// An order can use the independent second source while the original source
	// remains blocked, rather than freezing the customer's whole commission.
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "normal-source-still-works", ExpectedOwnershipVersion: 1, CommissionMoney: d("12")})
	if e != nil {
		t.Fatal(e)
	}
	views, e := f.s.Incentives.OrderCommissionFunding(f.ctx, nil, f.operator, f.customer, o.ID)
	if e != nil || len(views) != 1 || views[0].SourceEventID == f.sourceEvent.ID {
		t.Fatal("new order reused disputed source", views, e)
	}
	if _, e = f.s.Commerce.CloseOrder(f.ctx, nil, f.customer, o.ID, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "independent payment lookup unpaid"}); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Incentives.FailWithdrawal(f.ctx, nil, f.owner, f.operator, w.ID, "bank verified original instruction never executed", true); e != nil {
		t.Fatal(e)
	}
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "original reservation now cleared"); e != nil {
		t.Fatal(e)
	}
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	if e != nil || a.Held.Cmp(d("12")) != 0 || a.Available().Cmp(d("12")) != 0 {
		t.Fatal(a, e)
	}
}
func TestPostgresVerifiedCommissionPayoutFactAfterDisputeRemainsRecordable(t *testing.T) {
	f := commissionSources(t)
	w := f.withdrawal(t, "already-executed-source-withdrawal", true)
	cb := f.chargeback(t, f.source, "dispute-after-bank-instruction", "80")
	if _, e := f.s.Incentives.VerifyWithdrawalPaid(f.ctx, nil, f.owner, f.operator, w.ID, incentives.WithdrawalPaymentProof{PaymentReference: "real-bank-payout", Evidence: "independently verified completed original instruction", Currency: "USD", OriginalAmount: "12", PaidMoney: d("12"), ValuationEvidence: "actual USD", PaidAt: time.Now().UTC()}); e != nil {
		t.Fatal("real executed fact rolled back by new gate", e)
	}
	if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "freeze only still controllable original source"); e != nil {
		t.Fatal(e)
	}
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	if e != nil || !a.Held.IsZero() || a.Available().Cmp(d("12")) != 0 {
		t.Fatal("spent disputed source seized another source", a, e)
	}
}
func TestPostgresCommissionDisputeAndPayoutInstructionRaceUsesOneBusinessOrder(t *testing.T) {
	f := commissionSources(t)
	w := f.withdrawal(t, "racing-source-withdrawal", false)
	var wg sync.WaitGroup
	var cb Chargeback
	var cbErr, payErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		cb, cbErr = f.s.RecordChargeback(f.ctx, ChargebackInput{ActorID: f.owner, OperatorID: f.operator, ReceiptID: f.source.ID, Provider: "verified-bank", ProviderCaseID: "racing-dispute", AmountMoney: d("80"), Reason: "provider dispute", Evidence: "independently queried dispute"})
	}()
	go func() {
		defer wg.Done()
		_, payErr = f.s.Incentives.BeginWithdrawalPayment(f.ctx, nil, f.owner, f.operator, w.ID)
	}()
	wg.Wait()
	if cbErr != nil {
		t.Fatal(cbErr)
	}
	if payErr != nil && !errors.Is(payErr, incentives.ErrBlocked) {
		t.Fatal(payErr)
	}
	if payErr == nil {
		var event incentives.Event
		if e := f.db.Where("aggregate_id=? AND kind='withdrawal.payment_intent_recorded'", w.ID).First(&event).Error; e != nil {
			t.Fatal(e)
		}
		if event.CreatedAt.After(cb.CreatedAt) {
			t.Fatal("a newly issued payout escaped an already registered source dispute", event.CreatedAt, cb.CreatedAt)
		}
	}
}

func TestPostgresCommissionDisputeBlocksPreviouslyReservedOrderDeliveryWithoutErasingActualReceipt(t *testing.T) {
	f := commissionSources(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "funded-before-source-dispute", ExpectedOwnershipVersion: 1, CommissionMoney: d("12")})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "actual-payer-rest-before-dispute", "68")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "actual-rest-funded", AmountMoney: d("68")}); e != nil {
		t.Fatal(e)
	}
	cb := f.chargeback(t, f.source, "source-dispute-before-delivery", "80")
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); !errors.Is(e, incentives.ErrBlocked) {
		t.Fatal("delivered using a newly disputed original commission source", e)
	}
	got, e := f.s.Commerce.GetOrder(f.ctx, nil, f.customer, o.ID)
	if e != nil || got.Status != "paid" || got.ExternalAllocatedMoney.Cmp(d("68")) != 0 {
		t.Fatal("real external receipt erased after blocked fulfilment", got, e)
	}
	var deliveries int64
	if e = f.db.Table("commerce_deliveries").Where("order_id=?", o.ID).Count(&deliveries).Error; e != nil || deliveries != 0 {
		t.Fatal("blocked source minted entitlements", deliveries, e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "merchant_collection_risk", "original payment independently proved valid"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal("unrelated collection issue permanently damaged order", e)
	}
}

func TestPostgresPartialCommissionDisputesFreezeOnlyTheirOwnShareAndReleaseIndependently(t *testing.T) {
	f := commissionSources(t)
	a := f.chargeback(t, f.source, "commission-partial-a", "40")
	b := f.chargeback(t, f.source, "commission-partial-b", "40")
	for _, cb := range []Chargeback{a, b} {
		if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "two independent partial cases"); e != nil {
			t.Fatal(e)
		}
	}
	account, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	if e != nil || account.Held.Cmp(d("12")) != 0 || account.Available().Cmp(d("12")) != 0 {
		t.Fatal("two partial commission disputes compounded/overfroze", account, e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, a.ID, "merchant_collection_risk", "only first case proved valid"); e != nil {
		t.Fatal(e)
	}
	account, e = f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	spendable, bookErr := f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || bookErr != nil || account.Held.Cmp(d("6")) != 0 || spendable.Cmp(d("18")) != 0 {
		t.Fatal("one resolution released another case", account, spendable, e, bookErr)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, b.ID, "invalid_payment", "second half actually invalid"); e != nil {
		t.Fatal(e)
	}
	account, e = f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	spendable, bookErr = f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || bookErr != nil || !account.Held.IsZero() || account.Balance.Cmp(d("18")) != 0 || spendable.Cmp(d("18")) != 0 {
		t.Fatal("partial correction broke source conservation", account, spendable, e, bookErr)
	}
}

func TestPostgresRestoringSpentCommissionDuringDisputeKeepsItsOriginalSourceGate(t *testing.T) {
	f := commissionSources(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "spent-before-dispute", ExpectedOwnershipVersion: 1, CommissionMoney: d("12")})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "actual-rest-of-spent-commission", "68")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "spent-rest", AmountMoney: d("68")}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	cb := f.chargeback(t, f.source, "spent-source-then-restored", "80")
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "original commission already spent"); e != nil {
		t.Fatal(e)
	}
	c, e := f.s.RequestPackageException(f.ctx, PackageExceptionInput{OperatorID: f.operator, CustomerID: f.customer, OrderID: o.ID, BusinessKey: "restore-after-source-dispute", ActorStaffID: f.owner, Reason: "unable_to_fulfill", Evidence: "merchant cannot perform independent purchased package"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified unused package return"); e != nil {
		t.Fatal(e)
	}
	spendable, e := f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || spendable.Cmp(d("12")) != 0 {
		t.Fatal("returned disputed commission became a clean new source", spendable, e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "invalid_payment", "verified original payment invalid"); e != nil {
		t.Fatal(e)
	}
	a, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	spendable, bookErr := f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || bookErr != nil || a.Available().Cmp(d("12")) != 0 || spendable.Cmp(d("12")) != 0 {
		t.Fatal("source correction failed after restoration", a, spendable, e, bookErr)
	}
	var originalSpent ledger.Decimal
	if e = f.db.Table("incentives_commission_funding_lots l").Select("l.consumed_money::text").Joins("JOIN incentives_commission_entries c ON c.id=l.entry_id").Joins("JOIN incentives_confirmation_events e ON e.id=c.event_id").Where("e.source_event_id=?", f.sourceEvent.ID).Row().Scan(&originalSpent); e != nil || originalSpent.Cmp(d("12")) != 0 {
		t.Fatal("correction consumed a normal source before original restored funds", originalSpent, e)
	}
}

func TestPostgresLaterPartialCommissionDisputeUsesBasisAfterPriorInvalidPayment(t *testing.T) {
	f := commissionSources(t)
	a := f.chargeback(t, f.source, "first-half-invalid", "40")
	if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, a.ID, "first half reviewed"); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.ResolveChargeback(f.ctx, f.owner, f.operator, a.ID, "invalid_payment", "first half proved invalid"); e != nil {
		t.Fatal(e)
	}
	b := f.chargeback(t, f.source, "later-second-half-disputed", "40")
	spendable, e := f.s.Incentives.CommissionSpendable(f.ctx, nil, f.operator, f.customer)
	if e != nil || spendable.Cmp(d("12")) != 0 {
		t.Fatal("later dispute used original instead of remaining paid basis", spendable, e)
	}
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, b.ID, "remaining half reviewed"); e != nil {
		t.Fatal(e)
	}
	account, e := f.s.account(f.ctx, f.db, f.operator, f.customer, "commission:available")
	if e != nil || account.Held.Cmp(d("6")) != 0 || account.Available().Cmp(d("12")) != 0 {
		t.Fatal("remaining commission source under-frozen", account, e)
	}
}
