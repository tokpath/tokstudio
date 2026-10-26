package remediation

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"testing"
	"time"
)

func (f fixture) receiptNamed(t *testing.T, key string) commerce.Receipt {
	t.Helper()
	var r commerce.Receipt
	if e := f.db.Table("commerce_receipts").Where("operator_id=? AND provider_receipt_id=?", f.operator, key).Take(&r).Error; e != nil {
		t.Fatal(e)
	}
	return r
}
func (f fixture) chargeback(t *testing.T, r commerce.Receipt, key, amount string) Chargeback {
	t.Helper()
	c, e := f.s.RecordChargeback(f.ctx, ChargebackInput{ActorID: f.owner, OperatorID: f.operator, ReceiptID: r.ID, Provider: "verified-bank", ProviderCaseID: key, AmountUSD: d(amount), Reason: "provider payment dispute", Evidence: "independently queried provider case"})
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func (f fixture) packageReceipts(t *testing.T) (commerce.Order, ledger.Grant, []commerce.Receipt) {
	t.Helper()
	var receipts []commerce.Receipt
	for _, key := range []string{"source-a", "source-b"} {
		topup, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "topup:" + key, ExpectedOwnershipVersion: 1, AmountUSD: d("40")})
		if e != nil {
			t.Fatal(e)
		}
		r := f.receipt(t, key, "40")
		receipts = append(receipts, r)
		if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: topup.ID, BusinessKey: key, AmountUSD: d("40")}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.s.Commerce.Fulfill(f.ctx, nil, topup.ID, f.owner); e != nil {
			t.Fatal(e)
		}
	}
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{OperatorID: f.operator, CustomerID: f.customer, QuoteID: f.quote.ID, BusinessKey: "source-package", ExpectedOwnershipVersion: 1, InternalUSD: d("80")})
	if e != nil {
		t.Fatal(e)
	}
	delivery, e := f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner)
	if e != nil {
		t.Fatal(e)
	}
	g, e := f.s.Ledger.GetGrant(f.ctx, nil, *delivery.GrantID)
	if e != nil {
		t.Fatal(e)
	}
	return o, g, receipts
}
func (f fixture) consumeObserved(t *testing.T, quantity string) []ledger.GrantEvent {
	t.Helper()
	var events []ledger.GrantEvent
	e := f.db.Transaction(func(tx *gorm.DB) error {
		if e := f.s.Cooperation.Lock(f.ctx, tx, f.operator); e != nil {
			return e
		}
		if e := f.s.Incentives.LockBusiness(f.ctx, tx, f.operator); e != nil {
			return e
		}
		at, e := instant(tx)
		if e != nil {
			return e
		}
		key := "consume:" + uuid.NewString()
		h, e := f.s.Ledger.ReserveCredits(f.ctx, tx, ledger.CreditReservation{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: key, Estimate: d(quantity), At: at, ExpiresAt: at.Add(time.Hour)})
		if e != nil {
			return fmt.Errorf("test source reserve: %w", e)
		}
		result, e := f.s.Ledger.SettleCredits(f.ctx, tx, f.operator, f.customer, h.ID, key, d(quantity), at)
		if e != nil {
			return fmt.Errorf("test source settle: %w", e)
		}
		for _, event := range result.Events {
			g, e := f.s.Ledger.GetGrant(f.ctx, tx, event.GrantID)
			if e != nil {
				return e
			}
			if e = f.s.Incentives.ObserveGrantEvent(f.ctx, tx, g, event); e != nil {
				return fmt.Errorf("test source observe: %w", e)
			}
		}
		events = result.Events
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return events
}

func TestPostgresChargebackRepeatedPartialClaimsConserveWalletAndReleaseOnlyOwnSource(t *testing.T) {
	f := setup(t)
	f.topup(t, "wallet-source", "100")
	r := f.receiptNamed(t, "wallet-source")
	a := f.chargeback(t, r, "partial-a", "50")
	b := f.chargeback(t, r, "partial-b", "50")
	for _, cb := range []Chargeback{a, b} {
		if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "freeze only verified original source"); e != nil {
			t.Fatal(e)
		}
	}
	if cash := f.balance(t); cash.Held.Cmp(d("100")) != 0 {
		t.Fatalf("50%%+50%% left source spendable: %+v", cash)
	}
	if _, e := f.s.ResolveChargeback(f.ctx, f.owner, f.operator, a.ID, "merchant_collection_risk", "bank collection risk does not invalidate customer payment"); e != nil {
		t.Fatal(e)
	}
	if cash := f.balance(t); cash.Held.Cmp(d("50")) != 0 || cash.Available().Cmp(d("50")) != 0 {
		t.Fatalf("resolving one case released another: %+v", cash)
	}
	if _, e := f.s.ResolveChargeback(f.ctx, f.owner, f.operator, b.ID, "invalid_payment", "verified original payment partially invalid"); e != nil {
		t.Fatal(e)
	}
	if cash := f.balance(t); cash.Balance.Cmp(d("50")) != 0 || !cash.Held.IsZero() {
		t.Fatalf("source cancellation touched wrong cash: %+v", cash)
	}
	cap, e := f.s.ReceiptRefundCapacity(f.ctx, nil, f.operator, r.ID)
	if e != nil || cap.AvailableUSD.Cmp(d("50")) != 0 {
		t.Fatalf("capacity=%+v %v", cap, e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, b.ID, "invalid_payment", "replay classification"); e != nil {
		t.Fatal(e)
	}
}

func TestPostgresChargebackAcrossTwoReceiptsConservesGrantAndIndependentDisputeOwners(t *testing.T) {
	f := setup(t)
	order, g, receipts := f.packageReceipts(t)
	events := f.consumeObserved(t, "25")
	if len(events) != 1 || events[0].PaidUSD.Cmp(d("20")) != 0 {
		t.Fatal(events)
	}
	if e := f.s.Incentives.SetEventDispute(f.ctx, nil, f.owner, f.operator, events[0].ID, "separate manual billing review", true); e != nil {
		t.Fatal(e)
	}
	a := f.chargeback(t, receipts[0], "source-a-dispute", "40")
	b := f.chargeback(t, receipts[1], "source-b-dispute", "40")
	for _, cb := range []Chargeback{a, b} {
		if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "trace source fraction"); e != nil {
			t.Fatal(e)
		}
	}
	account, e := f.s.Ledger.AccountInTransaction(f.ctx, f.db, g.AccountID)
	if e != nil || account.Held.Cmp(d("75")) != 0 {
		t.Fatalf("two paid sources did not cover remaining grant: %+v %v", account, e)
	}
	var claimed ledger.Decimal
	if e = f.db.Model(&chargebackEvent{}).Select("sum(amount_usd)::text").Where("source_event_id=?", events[0].ID).Row().Scan(&claimed); e != nil || claimed.Cmp(d("20")) != 0 {
		t.Fatalf("event base double claimed=%s %v", claimed, e)
	}
	for _, cb := range []Chargeback{a, b} {
		if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "merchant_collection_risk", "verified valid customer payment"); e != nil {
			t.Fatal(e)
		}
	}
	state, e := f.s.Incentives.RemediationEvents(f.ctx, nil, f.operator, order.ID)
	if e != nil || len(state) != 1 || state[0].Status != "disputed" {
		t.Fatalf("chargeback release cleared unrelated review: %+v %v", state, e)
	}
	if e = f.s.Incentives.SetEventDispute(f.ctx, nil, f.owner, f.operator, events[0].ID, "manual review resolved independently", false); e != nil {
		t.Fatal(e)
	}
	state, e = f.s.Incentives.RemediationEvents(f.ctx, nil, f.operator, order.ID)
	if e != nil || state[0].Status != "pending" {
		t.Fatal(state, e)
	}
}

