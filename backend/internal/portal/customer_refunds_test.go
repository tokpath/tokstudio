package portal

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func TestPostgresCustomerRefundListShowsOneApplication(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "refund-list@example.test")
	f.login(t, "customer", "refund-list@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	op := f.a.Installation.OperatorID
	ctx := context.Background()
	amount := ledger.MustDecimal("20")
	now := time.Now().Add(-time.Minute)
	receipts := []string{}
	for _, reference := range []string{"refund-list-a", "refund-list-b"} {
		receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, f.a.Installation.OwnerStaffID, commerce.VerifiedReceipt{OperatorID: op, ProviderReceiptID: reference, GrossMoney: amount, FeeStatus: "pending", Currency: "USD", OriginalAmount: amount.String(), RecipientReference: "synthetic recipient", PayerReference: "synthetic payer", ValuationEvidence: "synthetic USD", VerificationEvidence: "synthetic isolated fixture", PaidAt: &now, ObservedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, receipt.ID)
	}
	refundID := uuid.NewString()
	refund := remediation.Case{ID: refundID, OperatorID: op, CustomerID: &customer, BusinessKey: "customer-refund-list", Fingerprint: "synthetic", Kind: "wallet", AmountMoney: amount.Add(amount), State: "pending_review", Reason: "线下退款方式：支付宝\n收款账号：alice@example.test\n退款原因：unused", Evidence: "customer requested unused USD return", RequestedBy: customer, CreatedAt: now}
	if err := f.a.DB.Create(&refund).Error; err != nil {
		t.Fatal(err)
	}
	for _, receiptID := range receipts {
		leg := remediation.RefundLeg{ID: uuid.NewString(), CaseID: refundID, OperatorID: op, SourceOperatorID: op, ReceiptID: receiptID, AmountMoney: amount, State: "reserved", CreatedAt: now}
		if err := f.a.DB.Create(&leg).Error; err != nil {
			t.Fatal(err)
		}
	}
	status, page, _ := f.call(t, "GET", "/v1/me/refunds/workspace", nil, "")
	mustOK(t, status, page)
	items := page["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["id"] != refundID {
		t.Fatal("one refund application was listed once for each source receipt", items)
	}
	actions := items[0].(map[string]any)["allowed_actions"].([]any)
	if len(actions) != 3 {
		t.Fatal("cancel and both source actions belong on the same application", actions)
	}
}
