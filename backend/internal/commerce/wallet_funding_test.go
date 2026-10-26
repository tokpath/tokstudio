package commerce

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func (f fixture) separatelyFundedTopups(t *testing.T) ([]Order, []Receipt) {
	t.Helper()
	var orders []Order
	var receipts []Receipt
	for _, amount := range []string{"30", "70"} {
		key := uuid.NewString()
		o, err := f.s.CreateTopUpOrder(f.ctx, nil, CreateTopUpInput{CustomerID: f.customer, OperatorID: f.operator, BusinessKey: key, ExpectedOwnershipVersion: 1, AmountUSD: d(amount)})
		if err != nil {
			t.Fatal(err)
		}
		r := f.receipt(t, key, amount, "0", o.CreatedAt)
		f.allocate(t, key, r, o, amount)
		receipts = append(receipts, r)
		orders = append(orders, o)
		if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); err != nil {
			t.Fatal(err)
		}
	}
	return orders, receipts
}

func TestPostgresWalletFundingPreservesReceiptsAcrossCloseRestoreAndPartialRefund(t *testing.T) {
	f := setup(t)
	topups, receipts := f.separatelyFundedTopups(t)
	o := f.order(t, "source-purchase", "60", "0")
	sources, err := f.s.OrderWalletFunding(f.ctx, nil, f.operator, f.customer, o.ID)
	if err != nil || len(sources) != 2 {
		t.Fatalf("reserved sources=%+v %v", sources, err)
	}
	if sources[0].ReceiptID != receipts[0].ID || sources[0].AmountUSD.Cmp(d("30")) != 0 || sources[1].AmountUSD.Cmp(d("30")) != 0 {
		t.Fatalf("FIFO source split=%+v", sources)
	}
	if _, err = f.s.CloseOrder(f.ctx, nil, f.customer, o.ID, PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "provider verified unpaid"}); err != nil {
		t.Fatal(err)
	}
	// A timely payment reported after closure re-reserves provenance and cash.
	receipt := f.receipt(t, "late-notification", "20", "0", o.CreatedAt)
	f.allocate(t, "late-notification", receipt, o, "20")
	if _, err = f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); err != nil {
		t.Fatal(err)
	}
	sources, err = f.s.OrderWalletFunding(f.ctx, nil, f.operator, f.customer, o.ID)
	if err != nil || len(sources) != 2 || sources[0].Status != "consumed" {
		t.Fatalf("consumed sources=%+v %v", sources, err)
	}
	wallet := f.balance(t, "customer:usd")
	if wallet.Balance.Cmp(d("40")) != 0 {
		t.Fatal(wallet)
	}
	restoration := uuid.NewString()
	credit := func(tx *gorm.DB) error {
		r, e := f.s.RestoreWalletFunding(f.ctx, tx, f.operator, f.customer, o.ID, restoration, d("60"))
		if e != nil {
			return e
		}
		if len(r) != 2 {
			return errors.New("restoration must retain both receipts")
		}
		sales, e := f.s.account(f.ctx, tx, f.operator, f.operator, "control:sales")
		if e != nil {
			return e
		}
		_, e = f.ledger.Post(f.ctx, tx, ledger.Posting{OperatorID: f.operator, BusinessKey: "refund:" + restoration, Kind: "approved_order_correction", SourceID: restoration, ActorID: "test:finance", Reason: "restore original internal USD", Lines: []ledger.Line{{AccountID: wallet.ID, Amount: d("60")}, {AccountID: sales.ID, Amount: d("-60")}}})
		return e
	}
	if err = f.db.Transaction(credit); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Transaction(credit); err != nil {
		t.Fatalf("restoration replay: %v", err)
	}
	err = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.RestoreWalletFunding(f.ctx, tx, f.operator, f.customer, o.ID, uuid.NewString(), d("1"))
		return e
	})
	if !errors.Is(err, ErrExcessAllocation) {
		t.Fatalf("restored more than actual USD: %v", err)
	}
	sources, err = f.s.OrderWalletFunding(f.ctx, nil, f.operator, f.customer, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored := ledger.Zero()
	for _, a := range sources {
		restored = restored.Add(a.RestoredUSD)
		want := topups[0].ID
		if a.ReceiptID == receipts[1].ID {
			want = topups[1].ID
		}
		if a.TopUpOrderID != want || a.SourceOperatorID != f.operator {
			t.Fatal(a)
		}
	}
	if restored.Cmp(d("60")) != 0 {
		t.Fatal(restored)
	}
	refundID := uuid.NewString()
	var refund []WalletFundingAllocation
	holds := map[string]ledger.Hold{}
	err = f.db.Transaction(func(tx *gorm.DB) error {
		var e error
		refund, e = f.s.ReserveWalletRefund(f.ctx, tx, f.operator, f.customer, refundID, d("70"))
		if e != nil {
			return e
		}
		for _, a := range refund {
			h, e := f.ledger.Reserve(f.ctx, tx, ledger.Reservation{OperatorID: f.operator, BusinessKey: "refund-hold:" + a.ID, Purpose: "wallet_refund", ExpiresAt: time.Now().Add(time.Hour), Amounts: []ledger.Line{{AccountID: wallet.ID, Amount: a.AmountUSD}}})
			if e != nil {
				return e
			}
			holds[a.ID] = h
		}
		return nil
	})
	if err != nil || len(refund) != 2 {
		t.Fatalf("refund sources=%+v %v", refund, err)
	}
	if a := f.balance(t, "customer:usd"); a.Held.Cmp(d("70")) != 0 {
		t.Fatal(a)
	}
	paid, unpaid := refund[0], refund[1]
	err = f.db.Transaction(func(tx *gorm.DB) error {
		if e := f.s.CompleteWalletRefundAllocation(f.ctx, tx, f.operator, f.customer, refundID, paid.ID); e != nil {
			return e
		}
		control, e := f.s.account(f.ctx, tx, f.operator, f.operator, "control:refunds")
		if e != nil {
			return e
		}
		_, e = f.ledger.Capture(f.ctx, tx, holds[paid.ID].ID, ledger.Posting{OperatorID: f.operator, BusinessKey: "actual-refund:" + paid.ID, Kind: "verified_wallet_refund", SourceID: refundID, ActorID: "test:verified-payment", Reason: "actual receipt refund leg", Lines: []ledger.Line{{AccountID: wallet.ID, Amount: paid.AmountUSD.Neg()}, {AccountID: control.ID, Amount: paid.AmountUSD}}}, nil)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	err = f.db.Transaction(func(tx *gorm.DB) error { return f.s.ReleaseWalletRefund(f.ctx, tx, f.operator, f.customer, refundID) })
	if !errors.Is(err, ErrOrderState) {
		t.Fatalf("whole cancellation released paid leg: %v", err)
	}
	err = f.db.Transaction(func(tx *gorm.DB) error {
		if e := f.s.ReleaseWalletRefundAllocation(f.ctx, tx, f.operator, f.customer, refundID, unpaid.ID); e != nil {
			return e
		}
		_, e := f.ledger.Release(f.ctx, tx, f.operator, holds[unpaid.ID].ID, "verified-unpaid:"+unpaid.ID)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if a := f.balance(t, "customer:usd"); a.Balance.Cmp(d("70")) != 0 || !a.Held.IsZero() {
		t.Fatalf("partial real refund cash=%+v", a)
	}
	err = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ReserveWalletRefund(f.ctx, tx, f.operator, f.customer, uuid.NewString(), d("71"))
		return e
	})
	if !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatalf("refunded receipt became spendable again: %v", err)
	}
	injected := errors.New("caller failed after provenance reservation")
	err = f.db.Transaction(func(tx *gorm.DB) error {
		if _, e := f.s.ReserveWalletRefund(f.ctx, tx, f.operator, f.customer, uuid.NewString(), d("70")); e != nil {
			return e
		}
		return injected
	})
	if !errors.Is(err, injected) {
		t.Fatal(err)
	}
	if _, err = f.s.ReserveWalletRefund(f.ctx, nil, f.operator, f.customer, uuid.NewString(), d("1")); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("refund provenance must share outer cash transaction")
	}
}

