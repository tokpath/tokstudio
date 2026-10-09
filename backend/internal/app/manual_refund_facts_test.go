package app_test

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestManualRefundRecordedAndLegacyReadFacts(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	marker := "manual-refund-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	reg, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "@example.test", Password: "isolated-test-password", PromotionCode: "THA1"})
	if err != nil {
		t.Fatal(err)
	}
	in := payment.OfflineReceiptInput{OperationID: marker, UserID: reg.User.ID, AmountMinor: 12345, CreditMinor: 2000000, Currency: "CNY", OccurredAt: time.Now().UTC().Add(-time.Hour), ExpectedIssueRatioBPS: billing.DefaultIssueRatioBPS}
	order, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, in, a.Audit, audit.RecordInput{ActorUserID: "receipt-operator"})
	if err != nil {
		t.Fatal(err)
	}
	when := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	refunded, err := a.Payment.RefundRecorded(ctx, order.ID, "refund-operator", when)
	if err != nil {
		t.Fatal(err)
	}
	if refunded.Status != payment.StatusRefunded || refunded.RefundStatus != payment.StatusRefunded || refunded.RefundAmountMinor == nil || *refunded.RefundAmountMinor != in.AmountMinor || refunded.Currency != in.Currency || refunded.CreditMinor != in.CreditMinor || refunded.RefundRecordedBy != "refund-operator" || refunded.RefundedAt == nil || !refunded.RefundedAt.Equal(when) {
		t.Fatalf("new refund did not retain original cash and actual registration: %+v", refunded)
	}
	var stored struct {
		RefundStatus      string
		RefundAmountMinor *int64
	}
	if err := a.DB.Table("payment_orders").Select("refund_status, refund_amount_minor").Where("id = ?", order.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.RefundStatus != payment.StatusRefunded || stored.RefundAmountMinor == nil || *stored.RefundAmountMinor != in.AmountMinor {
		t.Fatalf("new refund facts were not persisted: %+v", stored)
	}

	// Reproduce a pre-existing registered full manual refund, without migrating it.
	if err := a.DB.Table("payment_orders").Where("id = ?", order.ID).Updates(map[string]any{"refund_status": "", "refund_amount_minor": nil}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := func() string {
		var value string
		if err := a.DB.Raw("SELECT to_jsonb(payment_orders)::text FROM payment_orders WHERE id = ?", order.ID).Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := snapshot()
	facts, err := a.Payment.OrderFacts(ctx, order.ID, identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	legacy := facts.Order
	if legacy.RefundAmountMinor == nil || *legacy.RefundAmountMinor != in.AmountMinor || legacy.RefundStatus != payment.StatusRefunded || legacy.Currency != "CNY" || legacy.CreditMinor != in.CreditMinor || legacy.RefundRecordedBy != "refund-operator" || legacy.RefundedAt == nil || !legacy.RefundedAt.Equal(when) {
		t.Fatalf("legacy registration was not readable: %+v", legacy)
	}
	if after := snapshot(); before != after {
		t.Fatal("reading legacy facts rewrote the original transaction")
	}
}