func TestPostgresChargebackInvalidTwoSourcesCancelsOnlyRemainingRightsAndCorrectsPaidBase(t *testing.T) {
	f := setup(t)
	_, g, receipts := f.packageReceipts(t)
	events := f.consumeObserved(t, "25")
	a := f.chargeback(t, receipts[0], "invalid-a", "40")
	b := f.chargeback(t, receipts[1], "invalid-b", "40")
	for _, cb := range []Chargeback{a, b} {
		if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "verified payment source under review"); e != nil {
			t.Fatal(e)
		}
	}
	for _, cb := range []Chargeback{a, b} {
		if _, e := f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "invalid_payment", "verified original source invalid"); e != nil {
			t.Fatal(e)
		}
	}
	g, e := f.s.Ledger.GetGrant(f.ctx, nil, g.ID)
	if e != nil || g.Cancelled.Cmp(d("75")) != 0 || g.CancelledPaidUSD.Cmp(d("60")) != 0 || g.ConsumedPaidUSD.Cmp(d("20")) != 0 {
		t.Fatalf("invalid source grant classification=%+v %v", g, e)
	}
	var corrected ledger.Decimal
	if e = f.db.Table("incentives_corrections c").Select("sum(c.amount_usd)::text").Joins("JOIN incentives_confirmation_events e ON e.id=c.original_event_id").Where("e.source_event_id=?", events[0].ID).Row().Scan(&corrected); e != nil || corrected.Cmp(d("20")) != 0 {
		t.Fatalf("original paid consumption over/under-reversed=%s %v", corrected, e)
	}
	q, e := f.s.Incentives.Advance(f.ctx, nil, f.operator, f.customer, time.Now().Add(2*time.Hour))
	if e != nil || !q.ConfirmedUSD.IsZero() {
		t.Fatalf("invalid paid base confirmed qualification=%+v %v", q, e)
	}
}

