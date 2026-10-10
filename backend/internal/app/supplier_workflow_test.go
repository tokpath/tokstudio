package app_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestSupplierWorkflowActualFactsAtomicReplayAndScope(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	admin := a.Config.BootstrapAdmin
	originalBalance, err := a.Billing.Balance(ctx, fx.userID, "")
	if err != nil {
		t.Fatal(err)
	}
	in := billing.SupplierInput{AmountMinor: 200000, Currency: "USD", SourceType: "provider_invoice", VendorName: "Test vendor", Memo: "same note", OccurredAt: time.Now().UTC().Add(-time.Minute), Confirmed: true, IdempotencyKey: id.New("supplier")}
	target := fx.server.URL + "/admin/supplier-entries"
	callback := "t07_supplier_audit_failure"
	if err := a.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("injected audit error"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	code, _ := doJSON(t, http.MethodPost, target, admin, true, in)
	a.DB.Callback().Create().Remove(callback)
	if code != 500 {
		t.Fatalf("audit failure misclassified %d", code)
	}
	var count int64
	if err := a.DB.Table("billing_supplier_entries").Where("idempotency_key=?", in.IdempotencyKey).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("fact survived rollback %d %v", count, err)
	}
	var wg sync.WaitGroup
	ids := make(chan string, 3)
	for n := 0; n < 3; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := doJSON(t, http.MethodPost, target, admin, true, in)
			if code != 201 {
				t.Errorf("concurrent supplier %d %+v", code, body)
				return
			}
			ids <- body["item"].(map[string]any)["id"].(string)
		}()
	}
	wg.Wait()
	close(ids)
	originalID := ""
	for actual := range ids {
		if originalID != "" && actual != originalID {
			t.Fatal("duplicated original supplier payment")
		}
		originalID = actual
	}
	if originalID == "" {
		t.Fatal("missing record")
	}
	changed := in
	changed.AmountMinor++
	if code, _ := doJSON(t, http.MethodPost, target, admin, true, changed); code != 409 {
		t.Fatalf("original amount mutable %d", code)
	}
	distinct := in
	distinct.IdempotencyKey = id.New("supplier")
	if code, _ := doJSON(t, http.MethodPost, target, admin, true, distinct); code != 201 {
		t.Fatalf("same note treated as reference %d", code)
	}
	reference := in
	reference.IdempotencyKey = id.New("supplier")
	reference.BankRef = id.New("BANK")
	if code, _ := doJSON(t, http.MethodPost, target, admin, true, reference); code != 201 {
		t.Fatalf("actual reference %d", code)
	}
	reference.IdempotencyKey = id.New("supplier")
	if code, _ := doJSON(t, http.MethodPost, target, admin, true, reference); code != 409 {
		t.Fatalf("same actual payment counted twice %d", code)
	}
	if code, _ := doJSON(t, http.MethodPost, fx.server.URL+"/channel/supplier-entries", admin+"-c", true, reference); code != 201 {
		t.Fatalf("reference from another book falsely blocked %d", code)
	}
	if code, _ := doJSON(t, http.MethodGet, target+"?channel_id="+identity.OEMChannelID, admin, false, nil); code != 403 {
		t.Fatalf("cross-brand list %d", code)
	}
	lookup := target + "?operation_id=" + in.IdempotencyKey
	if code, body := doJSON(t, http.MethodGet, lookup, admin, false, nil); code != 200 || body["operation_status"] != "recorded" || body["item"].(map[string]any)["id"] != originalID {
		t.Fatalf("original recovery %d %+v", code, body)
	}
	if code, body := doJSON(t, http.MethodGet, fx.server.URL+"/channel/supplier-entries?operation_id="+in.IdempotencyKey, admin+"-c", false, nil); code != 200 || body["operation_status"] != "not_found" {
		t.Fatalf("cross-brand operation %d %+v", code, body)
	}
	reverse := map[string]any{"operation_id": id.New("supplier_reverse"), "reason": "duplicate entry correction"}
	reverseURL := target + "/" + originalID + "/reverse"
	reversalIDs := make(chan string, 3)
	for n := 0; n < 3; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := doJSON(t, http.MethodPost, reverseURL, admin, true, reverse)
			if code != 200 {
				t.Errorf("concurrent reversal %d %+v", code, body)
				return
			}
			reversalIDs <- body["item"].(map[string]any)["id"].(string)
		}()
	}
	wg.Wait()
	close(reversalIDs)
	reversalID := ""
	for actual := range reversalIDs {
		if reversalID != "" && actual != reversalID {
			t.Fatal("duplicated reversal")
		}
		reversalID = actual
	}
	reverse["reason"] = "changed reason"
	if code, _ := doJSON(t, http.MethodPost, reverseURL, admin, true, reverse); code != 409 {
		t.Fatalf("original reversal reason mutable %d", code)
	}
	reverse["operation_id"] = id.New("supplier_reverse")
	if code, _ := doJSON(t, http.MethodPost, reverseURL, admin, true, reverse); code != 409 {
		t.Fatalf("original reversed again %d", code)
	}
	after, err := a.Billing.Balance(ctx, fx.userID, "")
	if err != nil || after.AvailableMinor != originalBalance.AvailableMinor || after.GiftMinor != originalBalance.GiftMinor || after.CommissionAvailableMinor != originalBalance.CommissionAvailableMinor {
		t.Fatalf("supplier cash changed wallets %+v %v", after, err)
	}
}

