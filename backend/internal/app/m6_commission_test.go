package app_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	quotaBefore := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+identity.ResellerChannelID, "m6_admin")["quota"].(map[string]any)
	availBefore := asInt(quotaBefore["available_minor"])
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})
	quotaIssued := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+identity.ResellerChannelID, "m6_admin")["quota"].(map[string]any)
	if asInt(quotaIssued["available_minor"]) != availBefore-10*billing.MinorPerUSD {
		t.Fatalf("redeem should issue 1:1 from channel available: before=%d after=%v", availBefore, quotaIssued)
	}
	if asInt(quotaIssued["issued_minor"]) < 10*billing.MinorPerUSD {
		t.Fatalf("quota should record issued allocation: %+v", quotaIssued)
	}
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m6"})["item"].(map[string]any)["key"].(string)
	chat := postEchoUsage(t, server.URL+"/v1/chat/completions", apiKey, "kol2-usage")
	quotaAfterChat := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+identity.ResellerChannelID, "m6_admin")["quota"].(map[string]any)
	if asInt(quotaAfterChat["available_minor"]) != asInt(quotaIssued["available_minor"]) {
		t.Fatalf("chat must not deduct channel available again: issued=%v after=%v", quotaIssued, quotaAfterChat)
	}
	allocs := getAuthJSON(t, server.URL+"/channel/allocations?channel_id="+identity.ResellerChannelID, "m6_admin")["items"].([]any)
	if len(allocs) == 0 {
		t.Fatal("channel allocations should list the issued grant")
	}
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
	if !kinds[commission.KindDirect] || !kinds[commission.KindIndirect] {
		t.Fatalf("expected direct+indirect splits, got %+v", entries)
	}
	if kinds[commission.KindOverride] || kinds[commission.KindChannel] || kinds[commission.KindTeam] {
		t.Fatalf("old four-bucket kinds must not appear: %+v", entries)
	}

	if mustStatusJSON(t, http.MethodGet, server.URL+"/v1/partner/users", "", nil) != http.StatusForbidden {
		t.Fatal("unauth partner users must be 403")
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/v1/partner/me", "m6_user", nil) != http.StatusForbidden {
		t.Fatal("plain end user is not a partner")
	}
	meAgent := getAuthJSON(t, server.URL+"/v1/partner/me", "m6_admin-agent")
	if meAgent["role_type"] != identity.AcqAgent {
		t.Fatalf("agent me: %+v", meAgent)
	}
	meKOL2 := getAuthJSON(t, server.URL+"/v1/partner/me", "m6_admin-kol2")
	if meKOL2["role_type"] != identity.AcqKOL2 || meKOL2["sees_downline"] != false {
		t.Fatalf("kol2 me: %+v", meKOL2)
	}
	_ = postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "agent-attr-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoAgentB,
	})
	agentUsers := getAuthJSON(t, server.URL+"/v1/partner/users", "m6_admin-agent")["items"].([]any)
	if len(agentUsers) == 0 {
		t.Fatal("agent should see descendants")
	}
	email := agentUsers[0].(map[string]any)["email"].(string)
	if !strings.Contains(email, "***") {
		t.Fatalf("agent must see masked email: %s", email)
	}
	sawAgent, sawKOL2 := false, false
	for _, raw := range agentUsers {
		item := raw.(map[string]any)
		if item["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("agent leaked other channel: %+v", item)
		}
		if item["source_code"] == identity.PromoAgentB {
			sawAgent = true
		}
		if item["source_code"] == identity.PromoKOL2B {
			sawKOL2 = true
		}
	}
	if !sawAgent || !sawKOL2 {
		t.Fatalf("agent should see THB-AGENT and THB-KOL2: %+v", agentUsers)
	}
	kol2Users := getAuthJSON(t, server.URL+"/v1/partner/users", "m6_admin-kol2")["items"].([]any)
	for _, raw := range kol2Users {
		item := raw.(map[string]any)
		if item["source_code"] == identity.PromoAgentB {
			t.Fatalf("kol2 must not see agent-attributed users: %+v", item)
		}
		if item["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("kol2 leaked other channel: %+v", item)
		}
	}
	kol1Users := getAuthJSON(t, server.URL+"/v1/partner/users", "m6_admin-kol1")["items"].([]any)
	for _, raw := range kol1Users {
		if raw.(map[string]any)["source_code"] == identity.PromoAgentB {
			t.Fatalf("kol1 must not see agent-only users: %+v", raw)
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
	_ = postEchoUsage(t, server.URL+"/v1/chat/completions", k2, "settle-me")
	u2 := getAuthJSON(t, server.URL+"/v1/me/usage", s2)["items"].([]any)[0].(map[string]any)["id"].(string)
	if mustStatusJSON(t, http.MethodPost, server.URL+"/admin/commissions/unfreeze", "m6_admin", map[string]string{"usage_event_id": u2}) != http.StatusConflict {
		t.Fatal("unfreeze without confirm must be 409")
	}
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
	regBlocked := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "quota-block-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	blocked := mustStatusBody(t, http.MethodPost, server.URL+"/v1/topups/redeem", tokenOf(regBlocked), map[string]any{"code": billing.RedeemE2E})
	if blocked.status != http.StatusPaymentRequired {
		t.Fatalf("empty channel quota should reject redeem with 402, got %d %+v", blocked.status, blocked.body)
	}
	_ = postJSONRaw(t, server.URL+"/admin/channel-quotas/grant", "m6_admin", map[string]any{
		"channel_org_id": identity.ResellerChannelID, "amount_minor": avail - 1,
	})

	policy := getAuthJSON(t, server.URL+"/admin/commission-policy", "m6_admin")["policy"].(map[string]any)
	if asInt(policy["direct_bps"]) != commission.DefaultDirect {
		t.Fatalf("default policy: %+v", policy)
	}
	noConfirm, _ := http.NewRequest(http.MethodPatch, server.URL+"/admin/commission-policy", strings.NewReader(`{"direct_bps":1600,"indirect_bps":500,"total_bps":2000,"freeze_days":7,"min_settle_minor":1000000}`))
	noConfirm.Header.Set("Authorization", "Bearer m6_admin")
	noConfirm.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(noConfirm)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("policy patch without confirm should 409, got %d", resp.StatusCode)
	}
	updated := patchJSONRaw(t, server.URL+"/admin/commission-policy", "m6_admin", map[string]any{
		"direct_bps": 1600, "indirect_bps": 500, "total_bps": 2500, "freeze_days": 7, "min_settle_minor": 1_000_000, "version": "m6-v1-e2e",
	})
	if asInt(updated["policy"].(map[string]any)["direct_bps"]) != 1600 {
		t.Fatalf("policy patch: %+v", updated)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/commission-policy", "m6_admin", map[string]any{
		"direct_bps": commission.DefaultDirect, "indirect_bps": commission.DefaultIndirect,
		"total_bps": commission.DefaultTotal, "freeze_days": commission.FreezeDays,
		"min_settle_minor": billing.MinorPerUSD, "version": commission.PolicyM6,
	})

	regHold := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "hold-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	holdSession := tokenOf(regHold)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", holdSession, map[string]any{"code": billing.RedeemE2E})
	holdKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", holdSession, map[string]any{"name": "hold"})["item"].(map[string]any)["key"].(string)
	_ = postEchoUsage(t, server.URL+"/v1/chat/completions", holdKey, "hold-me")
	holdUsage := getAuthJSON(t, server.URL+"/v1/me/usage", holdSession)["items"].([]any)[0].(map[string]any)["id"].(string)
	holdUser := getAuthJSON(t, server.URL+"/v1/me", holdSession)["user"].(map[string]any)["id"].(string)
	held, err := application.Commission.HoldUnsettledForUser(ctx, holdUser)
	if err != nil || held == 0 {
		t.Fatalf("hold unsettled: n=%d err=%v", held, err)
	}
	for _, raw := range getAuthJSON(t, server.URL+"/admin/commissions?usage_event_id="+holdUsage, "m6_admin")["items"].([]any) {
		if raw.(map[string]any)["status"] != commission.StatusHeld {
			t.Fatalf("ban should hold commission: %+v", raw)
		}
	}
	released, err := application.Commission.ReleaseHeldForUser(ctx, holdUser)
	if err != nil || released == 0 {
		t.Fatalf("release held: n=%d err=%v", released, err)
	}
}

