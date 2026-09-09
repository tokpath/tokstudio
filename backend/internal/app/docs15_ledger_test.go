package app_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestChannelSupplierPnLAndSignupGift(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "docs15_admin"
	cfg.BootstrapUser = "docs15_user"
	cfg.BootstrapChannel = "docs15_channel"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	if mustStatusJSON(t, http.MethodPost, server.URL+"/admin/supplier-entries", "docs15_admin", map[string]string{
		"amount_minor": "1", "source_type": "provider_invoice", "idempotency_key": "no-confirm",
	}) != http.StatusConflict {
		t.Fatal("supplier post without confirm must 409")
	}

	idem := "spe-docs15-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	created := postJSONRaw(t, server.URL+"/admin/supplier-entries", "docs15_admin", map[string]any{
		"amount_minor": 2 * billing.MinorPerUSD,
		"source_type":  "provider_invoice", "idempotency_key": idem,
		"vendor_name": "Echo Labs", "invoice_no": "INV-1", "payment_method": "wire", "memo": "offline",
	})
	item := created["item"].(map[string]any)
	if asInt(item["amount_minor"]) != -2*billing.MinorPerUSD {
		t.Fatalf("supplier stored amount must be negative: %+v", item)
	}
	if item["channel_org_id"] != identity.OfficialChannelID {
		t.Fatalf("admin supplier must auto-book official channel: %+v", item)
	}
	again := postJSONRaw(t, server.URL+"/admin/supplier-entries", "docs15_admin", map[string]any{
		"channel_org_id": identity.ResellerChannelID, "amount_minor": 2 * billing.MinorPerUSD,
		"source_type": "provider_invoice", "idempotency_key": idem,
	})
	if again["item"].(map[string]any)["id"] != item["id"] {
		t.Fatalf("supplier idempotency: %+v", again)
	}

	bEntry := postJSONRaw(t, server.URL+"/channel/supplier-entries", "docs15_channel", map[string]any{
		"amount_minor": billing.MinorPerUSD, "source_type": "platform_recharge",
		"idempotency_key": "spe-b-" + strconv.FormatInt(time.Now().UnixNano(), 10),
	})
	if bEntry["item"].(map[string]any)["channel_org_id"] != identity.ResellerChannelID {
		t.Fatalf("B channel must auto-book self: %+v", bEntry)
	}

	pnl := getAuthJSON(t, server.URL+"/admin/channels/"+identity.OfficialChannelID+"/pnl", "docs15_admin")["pnl"].(map[string]any)
	if asInt(pnl["supplier_minor"]) > -2*billing.MinorPerUSD {
		t.Fatalf("supplier total missing: %+v", pnl)
	}
	if asInt(pnl["pnl_minor"]) > asInt(pnl["consumed_minor"]) {
		t.Fatalf("pnl should include negative supplier: %+v", pnl)
	}

	rev := postJSONRaw(t, server.URL+"/admin/supplier-entries/"+item["id"].(string)+"/reverse", "docs15_admin", map[string]any{"reason": "void"})
	if asInt(rev["item"].(map[string]any)["amount_minor"]) != 2*billing.MinorPerUSD {
		t.Fatalf("reverse must flip sign: %+v", rev)
	}

	bReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/channel/commission-policy", bytes.NewReader([]byte(
		`{"direct_bps":1000,"indirect_bps":500,"total_bps":2500,"freeze_days":7}`,
	)))
	bReq.Header.Set("Authorization", "Bearer docs15_channel")
	bReq.Header.Set("Content-Type", "application/json")
	bReq.Header.Set("X-Tokenhub-Confirm", "1")
	bResp, err := http.DefaultClient.Do(bReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = bResp.Body.Close()
	if bResp.StatusCode != http.StatusForbidden {
		t.Fatalf("B channel must not write commission policy, got %d", bResp.StatusCode)
	}

	patched := patchJSONRaw(t, server.URL+"/channel/commission-policy", "docs15_admin-c", map[string]any{
		"direct_bps": 1200, "indirect_bps": 400, "total_bps": 2500, "freeze_days": 7, "min_settle_minor": billing.MinorPerUSD, "version": "c-oem-e2e",
	})
	if asInt(patched["policy"].(map[string]any)["direct_bps"]) != 1200 {
		t.Fatalf("C policy patch: %+v", patched)
	}
	cPolicy := getAuthJSON(t, server.URL+"/channel/commission-policy", "docs15_admin-c")["policy"].(map[string]any)
	if asInt(cPolicy["direct_bps"]) != 1200 {
		t.Fatalf("C policy get: %+v", cPolicy)
	}
	plat := getAuthJSON(t, server.URL+"/admin/commission-policy", "docs15_admin")["policy"].(map[string]any)
	if asInt(plat["direct_bps"]) == 1200 && plat["version"] == "c-oem-e2e" {
		t.Fatalf("platform policy must stay independent: %+v", plat)
	}

	regA := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "gift-a-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.OfficialPromotionCode,
	})
	balA := getAuthJSON(t, server.URL+"/v1/balance", tokenOf(regA))
	if asInt(balA["balance"].(map[string]any)["gift_minor"]) != 0 {
		t.Fatalf("THA1 must not grant signup gift: %+v", balA)
	}
	partner := getAuthJSON(t, server.URL+"/v1/partner/me", tokenOf(regA))
	roleID, _ := partner["role_id"].(string)
	if roleID == "" {
		t.Fatalf("referrer role missing: %+v", partner)
	}
	code := ""
	promos := getAuthJSON(t, server.URL+"/channel/promotion-codes?channel_id="+identity.OfficialChannelID, "docs15_admin")
	for _, raw := range promos["items"].([]any) {
		row := raw.(map[string]any)
		if row["acquisition_role_id"] == roleID {
			code, _ = row["code"].(string)
			break
		}
	}
	if code == "" {
		t.Fatalf("user promo missing: %+v", promos)
	}
	regB := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "gift-b-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": code,
	})
	balB := getAuthJSON(t, server.URL+"/v1/balance", tokenOf(regB))
	if asInt(balB["balance"].(map[string]any)["gift_minor"]) != identity.DefaultEligibilityGiftMinor {
		t.Fatalf("referred user should get gift: %+v", balB)
	}
	pnlGift := getAuthJSON(t, server.URL+"/admin/channels/"+identity.OfficialChannelID+"/pnl", "docs15_admin")["pnl"].(map[string]any)
	if asInt(pnlGift["marketing_issued_minor"]) > -identity.DefaultEligibilityGiftMinor {
		t.Fatalf("signup gift must post issued marketing: %+v", pnlGift)
	}

	regKOL := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "gift-kol-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	balKOL := getAuthJSON(t, server.URL+"/v1/balance", tokenOf(regKOL))
	if asInt(balKOL["balance"].(map[string]any)["gift_minor"]) != 0 {
		t.Fatalf("commission-qualified referrer must not grant gift: %+v", balKOL)
	}

	bGrant, _ := http.NewRequest(http.MethodPost, server.URL+"/channel/quotas/grant", bytes.NewReader([]byte(
		`{"channel_org_id":"`+identity.ResellerChannelID+`","amount_minor":1}`,
	)))
	bGrant.Header.Set("Authorization", "Bearer docs15_channel")
	bGrant.Header.Set("Content-Type", "application/json")
	bGrant.Header.Set("X-Tokenhub-Confirm", "1")
	bGrantResp, err := http.DefaultClient.Do(bGrant)
	if err != nil {
		t.Fatal(err)
	}
	_ = bGrantResp.Body.Close()
	if bGrantResp.StatusCode != http.StatusForbidden {
		t.Fatalf("B must not wholesale quota, got %d", bGrantResp.StatusCode)
	}
	childCode := "c-b-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	createdB := postJSONRaw(t, server.URL+"/admin/channels", "docs15_admin-c", map[string]any{
		"code": childCode, "type": "B",
	})
	childID := createdB["item"].(map[string]any)["id"].(string)
	if createdB["item"].(map[string]any)["parent_id"] != identity.OEMChannelID {
		t.Fatalf("C-created B must hang under C: %+v", createdB)
	}
	listed := getAuthJSON(t, server.URL+"/admin/channels", "docs15_admin-c")
	foundChild := false
	for _, raw := range listed["items"].([]any) {
		if raw.(map[string]any)["id"] == childID {
			foundChild = true
		}
	}
	if !foundChild {
		t.Fatalf("C should list child B: %+v", listed)
	}
	cBefore := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+identity.OEMChannelID, "docs15_admin-c")["quota"].(map[string]any)
	sold := 5 * billing.MinorPerUSD
	toB := postJSONRaw(t, server.URL+"/channel/quotas/grant", "docs15_admin-c", map[string]any{
		"channel_org_id": childID, "amount_minor": sold,
	})
	if asInt(toB["quota"].(map[string]any)["available_minor"]) != sold {
		t.Fatalf("child B pool should receive transfer: %+v", toB)
	}
	cAfter := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+identity.OEMChannelID, "docs15_admin-c")["quota"].(map[string]any)
	if asInt(cAfter["available_minor"]) != asInt(cBefore["available_minor"])-sold {
		t.Fatalf("C pool should debit wholesale: before=%v after=%v", cBefore, cAfter)
	}
}
