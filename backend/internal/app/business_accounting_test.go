package app_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
)

func TestBusinessOEMPurchaseAndSeparateProfit(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	marker := id.New("business")
	owner := identity.OEMChannelID
	platformBefore, err := a.Billing.PlatformReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := a.Identity.BrandChannelIDs(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	oemBefore, _, err := a.Billing.BrandReport(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	quotaBefore, err := a.Billing.ChannelQuota(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	input := billing.OEMPurchaseInput{OperationID: marker, OEMChannelOrgID: owner, CashAmountMinor: 1000 * billing.MinorPerUSD, CashCurrency: "USD", SaleAmountMinor: 1000 * billing.MinorPerUSD, QuotaAmountMinor: 900 * billing.MinorPerUSD, OccurredAt: time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond), Confirmed: true, Note: "private platform memo"}
	var wg sync.WaitGroup
	results := make(chan *billing.OEMPurchaseView, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := a.Billing.CompleteOEMPurchase(ctx, "purchase-operator", input)
			if e != nil {
				t.Error(e)
				return
			}
			results <- v
		}()
	}
	wg.Wait()
	close(results)
	original := ""
	for v := range results {
		if original == "" {
			original = v.ID
		}
		if v.ID != original || v.Quota.AvailableMinor != quotaBefore.AvailableMinor+input.QuotaAmountMinor {
			t.Fatalf("not atomic original sale/delivery %+v", v)
		}
	}
	if original == "" {
		t.Fatal("no completed purchase")
	}
	after, err := a.Billing.ChannelQuota(ctx, owner)
	if err != nil || after.AvailableMinor != quotaBefore.AvailableMinor+input.QuotaAmountMinor {
		t.Fatal("quota credited twice")
	}
	platformAfter, err := a.Billing.PlatformReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if platformAfter.RevenueMinor-platformBefore.RevenueMinor != input.SaleAmountMinor || platformAfter.OEMSalesMinor-platformBefore.OEMSalesMinor != input.SaleAmountMinor {
		t.Fatal("completed sale waits for OEM consumption or uses quota face value")
	}
	oemAfter, _, err := a.Billing.BrandReport(ctx, ids)
	if err != nil || *oemBefore != *oemAfter {
		t.Fatal("prepayment became OEM API cost")
	}
	changed := input
	changed.SaleAmountMinor++
	if _, err := a.Billing.CompleteOEMPurchase(ctx, "purchase-operator", changed); err == nil {
		t.Fatal("changed sale value accepted")
	}
	changed = input
	changed.OperationID = marker + "-wrong-owner"
	changed.OEMChannelOrgID = identity.ResellerChannelID
	if _, err := a.Billing.CompleteOEMPurchase(ctx, "purchase-operator", changed); err == nil {
		t.Fatal("channel got a purchase pool")
	}
	// An outer audit/storage failure must roll back both the sale and quota.
	rollback := errors.New("simulate audit failure")
	changed = input
	changed.OperationID = marker + "-rollback"
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if _, e := a.Billing.CompleteOEMPurchaseTx(tx, "purchase-operator", changed); e != nil {
			return e
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if _, err := a.Billing.OEMPurchaseOperation(ctx, "purchase-operator", owner, changed.OperationID); !errors.Is(err, billing.ErrNotFound) {
		t.Fatal("rolled-back sale remained")
	}
	afterRollback, err := a.Billing.ChannelQuota(ctx, owner)
	if err != nil || afterRollback.AvailableMinor != after.AvailableMinor {
		t.Fatal("audit failure left credited stock")
	}
	// A grant without payment is not a sale.
	if _, err := a.Billing.AdjustQuota(ctx, "purchase-operator", owner, marker+"-gift", billing.MinorPerUSD); err != nil {
		t.Fatal(err)
	}
	noSale, err := a.Billing.PlatformReport(ctx)
	if err != nil || noSale.RevenueMinor != platformAfter.RevenueMinor {
		t.Fatal("administrative grant fabricated revenue")
	}
	// Keep actual upstream cost after a customer reversal. OEM retail repricing
	// changes only OEM revenue; its platform settlement and supplier cost stay fixed.
	request := marker + "-request"
	if err := a.DB.Exec(`INSERT INTO billing_usage_events(id,request_id,user_id,channel_org_id,public_model_id,unit_usage_json,unit_prices_json,customer_amount_minor,upstream_cost_minor,wholesale_amount_minor,state,idempotency_key) VALUES (?,?,?,?,'business-model','{}','{}',?,?,?,'confirmed',?)`, request, request, fx.userID, owner, 150*billing.MinorPerUSD, 80*billing.MinorPerUSD, 100*billing.MinorPerUSD, request).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO billing_customer_charges(id,request_id,usage_event_id,amount_minor,status) VALUES (?,?,?,?,'committed')`, request, request, request, 150*billing.MinorPerUSD).Error; err != nil {
		t.Fatal(err)
	}
	// Reconciliation can replace a nonzero pending measurement with the actual
	// settled record. Its voided estimate must never become a second cost.
	if err := a.DB.Exec(`INSERT INTO billing_usage_events(id,request_id,user_id,channel_org_id,public_model_id,unit_usage_json,unit_prices_json,customer_amount_minor,upstream_cost_minor,wholesale_amount_minor,state,idempotency_key) VALUES (?,?,?,?,'business-model','{}','{}',?,0,?, 'pending_reconciliation',?)`, request+"-estimate", request, fx.userID, owner, 900*billing.MinorPerUSD, 900*billing.MinorPerUSD, request+"-estimate").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("billing_usage_events").Where("id=?", request+"-estimate").Update("state", "voided").Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO billing_cost_entries(id,request_id,attempt_id,provider_id,amount_minor,currency,unit_usage_json,unit_prices_json) VALUES (?,?,?,'test-provider',?,'USD','{}','{}')`, request, request, request, 80*billing.MinorPerUSD).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.DB.Table("billing_customer_charges").Where("request_id=?", request).Delete(nil)
		a.DB.Table("billing_cost_entries").Where("request_id=?", request).Delete(nil)
		a.DB.Table("billing_usage_events").Where("request_id=?", request).Delete(nil)
	})
	usage, _, err := a.Billing.BrandReport(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if usage.RevenueMinor-oemBefore.RevenueMinor != 150*billing.MinorPerUSD || usage.CostMinor-oemBefore.CostMinor != 100*billing.MinorPerUSD || usage.MarginMinor-oemBefore.MarginMinor != 50*billing.MinorPerUSD {
		t.Fatalf("OEM 150-100 != 50: %+v", usage)
	}
	platformUsage, err := a.Billing.PlatformReport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if platformUsage.RevenueMinor != platformAfter.RevenueMinor || platformUsage.UpstreamMinor-platformAfter.UpstreamMinor != 80*billing.MinorPerUSD {
		t.Fatal("OEM retail revenue leaked to platform or actual cost missing")
	}
	if err := a.DB.Table("billing_usage_events").Where("id=?", request).Update("customer_amount_minor", 120*billing.MinorPerUSD).Error; err != nil {
		t.Fatal(err)
	}
	reprice, err := a.Billing.PlatformReport(ctx)
	if err != nil || reprice.RevenueMinor != platformUsage.RevenueMinor || reprice.UpstreamMinor != platformUsage.UpstreamMinor {
		t.Fatal("OEM retail price changed platform business")
	}
	if err := a.DB.Table("billing_customer_charges").Where("id=?", request).Updates(map[string]any{"status": "reversed", "amount_minor": 120 * billing.MinorPerUSD}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("billing_usage_events").Where("id=?", request).Update("state", "voided").Error; err != nil {
		t.Fatal(err)
	}
	reversed, _, err := a.Billing.BrandReport(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	if reversed.RevenueMinor != oemBefore.RevenueMinor || reversed.CostMinor-oemBefore.CostMinor != 100*billing.MinorPerUSD {
		t.Fatal("customer refund erased already consumed OEM service cost")
	}
	preserved, err := a.Billing.PlatformReport(ctx)
	if err != nil || preserved.UpstreamMinor != platformUsage.UpstreamMinor || preserved.RevenueMinor != platformUsage.RevenueMinor {
		t.Fatal("customer refund implied supplier refund")
	}
	for _, dim := range []string{"model", "channel", "provider", "user", "api_key"} {
		if _, err := a.Billing.PlatformDimMoney(ctx, dim); err != nil {
			t.Fatalf("dimension %s: %v", dim, err)
		}
	}
	if _, err := a.Billing.PlatformDailySeries(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Reversing an incorrect registration has a separate immutable operation.
	// A failed audit rolls back both the reclaimed quota and sale reduction.
	reversal := billing.OEMPurchaseReversalInput{OperationID: marker + "-reverse", Reason: "wrong OEM selected"}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		if _, err := a.Billing.ReverseOEMPurchaseTx(tx, "purchase-operator", original, reversal); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	still, err := a.Billing.OEMPurchase(ctx, owner, original)
	if err != nil || still.Status != "completed" {
		t.Fatal("failed audit left reversed sale")
	}
	beforeReverseQuota, _ := a.Billing.ChannelQuota(ctx, owner)
	var reverseWG sync.WaitGroup
	for i := 0; i < 3; i++ {
		reverseWG.Add(1)
		go func() {
			defer reverseWG.Done()
			if err := a.DB.Transaction(func(tx *gorm.DB) error {
				_, err := a.Billing.ReverseOEMPurchaseTx(tx, "purchase-operator", original, reversal)
				return err
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	reverseWG.Wait()
	finalQuota, err := a.Billing.ChannelQuota(ctx, owner)
	if err != nil || finalQuota.AvailableMinor != beforeReverseQuota.AvailableMinor-input.QuotaAmountMinor {
		t.Fatal("reversal reclaimed quota repeatedly")
	}
	finalReport, err := a.Billing.PlatformReport(ctx)
	if err != nil || finalReport.OEMSalesMinor != platformBefore.OEMSalesMinor || finalReport.UpstreamMinor != platformUsage.UpstreamMinor {
		t.Fatal("incorrect registration was not reversed, or service cost was erased")
	}
	var reverseLedger int64
	a.DB.Table("billing_quota_ledger").Where("reference_type='oem_purchase_reversal' AND reference_id=?", original).Count(&reverseLedger)
	if reverseLedger != 1 {
		t.Fatal("missing original-record reverse ledger")
	}
	still, err = a.Billing.OEMPurchase(ctx, owner, original)
	if err != nil || still.CashAmountMinor != input.CashAmountMinor || still.Status != "reversed" || still.ReversalReason != reversal.Reason {
		t.Fatal("original cash fact was rewritten")
	}
	if _, err := a.Billing.PlatformDailySeries(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/billing/report", a.Config.BootstrapAdmin, false, nil)
	if code != 200 || body["report"].(map[string]any)["scope"] != "platform_business" {
		t.Fatalf("wrong report contract %d %+v", code, body)
	}
}

func TestBusinessPurchaseCashScopeAndExternalReplay(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	marker := id.New("business-cash")
	in := billing.OEMPurchaseInput{OperationID: marker, OEMChannelOrgID: identity.OEMChannelID, CashAmountMinor: 70000, CashCurrency: "CNY", SaleAmountMinor: 100 * billing.MinorPerUSD, QuotaAmountMinor: 100 * billing.MinorPerUSD, OccurredAt: time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond), Confirmed: true, ExternalReference: marker + "-bank", Note: "internal note"}
	code, body := doJSON(t, http.MethodPost, fx.server.URL+"/admin/oem-purchases", a.Config.BootstrapAdmin, true, in)
	if code != 200 {
		t.Fatalf("completed cash sale %d %+v", code, body)
	}
	v := body["item"].(map[string]any)
	if asInt(v["cash_amount_minor"]) != 70000 || asInt(v["sale_amount_minor"]) != 100*billing.MinorPerUSD {
		t.Fatal("cash currency was mixed with USD sales")
	}
	in.OperationID = marker + "-duplicate"
	if _, err := a.Billing.CompleteOEMPurchase(ctx, "different-actor", in); err == nil {
		t.Fatal("external transaction recorded twice")
	}
	code, duplicate := doJSON(t, http.MethodPost, fx.server.URL+"/admin/oem-purchases", a.Config.BootstrapAdmin, true, in)
	if code != 409 || duplicate["error"].(map[string]any)["param"].(map[string]any)["item"].(map[string]any)["id"] != v["id"] {
		t.Fatal("duplicate transaction cannot locate its original record")
	}

	if code, _ := doJSON(t, http.MethodPost, fx.server.URL+"/admin/oem-purchases", a.Config.BootstrapAdmin+"-audit", true, in); code != 403 {
		t.Fatal("audit wrote procurement")
	}
	code, own := doJSON(t, http.MethodGet, fx.server.URL+"/channel/oem-purchases", a.Config.BootstrapAdmin+"-c", false, nil)
	if code != 200 {
		t.Fatalf("OEM cannot read purchase %d %+v", code, own)
	}
	for _, raw := range own["items"].([]any) {
		row := raw.(map[string]any)
		if row["oem_channel_org_id"] != identity.OEMChannelID || row["note"] != nil || row["actor_user_id"] != "" {
			t.Fatalf("OEM internal/cross-brand leak %+v", row)
		}
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/oem-purchases", a.Config.BootstrapChannel, false, nil); code != 403 {
		t.Fatal("channel accessed procurement")
	}
	// A foreign cursor must not reveal another OEM's transaction boundary.
	brand, err := a.Identity.CreateBrand(ctx, identity.Principal{Roles: []string{"platform_admin"}}, identity.BrandInput{Name: marker, PrimaryDomain: strings.ReplaceAll(marker, "_", "-") + ".test", APIDomain: "api." + strings.ReplaceAll(marker, "_", "-") + ".test", AdminDomain: "admin." + strings.ReplaceAll(marker, "_", "-") + ".test"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := a.Identity.CreateChannel(ctx, identity.Principal{Roles: []string{"platform_admin"}}, identity.ChannelInput{Type: "C", Code: marker, BrandID: brand.ID})
	if err != nil {
		t.Fatal(err)
	}
	in.OperationID = marker + "-foreign"
	in.OEMChannelOrgID = foreign.ID
	in.ExternalReference = ""
	other, err := a.Billing.CompleteOEMPurchase(ctx, "purchase-operator", in)
	if err != nil {
		t.Fatal(err)
	}
	// A purchase whose stock is no longer available cannot be silently undone.
	if _, err := a.Billing.AdjustQuota(ctx, "purchase-operator", foreign.ID, marker+"-issued", -in.QuotaAmountMinor); err != nil {
		t.Fatal(err)
	}
	reverse := billing.OEMPurchaseReversalInput{OperationID: marker + "-blocked-reverse", Reason: "mistaken amount"}
	if err := a.DB.Transaction(func(tx *gorm.DB) error {
		_, err := a.Billing.ReverseOEMPurchaseTx(tx, "purchase-operator", other.ID, reverse)
		return err
	}); !errors.Is(err, billing.ErrInsufficientQuota) {
		t.Fatalf("unrecoverable quota reversal: %v", err)
	}
	unchanged, err := a.Billing.OEMPurchase(ctx, foreign.ID, other.ID)
	if err != nil || unchanged.Status != "completed" {
		t.Fatal("blocked reversal reduced sale")
	}
	for _, token := range []string{a.Config.BootstrapAdmin + "-audit", a.Config.BootstrapAdmin + "-c"} {
		if status, _ := doJSON(t, http.MethodPost, fx.server.URL+"/admin/oem-purchases/"+other.ID+"/reverse", token, true, reverse); status != 403 {
			t.Fatal("read-only/OEM reversed platform sale")
		}
	}
	if _, err := a.Billing.ListOEMPurchases(ctx, identity.OEMChannelID, other.ID, 30); !errors.Is(err, billing.ErrNotFound) {
		t.Fatal("foreign cursor was accepted")
	}
	// Both entries share a completed purchase and the system quota ledger.
	var count int64
	if err := a.DB.Table("billing_quota_ledger").Where("reference_type='oem_purchase' AND reference_id=?", v["id"]).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("missing original credit delivery %d %v", count, err)
	}
}