func TestSupplierWorkflowFullPaginationAndBrandPnL(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	prefix := id.New("t07supplier")
	for n := 0; n < 153; n++ {
		if _, err := a.Billing.RecordSupplier(ctx, "test_actor", billing.SupplierInput{ChannelOrgID: identity.OfficialChannelID, AmountMinor: 100, Currency: "USD", SourceType: "provider_invoice", VendorName: fmt.Sprintf("%s_%03d", prefix, n), OccurredAt: time.Now().Add(-time.Minute), IdempotencyKey: fmt.Sprintf("%s_%03d", prefix, n)}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/supplier-entries?q="+prefix+"&limit=40&cursor="+url.QueryEscape(cursor), a.Config.BootstrapAdmin+"-audit", false, nil)
		if code != 200 || asInt(body["total"]) != 153 {
			t.Fatalf("complete supplier list %d %+v", code, body)
		}
		for _, raw := range body["items"].([]any) {
			key := raw.(map[string]any)["id"].(string)
			if seen[key] {
				t.Fatal("repeated supplier page")
			}
			seen[key] = true
		}
		cursor = body["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 153 {
		t.Fatalf("truncated supplier facts %d", len(seen))
	}
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/supplier-entries?q="+prefix+"_000", a.Config.BootstrapAdmin, false, nil)
	if code != 200 || len(body["items"].([]any)) != 1 {
		t.Fatalf("oldest supplier unsearchable %d %+v", code, body)
	}
	// Source-pool movements do not substitute for actual net API consumption.
	channels, err := a.Identity.BrandChannelIDs(ctx, identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	expected, _, err := a.Billing.BrandReport(ctx, channels)
	if err != nil {
		t.Fatal(err)
	}
	code, body = doJSON(t, http.MethodGet, fx.server.URL+"/admin/channels/"+identity.OfficialChannelID+"/pnl", a.Config.BootstrapAdmin, false, nil)
	if code != 200 {
		t.Fatalf("brand PnL %d %+v", code, body)
	}
	pnl := body["pnl"].(map[string]any)
	if asInt(pnl["consumed_minor"]) != expected.RevenueMinor || asInt(pnl["pnl_minor"]) != asInt(pnl["margin_minor"])+asInt(pnl["marketing_minor"]) {
		t.Fatalf("not full net actual consumption %+v expected %+v", pnl, expected)
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/admin/channels/"+identity.OEMChannelID+"/pnl", a.Config.BootstrapAdmin, false, nil); code != 403 {
		t.Fatalf("foreign financial book read %d", code)
	}
}

func TestSupplierWorkflowAllocationBookAndPnLReadErrors(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	prefix := id.New("allocationpage")
	for n := 0; n < 153; n++ {
		if err := a.DB.Exec("INSERT INTO billing_quota_allocations(id,user_id,channel_org_id,pool_channel_org_id,source_type,source_id,granted_minor,consumed_minor,status,created_at) VALUES (?,?,?,?,'payment',?,100,1,'active',?)", fmt.Sprintf("%s_%03d", prefix, n), fx.userID, identity.OEMChannelID, identity.OEMChannelID, fmt.Sprintf("%s_%03d", prefix, n), time.Now().UTC().Add(time.Duration(n)*time.Second)).Error; err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		code, body := doJSON(t, http.MethodGet, fx.server.URL+"/channel/allocations?q="+prefix+"&limit=40&cursor="+url.QueryEscape(cursor), a.Config.BootstrapAdmin+"-c", false, nil)
		if code != 200 || asInt(body["total"]) != 153 {
			t.Fatalf("allocation full book %d %+v", code, body)
		}
		for _, raw := range body["items"].([]any) {
			key := raw.(map[string]any)["id"].(string)
			if seen[key] {
				t.Fatal("repeated allocation")
			}
			seen[key] = true
		}
		cursor = body["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 153 {
		t.Fatalf("allocations truncated %d", len(seen))
	}
	if code, _ := doJSON(t, http.MethodGet, fx.server.URL+"/channel/allocations?channel_id="+identity.OfficialChannelID, a.Config.BootstrapAdmin+"-c", false, nil); code != 403 {
		t.Fatalf("foreign allocation scope %d", code)
	}
	// A database read failure must be reported, not represented as a zero pool or revenue.
	callback := "t07_pnl_read_failure"
	if err := a.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_quota_accounts" {
			tx.AddError(errors.New("injected pool read error"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/admin/channels/"+identity.OfficialChannelID+"/pnl", a.Config.BootstrapAdmin, false, nil)
	a.DB.Callback().Query().Remove(callback)
	if code != 500 || body["pnl"] != nil {
		t.Fatalf("pool read failure became successful zero %d %+v", code, body)
	}
}

func TestSupplierWorkflowPnLUsesNetBrandConsumption(t *testing.T) {
	fx := newWMeterEnv(t)
	a := fx.app
	ctx := context.Background()
	marker := id.New("pnlbrand")
	child, err := a.Identity.CreateChannel(ctx, identity.Principal{Roles: []string{"channel_admin"}, ChannelOrgID: identity.OEMChannelID}, identity.ChannelInput{Code: marker, Type: "B"})
	if err != nil {
		t.Fatal(err)
	}
	beforeCode, beforeBody := doJSON(t, http.MethodGet, fx.server.URL+"/channel/pnl", a.Config.BootstrapAdmin+"-c", false, nil)
	if beforeCode != 200 {
		t.Fatalf("baseline pnl %d %+v", beforeCode, beforeBody)
	}
	before := beforeBody["pnl"].(map[string]any)
	for n, fact := range []struct {
		channel, state string
		amount         int64
	}{{identity.OEMChannelID, "confirmed", 1000000}, {child.ID, "confirmed", 3000000}, {child.ID, "voided", 2000000}, {child.ID, "pending_reconciliation", 5000000}} {
		key := fmt.Sprintf("%s_%d", marker, n)
		if err := a.DB.Exec("INSERT INTO billing_usage_events(id,request_id,user_id,channel_org_id,public_model_id,unit_usage_json,unit_prices_json,customer_amount_minor,upstream_cost_minor,wholesale_amount_minor,state,idempotency_key) VALUES (?,?,?,?,'echo','{}','{}',?,0,0,?,?)", key, key, fx.userID, fact.channel, fact.amount, fact.state, key).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { a.DB.Table("billing_usage_events").Where("id LIKE ?", marker+"_%").Delete(nil) })
	code, body := doJSON(t, http.MethodGet, fx.server.URL+"/channel/pnl", a.Config.BootstrapAdmin+"-c", false, nil)
	if code != 200 {
		t.Fatalf("brand pnl %d %+v", code, body)
	}
	actual := body["pnl"].(map[string]any)
	if asInt(actual["consumed_minor"])-asInt(before["consumed_minor"]) != 4000000 || asInt(actual["pnl_minor"])-asInt(before["pnl_minor"]) != 4000000 {
		t.Fatalf("not complete net C+B actual consumption before=%+v after=%+v", before, actual)
	}
	if asInt(actual["recharge_minor"]) != asInt(before["recharge_minor"]) || asInt(actual["unconsumed_minor"]) != asInt(before["unconsumed_minor"]) {
		t.Fatal("API consumption substituted for pool issuance")
	}
	if _, ok := actual["attempt_cost_minor"]; ok {
		t.Fatal("wholesale was misrepresented as actual supplier attempt cost")
	}
}
