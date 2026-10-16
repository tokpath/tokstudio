package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
)

func TestRecordedFactReplayPreservesCallerEvidence(t *testing.T) {
	db := paymentDB(t)
	ctx := context.Background()
	s := NewService(db, "replay-evidence", testPaymentAuth{}, NewTransport(true), nil)
	m := Merchant{ID: uuid.NewString(), OperatorID: uuid.NewString(), Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "test-bank", "instructions": "bank"}}
	if _, e := s.SaveMerchant(ctx, "finance", m, 0); e != nil {
		t.Fatal(e)
	}
	o := paymentTestOrder(m, "USD")
	if _, e := s.Create(ctx, m.OperatorID, m.ID, o); e != nil {
		t.Fatal(e)
	}
	r := Refund{ID: uuid.NewString(), Order: o, Money: Money{Currency: "USD", Amount: ledger.FromInt64(2)}, USDAmount: ledger.FromInt64(2)}
	f, e := refundFact(m, r, "actual-provider-refund", r.Money, nil, map[string]any{"provider_refund": "actual-provider-refund", "amount": 2, "currency": "USD", "status": "succeeded"})
	if e != nil {
		t.Fatal(e)
	}
	// Leave room for JSONB's inserted spaces, so decoding a shallow copy would
	// deterministically reuse and corrupt the caller's evidence backing array.
	original := bytes.Clone(f.Evidence)
	evidence := make([]byte, len(original), len(original)+1024)
	copy(evidence, original)
	f.Evidence = evidence
	for i := 0; i < 3; i++ {
		if e = s.RecordFact(ctx, m, o, f); e != nil {
			t.Fatal(e)
		}
		if !bytes.Equal(f.Evidence, original) {
			t.Fatalf("stored replay mutated caller evidence on attempt %d", i)
		}
		if _, e = json.Marshal(RefundStatus{State: "succeeded", Fact: &f}); e != nil {
			t.Fatalf("replayed refund cannot be persisted: %v", e)
		}
	}
}
