package app_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestCommissionHoldAccounting(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "hold_accounting_admin"
	cfg.BootstrapUser = "hold_accounting_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	ctx := context.Background()
	var beneficiary struct{ ID string }
	if err := a.DB.Table("identity_users").Where("email = ?", "kol2.b@tokenhub.local").First(&beneficiary).Error; err != nil {
		t.Fatal(err)
	}
	cash := func() int64 {
		b, err := a.Billing.Balance(ctx, beneficiary.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		return b.CommissionAvailableMinor
	}
	baseline := cash()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	channel := "held-channel-" + suffix
	user := "held-user-" + suffix
	accrue := func(usage string) int64 {
		n, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, RequestID: usage, UserID: user, ChannelOrgID: channel, RoleID: identity.KOL2BRoleID, WholesaleMinor: 2000000, CanCommission: true})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	status := func(usage string, want string) {
		entries, err := a.Commission.ListEntries(ctx, "", nil, usage)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Fatal("missing entry")
		}
		for _, e := range entries {
			if e.Status != want {
				t.Fatalf("status = %s, want %s", e.Status, want)
			}
		}
	}
	marketing := func(frozen, issued int64) {
		got, err := a.Commission.MarketingTotals(ctx, channel)
		if err != nil {
			t.Fatal(err)
		}
		if got.FrozenMinor != frozen || got.IssuedMinor != issued {
			t.Fatalf("marketing %+v, want frozen=%d issued=%d", got, frozen, issued)
		}
	}
	hold := func() {
		if _, err := a.Commission.HoldUnsettledForUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	release := func() {
		if _, err := a.Commission.ReleaseHeldForUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	due := func(usage string) {
		if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	usage := "held-due-" + suffix
	amount := accrue(usage)
	hold()
	release()
	status(usage, commission.StatusFrozen)
	if cash() != baseline {
		t.Fatal("release bypassed freeze period")
	}
	marketing(-amount, 0)
	hold()
	due(usage)
	injected := errors.New("commission credit unavailable")
	cb := "hold_credit_failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_ledger" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	n, err := a.Commission.ReleaseHeldForUser(ctx, user)
	a.DB.Callback().Create().Remove(cb)
	if !errors.Is(err, injected) || n != 0 {
		t.Fatalf("release rollback count=%d err=%v", n, err)
	}
	status(usage, commission.StatusHeld)
	marketing(-amount, 0)
	if cash() != baseline {
		t.Fatal("failed release changed wallet")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Commission.ReleaseHeldForUser(ctx, user); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	status(usage, commission.StatusAvailable)
	marketing(0, -amount)
	if cash() != baseline+amount {
		t.Fatal("due release did not credit exactly once")
	}
	hold()
	release()
	hold()
	release()
	if cash() != baseline+amount {
		t.Fatal("repeated hold/release duplicated cash")
	}
	marketing(0, -amount)
	hold()
	if err := a.Commission.Reverse(ctx, usage); err != nil {
		t.Fatal(err)
	}
	status(usage, commission.StatusReversed)
	marketing(0, 0)
	if cash() != baseline {
		t.Fatal("refund during hold left commission cash behind")
	}
	release()
	if err := a.Commission.Reverse(ctx, usage); err != nil {
		t.Fatal(err)
	}
	if cash() != baseline {
		t.Fatal("replay changed cash")
	}
	frozenUsage := "held-frozen-" + suffix
	frozenAmount := accrue(frozenUsage)
	hold()
	marketing(-frozenAmount, 0)
	if err := a.Commission.Reverse(ctx, frozenUsage); err != nil {
		t.Fatal(err)
	}
	status(frozenUsage, commission.StatusReversed)
	marketing(0, 0)
	if cash() != baseline {
		t.Fatal("frozen hold refund debited unearned cash")
	}
}
