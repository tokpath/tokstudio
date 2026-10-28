package payments

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"testing"
	"time"
)

type manualRefundTestApproval struct{}

func TestManualCashCheckoutAndVerificationNeedNoBankAccount(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "cash-test-key", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	ctx := context.Background()
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "Pay cash to the operator in person"}}
	if _, err := s.SaveMerchant(ctx, "finance", m, 0); err != nil {
		t.Fatal(err)
	}
	o := paymentTestOrder(m, "USD")
	checkout, err := s.Create(ctx, m.OperatorID, m.ID, o)
	if err != nil || checkout.PaymentAddress != "" || checkout.Instructions != m.Credentials["instructions"] {
		t.Fatal("cash checkout requires bank details", checkout, err)
	}
	proof := ManualVerification{Money: o.Money, PaidAt: time.Now().UTC()}
	first, err := s.RecordManual(ctx, "finance", m.OperatorID, m.ID, o.ID, proof)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.RecordManual(ctx, "finance", m.OperatorID, m.ID, o.ID, proof)
	if err != nil || first.ID != second.ID {
		t.Fatal("cash retry duplicated payment", first, second, err)
	}
}

func (manualRefundTestApproval) RequireApprovedRefund(context.Context, *gorm.DB, string, Refund) error {
	return nil
}
func (manualRefundTestApproval) RequireVerifiedManualRefund(context.Context, *gorm.DB, string, Refund) error {
	return nil
}

func TestActualRefundFactCannotSettleDifferentBusinessLeg(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "test-key", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	s.RefundApproval = manualRefundTestApproval{}
	s.RefundSink = testRefundSink{}
	ctx := context.Background()
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:unique-supplier", "instructions": "bank transfer"}}
	if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	o := paymentTestOrder(m, "USD")
	if _, e := s.Create(ctx, m.OperatorID, m.ID, o); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RecordManual(ctx, "finance", m.OperatorID, m.ID, o.ID, ManualVerification{ExternalReference: "original-bank-credit", Money: o.Money, PaidAt: time.Now(), EvidenceReference: "bank statement", PayerReference: "bank:original-payer"}); e != nil {
		t.Fatal(e)
	}
	links, e := s.ReceiptsForOrder(ctx, m.OperatorID, m.ID, o.ID)
	if e != nil || len(links) != 1 {
		t.Fatal(links, e)
	}
	v := ManualRefundVerification{ReceiptID: links[0].ReceiptID, ExternalReference: "one-actual-outbound-transfer", Money: Money{Currency: "USD", Amount: ledger.FromInt64(2)}, ExecutedAt: time.Now(), EvidenceReference: "verified outbound bank statement", RecipientReference: "bank:original-payer"}
	r := Refund{ID: uuid.NewString(), Order: o, Money: v.Money, USDAmount: ledger.FromInt64(2)}
	if _, e = s.RecordManualRefund(ctx, "finance", m.OperatorID, m.ID, r, v); e != nil {
		t.Fatal(e)
	}
	r.ID = uuid.NewString()
	if _, e = s.RecordManualRefund(ctx, "finance", m.OperatorID, m.ID, r, v); !errors.Is(e, ErrConflict) {
		t.Fatalf("same actual transfer settled another refund leg: %v", e)
	}
	var count int64
	if e = db.Table("payments_refund_fact_bindings").Count(&count).Error; e != nil || count != 1 {
		t.Fatal("binding was duplicated", count, e)
	}
}

func TestEVMPrivateKeysAreRejectedEvenForDisabledConfiguration(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "test-key", testPaymentAuth{}, NewTransport(true), nil)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "evm_wallet", Credentials: map[string]string{"private_key": "synthetic-never-store"}}
	if _, e := s.SaveMerchant(context.Background(), "finance", m, 0); !errors.Is(e, ErrForbidden) {
		t.Fatal("wallet private key accepted", e)
	}
	var n int64
	if e := db.Table("payments_merchants").Count(&n).Error; e != nil || n != 0 {
		t.Fatal("private key persisted", n, e)
	}
}

func TestManualBankTransferCannotBeCreditedThroughMerchantAliases(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "test-key", testPaymentAuth{}, NewTransport(true), &sqlReceiptSink{})
	ctx := context.Background()
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:canonical-recipient", "instructions": "bank transfer"}}
	if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	one := paymentTestOrder(m, "USD")
	if _, e := s.Create(ctx, m.OperatorID, m.ID, one); e != nil {
		t.Fatal(e)
	}
	proof := ManualVerification{ExternalReference: "unique-bank-transaction", Money: one.Money, PaidAt: time.Now().UTC(), EvidenceReference: "verified-bank-statement", PayerReference: "bank:payer"}
	if _, e := s.RecordManual(ctx, "finance", m.OperatorID, m.ID, one.ID, proof); e != nil {
		t.Fatal(e)
	}
	m.ID = uuid.NewString()
	if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	two := paymentTestOrder(m, "USD")
	if _, e := s.Create(ctx, m.OperatorID, m.ID, two); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RecordManual(ctx, "finance", m.OperatorID, m.ID, two.ID, proof); !errors.Is(e, ErrConflict) {
		t.Fatalf("same bank fact credited through another merchant: %v", e)
	}
	var facts int64
	if e := db.Table("payments_verified_facts").Count(&facts).Error; e != nil || facts != 1 {
		t.Fatalf("bank fact count=%d %v", facts, e)
	}
}

func TestExpiredOrderCannotStartNewExternalCollection(t *testing.T) {
	db := paymentDB(t)
	s := NewService(db, "test-key", testPaymentAuth{}, NewTransport(true), nil)
	ctx := context.Background()
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "bank:recipient", "instructions": "bank transfer"}}
	if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	now, e := databaseTime(ctx, db)
	if e != nil {
		t.Fatal(e)
	}
	o := paymentTestOrder(m, "USD")
	o.AcceptedAt = now.Add(-2 * time.Hour)
	o.ExpiresAt = now.Add(-time.Hour)
	o.Valuation.EffectiveAt = o.AcceptedAt.Add(-time.Minute)
	o.Valuation.ExpiresAt = now
	if _, e = s.Create(ctx, m.OperatorID, m.ID, o); !errors.Is(e, ErrInvalid) {
		t.Fatalf("expired order began collection: %v", e)
	}
	var intents int64
	if e = db.Table("payments_intents").Count(&intents).Error; e != nil || intents != 0 {
		t.Fatalf("expired order persisted new intent: %d %v", intents, e)
	}
}
