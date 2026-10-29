package payments

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestPostgresReceiptValuationGeneratesStableIdentityWithoutInventingEvidence(t *testing.T) {
	db := paymentDB(t)
	ctx := context.Background()
	s := NewService(db, "generated-valuation-reference", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "CNY", "recipient_reference": "verified-collector-bank", "instructions": "verified bank transfer"}}
	if _, err := s.SaveMerchant(ctx, "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	now, err := databaseTime(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	accepted, paid := now.Add(-time.Hour), now.Add(-time.Minute)
	o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "CNY", Amount: ledger.FromInt64(100)}, SettlementMoney: Money{Currency: "USD", Amount: ledger.FromInt64(10)}, AcceptedAt: accepted, ExpiresAt: now.Add(time.Hour)}
	if _, err = s.Create(ctx, m.OperatorID, m.ID, o); err != nil {
		t.Fatal(err)
	}
	fact, err := receiptFact(m, o, "real-bank-transaction", o.Money, &paid, map[string]string{"evidence": "real original receipt"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordFact(ctx, m, o, fact); !errors.Is(err, ErrAwaitingValuation) {
		t.Fatal("expired checkout estimate was reused", err)
	}
	in := ReceiptValuationConfirmation{BusinessKey: "same-valuation-confirmation", SettlementAmount: ledger.FromInt64(20), EvidenceReference: "actual valuation verification document"}
	bad := in
	bad.SettlementAmount = ledger.Zero()
	if _, err = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("server invented missing FX evidence", err)
	}
	bad = in
	bad.EvidenceReference = ""
	if _, err = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, bad); !errors.Is(err, ErrInvalid) {
		t.Fatal("server invented verification evidence", err)
	}
	var wg sync.WaitGroup
	results := make(chan ReceiptValuationResult, 4)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, in)
			results <- out
			failures <- err
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal("same empty-ID request conflicted on retry", err)
		}
	}
	generated := ""
	for out := range results {
		if out.Value == nil || out.Value.Amount.String() != "20" {
			t.Fatal(out)
		}
		id := out.Value.Valuation.ID
		if _, err = uuid.Parse(id); err != nil {
			t.Fatal("new valuation has no generated UUID", id)
		}
		if generated != "" && id != generated {
			t.Fatal("same fact acquired different identities", generated, id)
		}
		generated = id
	}
	changed := in
	changed.SettlementAmount = ledger.FromInt64(30)
	if _, err = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("generated reference concealed different estimate semantics", err)
	}
	var count int64
	if err = db.Table("payments_receipt_valuations").Where("fact_id=?", fact.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("valuation retry duplicated accounting fact", count, err)
	}
}