func TestPostgresChargebackFreezesAllocatedUndeliveredFundsAndPersistsException(t *testing.T) {
	f := setup(t)
	o, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "undelivered-topup", ExpectedOwnershipVersion: 1, AmountUSD: d("100")})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "undelivered-source", "100")
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "undelivered-allocation", AmountUSD: d("100")}); e != nil {
		t.Fatal(e)
	}
	cb := f.chargeback(t, r, "undelivered-dispute", "100")
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "protect allocated real payment before delivery"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); !errors.Is(e, ledger.ErrInsufficient) {
		t.Fatalf("disputed order funds delivered: %v", e)
	}
	var lots int64
	if e = f.db.Table("commerce_wallet_funding_lots").Count(&lots).Error; e != nil || lots != 0 {
		t.Fatalf("failed topup minted provenance=%d %v", lots, e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "invalid_payment", "provider independently verified invalid payment"); e != nil {
		t.Fatal(e)
	}
	o, e = f.s.Commerce.GetOrder(f.ctx, nil, f.customer, o.ID)
	if e != nil || o.Status != "exception" || o.ExternalAllocatedUSD.Cmp(d("100")) != 0 || o.ExceptionReason == "" {
		t.Fatalf("actual receipt erased instead of exception: %+v %v", o, e)
	}
}

