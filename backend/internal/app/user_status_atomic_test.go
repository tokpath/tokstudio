package app_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestUserStatusCommissionAtomicity(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "atomic_status_admin"
	cfg.BootstrapUser = "atomic_status_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": "status-" + suffix + "@example.test", "password": "password1", "promotion_code": identity.PromoKOL2B})
	uid := userIDOf(reg)
	usage := "status-usage-" + suffix
	if _, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, UserID: uid, RoleID: identity.KOL2BRoleID, ChannelOrgID: identity.ResellerChannelID, WholesaleMinor: 2000000, CanCommission: true}); err != nil {
		t.Fatal(err)
	}
	check := func(userState, entryState string) {
		var u struct{ Status string }
		if err := a.DB.Table("identity_users").Where("id = ?", uid).First(&u).Error; err != nil {
			t.Fatal(err)
		}
		entries, err := a.Commission.ListEntries(ctx, "", nil, usage)
		if err != nil {
			t.Fatal(err)
		}
		if u.Status != userState || len(entries) != 1 || entries[0].Status != entryState {
			t.Fatalf("partial state: user=%s entries=%+v", u.Status, entries)
		}
	}
	call := func(action string, want int) {
		code, _ := doJSON(t, "POST", server.URL+"/admin/users/"+uid+"/"+action, cfg.BootstrapAdmin, true, map[string]any{"reason": "test atomicity"})
		if code != want {
			t.Fatalf("%s status=%d want=%d", action, code, want)
		}
	}
	injected := errors.New("commission state unavailable")
	cb := "status_hold_failure"
	if err := a.DB.Callback().Update().Before("gorm:update").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "commission_entries" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	call("ban", 500)
	a.DB.Callback().Update().Remove(cb)
	check(identity.UserStatusActive, commission.StatusFrozen)
	call("ban", 200)
	check(identity.UserStatusBanned, commission.StatusHeld)
	if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	cb = "status_cash_failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_ledger" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	call("unban", 500)
	a.DB.Callback().Create().Remove(cb)
	check(identity.UserStatusBanned, commission.StatusHeld)
	call("unban", 200)
	check(identity.UserStatusActive, commission.StatusAvailable)
	call("unban", 200)
	check(identity.UserStatusActive, commission.StatusAvailable)
	if err := a.Commission.Reverse(ctx, usage); err != nil {
		t.Fatal(err)
	}
}
