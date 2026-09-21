package app_test

import (
	"context"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCommissionRecalcPreservesConfirmedEarnings(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("requires postgres")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "recalc_admin"
	cfg.BootstrapUser = "recalc_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": "recalc-" + suffix + "@example.test", "password": "password1", "promotion_code": identity.PromoKOL2B})
	uid := userIDOf(reg)
	req := "recalc-" + suffix
	if err := a.Billing.Credit(ctx, uid, req, 10000000, "test fixture"); err != nil {
		t.Fatal(err)
	}
	prices := []byte(`{"input":"1","output":"1","wholesale_input":"0.5","wholesale_output":"0.5"}`)
	_, err = a.Billing.Reserve(ctx, billing.ReserveInput{UserID: uid, ChannelOrgID: identity.ResellerChannelID, RequestID: req, PublicModelID: "recalc-fixture", ReserveMinor: 4000000, UnitPrices: prices})
	if err != nil {
		t.Fatal(err)
	}
	settled, err := a.Billing.Settle(ctx, billing.SettleInput{RequestID: req, UserID: uid, ChannelOrgID: identity.ResellerChannelID, PublicModelID: "recalc-fixture", Usage: map[string]int{"prompt_tokens": 3, "completion_tokens": 1}, UnitPrices: prices, FactSource: "test"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.Commission.ListEntries(ctx, "", nil, settled.UsageEventID)
	if err != nil || len(before) != 2 {
		t.Fatalf("missing commission: %+v %v", before, err)
	}
	if _, err := a.Billing.RecalcCommission(ctx, settled.UsageEventID); err != nil {
		t.Fatalf("recalc failed after valid accrual: %v", err)
	}
	after, err := a.Commission.ListEntries(ctx, "", nil, settled.UsageEventID)
	if err != nil || len(after) != len(before) {
		t.Fatalf("unchanged valid commission should remain unchanged: %+v %v", after, err)
	}
	for i := range before {
		if after[i].ID != before[i].ID || after[i].Status != before[i].Status || after[i].AmountMinor != before[i].AmountMinor {
			t.Fatal("recalc changed correct historical earnings")
		}
	}
	codeProbe, probeBody := doJSON(t, "POST", server.URL+"/admin/providers/prd_ark/health-check", cfg.BootstrapAdmin+"-tech", false, map[string]any{})
	if codeProbe != 501 || !containsText(probeBody, "probe_not_supported") {
		t.Fatalf("fake real-upstream success: %d %+v", codeProbe, probeBody)
	}

	// A later policy must not rewrite a confirmed historical calculation.
	policy, err := a.Commission.ActivePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	updated := *policy
	updated.Version = ""
	updated.DirectBPS = 1000
	updated.IndirectBPS = 100
	updated.OverrideBPS = 100
	updated.FreezeDays = 0
	fresh, err := a.Commission.UpdatePolicy(ctx, updated)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Version == policy.Version || fresh.FreezeDays != 0 {
		t.Fatal("policy not versioned or zero freeze ignored")
	}
	defer func() {
		restore := *policy
		restore.Version = ""
		if _, err := a.Commission.UpdatePolicy(ctx, restore); err != nil {
			t.Error(err)
		}
	}()
	check := func() {
		t.Helper()
		rows, err := a.Commission.ListEntries(ctx, "", nil, settled.UsageEventID)
		if err != nil || len(rows) != len(before) {
			t.Fatalf("changed history %+v %v", rows, err)
		}
		for i := range before {
			if rows[i].ID != before[i].ID || rows[i].AmountMinor != before[i].AmountMinor || rows[i].Status != before[i].Status {
				t.Fatal("history changed")
			}
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Billing.RecalcCommission(ctx, settled.UsageEventID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	check()
	// All commission work must share the caller transaction; one connection is enough.
	pool, err := a.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	limited, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, err = a.Billing.RecalcCommission(limited, req)
	cancel()
	pool.SetMaxOpenConns(10)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a bad frozen computation with matching original marketing entry.
	target := before[0]
	if err := a.DB.Table("commission_entries").Where("id = ?", target.ID).Update("amount_minor", target.AmountMinor+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("commission_marketing_entries").Where("commission_entry_id = ?", target.ID).Update("amount_minor", -(target.AmountMinor + 1)).Error; err != nil {
		t.Fatal(err)
	}
	injected := errors.New("recalc audit failure")
	cb := "recalc-audit-failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	code, _ := doJSON(t, "POST", server.URL+"/admin/commissions/recalc", cfg.BootstrapAdmin, true, map[string]any{"usage_event_id": req})
	a.DB.Callback().Create().Remove(cb)
	if code != 500 {
		t.Fatalf("want rollback 500 got %d", code)
	}
	rows, err := a.Commission.ListEntries(ctx, "", nil, settled.UsageEventID)
	if err != nil || len(rows) != 2 {
		t.Fatal("failed correction partially reversed records")
	}
	code, body := doJSON(t, "POST", server.URL+"/admin/commissions/recalc", cfg.BootstrapAdmin, true, map[string]any{"usage_event_id": req})
	if code != 200 {
		t.Fatalf("correction failed %+v", body)
	}
	rows, err = a.Commission.ListEntries(ctx, "", nil, settled.UsageEventID)
	if err != nil || len(rows) != 6 {
		t.Fatalf("missing preserved reversal history %+v %v", rows, err)
	}
	var active int
	for _, row := range rows {
		if row.Status != commission.StatusReversed {
			active++
			if row.PolicyVersion != policy.Version {
				t.Fatal("used today's policy")
			}
			for _, old := range before {
				if old.Kind == row.Kind && (old.AmountMinor != row.AmountMinor || !old.AvailableAt.Equal(*row.AvailableAt)) {
					t.Fatal("wrong amount or renewed freezing")
				}
			}
		}
	}
	if active != 2 {
		t.Fatalf("active=%d", active)
	}
	if _, err := a.Billing.RecalcCommission(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Billing.RefundCharge(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Billing.RecalcCommission(ctx, req); err != nil {
		t.Fatal(err)
	}
	rows, err = a.Commission.ListEntries(ctx, "", nil, settled.UsageEventID)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Status != commission.StatusReversed {
			t.Fatal("recalculation resurrected refunded earnings")
		}
	}

	// A billing failure after commission creation must not leave orphan commissions.
	req2 := req + "-rollback"
	_, err = a.Billing.Reserve(ctx, billing.ReserveInput{UserID: uid, ChannelOrgID: identity.ResellerChannelID, RequestID: req2, PublicModelID: "recalc-fixture", ReserveMinor: 4000000, UnitPrices: prices})
	if err != nil {
		t.Fatal(err)
	}
	input := billing.SettleInput{RequestID: req2, UserID: uid, ChannelOrgID: identity.ResellerChannelID, PublicModelID: "recalc-fixture", Usage: map[string]int{"prompt_tokens": 3, "completion_tokens": 1}, UnitPrices: prices, FactSource: "test"}
	cb = "settlement-after-commission-failure"
	if err := a.DB.Callback().Update().Before("gorm:update").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_authorizations" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.Billing.Settle(ctx, input)
	a.DB.Callback().Update().Remove(cb)
	if !errors.Is(err, injected) {
		t.Fatalf("missing injected settlement failure: %v", err)
	}
	var orphan int64
	if err := a.DB.Table("commission_entries").Where("request_id = ?", req2).Count(&orphan).Error; err != nil || orphan != 0 {
		t.Fatalf("orphan commission: %d %v", orphan, err)
	}
	if _, err := a.Billing.Settle(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("commission_entries").Where("request_id = ?", req2).Count(&orphan).Error; err != nil || orphan != 2 {
		t.Fatalf("retry didn't accrue exactly once: %d %v", orphan, err)
	}

}
