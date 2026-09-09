package app_test

import (
	"bytes"
	"context"
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

func TestCommissionEligibilityThresholds(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "elig_admin"
	cfg.BootstrapUser = "elig_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()
	restore := func() {
		_, _ = application.Identity.UpdatePlatformEligibility(ctx, identity.DefaultEligibilitySpendMinor, identity.DefaultEligibilityTopupMinor, identity.DefaultEligibilityGiftMinor)
	}
	restore()
	t.Cleanup(restore)

	rule := getAuthJSON(t, server.URL+"/admin/eligibility-rules", "elig_admin")["rule"].(map[string]any)
	if asInt(rule["topup_minor"]) != identity.DefaultEligibilityTopupMinor {
		t.Fatalf("default topup: %+v", rule)
	}
	if mustStatusJSON(t, "PATCH", server.URL+"/admin/eligibility-rules", "elig_admin", nil) != 409 {
		t.Fatal("eligibility patch without confirm must 409")
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "elig-small-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	if getAuthJSON(t, server.URL+"/v1/me", tokenOf(reg))["user"].(map[string]any)["can_commission"] != false {
		t.Fatal("new user must not have commission")
	}
	uid := userIDOf(reg)
	if err := application.Billing.Credit(ctx, uid, "elig-small-"+uid, billing.MinorPerUSD, "small"); err != nil {
		t.Fatal(err)
	}
	if getAuthJSON(t, server.URL+"/v1/me", tokenOf(reg))["user"].(map[string]any)["can_commission"] != false {
		t.Fatal("1 USD topup should not grant under default 10 USD")
	}

	reg10 := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "elig-redeem-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", tokenOf(reg10), map[string]any{"code": billing.RedeemE2E})
	if getAuthJSON(t, server.URL+"/v1/me", tokenOf(reg10))["user"].(map[string]any)["can_commission"] != true {
		t.Fatal("10 USD redeem should grant default topup line")
	}

	_ = patchJSONRaw(t, server.URL+"/admin/eligibility-rules", "elig_admin", map[string]any{
		"spend_minor": billing.MinorPerUSD, "topup_minor": 100 * billing.MinorPerUSD, "gift_minor": identity.DefaultEligibilityGiftMinor,
	})
	regSpend := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "elig-spend-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	suid := userIDOf(regSpend)
	if err := application.Billing.Credit(ctx, suid, "elig-spend-"+suid, 5*billing.MinorPerUSD, "spend"); err != nil {
		t.Fatal(err)
	}
	prices := []byte(`{"input":"1","output":"1"}`)
	reqID := "elig-spend-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: suid, ChannelOrgID: identity.OfficialChannelID, RequestID: reqID,
		ReserveMinor: 4 * billing.MinorPerUSD, UnitPrices: prices,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Billing.Settle(ctx, billing.SettleInput{
		RequestID: reqID, UserID: suid, ChannelOrgID: identity.OfficialChannelID,
		Usage: map[string]int{"prompt_tokens": 10, "completion_tokens": 1}, UnitPrices: prices,
	}); err != nil {
		t.Fatal(err)
	}
	if getAuthJSON(t, server.URL+"/v1/me", tokenOf(regSpend))["user"].(map[string]any)["can_commission"] != true {
		t.Fatal("lifetime spend should grant")
	}

	bReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/channel/eligibility-rules", bytes.NewReader([]byte(`{"spend_minor":1,"topup_minor":1}`)))
	bReq.Header.Set("Authorization", "Bearer elig_admin-b")
	bReq.Header.Set("Content-Type", "application/json")
	bReq.Header.Set("X-Tokenhub-Confirm", "1")
	bResp, err := http.DefaultClient.Do(bReq)
	if err != nil {
		t.Fatal(err)
	}
	_ = bResp.Body.Close()
	if bResp.StatusCode != http.StatusForbidden {
		t.Fatalf("B channel must not write eligibility, got %d", bResp.StatusCode)
	}
	_ = patchJSONRaw(t, server.URL+"/channel/eligibility-rules", "elig_admin-c", map[string]any{
		"spend_minor": 0, "topup_minor": billing.MinorPerUSD, "gift_minor": identity.DefaultEligibilityGiftMinor,
	})
	regC := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "elig-c-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THC1",
	})
	cuid := userIDOf(regC)
	if err := application.Billing.Credit(ctx, cuid, "elig-c-"+cuid, billing.MinorPerUSD, "c"); err != nil {
		t.Fatal(err)
	}
	if getAuthJSON(t, server.URL+"/v1/me", tokenOf(regC))["user"].(map[string]any)["can_commission"] != true {
		t.Fatal("C channel topup override should grant")
	}
}