func TestD82QuotaRatio(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "d82_admin"
	cfg.BootstrapUser = "d82_user"
	cfg.BootstrapChannel = "d82_channel"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	created := postJSONRaw(t, server.URL+"/admin/channels", "d82_admin", map[string]any{
		"code": "d82-" + strconv.FormatInt(time.Now().UnixNano(), 10), "type": "B", "status": "active",
	})
	channelID := created["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, server.URL+"/admin/channel-quotas/grant", "d82_admin", map[string]any{
		"channel_org_id": channelID, "amount_minor": 100 * billing.MinorPerUSD,
	})
	promo := "THX-D82-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_ = postJSONRaw(t, server.URL+"/admin/promotion-codes", "d82_admin", map[string]any{
		"channel_org_id": channelID, "code": promo,
	})

	defRule := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", "d82_admin")["rule"].(map[string]any)
	if asInt(defRule["issue_ratio_bps"]) != billing.DefaultIssueRatioBPS {
		t.Fatalf("missing rule must default to 1:1: %+v", defRule)
	}
	quota := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+channelID, "d82_admin")["quota"].(map[string]any)
	if asInt(quota["issue_ratio_bps"]) != billing.DefaultIssueRatioBPS {
		t.Fatalf("quota should expose default ratio: %+v", quota)
	}

	if mustStatusJSON(t, http.MethodPatch, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", "d82_admin", map[string]string{}) != http.StatusConflict {
		t.Fatal("issue-rule patch without confirm must be 409")
	}
	if mustStatusJSON(t, http.MethodPatch, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", "d82_channel", map[string]string{}) != http.StatusForbidden {
		t.Fatal("channel admin cannot change issue ratio")
	}
	if getStatus(t, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", "d82_channel") != http.StatusForbidden {
		t.Fatal("channel admin cannot read another channel issue rule")
	}
	bad := mustStatusBody(t, http.MethodPatch, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", "d82_admin", map[string]any{
		"issue_ratio_bps": 0,
	})
	if bad.status != http.StatusConflict && bad.status != http.StatusBadRequest {
		t.Fatalf("invalid bps should 400/409, got %d %+v", bad.status, bad.body)
	}
	// mustStatusBody does not send confirm; 409 is expected first. Confirm + invalid:
	req, _ := http.NewRequest(http.MethodPatch, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", strings.NewReader(`{"issue_ratio_bps":0}`))
	req.Header.Set("Authorization", "Bearer d82_admin")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid bps with confirm should 400, got %d", resp.StatusCode)
	}

	updated := patchJSONRaw(t, server.URL+"/admin/channel-quotas/"+channelID+"/issue-rule", "d82_admin", map[string]any{
		"issue_ratio_bps": 12_000,
	})
	if asInt(updated["rule"].(map[string]any)["issue_ratio_bps"]) != 12_000 {
		t.Fatalf("patch 1.2x: %+v", updated)
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "d82-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": promo,
	})
	if channelOf(reg) != channelID {
		t.Fatalf("d82 user should bind new channel: %+v", reg)
	}
	before := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+channelID, "d82_admin")["quota"].(map[string]any)
	availBefore := asInt(before["available_minor"])
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", tokenOf(reg), map[string]any{"code": billing.RedeemE2E})
	after := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+channelID, "d82_admin")["quota"].(map[string]any)
	if asInt(after["available_minor"]) != availBefore-12*billing.MinorPerUSD {
		t.Fatalf("1.2x redeem should issue 12 USD from channel: before=%d after=%v", availBefore, after)
	}
	if asInt(after["issued_minor"]) < 12*billing.MinorPerUSD {
		t.Fatalf("issued allocation should be 12 USD: %+v", after)
	}
	if asInt(after["issue_ratio_bps"]) != 12_000 {
		t.Fatalf("quota should keep 1.2x ratio: %+v", after)
	}
	allocs := getAuthJSON(t, server.URL+"/channel/allocations?channel_id="+channelID, "d82_admin")["items"].([]any)
	if len(allocs) == 0 || asInt(allocs[0].(map[string]any)["granted_minor"]) != 12*billing.MinorPerUSD {
		t.Fatalf("allocation granted should be 12 USD: %+v", allocs)
	}

	oneToOne := postJSONRaw(t, server.URL+"/admin/channels", "d82_admin", map[string]any{
		"code": "d82-1to1-" + strconv.FormatInt(time.Now().UnixNano(), 10), "type": "B", "status": "active",
	})
	plainID := oneToOne["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, server.URL+"/admin/channel-quotas/grant", "d82_admin", map[string]any{
		"channel_org_id": plainID, "amount_minor": 100 * billing.MinorPerUSD,
	})
	plainPromo := "THX-D82B-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_ = postJSONRaw(t, server.URL+"/admin/promotion-codes", "d82_admin", map[string]any{
		"channel_org_id": plainID, "code": plainPromo,
	})
	plainReg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "d82b-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": plainPromo,
	})
	plainBefore := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+plainID, "d82_admin")["quota"].(map[string]any)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", tokenOf(plainReg), map[string]any{"code": billing.RedeemE2E})
	plainAfter := getAuthJSON(t, server.URL+"/admin/channel-quotas/"+plainID, "d82_admin")["quota"].(map[string]any)
	if asInt(plainAfter["available_minor"]) != asInt(plainBefore["available_minor"])-10*billing.MinorPerUSD {
		t.Fatalf("unset rule must stay 1:1: before=%v after=%v", plainBefore, plainAfter)
	}
}

// postEchoUsage 用 content 沙箱把批发价抬过间接档整数除法门槛（默认 500 BPS）。
func postEchoUsage(t *testing.T, url, token, content string) map[string]any {
	t.Helper()
	payload := map[string]any{
		"model":    catalog.EchoModelID,
		"messages": []map[string]string{{"role": "user", "content": content + strings.Repeat("x", 48)}},
	}
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Sandbox-Mode", "content")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("POST %s %d %v", url, resp.StatusCode, out)
	}
	return out
}