func TestPostgresWalletFundingConcurrentOrderAndRefundCannotUseSameSource(t *testing.T) {
	f := setup(t)
	f.separatelyFundedTopups(t)
	wallet := f.balance(t, "customer:usd")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, e := f.s.CreateOrder(f.ctx, nil, CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: "race-purchase", ExpectedOwnershipVersion: 1, InternalUSD: d("80")})
		results <- e
	}()
	go func() {
		defer wg.Done()
		<-start
		refundID := uuid.NewString()
		results <- f.db.Transaction(func(tx *gorm.DB) error {
			if _, e := f.s.ReserveWalletRefund(f.ctx, tx, f.operator, f.customer, refundID, d("80")); e != nil {
				return e
			}
			_, e := f.ledger.Reserve(f.ctx, tx, ledger.Reservation{OperatorID: f.operator, BusinessKey: "race-refund:" + refundID, Purpose: "wallet_refund", ExpiresAt: time.Now().Add(time.Hour), Amounts: []ledger.Line{{AccountID: wallet.ID, Amount: d("80")}}})
			return e
		})
	}()
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, ledger.ErrInsufficient) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatalf("source used %d times", success)
	}
	var reserved ledger.Decimal
	if err := f.db.Raw(`SELECT COALESCE(sum(reserved_usd),0)::text FROM commerce_wallet_funding_lots`).Row().Scan(&reserved); err != nil {
		t.Fatal(err)
	}
	if reserved.Cmp(d("80")) != 0 || f.balance(t, "customer:usd").Held.Cmp(d("80")) != 0 {
		t.Fatalf("provenance/cash conservation=%s", reserved)
	}
}

