package remediation

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func TestPostgresClosedOrderPendingOriginalMoneyBlocksTakeoverAndFormalExitUntilRepaid(t *testing.T) {
	brand, channel, _, _ := takeoverFixture(t)
	order, e := channel.s.Commerce.CreateTopUpOrder(channel.ctx, nil, commerce.CreateTopUpInput{OperatorID: channel.operator, CustomerID: channel.customer, BusinessKey: "late-foreign-payment", AmountUSD: d("20"), ExpectedOwnershipVersion: 1, Validity: 2 * time.Second})
	if e != nil {
		t.Fatal(e)
	}
	pay := brand.s.Payments
	m := payments.Merchant{ID: uuid.NewString(), OperatorID: channel.operator, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "CNY", "recipient_reference": "bank:channel-original-cny-account", "instructions": "independent bank verification"}}
	if _, e = pay.SaveMerchant(channel.ctx, channel.owner, m, 0); e != nil {
		t.Fatal(e)
	}
	at, e := instant(brand.db)
	if e != nil {
		t.Fatal(e)
	}
	oldRate := payments.Valuation{ID: "indicative-checkout-quote", Currency: "CNY", USDPerUnit: d("0.2"), Source: "checkout indicative source", EffectiveAt: at.Add(-time.Minute), ExpiresAt: at.Add(200 * time.Millisecond)}
	po := payments.Order{ID: order.ID, OperatorID: channel.operator, MerchantID: m.ID, Money: payments.Money{Currency: "CNY", Amount: d("100")}, Valuation: oldRate, AcceptedAt: at, ExpiresAt: order.ExpiresAt}
	if _, e = pay.Create(channel.ctx, channel.operator, m.ID, po); e != nil {
		t.Fatal(e)
	}
	// No payment is known yet. Closing an unpaid order cannot erase a later
	// verified bank fact, even after the original checkout has expired.
	if _, e = channel.s.Commerce.CloseOrder(channel.ctx, nil, channel.customer, order.ID, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "bank had no payment at closure"}); e != nil {
		t.Fatal(e)
	}
	p := beginTakeover(t, brand, channel)
	p, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Until(order.ExpiresAt.Add(50 * time.Millisecond)))
	paidAt, e := instant(brand.db)
	if e != nil {
		t.Fatal(e)
	}
	verification := payments.ManualVerification{ExternalReference: "actual-late-cny-bank-transfer", Money: po.Money, PaidAt: paidAt, EvidenceReference: "bank confirms actual CNY100 after expiry", PayerReference: "bank:independently-verified-original-payer"}
	// Actual receipt persistence waits for the SAME outer operator fence used
	// by takeover/exit; it cannot commit unnoticed halfway through their check.
	tx := brand.db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if e = tx.Exec("SELECT id FROM organization_operators WHERE id=? FOR UPDATE", channel.operator).Error; e != nil {
		t.Fatal(e)
	}
	type recorded struct {
		fact payments.Fact
		err  error
	}
	done := make(chan recorded, 1)
	go func() {
		f, e := pay.RecordManual(channel.ctx, channel.owner, channel.operator, m.ID, order.ID, verification)
		done <- recorded{f, e}
	}()
	select {
	case result := <-done:
		t.Fatalf("actual payment escaped the operator responsibility fence: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	if e = tx.Rollback().Error; e != nil {
		t.Fatal(e)
	}
	var result recorded
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("actual payment did not resume after the operator fence")
	}
	if !errors.Is(result.err, payments.ErrAwaitingValuation) || result.fact.ID == "" {
		t.Fatal(result)
	}
	if _, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID); !errors.Is(e, ErrTakeoverBlocked) {
		t.Fatal("pending original currency was treated as settled at refresh", e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); !errors.Is(e, ErrTakeoverBlocked) {
		t.Fatal("stale prepared snapshot ignored a new real payment", e)
	}
	if e = brand.s.Organization.SetSupplyStatus(brand.ctx, nil, brand.owner, channel.operator, "exited", "attempt formal exit with actual pending receipt"); !errors.Is(e, organization.ErrReadiness) {
		t.Fatal("formal exit ignored actual original money", e)
	}
	e = brand.db.Transaction(func(tx *gorm.DB) error {
		pending, e := pay.PendingReceiptObligations(brand.ctx, tx, channel.operator, []string{order.ID})
		if e != nil {
			return e
		}
		if len(pending) != 1 || pending[0].Money.Currency != "CNY" || pending[0].Money.Amount.Cmp(d("100")) != 0 || pending[0].State != "awaiting_valuation" {
			t.Fatal("durable original money disappeared with the closed order", pending)
		}
		other, e := brand.s.CheckOperatorExit(brand.ctx, tx, brand.operator)
		if e != nil {
			return e
		}
		for _, check := range other {
			if !check.Ready {
				t.Fatal("channel raw money blocked unrelated brand receipts", other)
			}
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	actualRate := payments.Valuation{ID: "verified-at-actual-payment", Currency: "CNY", USDPerUnit: d("0.2"), Source: "independent contemporaneous valuation evidence", EffectiveAt: paidAt.Add(-time.Second), ExpiresAt: paidAt.Add(time.Second)}
	confirmed, e := pay.ConfirmReceiptValuation(channel.ctx, channel.owner, channel.operator, result.fact.ID, payments.ReceiptValuationConfirmation{BusinessKey: "verified-late-cny-rate", Valuation: actualRate, EvidenceReference: "finance independently verified actual payment-time rate"})
	if e != nil || confirmed.ReceiptID == "" || confirmed.Value == nil || confirmed.Value.Amount.Cmp(d("20")) != 0 {
		t.Fatal(confirmed, e)
	}
	if _, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID); !errors.Is(e, ErrTakeoverBlocked) {
		t.Fatal("valued but unallocated customer money was treated as settled", e)
	}
	if e = brand.s.Organization.SetSupplyStatus(brand.ctx, nil, brand.owner, channel.operator, "exited", "attempt exit before refund actual principal"); !errors.Is(e, organization.ErrReadiness) {
		t.Fatal("valuation alone released the formal exit gate", e)
	}
	c, e := brand.s.RequestUnallocatedRefund(channel.ctx, channel.owner, channel.operator, confirmed.ReceiptID, "return-late-cny", "late payment after order expiry", "original payer account independently verified", d("20"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = brand.s.ReviewRefund(channel.ctx, channel.owner, channel.operator, c.ID, "approve", "refund original verified actual late payment"); e != nil {
		t.Fatal(e)
	}
	legs, e := brand.s.Legs(channel.ctx, nil, channel.operator, c.ID)
	if e != nil || len(legs) != 1 {
		t.Fatal(legs, e)
	}
	if _, e = brand.s.BeginManualRefund(channel.ctx, channel.owner, channel.operator, legs[0].ID, "return-cny-instruction", "bank instruction to independently verified original payer"); e != nil {
		t.Fatal(e)
	}
	at, _ = instant(brand.db)
	if _, e = brand.s.RecordManualRefund(channel.ctx, channel.owner, channel.operator, legs[0].ID, ExecutionProof{BusinessKey: "actual-cny-return", Reference: "bank-actual-cny100-return", Evidence: "actual CNY100 return valued USD20 with contemporaneous execution evidence", Currency: "CNY", Amount: "100", ActualUSD: d("20"), FeeStatus: "pending", OccurredAt: at}); e != nil {
		t.Fatal(e)
	}
	// AllocatedUSD remains zero on this repaid original receipt. The real
	// remaining receipt account, rather than Gross-Allocated, clears readiness.
	e = brand.db.Transaction(func(tx *gorm.DB) error {
		funds, e := brand.s.Commerce.UnsettledReceiptFunds(channel.ctx, tx, channel.operator, []string{confirmed.ReceiptID})
		if e != nil {
			return e
		}
		if len(funds) != 0 {
			t.Fatal("fully repaid source retained a fictional obligation", funds)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	p, e = brand.s.RefreshTakeover(brand.ctx, brand.owner, brand.operator, p.ID)
	if e != nil {
		t.Fatal("completed actual refund did not unblock responsibility closing", e)
	}
	if _, e = brand.s.ExecuteTakeover(brand.ctx, brand.owner, brand.operator, p.ID, p.SnapshotFingerprint); e != nil {
		t.Fatal(e)
	}
}
