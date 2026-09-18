package app_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestCommissionPayoutAccounting(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "payout_cash_admin"
	cfg.BootstrapUser = "payout_cash_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var owner struct{ ID string }
	if err := a.DB.Table("identity_users").Where("email = ?", "kol2.b@tokenhub.local").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	balance := func() *billing.BalanceView {
		v, err := a.Billing.Balance(ctx, owner.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := balance()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	channel := "payout-cash-" + suffix
	accrue := func(usage string) int64 {
		n, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, RequestID: usage, ChannelOrgID: channel, RoleID: identity.KOL2BRoleID, WholesaleMinor: 2000000, CanCommission: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
		return n
	}
	first := "payout-cash-first-" + suffix
	amount := accrue(first)
	// A single connection must suffice: beneficiary lookup cannot escape the transaction.
	pool, err := a.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	if _, err := a.Commission.UnfreezeUsage(ctx, time.Now(), first); err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(10)
	entries, err := a.Commission.ListEntries(ctx, channel, nil, first)
	if err != nil {
		t.Fatal(err)
	}
	entryID := entries[0].ID
	if balance().CommissionAvailableMinor != before.CommissionAvailableMinor+amount {
		t.Fatal("missing earned cash")
	}
	batch, err := a.Commission.CreateMonthlySettlement(ctx, time.Now(), true)
	if err != nil {
		t.Fatal(err)
	}
	sid := ""
	for _, s := range batch {
		if s.ChannelOrgID == channel {
			sid = s.ID
		}
	}
	if sid == "" {
		t.Fatal("missing settlement")
	}
	injected := errors.New("receipt persistence failure")
	cb := "cash_payout_receipt_failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "commission_payouts" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.Commission.Payout(ctx, sid, "manual", "CASH-TEST", owner.ID)
	a.DB.Callback().Create().Remove(cb)
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected failure: %v", err)
	}
	if balance().CommissionAvailableMinor != before.CommissionAvailableMinor+amount {
		t.Fatal("failed receipt committed cash debit")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Commission.Payout(ctx, sid, "manual", "CASH-TEST", owner.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	after := balance()
	if after.CommissionAvailableMinor != before.CommissionAvailableMinor || after.AvailableMinor != before.AvailableMinor || after.GiftMinor != before.GiftMinor {
		t.Fatal("payout changed wrong wallet or debited twice")
	}
	var count int64
	if err := a.DB.Table("billing_ledger").Where("idempotency_key = ?", "comm-cash-payout:"+entryID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("payout ledger count=%d err=%v", count, err)
	}
	// Future earnings must not silently pay off a reversed, already paid commission.
	second := "payout-cash-second-" + suffix
	amount2 := accrue(second)
	if _, err := a.Commission.UnfreezeUsage(ctx, time.Now(), second); err != nil {
		t.Fatal(err)
	}
	cb = "cash_recovery_failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_commission_recoveries" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err = a.Commission.Reverse(ctx, first)
	a.DB.Callback().Create().Remove(cb)
	if !errors.Is(err, injected) {
		t.Fatalf("expected recovery failure: %v", err)
	}
	entries, err = a.Commission.ListEntries(ctx, channel, nil, first)
	if err != nil || entries[0].Status != commission.StatusPaid {
		t.Fatal("failed recovery committed reversal")
	}
	for i := 0; i < 2; i++ {
		if err := a.Commission.Reverse(ctx, first); err != nil {
			t.Fatal(err)
		}
	}
	after = balance()
	if after.CommissionAvailableMinor != before.CommissionAvailableMinor+amount2 || after.CommissionRecoveryMinor != before.CommissionRecoveryMinor+amount {
		t.Fatalf("recovery double debited cash or lost receivable: %+v", after)
	}
	if _, err := a.Commission.Payout(ctx, sid, "manual", "CASH-TEST", owner.ID); err != nil {
		t.Fatal(err)
	}
	var recovery struct {
		AmountMinor                                  int64
		SettlementID, CreditLedgerID, PayoutLedgerID string
	}
	if err := a.DB.Table("billing_commission_recoveries").Where("commission_entry_id = ?", entryID).First(&recovery).Error; err != nil {
		t.Fatal(err)
	}
	if recovery.AmountMinor != amount || recovery.SettlementID != sid || recovery.CreditLedgerID == "" || recovery.PayoutLedgerID == "" {
		t.Fatalf("untraceable recovery %+v", recovery)
	}
	if err := a.Commission.Reverse(ctx, second); err != nil {
		t.Fatal(err)
	}
	if balance().CommissionAvailableMinor != before.CommissionAvailableMinor {
		t.Fatal("unpaid reversal failed")
	}
	// Missing original wallet credit must prevent an otherwise valid settlement payout.
	missing := "payout-missing-" + suffix
	if _, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: missing, RoleID: "unbound-" + suffix, ChannelOrgID: channel, WholesaleMinor: 2000000, CanCommission: true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Commission.ForceAvailableAt(ctx, missing, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Commission.UnfreezeUsage(ctx, time.Now(), missing); err != nil {
		t.Fatal(err)
	}
	batch, err = a.Commission.CreateMonthlySettlement(ctx, time.Now(), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range batch {
		if s.ChannelOrgID == channel {
			if _, err := a.Commission.Payout(ctx, s.ID, "manual", "MISSING", owner.ID); !errors.Is(err, commission.ErrWalletMismatch) {
				t.Fatalf("unfunded payout: %v", err)
			}
		}
	}
}
