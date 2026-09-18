package app_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestSettlementJourney(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "settlement_journey_admin"
	cfg.BootstrapUser = "settlement_journey_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	// This test isolates settlement state using synthetic roles with no cash beneficiary.
	// Real wallet payout and recovery are covered by TestCommissionPayoutAccounting.
	application.Commission.SetCashier(nil)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	accrue := func(role, usage string) int64 {
		n, err := application.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, RequestID: "req-" + usage, RoleID: role, ChannelOrgID: "settlement-test-" + suffix, WholesaleMinor: 2000000, CanCommission: true})
		if err != nil {
			t.Fatal(err)
		}
		if n <= 0 {
			t.Fatal("missing fixture commission")
		}
		return n
	}
	due := func(usage string) {
		if err := application.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		if _, err := application.Commission.UnfreezeUsage(ctx, time.Now(), usage); err != nil {
			t.Fatal(err)
		}
	}
	list := func(role string) []commission.SettlementView {
		items, err := application.Commission.ListSettlements(ctx, "", []string{role})
		if err != nil {
			t.Fatal(err)
		}
		return items
	}
	batch := func() {
		if _, err := application.Commission.CreateMonthlySettlement(ctx, time.Now(), true); err != nil {
			t.Fatal(err)
		}
	}
	role := "settlement-role-" + suffix
	u1 := "settlement-one-" + suffix
	u2 := "settlement-two-" + suffix
	amount := accrue(role, u1)
	accrue(role, u2)
	// The public action must not bring a future freeze date forward.
	before, err := application.Commission.ListEntries(ctx, "", []string{role}, u1)
	if err != nil {
		t.Fatal(err)
	}
	result := postJSONRaw(t, server.URL+"/admin/commissions/unfreeze", cfg.BootstrapAdmin, map[string]any{"usage_event_id": u1})
	if asInt(result["unfrozen"]) != 0 {
		t.Fatal("public unfreeze bypassed freeze date")
	}
	after, err := application.Commission.ListEntries(ctx, "", []string{role}, u1)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Status != commission.StatusFrozen || !after[0].AvailableAt.Equal(*before[0].AvailableAt) {
		t.Fatal("unfreeze changed future deadline")
	}
	due(u1)
	due(u2)
	// A default monthly run respects the minimum amount; explicit override is separate.
	if _, err := application.Commission.CreateMonthlySettlement(ctx, time.Now(), false); err != nil {
		t.Fatal(err)
	}
	if len(list(role)) != 0 {
		t.Fatal("minimum threshold was bypassed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := application.Commission.CreateMonthlySettlement(ctx, time.Now(), true); err != nil {
				t.Errorf("concurrent batch: %v", err)
			}
		}()
	}
	wg.Wait()
	items := list(role)
	if len(items) != 1 || items[0].AmountMinor != 2*amount {
		t.Fatalf("duplicate or mismatched monthly snapshot: %+v", items)
	}
	oldID := items[0].ID
	if err := application.Commission.Reverse(ctx, u1); err != nil {
		t.Fatal(err)
	}
	items = list(role)
	if items[0].Status != commission.StatusCancelled || items[0].AmountMinor != 2*amount {
		t.Fatalf("original snapshot must be retained and cancelled: %+v", items)
	}
	if _, err := application.Commission.Payout(ctx, oldID, "manual", "wire-old", "test"); !errors.Is(err, commission.ErrSettlementChanged) {
		t.Fatalf("cancelled payout: %v", err)
	}
	left, err := application.Commission.ListEntries(ctx, "", []string{role}, u2)
	if err != nil {
		t.Fatal(err)
	}
	if left[0].Status != commission.StatusAvailable || left[0].SettlementID != "" {
		t.Fatalf("unaffected commission not released: %+v", left)
	}
	batch()
	items = list(role)
	var current commission.SettlementView
	for _, item := range items {
		if item.Status == commission.StatusSettled {
			current = item
		}
	}
	if current.ID == "" || current.AmountMinor != amount {
		t.Fatalf("regenerated amount: %+v", items)
	}
	if code, _ := doJSON(t, "POST", server.URL+"/admin/settlements/"+current.ID+"/payout", cfg.BootstrapAdmin, true, map[string]any{"method": "manual", "reference": "  "}); code != 400 {
		t.Fatalf("empty reference: %d", code)
	}
	for _, token := range []string{cfg.BootstrapAdmin + "-audit", cfg.BootstrapAdmin + "-ops", cfg.BootstrapAdmin + "-tech"} {
		if code, _ := doJSON(t, "POST", server.URL+"/admin/settlements/"+current.ID+"/payout", token, true, map[string]any{"method": "manual", "reference": "wire"}); code != 403 {
			t.Fatalf("payout privilege %s: %d", token, code)
		}
	}
	injected := errors.New("payout persistence failed")
	cb := "settlement_payout_failure"
	if err := application.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "commission_payouts" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = application.Commission.Payout(ctx, current.ID, "manual", "wire-one", "test")
	application.DB.Callback().Create().Remove(cb)
	if !errors.Is(err, injected) {
		t.Fatalf("payout failure: %v", err)
	}
	for _, item := range list(role) {
		if item.ID == current.ID && item.Status != commission.StatusSettled {
			t.Fatal("failed payout committed status")
		}
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := application.Commission.Payout(ctx, current.ID, "manual", "wire-one", "test"); err != nil {
				t.Errorf("payout retry: %v", err)
			}
		}()
	}
	wg.Wait()
	var n int64
	if err := application.DB.Table("commission_payouts").Where("settlement_id = ?", current.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("duplicate payout receipts: %d", n)
	}
	if _, err := application.Commission.Payout(ctx, current.ID, "manual", "wire-other", "test"); !errors.Is(err, commission.ErrConflict) {
		t.Fatalf("changed receipt must conflict: %v", err)
	}
	if err := application.Commission.Reverse(ctx, u2); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Commission.Payout(ctx, current.ID, "manual", "wire-one", "test"); err != nil {
		t.Fatal(err)
	}
	entries, err := application.Commission.ListEntries(ctx, "", []string{role}, u2)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Status != commission.StatusReversed {
			t.Fatal("payout replay resurrected reversed entry")
		}
	}
	for _, item := range list(role) {
		if item.ID == current.ID && (item.Status != commission.StatusPaid || item.PayoutReference != "wire-one" || item.ReversedMinor != amount) {
			t.Fatalf("paid history/reversal disclosure: %+v", item)
		}
	}
	// Competing refund and payout must resolve to either a cancelled unpaid snapshot,
	// or a paid snapshot with visible reversal. Neither can resurrect an entry.
	raceRole := "settlement-race-" + suffix
	raceUsage := "settlement-race-usage-" + suffix
	accrue(raceRole, raceUsage)
	due(raceUsage)
	batch()
	raceID := list(raceRole)[0].ID
	errorsOut := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errorsOut <- application.Commission.Reverse(ctx, raceUsage) }()
	go func() {
		defer wg.Done()
		_, err := application.Commission.Payout(ctx, raceID, "manual", "wire-race", "test")
		errorsOut <- err
	}()
	wg.Wait()
	close(errorsOut)
	for err := range errorsOut {
		if err != nil && !errors.Is(err, commission.ErrSettlementChanged) {
			t.Fatal(err)
		}
	}
	raceEntries, err := application.Commission.ListEntries(ctx, "", []string{raceRole}, raceUsage)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range raceEntries {
		if entry.Status != commission.StatusReversed {
			t.Fatal("race resurrected original commission")
		}
	}
	race := list(raceRole)[0]
	if race.Status != commission.StatusCancelled && race.Status != commission.StatusPaid {
		t.Fatalf("invalid race outcome: %+v", race)
	}
}
