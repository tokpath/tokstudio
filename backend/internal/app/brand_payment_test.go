package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestBrandPaymentOwnershipAndOfflineAllocation(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin, cfg.BootstrapChannel, cfg.BootstrapUser = "brandpay_admin", "brandpay_b", "brandpay_user"
	cfg.EncryptionKey, cfg.PaymentSignKey = "dev-only-32-byte-key-change-me!!", "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	ctx := context.Background()
	marker := "brandpay-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	oem := "brandpay_admin-c"
	child, err := a.Identity.CreateChannel(ctx, identity.Principal{Roles: []string{"channel_admin"}, ChannelOrgID: identity.OEMChannelID}, identity.ChannelInput{Code: marker, Type: "B"})
	if err != nil {
		t.Fatal(err)
	}
	register := func(suffix, promo string) map[string]any {
		return postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": marker + suffix + "@example.test", "password": "password1", "promotion_code": promo})
	}
	bCustomer, cCustomer := register("-b", "THB1"), register("-c", "THC1")
	bUser, cUser := userIDOf(bCustomer), userIDOf(cCustomer)
	if err := a.DB.Exec("UPDATE identity_users SET channel_org_id = ? WHERE id = ?", child.ID, cUser).Error; err != nil {
		t.Fatal(err)
	}
	for channel, owner := range map[string]string{identity.ResellerChannelID: identity.OfficialChannelID, child.ID: identity.OEMChannelID, identity.OEMChannelID: identity.OEMChannelID} {
		actual, err := a.Identity.ResolvePaymentOwnerID(ctx, channel)
		if err != nil || actual != owner {
			t.Fatalf("owner %s: %s %v", channel, actual, err)
		}
	}
	for _, path := range []string{"/channel/payments/overview", "/channel/payments/orders", "/channel/payments/instances", "/channel/quota", "/channel/supplier-entries"} {
		if status := getStatus(t, server.URL+path, "brandpay_b"); status != 403 {
			t.Fatalf("B accessed %s: %d", path, status)
		}
	}
	if _, err := a.Payment.CreateInstance(ctx, child.ID, payment.InstanceInput{Adapter: payment.AdapterStripe}); !errors.Is(err, payment.ErrCollectorRequired) {
		t.Fatalf("B merchant creation allowed: %v", err)
	}
	instances, err := a.Payment.ListInstances(ctx, identity.OEMChannelID, payment.AdapterStripe)
	if err != nil || len(instances) == 0 {
		t.Fatalf("missing OEM fixture: %v", err)
	}
	pk := "pk_test_" + marker
	if _, err := a.Payment.PatchInstance(ctx, identity.OEMChannelID, instances[0].ID, payment.InstanceInput{Credentials: map[string]string{"publishable_key": pk}}); err != nil {
		t.Fatal(err)
	}
	online, err := a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: cUser, ChannelOrgID: child.ID, Adapter: payment.AdapterStripe, PayMajor: 10})
	if err != nil {
		t.Fatal(err)
	}
	checkout, err := a.Payment.Checkout(ctx, online, "https://oem.example.test")
	if err != nil || checkout.PublishableKey != pk || online.PayeeChannelOrgID != identity.OEMChannelID {
		t.Fatalf("OEM child used a foreign merchant: %+v %v", checkout, err)
	}
	disabled := false
	for _, inst := range instances {
		if _, err := a.Payment.PatchInstance(ctx, identity.OEMChannelID, inst.ID, payment.InstanceInput{Enabled: &disabled}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: cUser, ChannelOrgID: child.ID, Adapter: payment.AdapterStripe, PayMajor: 10}); !errors.Is(err, payment.ErrMethodUnavailable) {
		t.Fatalf("unconfigured OEM fell back to platform: %v", err)
	}
	enabled := true
	for _, inst := range instances {
		if _, err := a.Payment.PatchInstance(ctx, identity.OEMChannelID, inst.ID, payment.InstanceInput{Enabled: &enabled}); err != nil {
			t.Fatal(err)
		}
	}
	for user, owner := range map[string]string{bUser: identity.OfficialChannelID, cUser: identity.OEMChannelID} {
		channel := identity.ResellerChannelID
		if user == cUser {
			channel = child.ID
		}
		order, err := a.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: user, ChannelOrgID: channel, Adapter: payment.AdapterManual, AmountMinor: 1000000})
		if err != nil || order.PayeeChannelOrgID != owner || order.ChannelOrgID != channel {
			t.Fatalf("order attribution/payee: %+v %v", order, err)
		}
	}
	// Customer search is scoped before limiting results, including OEM B customers.
	for token, ids := range map[string][2]string{cfg.BootstrapAdmin: {bUser, cUser}, oem: {cUser, bUser}} {
		path := "/admin/payments/recipients"
		if token == oem {
			path = "/channel/payments/recipients"
		}
		body := getAuthJSON(t, server.URL+path+"?q="+marker, token)
		items, ok := body["items"].([]any)
		if !ok {
			t.Fatalf("recipient search %s: %+v", path, body)
		}
		found := false
		for _, raw := range items {
			uid := raw.(map[string]any)["id"]
			if uid == ids[1] {
				t.Fatal("foreign customer exposed")
			}
			if uid == ids[0] {
				found = true
			}
		}
		if !found {
			t.Fatal("own brand customer missing")
		}
	}
	before, _ := a.Billing.Balance(ctx, cUser, child.ID)
	poolBefore, _ := a.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	in := payment.OfflineReceiptInput{OperationID: marker + "-receipt-op", OccurredAt: time.Now().UTC(), ExpectedIssueRatioBPS: billing.DefaultIssueRatioBPS, UserID: cUser, AmountMinor: 10000, CreditMinor: 2000000, Currency: "CNY", Reference: marker + "-wire"}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var receipts []*payment.OrderView
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, err := a.Payment.RecordOfflineReceipt(ctx, identity.OEMChannelID, in, a.Audit, audit.RecordInput{ActorUserID: "finance"})
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			receipts = append(receipts, receipt)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(receipts) != 4 {
		t.Fatal("receipt creation failed")
	}
	for _, receipt := range receipts {
		if receipt.ID != receipts[0].ID || receipt.FulfilledAt == nil || receipt.ChannelOrgID != child.ID {
			t.Fatalf("duplicate or wrong attribution: %+v", receipt)
		}
	}
	paid := receipts[0]
	var count int64
	if err := a.DB.Table("audit_logs").Where("action = ? AND resource_id = ?", "payment.offline.allocate", paid.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit count %d %v", count, err)
	}
	after, _ := a.Billing.Balance(ctx, cUser, child.ID)
	poolAfter, _ := a.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	if after.AvailableMinor != before.AvailableMinor+in.CreditMinor || poolAfter.AvailableMinor != poolBefore.AvailableMinor-in.CreditMinor {
		t.Fatalf("wallet/pool mismatch: %+v %+v", after, poolAfter)
	}
	var alloc struct {
		PoolChannelOrgID string
		ChannelOrgID     string
		ExpiresAt        *time.Time
	}
	if err := a.DB.Table("billing_quota_allocations").Where("source_id = ?", paid.ReferenceID).First(&alloc).Error; err != nil || alloc.PoolChannelOrgID != identity.OEMChannelID || alloc.ChannelOrgID != child.ID || alloc.ExpiresAt != nil {
		t.Fatalf("allocation snapshot: %+v %v", alloc, err)
	}
	// Already issued customer credits remain usable when no unallocated stock remains.
	if _, err := a.Billing.GrantChannelQuota(ctx, identity.OEMChannelID, -poolAfter.AvailableMinor, "finance"); err != nil {
		t.Fatal(err)
	}
	reservationID := marker + "-empty-pool"
	_, reserveErr := a.Billing.Reserve(ctx, billing.ReserveInput{UserID: cUser, ChannelOrgID: child.ID, RequestID: reservationID, PublicModelID: catalog.EchoModelID, ReserveMinor: 100, UnitPrices: []byte(`{"input":"0.000001","output":"0.000002"}`)})
	if reserveErr == nil {
		if err := a.Billing.Release(ctx, reservationID); err != nil {
			t.Fatal(err)
		}
	}
	_, restoreErr := a.Billing.GrantChannelQuota(ctx, identity.OEMChannelID, poolAfter.AvailableMinor, "finance")
	if reserveErr != nil || restoreErr != nil {
		t.Fatalf("issued credit blocked by empty pool: %v %v", reserveErr, restoreErr)
	}
	in.CreditMinor++
	if _, err := a.Payment.RecordOfflineReceipt(ctx, identity.OEMChannelID, in, a.Audit, audit.RecordInput{ActorUserID: "finance"}); !errors.Is(err, payment.ErrReceiptConflict) {
		t.Fatalf("receipt conflict: %v", err)
	}
	in.CreditMinor--
	for token, path := range map[string]string{cfg.BootstrapAdmin: "/admin/payments/", "brandpay_b": "/channel/payments/orders/"} {
		if status, _ := doJSON(t, http.MethodPost, server.URL+path+paid.ID+"/refund", token, true, map[string]any{}); status != 403 {
			t.Fatalf("foreign refund accepted: %d", status)
		}
	}
	if status, _ := doJSON(t, http.MethodPost, server.URL+"/channel/payments/offline", oem, true, map[string]any{"operation_id": marker + "-foreign-op", "occurred_at": time.Now().UTC(), "expected_issue_ratio_bps": billing.DefaultIssueRatioBPS, "user_id": bUser, "amount_minor": 10000, "credit_minor": 1000000, "currency": "CNY", "reference": marker + "-foreign"}); status != 403 {
		t.Fatalf("cross-brand allocation accepted: %d", status)
	}
	// Reject audit insertion after wallet/stock writes; the transaction must roll back.
	originalReceipt := in
	in.OperationID = marker + "-rollback-op"
	in.Reference = marker + "-rollback"
	callback := marker + "-audit-failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("audit unavailable"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err = a.Payment.RecordOfflineReceipt(ctx, identity.OEMChannelID, in, a.Audit, audit.RecordInput{ActorUserID: "finance"})
	a.DB.Callback().Create().Remove(callback)
	if err == nil {
		t.Fatal("audit failure accepted")
	}
	rolled, _ := a.Billing.Balance(ctx, cUser, child.ID)
	if rolled.AvailableMinor != after.AvailableMinor {
		t.Fatal("partial wallet write survived audit failure")
	}
	a.DB.Table("payment_orders").Where("receipt_reference = ?", in.Reference).Count(&count)
	if count != 0 {
		t.Fatal("partial receipt survived")
	}
	if status, body := doJSON(t, http.MethodPost, server.URL+"/channel/payments/orders/"+paid.ID+"/refund", oem, true, map[string]any{"occurred_at": time.Now().UTC()}); status != 200 {
		t.Fatalf("own brand refund: %d %+v", status, body)
	}
	refundedPool, _ := a.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	if refundedPool.AvailableMinor != poolBefore.AvailableMinor {
		t.Fatal("refund did not return to original OEM pool")
	}
	refundedWallet, _ := a.Billing.Balance(ctx, cUser, child.ID)
	if refundedWallet.AvailableMinor != before.AvailableMinor {
		t.Fatal("refund did not reverse credits")
	}
	in = originalReceipt
	if replay, err := a.Payment.RecordOfflineReceipt(ctx, identity.OEMChannelID, in, a.Audit, audit.RecordInput{ActorUserID: "finance"}); err != nil || replay.ID != paid.ID || replay.Status != payment.StatusRefunded {
		t.Fatalf("refunded operation must return original current facts: %+v %v", replay, err)
	}
	// Platform B receives permanent credit without drawing from a B quota pool.
	if _, err := a.Payment.RecordOfflineReceipt(ctx, identity.OfficialChannelID, payment.OfflineReceiptInput{OperationID: marker + "-platform-op", OccurredAt: time.Now().UTC(), ExpectedIssueRatioBPS: billing.DefaultIssueRatioBPS, UserID: bUser, AmountMinor: 10000, CreditMinor: billing.MinorPerUSD, Currency: "CNY", Reference: marker + "-platform"}, a.Audit, audit.RecordInput{ActorUserID: "finance"}); err != nil {
		t.Fatal(err)
	}
	// OEM settlement and payout can only affect its own brand, including direct B customers.
	role, err := a.Identity.CreateAcquisitionRole(ctx, child.ID, identity.AcqPromoter, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Identity.BindRoleMember(ctx, cUser, role.ID); err != nil {
		t.Fatal(err)
	}
	cUsage, bUsage := marker+"-c-commission", marker+"-b-commission"
	for usage, input := range map[string]commission.AccrueInput{
		cUsage: {ChannelOrgID: child.ID, PolicyChannelID: identity.OEMChannelID, RoleID: role.ID},
		bUsage: {ChannelOrgID: identity.ResellerChannelID, RoleID: identity.KOL2BRoleID},
	} {
		input.UsageEventID, input.WholesaleMinor, input.CanCommission = usage, 2*billing.MinorPerUSD, true
		if _, err := a.Commission.Accrue(ctx, input); err != nil {
			t.Fatal(err)
		}
		if err := a.Commission.ForceAvailableAt(ctx, usage, time.Now().Add(-time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	postJSONRaw(t, server.URL+"/channel/commissions/unfreeze", oem, map[string]any{})
	bEntries, err := a.Commission.ListEntries(ctx, identity.ResellerChannelID, nil, bUsage)
	if err != nil || len(bEntries) == 0 || bEntries[0].Status != commission.StatusFrozen {
		t.Fatalf("OEM unfroze platform commission: %+v %v", bEntries, err)
	}
	settlements := postJSONRaw(t, server.URL+"/channel/commissions/settle?ignore_minimum=1", oem, map[string]any{})
	sid := ""
	for _, raw := range settlements["items"].([]any) {
		item := raw.(map[string]any)
		if item["channel_org_id"] == child.ID {
			sid = item["id"].(string)
		}
		if item["channel_org_id"] == identity.ResellerChannelID {
			t.Fatal("OEM settled a platform commission")
		}
	}
	if sid == "" {
		t.Fatal("missing OEM child settlement")
	}
	if hasPlan(getAuthJSON(t, server.URL+"/admin/settlements", cfg.BootstrapAdmin), sid) {
		t.Fatal("OEM settlement leaked to platform payment list")
	}
	if status, _ := doJSON(t, http.MethodPost, server.URL+"/admin/settlements/"+sid+"/payout", cfg.BootstrapAdmin, true, map[string]any{"method": "manual", "reference": marker + "-wrong"}); status != 404 {
		t.Fatalf("platform recorded OEM payout: %d", status)
	}
	postJSONRaw(t, server.URL+"/channel/settlements/"+sid+"/payout", oem, map[string]any{"method": "manual", "reference": marker + "-payout"})
	var payout struct {
		ActorUserID string
		CreatedAt   time.Time
		Reference   string
	}
	if err := a.DB.Table("commission_payouts").Where("settlement_id = ?", sid).First(&payout).Error; err != nil || payout.ActorUserID == "" || payout.CreatedAt.IsZero() || payout.Reference != marker+"-payout" {
		t.Fatalf("missing payout audit fields: %+v %v", payout, err)
	}
}
