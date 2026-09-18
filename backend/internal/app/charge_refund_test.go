package app_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestChargeRefundAtomicity(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "refund_atomic_admin"
	cfg.BootstrapUser = "refund_atomic_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "refund-atomic-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	uid, session := userIDOf(reg), tokenOf(reg)
	postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})
	if _, err := application.Plans.GrantBonus(ctx, "refund-test", uid, uid, "usd_credit", 1, time.Hour); err != nil {
		t.Fatal(err)
	}
	beforeWallet, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	key := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "refund-atomic"})["item"].(map[string]any)["key"].(string)
	postEchoUsage(t, server.URL+"/v1/chat/completions", key, "refund-atomic")
	usage := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)[0].(map[string]any)
	requestID, usageID := usage["request_id"].(string), usage["id"].(string)
	previewURL := server.URL + "/admin/refunds/preview?request_id=" + requestID
	for _, token := range []string{cfg.BootstrapAdmin, cfg.BootstrapAdmin + "-finance"} {
		preview := getAuthJSON(t, previewURL, token)
		item := preview["item"].(map[string]any)
		if item["user_id"] != uid || asInt(item["entitlement_minor"]) != 1 || asInt(item["wallet_minor"])+asInt(item["entitlement_minor"])+asInt(item["gift_minor"]) != asInt(item["amount_minor"]) {
			t.Fatalf("incorrect preview: %+v", preview)
		}
	}
	for _, token := range []string{session, cfg.BootstrapAdmin + "-ops", cfg.BootstrapAdmin + "-audit", cfg.BootstrapAdmin + "-tech"} {
		if code, _ := doJSON(t, "GET", previewURL, token, false, nil); code != 403 {
			t.Fatalf("preview permission: %d", code)
		}
	}
	entries := getAuthJSON(t, server.URL+"/admin/commissions?usage_event_id="+usageID, cfg.BootstrapAdmin)["items"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected direct and indirect commissions: %+v", entries)
	}
	// Include available commission cash, not just frozen accounting entries.
	if _, err := application.Commission.Unfreeze(ctx, time.Now().AddDate(0, 0, 30)); err != nil {
		t.Fatal(err)
	}
	tables := []string{"billing_wallets", "billing_ledger", "billing_authorizations", "billing_customer_charges", "billing_usage_events", "billing_quota_consumes", "billing_quota_allocations", "commission_entries", "commission_marketing_entries", "plans_entitlement_accounts", "plans_entitlement_ledger", "outbox_events"}
	snapshot := func() map[string]string {
		out := map[string]string{}
		for _, table := range tables {
			var value string
			if err := application.DB.Raw("SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text), '[]'::jsonb)::text FROM " + table + " r").Scan(&value).Error; err != nil {
				t.Fatal(err)
			}
			out[table] = value
		}
		return out
	}
	before := snapshot()
	injected := errors.New("injected refund write failure")
	for _, target := range []struct{ table, operation string }{
		{"commission_entries", "create"}, {"plans_entitlement_ledger", "create"},
		{"billing_usage_events", "update"}, {"billing_authorizations", "update"}, {"outbox_events", "create"},
	} {
		t.Run(target.table, func(t *testing.T) {
			name := "refund_fault_" + target.table
			callback := func(tx *gorm.DB) {
				if tx.Statement.Table == target.table {
					tx.AddError(injected)
				}
			}
			if target.operation == "create" {
				if err := application.DB.Callback().Create().Before("gorm:create").Register(name, callback); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := application.DB.Callback().Update().Before("gorm:update").Register(name, callback); err != nil {
					t.Fatal(err)
				}
			}
			_, refundErr := application.Billing.RefundCharge(ctx, requestID)
			if target.operation == "create" {
				application.DB.Callback().Create().Remove(name)
			} else {
				application.DB.Callback().Update().Remove(name)
			}
			if !errors.Is(refundErr, injected) {
				t.Fatalf("expected propagated failure, got %v", refundErr)
			}
			after := snapshot()
			for _, table := range tables {
				if before[table] != after[table] {
					t.Errorf("partial refund changed %s", table)
				}
			}
		})
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := application.Billing.RefundCharge(ctx, requestID); err != nil {
				t.Errorf("concurrent refund: %v", err)
			}
		}()
	}
	wg.Wait()
	afterWallet, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if afterWallet.PurchasedMinor != beforeWallet.PurchasedMinor {
		t.Fatalf("wallet not restored: before=%+v after=%+v", beforeWallet, afterWallet)
	}
	var consumed int64
	if err := application.DB.Table("plans_entitlement_accounts").Where("user_id = ?", uid).Select("COALESCE(SUM(consumed),0)").Scan(&consumed).Error; err != nil {
		t.Fatal(err)
	}
	if consumed != 0 {
		t.Fatalf("entitlement not restored: %d", consumed)
	}
	var count int64
	if err := application.DB.Table("commission_entries").Where("usage_event_id = ? AND reversal_of IS NOT NULL", usageID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(entries)) {
		t.Fatalf("duplicate/missing commission reversals: %d", count)
	}
	completed := snapshot()
	if _, err := application.Billing.RefundCharge(ctx, requestID); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(completed, snapshot()) {
		t.Fatal("retry changed completed refund")
	}
}
