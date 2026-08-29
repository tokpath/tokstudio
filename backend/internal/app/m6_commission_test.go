package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commission"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM6CommissionDistribution(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m6_admin"
	cfg.BootstrapUser = "m6_user"
	cfg.BootstrapChannel = "m6_channel"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()

	health := getJSON(t, server.URL+"/healthz", "")
	if ver, _ := health["version"].(string); !strings.HasPrefix(ver, "0.1.0-m") {
		t.Fatalf("health: %+v", health)
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "kol2-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	session := tokenOf(reg)
	if channelOf(reg) != identity.ResellerChannelID {
		t.Fatalf("kol2 should bind reseller: %+v", reg)
	}
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m6"})["item"].(map[string]any)["key"].(string)
	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "kol2-usage"}},
	})
	usage := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)
	if len(usage) == 0 {
		t.Fatalf("usage missing: %+v", chat)
	}
	usageID := usage[0].(map[string]any)["id"].(string)

	entries := getAuthJSON(t, server.URL+"/admin/commissions?usage_event_id="+usageID, "m6_admin")["items"].([]any)
	kinds := map[string]bool{}
	for _, raw := range entries {
		item := raw.(map[string]any)
		kinds[item["kind"].(string)] = true
		if item["status"] != commission.StatusFrozen {
			t.Fatalf("new commission should be frozen: %+v", item)
		}
	}
	if !kinds[commission.KindDirect] || !kinds[commission.KindOverride] || !kinds[commission.KindChannel] {
		t.Fatalf("expected hierarchy splits, got %+v", entries)
	}

	agentUsers := getAuthJSON(t, server.URL+"/v1/partner/users", "m6_admin-agent")["items"].([]any)
	if len(agentUsers) == 0 {
		t.Fatal("agent should see descendants")
	}
	email := agentUsers[0].(map[string]any)["email"].(string)
	if !strings.Contains(email, "***") {
		t.Fatalf("agent must see masked email: %s", email)
	}
	for _, raw := range agentUsers {
		item := raw.(map[string]any)
		if item["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("agent leaked other channel: %+v", item)
		}
	}

	exportReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/partner/export", nil)
	exportReq.Header.Set("Authorization", "Bearer m6_admin-agent")
	exportResp, err := http.DefaultClient.Do(exportReq)
	if err != nil {
		t.Fatal(err)
	}
	defer exportResp.Body.Close()
	if exportResp.StatusCode != http.StatusOK {
		t.Fatalf("export %d", exportResp.StatusCode)
	}

	_ = postJSONRaw(t, server.URL+"/admin/refunds", "m6_admin", map[string]any{"request_id": chat["request_id"]})
	afterRefund := getAuthJSON(t, server.URL+"/admin/commissions?usage_event_id="+usageID, "m6_admin")["items"].([]any)
	sawRev := false
	for _, raw := range afterRefund {
		if raw.(map[string]any)["status"] == commission.StatusReversed {
			sawRev = true
		}
	}
	if !sawRev {
		t.Fatalf("refund should reverse commission details: %+v", afterRefund)
	}

	reg2 := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "kol2b-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	s2 := tokenOf(reg2)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", s2, map[string]any{"code": billing.RedeemE2E})
	k2 := postJSONRaw(t, server.URL+"/v1/me/api-keys", s2, map[string]any{"name": "m6b"})["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/chat/completions", k2, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "settle-me"}},
	})
	u2 := getAuthJSON(t, server.URL+"/v1/me/usage", s2)["items"].([]any)[0].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, server.URL+"/admin/commissions/unfreeze", "m6_admin", map[string]any{"usage_event_id": u2})
	settled := postJSONRaw(t, server.URL+"/admin/commissions/settle?ignore_minimum=1", "m6_admin", map[string]any{})
	items, _ := settled["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected settlement batch: %+v", settled)
	}
	sid := items[0].(map[string]any)["id"].(string)
	paid := postJSONRaw(t, server.URL+"/admin/settlements/"+sid+"/payout", "m6_admin", map[string]any{
		"method": "manual", "reference": "wire-e2e",
	})
	if paid["item"].(map[string]any)["status"] != commission.StatusPaid {
		t.Fatalf("payout: %+v", paid)
	}

	q := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+identity.ResellerChannelID, "m6_admin")["quota"].(map[string]any)
	avail := asInt(q["available_minor"])
	_ = postJSONRaw(t, server.URL+"/admin/channel-quotas/grant", "m6_admin", map[string]any{
		"channel_org_id": identity.ResellerChannelID, "amount_minor": -(avail - 1),
	})
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: userIDOf(reg2), ChannelOrgID: identity.ResellerChannelID,
		RequestID:     "quota-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		PublicModelID: catalog.EchoModelID, ReserveMinor: 100,
		UnitPrices: []byte(`{"input":"0.000001","output":"0.000002"}`),
	}); err == nil {
		t.Fatal("channel quota should block over-issue")
	} else if err != billing.ErrInsufficientQuota {
		t.Fatalf("expected insufficient quota, got %v", err)
	}
	_ = postJSONRaw(t, server.URL+"/admin/channel-quotas/grant", "m6_admin", map[string]any{
		"channel_org_id": identity.ResellerChannelID, "amount_minor": avail - 1,
	})
}