func TestPostgresWalletFundingRejectsCashWithoutVerifiedOrigin(t *testing.T) {
	f := setup(t)
	wallet := f.balance(t, "customer:usd")
	control, err := f.s.account(f.ctx, nil, f.operator, f.operator, "control:test-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.ledger.Post(f.ctx, nil, ledger.Posting{OperatorID: f.operator, BusinessKey: "unproven-cash", Kind: "test_fixture", SourceID: uuid.NewString(), ActorID: "test:fixture", Reason: "deliberately unproven origin", Lines: []ledger.Line{{AccountID: wallet.ID, Amount: d("80")}, {AccountID: control.ID, Amount: d("-80")}}}); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateOrder(f.ctx, nil, CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: "no-provenance", ExpectedOwnershipVersion: 1, InternalUSD: d("80")})
	if !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatalf("invented wallet source: %v", err)
	}
}

func TestPostgresWalletReceiptFreezeNeverFallsBackToUnrelatedFunding(t *testing.T) {
	f := setup(t)
	_, receipts := f.separatelyFundedTopups(t)
	reservationID := uuid.NewString()
	err := f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ReserveWalletReceipt(f.ctx, tx, f.operator, f.customer, reservationID, receipts[0].ID, d("31"))
		return e
	})
	if !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatalf("specific receipt fell back to other money: %v", err)
	}
	var frozen []WalletFundingAllocation
	err = f.db.Transaction(func(tx *gorm.DB) error {
		var e error
		frozen, e = f.s.ReserveWalletReceipt(f.ctx, tx, f.operator, f.customer, reservationID, receipts[0].ID, d("30"))
		return e
	})
	if err != nil || len(frozen) != 1 || frozen[0].ReceiptID != receipts[0].ID {
		t.Fatalf("receipt-specific freeze=%+v %v", frozen, err)
	}
	err = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ReserveWalletReceipt(f.ctx, tx, f.operator, f.customer, reservationID, receipts[1].ID, d("30"))
		return e
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("freeze idempotency switched receipt: %v", err)
	}
	_, err = f.s.CreateOrder(f.ctx, nil, CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: "cannot-spend-frozen-origin", ExpectedOwnershipVersion: 1, InternalUSD: d("80")})
	if !errors.Is(err, ledger.ErrInsufficient) {
		t.Fatalf("spent disputed receipt despite provenance freeze: %v", err)
	}
	if err = f.db.Transaction(func(tx *gorm.DB) error {
		return f.s.ReleaseWalletRefund(f.ctx, tx, f.operator, f.customer, reservationID)
	}); err != nil {
		t.Fatal(err)
	}
	f.order(t, "funding-released-after-review", "80", "0")
}
