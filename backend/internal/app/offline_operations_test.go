package app_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestOverhaulOfflineOperationsAndOrderFacts(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	server := httptest.NewServer(a.Router())
	defer server.Close()
	marker := "offline-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	reg, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "@example.test", Password: "isolated-test-password", PromotionCode: "THA1"})
	if err != nil {
		t.Fatal(err)
	}
	input := payment.OfflineReceiptInput{OperationID: marker, UserID: reg.User.ID, AmountMinor: billing.MinorPerUSD, CreditMinor: billing.MinorPerUSD, Currency: "USD", OccurredAt: time.Now().UTC(), ExpectedIssueRatioBPS: billing.DefaultIssueRatioBPS}
	before, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	orders := make(chan *payment.OrderView, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, input, a.Audit, audit.RecordInput{ActorUserID: "finance"})
			if err != nil {
				t.Error(err)
				return
			}
			orders <- order
		}()
	}
	wg.Wait()
	close(orders)
	originalID := ""
	for order := range orders {
		if originalID == "" {
			originalID = order.ID
		}
		if order.ID != originalID || order.ReceiptReference != "" || order.ReceivedAt == nil {
			t.Fatalf("wrong original facts: %+v", order)
		}
	}
	if originalID == "" {
		t.Fatal("no order")
	}
	after, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil || after.AvailableMinor != before.AvailableMinor+input.CreditMinor {
		t.Fatalf("duplicate wallet credit: %+v %v", after, err)
	}
	altered := input
	altered.Note = "different payload"
	if _, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, altered, a.Audit, audit.RecordInput{ActorUserID: "finance"}); !errors.Is(err, payment.ErrReceiptConflict) {
		t.Fatalf("changed payload accepted: %v", err)
	}
	if _, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, input, a.Audit, audit.RecordInput{ActorUserID: "someone-else"}); !errors.Is(err, payment.ErrReceiptConflict) {
		t.Fatalf("actor conflict: %v", err)
	}
	if _, err := a.Payment.OfflineOperation(ctx, input.OperationID, "someone-else", identity.OfficialChannelID); !errors.Is(err, payment.ErrNotFound) {
		t.Fatalf("foreign lookup: %v", err)
	}
	if _, err := a.Payment.OrderFacts(ctx, originalID, identity.OEMChannelID); !errors.Is(err, payment.ErrNotFound) {
		t.Fatalf("foreign order facts: %v", err)
	}
	var ledgerBefore, ledgerAfter int64
	if err := a.DB.Table("billing_ledger").Where("wallet_id IN (SELECT id FROM billing_wallets WHERE user_id = ?)", reg.User.ID).Count(&ledgerBefore).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := a.Payment.PreviewRefund(ctx, originalID, identity.OfficialChannelID)
	if err != nil || !preview.CanRefund || preview.CreditReclaimMinor != input.CreditMinor {
		t.Fatalf("refund preview %+v %v", preview, err)
	}
	unchanged, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil || unchanged.AvailableMinor != after.AvailableMinor {
		t.Fatal("preview changed wallet")
	}
	if err := a.DB.Table("billing_ledger").Where("wallet_id IN (SELECT id FROM billing_wallets WHERE user_id = ?)", reg.User.ID).Count(&ledgerAfter).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerBefore != ledgerAfter {
		t.Fatal("preview wrote a ledger entry")
	}
	// Repeated notes are allowed. A supplied real transaction ID protects against duplicate cash receipts.
	for i := 0; i < 2; i++ {
		distinct := input
		distinct.OperationID = fmt.Sprintf("%s-shared-%d", marker, i)
		distinct.Note = "same note"
		if _, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, distinct, a.Audit, audit.RecordInput{ActorUserID: "finance"}); err != nil {
			t.Fatal(err)
		}
	}
	external := input
	external.OperationID = marker + "-external-1"
	external.Reference = marker + "-bank-transaction"
	first, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, external, a.Audit, audit.RecordInput{ActorUserID: "finance"})
	if err != nil {
		t.Fatal(err)
	}
	external.OperationID = marker + "-external-2"
	_, err = a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, external, a.Audit, audit.RecordInput{ActorUserID: "finance"})
	var externalConflict *payment.ExternalReceiptConflict
	if !errors.As(err, &externalConflict) || externalConflict.OrderID != first.ID {
		t.Fatalf("external transaction was recorded twice: %v", err)
	}

	// More than 200 matching orders must remain reachable and searchable.
	rows := make([]map[string]any, 230)
	now := time.Now().UTC()
	for i := range rows {
		rows[i] = map[string]any{"id": fmt.Sprintf("%s-page-%03d", marker, i), "user_id": reg.User.ID, "channel_org_id": identity.OfficialChannelID, "payee_channel_org_id": identity.OfficialChannelID, "adapter": "manual", "purpose": "wallet", "amount_minor": 1, "credit_minor": 1, "currency": "USD", "status": "pending", "created_at": now.Add(-time.Duration(i) * time.Second), "updated_at": now}
	}
	if err := a.DB.Table("payment_orders").Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 8; page++ {
		items, err := a.Payment.ListOrders(ctx, payment.ListOrdersFilter{PayeeChannelOrgID: identity.OfficialChannelID, Query: marker + "-page", Cursor: cursor, Limit: 40})
		if err != nil {
			t.Fatal(err)
		}
		if len(items) > 40 {
			items = items[:40]
		}
		for _, item := range items {
			if seen[item.ID] {
				t.Fatal("duplicate page item")
			}
			seen[item.ID] = true
		}
		if len(items) < 40 {
			break
		}
		cursor = items[len(items)-1].ID
	}
	if len(seen) != 230 {
		t.Fatalf("paging stopped at %d", len(seen))
	}
	found, err := a.Payment.ListOrders(ctx, payment.ListOrdersFilter{PayeeChannelOrgID: identity.OfficialChannelID, Query: marker + "-page-229", Limit: 20})
	if err != nil || len(found) != 1 {
		t.Fatalf("late order search: %d %v", len(found), err)
	}
}