func TestPostgresChargebackWaitsForInFlightUsageAndDoesNotPrematurelyMarkApplied(t *testing.T) {
	f := setup(t)
	_, g, receipts := f.packageReceipts(t)
	at, e := instant(f.db)
	if e != nil {
		t.Fatal(e)
	}
	h, e := f.s.Ledger.ReserveCredits(f.ctx, nil, ledger.CreditReservation{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "existing-in-flight", Estimate: d("20"), At: at, ExpiresAt: at.Add(time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	cb := f.chargeback(t, receipts[0], "in-flight-dispute", "40")
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "review must wait for actual in-flight settlement"); !errors.Is(e, ErrReview) {
		t.Fatalf("unsettled usage source silently skipped: %v", e)
	}
	var stored Chargeback
	if e = f.db.First(&stored, "id=?", cb.ID).Error; e != nil || stored.RestrictionsAppliedAt != nil {
		t.Fatalf("failed restriction marked complete: %+v %v", stored, e)
	}
	if _, e = f.s.Ledger.SettleCredits(f.ctx, nil, f.operator, f.customer, h.ID, "verified-no-charge", ledger.Zero(), at); e != nil {
		t.Fatal(e)
	}
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "source complete after actual settlement"); e != nil {
		t.Fatal(e)
	}
	account, e := f.s.Ledger.AccountInTransaction(f.ctx, f.db, g.AccountID)
	if e != nil || account.Held.Cmp(d("50")) != 0 {
		t.Fatalf("retry source restriction=%+v %v", account, e)
	}
}

func TestPostgresChargebackStopsApprovedNewPayoutButPreservesAlreadyVerifiedPayment(t *testing.T) {
	f := setup(t)
	f.topup(t, "refund-source", "100")
	r := f.receiptNamed(t, "refund-source")
	c, e := f.s.RequestWalletRefund(f.ctx, WalletRefundInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "prior-approved", AmountUSD: d("20"), Reason: "return unused balance"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", "verified unused balance"); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil {
		t.Fatal(e)
	}
	cb := f.chargeback(t, r, "new-dispute-after-approval", "50")
	if _, e = f.s.BeginManualRefund(f.ctx, f.owner, f.operator, legs[0].ID, "do-not-pay-disputed-source", "bank execution requested"); !errors.Is(e, ErrReview) {
		t.Fatalf("new payment ignored source dispute: %v", e)
	}
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "refund hold must be reviewed first"); !errors.Is(e, ErrReview) {
		t.Fatalf("refund source hold silently omitted: %v", e)
	}
	if _, e = f.s.CustomerCancelRefund(f.ctx, f.customer, f.operator, c.ID); e != nil {
		t.Fatal(e)
	}
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "refund explicitly cancelled before restriction"); e != nil {
		t.Fatal(e)
	}
}

func TestPostgresChargebackEventOnlyRestrictionReplayIsIdempotent(t *testing.T) {
	f := setup(t)
	order, _, receipts := f.packageReceipts(t)
	events := f.consumeObserved(t, "100")
	cb := f.chargeback(t, receipts[0], "consumed-dispute", "40")
	for i := 0; i < 2; i++ {
		if e := f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "already consumed source event review"); e != nil {
			t.Fatal(e)
		}
	}
	var count int64
	if e := f.db.Model(&chargebackEvent{}).Where("chargeback_id=?", cb.ID).Count(&count).Error; e != nil || count != 1 {
		t.Fatalf("event-only restriction replay duplicated=%d %v", count, e)
	}
	rows, e := f.s.Incentives.RemediationEvents(f.ctx, nil, f.operator, order.ID)
	if e != nil || rows[0].Status != "disputed" || len(events) != 1 {
		t.Fatal(rows, e)
	}
	var outbox int64
	if e = f.db.Table("remediation_events").Where("subject_id=? AND action='chargeback.source_restrictions_applied'", cb.ID).Count(&outbox).Error; e != nil || outbox != 1 {
		t.Fatalf("restriction replay re-emitted side effect=%d %v", outbox, e)
	}
	// A different manual review continues to own its own dispute reference.
	if e = f.s.Incentives.HoldEventDispute(f.ctx, nil, f.owner, f.operator, events[0].ID, "manual:independent", "separate user appeal"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "merchant_collection_risk", "valid customer payment"); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Incentives.ReleaseEventDispute(f.ctx, nil, f.owner, f.operator, events[0].ID, "manual:independent", "independent appeal completed"); e != nil {
		t.Fatal(e)
	}
}
