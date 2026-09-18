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

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestCommissionRecoveryReceipts(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "recovery_receipt_admin"
	cfg.BootstrapUser = "recovery_receipt_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	login := func(email string) string {
		return tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": email + "@tokenhub.local", "password": "password1"}))
	}
	finance := login("finance")
	auditToken := login("audit")
	ownerToken := login("kol2.b")
	otherToken := login("kol1.b")
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
	usage := "recovery-receipt-" + suffix
	channel := "recovery-channel-" + suffix
	amount, err := a.Commission.Accrue(ctx, commission.AccrueInput{UsageEventID: usage, RoleID: identity.KOL2BRoleID, ChannelOrgID: channel, WholesaleMinor: 2000000, CanCommission: true})
	if err != nil || amount <= 0 {
		t.Fatalf("accrue %d %v", amount, err)
	}
	if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Commission.UnfreezeUsage(ctx, time.Now(), usage); err != nil {
		t.Fatal(err)
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
	if _, err := a.Commission.Payout(ctx, sid, "manual", "RECOVERY-PAYOUT-"+suffix, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Commission.Reverse(ctx, usage); err != nil {
		t.Fatal(err)
	}
	var claim struct{ ID string }
	if err := a.DB.Table("billing_commission_recoveries").Where("settlement_id = ?", sid).First(&claim).Error; err != nil {
		t.Fatal(err)
	}
	url := server.URL + "/admin/commission-recoveries/" + claim.ID + "/receipts"
	in := billing.RecoveryReceiptInput{AmountMinor: amount / 3, Reference: "RECOVERY-PART1-" + suffix, Note: "internal receipt note", IdempotencyKey: "recovery-first-" + suffix}
	call := func(token string, confirm bool, payload billing.RecoveryReceiptInput, want int) {
		t.Helper()
		code, body := doJSON(t, "POST", url, token, confirm, payload)
		if code != want {
			t.Fatalf("receipt status %d want %d: %+v", code, want, body)
		}
	}
	check := func(recovered int64, status string, count int64) {
		t.Helper()
		var row struct {
			RecoveredMinor int64
			Status         string
		}
		if err := a.DB.Table("billing_commission_recoveries").Where("id = ?", claim.ID).First(&row).Error; err != nil {
			t.Fatal(err)
		}
		if row.RecoveredMinor != recovered || row.Status != status {
			t.Fatalf("claim %+v", row)
		}
		v := balance()
		if v.CommissionRecoveryMinor != before.CommissionRecoveryMinor+amount-recovered || v.CommissionAvailableMinor != before.CommissionAvailableMinor || v.AvailableMinor != before.AvailableMinor || v.GiftMinor != before.GiftMinor {
			t.Fatalf("wallet changed or remaining wrong %+v", v)
		}
		for _, table := range []string{"billing_commission_recovery_receipts", "audit_logs"} {
			q := a.DB.Table(table)
			if table == "audit_logs" {
				q = q.Where("resource_id = ? AND action = ?", claim.ID, "commission.recovery.received")
			} else {
				q = q.Where("recovery_id = ?", claim.ID)
			}
			var n int64
			if err := q.Count(&n).Error; err != nil || n != count {
				t.Fatalf("%s count %d want %d err %v", table, n, count, err)
			}
		}
		var events int64
		if err := a.DB.Table("outbox_events").Where("aggregate_id IN (?)", a.DB.Table("audit_logs").Select("id").Where("resource_id = ? AND action = ?", claim.ID, "commission.recovery.received")).Count(&events).Error; err != nil || events != count {
			t.Fatalf("outbox %d want %d: %v", events, count, err)
		}
	}
	check(0, "pending", 0)
	for _, token := range []string{auditToken, login("ops"), login("tech"), login("channel.b"), ownerToken, cfg.BootstrapUser} {
		call(token, true, in, 403)
	}
	for _, token := range []string{finance, auditToken, cfg.BootstrapAdmin} {
		code, _ := doJSON(t, "GET", server.URL+"/admin/commission-recoveries", token, false, nil)
		if code != 200 {
			t.Fatalf("read denied %d", code)
		}
	}
	for _, token := range []string{login("ops"), login("tech"), ownerToken, cfg.BootstrapUser} {
		code, _ := doJSON(t, "GET", server.URL+"/admin/commission-recoveries", token, false, nil)
		if code != 403 {
			t.Fatalf("read allowed %d", code)
		}
	}
	call(finance, false, in, 409)
	bad := in
	bad.AmountMinor = amount + 1
	call(finance, true, bad, 400)
	bad = in
	bad.Reference = " "
	call(finance, true, bad, 400)
	// Receipt, running total, audit, and audit outbox must roll back as one unit.
	for _, table := range []string{"audit_logs", "outbox_events"} {
		cb := "recovery_fail_" + table
		if err := a.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
			if tx.Statement.Table == table {
				tx.AddError(errors.New("injected receipt audit failure"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		call(finance, true, in, 500)
		a.DB.Callback().Create().Remove(cb)
		check(0, "pending", 0)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); call(finance, true, in, 200) }()
	}
	wg.Wait()
	check(in.AmountMinor, "pending", 1)
	bad = in
	bad.AmountMinor++
	call(finance, true, bad, 409)
	bad = in
	bad.IdempotencyKey += "-duplicate-reference"
	call(finance, true, bad, 409)
	adminHistory := getAuthJSON(t, server.URL+"/admin/commission-recoveries", finance)
	if !containsText(adminHistory, "finance@tokenhub.local") {
		t.Fatal("receipt operator email missing")
	}
	// User sees only their receipts; finance-only notes and operator identifiers stay private.
	own := getAuthJSON(t, server.URL+"/v1/me/commission-recoveries", ownerToken)
	if !containsText(own, in.Reference) || containsText(own, in.Note) || containsText(own, "actor_user_id") || containsText(own, "actor_email") {
		t.Fatalf("own receipt missing or leaks internal fields: %+v", own)
	}
	other := getAuthJSON(t, server.URL+"/v1/me/commission-recoveries", otherToken)
	if containsText(other, claim.ID) {
		t.Fatal("cross-user claim leak")
	}
	// Two distinct receipts compete for the entire remaining amount; only one may commit.
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			last := billing.RecoveryReceiptInput{AmountMinor: amount - in.AmountMinor, Reference: "RECOVERY-LAST-" + suffix + strconv.Itoa(i), IdempotencyKey: "recovery-last-" + suffix + strconv.Itoa(i)}
			code, _ := doJSON(t, "POST", url, finance, true, last)
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	successes := 0
	for code := range codes {
		if code == 200 {
			successes++
		} else if code != 409 {
			t.Fatalf("unexpected concurrent status %d", code)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes=%d", successes)
	}
	check(amount, "closed", 2)
	call(finance, true, in, 200)
	if err := a.Commission.Reverse(ctx, usage); err != nil {
		t.Fatal(err)
	}
	check(amount, "closed", 2)
	settlements, err := a.Commission.ListSettlements(ctx, channel, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range settlements {
		if s.ID == sid {
			found = true
			if !s.RecoveryTracked || s.RecoveredMinor != amount || s.RecoveryPendingMinor != 0 {
				t.Fatalf("stale settlement recovery %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("settlement missing")
	}
}
