package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

type refundFactAdapter struct{ payment.Adapter }

func (a refundFactAdapter) ParseWebhook(ctx context.Context, in payment.WebhookRequest) (*payment.WebhookEvent, error) {
	ev, err := a.Adapter.ParseWebhook(ctx, in)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Amount   int64  `json:"refund_amount"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(in.Body, &payload); err != nil {
		return nil, err
	}
	if payload.Currency != "" {
		ev.CheckRefundAmount = true
		ev.RefundAmountMinor = &payload.Amount
		ev.Currency = payload.Currency
	}
	return ev, nil
}

func TestOverhaulWebhookFinancialRecovery(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	marker := "callback-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	reg, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "@example.test", Password: "isolated-password", PromotionCode: "THA1"})
	if err != nil {
		t.Fatal(err)
	}
	original, _ := a.Payment.Registry().Get(payment.AdapterManual)
	a.Payment.Registry().MustRegister(refundFactAdapter{original})
	order, err := a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: reg.User.ID, ChannelOrgID: identity.OfficialChannelID, Adapter: payment.AdapterManual, Purpose: payment.PurposeWallet, AmountMinor: billing.MinorPerUSD, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	callback := func(event, status string, amount int64, extra string) (*payment.EventView, error) {
		payload := map[string]any{"event_id": event, "order_id": order.ID, "status": status, "trade_id": "original-trade"}
		if amount >= 0 {
			payload["refund_amount"] = amount
			payload["currency"] = "USD"
		}
		if extra != "" {
			payload["note"] = extra
		}
		body, _ := json.Marshal(payload)
		headers := http.Header{"X-Tokenhub-Payment-Signature": {payment.SignWebhook(a.Payment.SignKey(), event, order.ID, status)}}
		return a.Payment.HandleWebhook(ctx, payment.AdapterManual, headers, body)
	}
	updateHook := marker + "-payment-failure"
	if err := a.DB.Callback().Update().Before("gorm:update").Register(updateHook, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "payment_orders" {
			tx.AddError(errors.New("injected payment apply failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer a.DB.Callback().Update().Remove(updateHook)
	if _, err := callback(marker+"-paid", payment.StatusPaid, -1, ""); err == nil {
		t.Fatal("expected local payment failure")
	}
	a.DB.Callback().Update().Remove(updateHook)
	current, err := a.Payment.GetOrder(ctx, order.ID, "")
	if err != nil || current.Status != payment.StatusPending {
		t.Fatalf("failed application changed order: %+v %v", current, err)
	}
	paid, err := callback(marker+"-paid", payment.StatusPaid, -1, "")
	if err != nil || !paid.Duplicate {
		t.Fatalf("stored paid callback did not recover: %+v %v", paid, err)
	}
	balance, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil || balance.AvailableMinor != billing.MinorPerUSD {
		t.Fatalf("incorrect credit: %+v %v", balance, err)
	}
	for _, state := range []string{payment.StatusRefunding, payment.StatusRefundFailed} {
		if _, err := callback(marker+"-"+state, state, billing.MinorPerUSD, ""); err != nil {
			t.Fatal(err)
		}
		current, err = a.Payment.GetOrder(ctx, order.ID, "")
		if err != nil || current.Status != payment.StatusPaid || current.RefundStatus != state {
			t.Fatalf("refund state misclassified: %+v %v", current, err)
		}
	}
	if _, err := callback(marker+"-partial", payment.StatusRefunded, billing.MinorPerUSD/2, ""); err != nil {
		t.Fatal(err)
	}
	current, err = a.Payment.GetOrder(ctx, order.ID, "")
	if err != nil || current.Status != payment.StatusPaid || current.RefundStatus != payment.StatusRefundPartial || current.RefundAmountMinor == nil || *current.RefundAmountMinor != billing.MinorPerUSD/2 {
		t.Fatalf("partial refund lost actual facts: %+v %v", current, err)
	}
	unchanged, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil || unchanged.AvailableMinor != balance.AvailableMinor {
		t.Fatal("partial callback reversed the whole wallet")
	}
	if _, err := a.Payment.Refund(ctx, order.ID); !errors.Is(err, payment.ErrRefundNeedsReview) {
		t.Fatalf("unreviewed additional refund accepted: %v", err)
	}
	createHook := marker + "-reversal-failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(createHook, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "billing_ledger" {
			tx.AddError(errors.New("injected reversal apply failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer a.DB.Callback().Create().Remove(createHook)
	if _, err := callback(marker+"-refund", payment.StatusRefunded, billing.MinorPerUSD, ""); err == nil {
		t.Fatal("expected local reversal failure")
	}
	a.DB.Callback().Create().Remove(createHook)
	facts, err := a.Payment.OrderFacts(ctx, order.ID, identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Order.Status != payment.StatusPaid || facts.Order.RefundStatus != payment.StatusRefunded || facts.Events[len(facts.Events)-1].ProcessingError != "local_application_failed" {
		t.Fatalf("lost confirmed refund or pending recovery: %+v", facts)
	}
	recovered, err := callback(marker+"-refund", payment.StatusRefunded, billing.MinorPerUSD, "")
	if err != nil || !recovered.Duplicate {
		t.Fatalf("stored refund callback did not recover: %+v %v", recovered, err)
	}
	final, err := a.Payment.GetOrder(ctx, order.ID, "")
	if err != nil || final.Status != payment.StatusRefunded || final.RefundedAt == nil {
		t.Fatalf("refund did not complete: %+v %v", final, err)
	}
	for i := 0; i < 3; i++ {
		if _, err := callback(marker+"-refund", payment.StatusRefunded, billing.MinorPerUSD, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := callback(marker+"-paid", payment.StatusPaid, -1, ""); err != nil {
		t.Fatalf("late original paid replay failed: %v", err)
	}
	if _, err := callback(marker+"-refund", payment.StatusRefunded, billing.MinorPerUSD, "altered event"); !errors.Is(err, payment.ErrInvalidEvent) {
		t.Fatalf("changed event payload accepted: %v", err)
	}
	balance, err = a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil || balance.AvailableMinor != 0 {
		t.Fatalf("replayed refund moved money twice: %+v %v", balance, err)
	}
	var refunds int64
	if err := a.DB.Table("billing_ledger").Where("wallet_id IN (SELECT id FROM billing_wallets WHERE user_id = ?) AND reference_id = ? AND event_type = ?", reg.User.ID, order.ReferenceID, "refund").Count(&refunds).Error; err != nil {
		t.Fatal(err)
	}
	if refunds != 1 {
		t.Fatalf("expected one reversal ledger row, got %d", refunds)
	}
	order, err = a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: reg.User.ID, ChannelOrgID: identity.OfficialChannelID, Adapter: payment.AdapterManual, Purpose: payment.PurposeWallet, AmountMinor: billing.MinorPerUSD, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Callback().Update().Before("gorm:update").Register(updateHook, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "payment_orders" {
			tx.AddError(errors.New("worker apply failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := callback(marker+"-worker", payment.StatusPaid, -1, ""); err == nil {
		t.Fatal("expected apply failure before worker")
	}
	a.DB.Callback().Update().Remove(updateHook)
	if _, err := a.Payment.RetryUnappliedEvents(ctx); err != nil {
		t.Fatal(err)
	}
	current, err = a.Payment.GetOrder(ctx, order.ID, "")
	if err != nil || current.Status != payment.StatusPaid || current.FulfilledAt == nil || current.TradeID != "original-trade" {
		t.Fatalf("worker recovery lost original payment: %+v %v", current, err)
	}

	beforeEarly, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	order, err = a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: reg.User.ID, ChannelOrgID: identity.OfficialChannelID, Adapter: payment.AdapterManual, Purpose: payment.PurposeWallet, AmountMinor: billing.MinorPerUSD, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callback(marker+"-early-refund", payment.StatusRefunded, billing.MinorPerUSD, ""); err != nil {
		t.Fatalf("early full refund failed: %v", err)
	}
	if _, err := callback(marker+"-late-paid", payment.StatusPaid, -1, ""); err != nil {
		t.Fatalf("late paid callback failed: %v", err)
	}
	current, err = a.Payment.GetOrder(ctx, order.ID, "")
	if err != nil || current.Status != payment.StatusRefunded || current.FulfilledAt != nil {
		t.Fatalf("late paid event reactivated refunded order: %+v %v", current, err)
	}
	afterEarly, err := a.Billing.Balance(ctx, reg.User.ID, identity.OfficialChannelID)
	if err != nil || beforeEarly.AvailableMinor != afterEarly.AvailableMinor {
		t.Fatal("uncredited refund moved wallet money")
	}
	sub, err := a.Plans.CreateSubscription(ctx, reg.User.ID, identity.OfficialChannelID, "pln_echo_month", payment.AdapterManual, "", identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	order, err = a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: reg.User.ID, ChannelOrgID: identity.OfficialChannelID, Adapter: payment.AdapterManual, Purpose: payment.PurposeSubscription, ReferenceType: payment.PurposeSubscription, ReferenceID: sub.ID, AmountMinor: billing.MinorPerUSD, Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callback(marker+"-plan-paid", payment.StatusPaid, -1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := callback(marker+"-plan-refunded", payment.StatusRefunded, billing.MinorPerUSD, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := callback(marker+"-plan-late-paid", payment.StatusPaid, -1, ""); err != nil {
		t.Fatal(err)
	}
	sub, err = a.Plans.GetSubscription(ctx, sub.ID, "")
	if err != nil || sub.Status != "cancelled" || sub.RenewalPolicy != "manual" {
		t.Fatalf("refunded subscription still renews: %+v %v", sub, err)
	}
	ents, err := a.Plans.ListEntitlements(ctx, reg.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if ent.SourceID == sub.ID && ent.Status != "reversed" {
			t.Fatalf("refund retained active entitlement: %+v", ent)
		}
	}
	if err := a.Plans.ForcePeriodEnd(ctx, sub.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	if _, err := a.Plans.ProcessRenewals(ctx, time.Now(), func(id, _, _ string) error {
		if id == sub.ID {
			calls++
		}
		return payment.ErrChargeFailed
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("refunded subscription reached renewal charger")
	}

}
