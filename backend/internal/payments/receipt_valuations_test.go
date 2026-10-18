package payments

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func TestReceiptValuationUsesActualTimeAndPreservesPendingOriginalMoney(t *testing.T) {
	for _, mode := range []string{"expired_at_actual_payment", "actual_payment_time_unknown"} {
		t.Run(mode, func(t *testing.T) {
			db := paymentDB(t)
			ctx := context.Background()
			sink := &sqlReceiptSink{}
			s := NewService(db, "execution-time-valuation", testPaymentAuth{}, NewTransport(true), sink)
			m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "CNY", "recipient_reference": "verified-bank-account", "instructions": "actual bank transfer"}}
			if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
				t.Fatal(e)
			}
			now, e := databaseTime(ctx, db)
			if e != nil {
				t.Fatal(e)
			}
			accepted := now.Add(-30 * time.Minute)
			paid := now.Add(-time.Minute)
			oldRate := Valuation{ID: "historical-quote", Currency: "CNY", USDPerUnit: ledger.MustDecimal("0.1"), Source: "historical checkout indicative price", EffectiveAt: accepted.Add(-time.Minute), ExpiresAt: accepted.Add(10 * time.Minute)}
			o := Order{ID: uuid.NewString(), OperatorID: m.OperatorID, MerchantID: m.ID, Money: Money{Currency: "CNY", Amount: ledger.FromInt64(100)}, Valuation: oldRate, AcceptedAt: accepted, ExpiresAt: now.Add(time.Hour)}
			if _, e = s.Create(ctx, m.OperatorID, m.ID, o); e != nil {
				t.Fatal(e)
			}
			var occurred *time.Time
			if mode != "actual_payment_time_unknown" {
				occurred = &paid
			}
			fact, e := receiptFact(m, o, "real-bank-transfer-"+uuid.NewString(), o.Money, occurred, map[string]string{"provider": "independent verified bank transfer"})
			if e != nil {
				t.Fatal(e)
			}
			for i := 0; i < 2; i++ {
				if e = s.RecordFact(ctx, m, o, fact); !errors.Is(e, ErrAwaitingValuation) {
					t.Fatalf("stale or unknown-time conversion accepted: %v", e)
				}
			}
			var row receiptFactRow
			if e = db.Table("payments_verified_facts").Where("id=?", fact.ID).Take(&row).Error; e != nil {
				t.Fatal(e)
			}
			if row.State != "awaiting_valuation" || string(row.Valuation) != "null" || row.ReceiptID != "" || row.ValuationVersion != 0 {
				t.Fatalf("pending fact faked USD or disappeared: %+v", row)
			}
			var stored Fact
			if json.Unmarshal(row.Fact, &stored) != nil || stored.Money.Amount.String() != "100" || stored.Money.Currency != "CNY" {
				t.Fatalf("actual original currency lost: %+v", stored)
			}
			if e = db.Transaction(func(tx *gorm.DB) error {
				pending, e := s.PendingReceiptObligations(ctx, tx, m.OperatorID, nil)
				if e != nil {
					return e
				}
				if len(pending) != 1 || pending[0].FactID != fact.ID || pending[0].Money.Amount.String() != "100" {
					t.Fatalf("lifecycle pending obligation missing: %+v", pending)
				}
				empty, e := s.PendingReceiptObligations(ctx, tx, m.OperatorID, []string{})
				if e != nil {
					return e
				}
				if len(empty) != 0 {
					t.Fatal("empty explicit order scope queried every order")
				}
				wrong, e := s.PendingReceiptObligations(ctx, tx, m.OperatorID, []string{uuid.NewString()})
				if e != nil {
					return e
				}
				if len(wrong) != 0 {
					t.Fatal("pending fact crossed order scope")
				}
				links, e := s.RecordedReceiptLinks(ctx, tx, m.OperatorID, []string{o.ID})
				if e != nil {
					return e
				}
				if len(links) != 0 {
					t.Fatal("unvalued original money appeared recorded")
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			var count int64
			if e = db.Table("payment_test_receipts").Count(&count).Error; e != nil || count != 0 {
				t.Fatalf("pending money delivered: %d %v", count, e)
			}
			list, _, e := s.PendingReceiptValuations(ctx, "finance", m.OperatorID, "", 10)
			if e != nil || len(list) != 1 || list[0].FactID != fact.ID {
				t.Fatal(list, e)
			}

			filtered, _, e := s.PendingReceiptValuations(ctx, "finance", m.OperatorID, "", 10, "CNY", "awaiting_valuation")
			if e != nil || len(filtered) != 1 || filtered[0].FactID != fact.ID {
				t.Fatal("pending original currency search failed", filtered, e)
			}
			filtered, _, e = s.PendingReceiptValuations(ctx, "finance", m.OperatorID, "", 10, "CNY", "completed")
			if e != nil || len(filtered) != 0 {
				t.Fatal("pending valuation status filter failed", filtered, e)
			}
			if _, _, e = s.PendingReceiptValuations(ctx, "customer", m.OperatorID, "", 10); !errors.Is(e, ErrForbidden) {
				t.Fatalf("customer enumerated private pending facts: %v", e)
			}
			other, _, e := s.PendingReceiptValuations(ctx, "finance", uuid.NewString(), "", 10)
			if e != nil || len(other) != 0 {
				t.Fatal("other operator leaked pending fact", other, e)
			}
			rate := Valuation{ID: "actual-execution-price", Currency: "CNY", USDPerUnit: ledger.MustDecimal("0.2"), Source: "independent contemporaneous actual payment valuation", EffectiveAt: paid.Add(-time.Second), ExpiresAt: paid.Add(time.Second)}
			in := ReceiptValuationConfirmation{BusinessKey: "finance-current-evidence", Valuation: rate, EvidenceReference: "verified execution-time valuation document"}
			if mode == "actual_payment_time_unknown" {
				if _, e = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, in); !errors.Is(e, ErrAwaitingValuation) {
					t.Fatalf("observation time used as actual FX time: %v", e)
				}
				in.PaidAt = &paid
				if _, e = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, in); !errors.Is(e, ErrInvalid) {
					t.Fatalf("time supplied without verification proof: %v", e)
				}
				in.PaymentTimeEvidence = "independent actual payment timestamp proof"
			}
			if _, e = s.ConfirmReceiptValuation(ctx, "customer", m.OperatorID, fact.ID, in); !errors.Is(e, ErrForbidden) {
				t.Fatalf("customer confirmed valuation: %v", e)
			}
			bad := in
			bad.Valuation = oldRate
			if _, e = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, bad); !errors.Is(e, ErrInvalid) {
				t.Fatalf("checkout rate accepted for late actual payment: %v", e)
			}
			sink.fail.Store(true)
			confirmed, e := s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, in)
			if !errors.Is(e, ErrProvider) || confirmed.Version != 1 || confirmed.Value == nil || confirmed.Value.Amount.String() != "20" || confirmed.ReceiptID != "" || confirmed.State != "verified_pending_receipt" {
				t.Fatalf("known FX value lost on receipt matching failure: %+v %v", confirmed, e)
			}
			if e = s.RecordFact(ctx, m, o, fact); !errors.Is(e, ErrProvider) {
				t.Fatalf("stored USD fact was revalued on repeated callback: %v", e)
			}
			sink.fail.Store(false)
			recovered, e := s.RunPendingFacts(ctx, 10)
			if e != nil || len(recovered) != 1 || recovered[0].State != "processed" {
				t.Fatalf("durable original fact was not recovered: %+v %v", recovered, e)
			}
			if e = db.Transaction(func(tx *gorm.DB) error {
				pending, e := s.PendingReceiptObligations(ctx, tx, m.OperatorID, []string{o.ID})
				if e != nil {
					return e
				}
				links, e := s.RecordedReceiptLinks(ctx, tx, m.OperatorID, []string{o.ID})
				if e != nil {
					return e
				}
				if len(pending) != 0 || len(links) != 1 {
					t.Fatalf("recovered receipt projection: %+v %+v", pending, links)
				}
				return nil
			}); e != nil {
				t.Fatal(e)
			}
			var wg sync.WaitGroup
			errs := make(chan error, 12)
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					out, e := s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, in)
					if e == nil && (out.Version != 1 || out.ReceiptID == "" || out.Value == nil || out.Value.Amount.String() != "20") {
						errs <- ErrInvalid
						return
					}
					errs <- e
				}()
			}
			wg.Wait()
			close(errs)
			for e := range errs {
				if e != nil {
					t.Fatal(e)
				}
			}
			if e = s.RecordFact(ctx, m, o, fact); e != nil {
				t.Fatalf("late repeated callback discarded confirmed valuation: %v", e)
			}
			versions := int64(0)
			if e = db.Table("payments_receipt_valuations").Where("fact_id=?", fact.ID).Count(&versions).Error; e != nil || versions != 1 {
				t.Fatalf("valuation duplicated: %d %v", versions, e)
			}
			if e = db.Table("payment_test_receipts").Count(&count).Error; e != nil || count != 1 {
				t.Fatalf("receipt duplicated: %d %v", count, e)
			}
			var amount string
			if e = db.Table("payment_test_receipts").Select("amount::text").Take(&amount).Error; e != nil || ledger.MustDecimal(amount).String() != "20" {
				t.Fatalf("USD actual amount=%s %v", amount, e)
			}
			in.BusinessKey = "replace-already-posted-rate"
			in.Valuation.USDPerUnit = ledger.MustDecimal("0.3")
			if _, e = s.ConfirmReceiptValuation(ctx, "finance", m.OperatorID, fact.ID, in); !errors.Is(e, ErrConflict) {
				t.Fatalf("posted payment silently revalued: %v", e)
			}
		})
	}
}

func TestNativeUSDReceiptNeedsNoFXOrInventedPaymentTime(t *testing.T) {
	db := paymentDB(t)
	ctx := context.Background()
	s := NewService(db, "native-USD", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "own-account", "instructions": "bank"}}
	if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	o := paymentTestOrder(m, "USD")
	if _, e := s.Create(ctx, m.OperatorID, m.ID, o); e != nil {
		t.Fatal(e)
	}
	f, e := receiptFact(m, o, "verified-native-usd", o.Money, nil, map[string]string{"payment": "verified successful"})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.RecordFact(ctx, m, o, f); e != nil {
		t.Fatal(e)
	}
	var row receiptFactRow
	if e = db.Table("payments_verified_facts").Where("id=?", f.ID).Take(&row).Error; e != nil {
		t.Fatal(e)
	}
	var value USDValue
	if json.Unmarshal(row.Valuation, &value) != nil || value.Amount.Cmp(o.Money.Amount) != 0 || value.Valuation.Source != "native_usd_identity" || row.ReceiptID == "" {
		t.Fatal(row, value)
	}
	var stored Fact
	if json.Unmarshal(row.Fact, &stored) != nil || stored.OccurredAt != nil {
		t.Fatal("native USD invented an actual payment time")
	}
}
