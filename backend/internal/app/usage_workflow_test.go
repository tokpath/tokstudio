package app_test

import (
	"context"
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestOverhaulUsageFullScopeAndRequestFacts(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("isolated postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	a := mustApp(t, cfg)
	ctx := context.Background()
	marker := "usage-scope-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	user, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "@example.test", Password: "isolated-password", PromotionCode: "THA1"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Router())
	defer server.Close()
	key := postJSONRaw(t, server.URL+"/v1/me/api-keys", user.Token, map[string]any{"name": "Readable usage key"})["item"].(map[string]any)["id"].(string)
	at := time.Date(2026, 10, 9, 16, 0, 0, 0, time.UTC) // next day in Shanghai
	for i := 0; i < 230; i++ {
		req := marker + "-" + strconv.Itoa(i)
		state := billing.UsageConfirmed
		if i >= 210 {
			state = billing.UsagePending
		}
		if i >= 225 {
			state = billing.UsageVoided
		}
		usage, _ := json.Marshal(map[string]int{"prompt_tokens": 2, "completion_tokens": 3})
		keyID := key
		if i == 0 {
			keyID = "legacy-other-key"
		}
		if err := a.DB.Table("billing_usage_events").Create(map[string]any{"id": req, "request_id": req, "user_id": user.User.ID, "api_key_id": keyID, "channel_org_id": identity.OfficialChannelID, "public_model_id": "scope/model", "unit_usage_json": string(usage), "unit_prices_json": "{}", "customer_amount_minor": 100, "upstream_cost_minor": 17, "wholesale_amount_minor": 21, "state": state, "idempotency_key": req, "occurred_at": at}).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Table("gateway_requests").Create(map[string]any{"id": "g-" + req, "request_id": req, "user_id": user.User.ID, "api_key_id": keyID, "channel_org_id": identity.OfficialChannelID, "public_model_id": "scope/model", "protocol": "chat", "status": "succeeded", "started_at": at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	summary := getAuthJSON(t, server.URL+"/v1/me/usage/summary?from=2026-10-10&to=2026-10-10&time_zone=Asia%2FShanghai", user.Token)
	totals := summary["totals"].(map[string]any)
	if totals["requests"] != float64(230) || totals["amount_minor"] != float64(21000) || totals["pending"] != float64(15) || totals["voided"] != float64(5) || totals["prompt_tokens"] != float64(420) {
		t.Fatalf("aggregate truncated or mixed states: %+v", summary)
	}
	if daily := summary["daily"].([]any); len(daily) != 1 || daily[0].(map[string]any)["key"] != "2026-10-10" {
		t.Fatalf("wrong financial timezone: %+v", daily)
	}
	filtered := getAuthJSON(t, server.URL+"/v1/me/usage/summary?api_key_id="+url.QueryEscape(key), user.Token)
	if filtered["totals"].(map[string]any)["requests"] != float64(229) {
		t.Fatalf("selected key filter lost %+v", filtered)
	}
	if len(filtered["facets"].(map[string]any)["keys"].([]any)) != 2 || filtered["key_labels"].(map[string]any)[key] != "Readable usage key" {
		t.Fatalf("facets disappeared or key name missing %+v", filtered)
	}
	seen := map[string]bool{}
	cursor := ""
	for i := 0; i < 8; i++ {
		response := getAuthJSON(t, server.URL+"/v1/me/requests?limit=50&cursor="+url.QueryEscape(cursor), user.Token)
		for _, row := range response["items"].([]any) {
			id := row.(map[string]any)["request_id"].(string)
			if seen[id] {
				t.Fatalf("cursor repeated %s", id)
			}
			seen[id] = true
		}
		cursor, _ = response["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 230 {
		t.Fatalf("cursor skipped records tied on time: %d", len(seen))
	}
	searched := getAuthJSON(t, server.URL+"/v1/me/requests?q="+url.QueryEscape(marker+"-229"), user.Token)
	if len(searched["items"].([]any)) != 1 {
		t.Fatalf("whole scope search missed final record: %+v", searched)
	}
	req := marker + "-229"
	details := getAuthJSON(t, server.URL+"/v1/me/requests/"+req, user.Token)
	if _, ok := details["diagnostic_usage"]; ok {
		t.Fatal("public request contains provider cost")
	}
	usage := details["usage"].([]any)[0].(map[string]any)
	if _, ok := usage["upstream_cost_minor"]; ok {
		t.Fatal("public request contains upstream cost")
	}
	other, err := a.Identity.Register(ctx, identity.RegisterInput{Email: marker + "-other@example.test", Password: "isolated-password", PromotionCode: "THA1"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := doJSON(t, http.MethodGet, server.URL+"/v1/me/requests/"+req, other.Token, false, nil); code != 404 {
		t.Fatalf("another account read request: %d", code)
	}
	scoped, err := a.Gateway.ListScopedRequests(ctx, gateway.QueryRequestsInput{ChannelOrgIDs: []string{}, Limit: 50})
	if err != nil || len(scoped) != 0 {
		t.Fatalf("empty scope widened: %v %v", scoped, err)
	}
	result, err := a.Billing.ResolvePending(ctx, billing.ResolvePendingInput{IDs: []string{marker + "-210", marker + "-0", "does-not-exist"}})
	if err != nil || len(result.Results) != 3 || len(result.Items) != 1 || result.Results[0].Status != "resolved" || result.Results[1].Error != "already_charged" || result.Results[2].Error != "not_found" {
		t.Fatalf("partial results lost: %+v %v", result, err)
	}
	if code, _ := doJSON(t, http.MethodGet, server.URL+"/v1/me/requests?cursor=not-a-cursor", user.Token, false, nil); code != 400 {
		t.Fatalf("bad cursor not rejected: %d", code)
	}
}

func TestOverhaulUsageObjectAndMixedBatchScope(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("isolated postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "scope-finance-admin"
	cfg.BootstrapUser = "scope-finance-user"
	a := mustApp(t, cfg)
	ctx := context.Background()
	server := httptest.NewServer(a.Router())
	defer server.Close()
	marker := "usage-permission-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	email := marker + "-finance@example.test"
	code, created := doJSON(t, http.MethodPost, server.URL+"/admin/staff", cfg.BootstrapAdmin, true, map[string]any{"email": email, "password": "password1", "display_name": "Scope finance", "roles": []string{"finance_admin"}})
	if code != 201 {
		t.Fatalf("finance fixture %d %+v", code, created)
	}
	finance := tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": email, "password": "password1"}))
	for _, scope := range []struct{ name, channel string }{{"own", identity.OfficialChannelID}, {"oem", identity.OEMChannelID}} {
		id := marker + "-" + scope.name
		if err := a.DB.Table("billing_usage_events").Create(map[string]any{"id": id, "request_id": id, "user_id": "scope-user", "api_key_id": "scope-key", "channel_org_id": scope.channel, "public_model_id": "scope/model", "unit_usage_json": "{}", "unit_prices_json": "{}", "customer_amount_minor": 0, "upstream_cost_minor": 0, "wholesale_amount_minor": 0, "state": billing.UsagePending, "idempotency_key": id, "occurred_at": time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Table("gateway_requests").Create(map[string]any{"id": "g-" + id, "request_id": id, "user_id": "scope-user", "channel_org_id": scope.channel, "public_model_id": "scope/model", "protocol": "chat", "status": "succeeded", "started_at": time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
	}

	charged := marker + "-oem-charged"
	if err := a.DB.Table("billing_usage_events").Create(map[string]any{"id": charged, "request_id": charged, "user_id": "scope-user", "channel_org_id": identity.OEMChannelID, "public_model_id": "scope/model", "unit_usage_json": "{}", "unit_prices_json": "{}", "customer_amount_minor": 10, "upstream_cost_minor": 0, "wholesale_amount_minor": 0, "state": billing.UsageConfirmed, "idempotency_key": charged, "occurred_at": time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Table("billing_customer_charges").Create(map[string]any{"id": "chg-" + charged, "request_id": charged, "usage_event_id": charged, "amount_minor": 10, "status": "committed"}).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := doJSON(t, http.MethodGet, server.URL+"/admin/refunds/preview?request_id="+charged, cfg.BootstrapAdmin, false, nil); code != 200 {
		t.Fatalf("charged fixture must be readable: %d", code)
	}
	foreign := marker + "-oem"
	for _, path := range []string{"/admin/requests/" + foreign, "/admin/usage/pending/" + foreign, "/admin/refunds/preview?request_id=" + foreign} {
		if code, _ := doJSON(t, http.MethodGet, server.URL+path, finance, false, nil); code != 404 {
			t.Fatalf("finance read foreign %s: %d", path, code)
		}
	}
	for _, call := range []struct {
		path string
		body any
	}{{"/admin/requests/" + foreign + "/reconcile", map[string]any{"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1}}}, {"/admin/usage/replay", map[string]any{"request_id": foreign, "usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1}}}, {"/admin/refunds", map[string]any{"request_id": foreign}}} {
		if code, _ := doJSON(t, http.MethodPost, server.URL+call.path, finance, true, call.body); code != 404 {
			t.Fatalf("finance wrote foreign %s: %d", call.path, code)
		}
	}
	code, result := doJSON(t, http.MethodPost, server.URL+"/admin/usage/pending/resolve", finance, true, map[string]any{"ids": []string{marker + "-own", foreign}})
	if code != 200 {
		t.Fatalf("mixed batch %d %+v", code, result)
	}
	rows := result["item"].(map[string]any)["results"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["status"] != "resolved" || rows[1].(map[string]any)["error"] != "not_found" {
		t.Fatalf("mixed scope result %+v", rows)
	}
	gap, err := a.Billing.GetUsageGap(ctx, foreign)
	if err != nil || gap.State != billing.UsagePending {
		t.Fatalf("foreign fact changed: %+v %v", gap, err)
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/refunds", cfg.BootstrapAdmin, true, map[string]any{"topup_id": "legacy-topup"}); code != 410 {
		t.Fatalf("legacy topup write still available: %d", code)
	}

	for _, combination := range []struct {
		suffix     string
		roles      []string
		canResolve bool
	}{{"audit", []string{"finance_admin", "audit_readonly"}, false}, {"ops", []string{"finance_admin", "ops_admin"}, true}} {
		email := marker + "-" + combination.suffix + "@example.test"
		if code, created := doJSON(t, http.MethodPost, server.URL+"/admin/staff", cfg.BootstrapAdmin, true, map[string]any{"email": email, "password": "password1", "roles": combination.roles}); code != 201 {
			t.Fatalf("mixed role fixture %d %+v", code, created)
		}
		token := tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": email, "password": "password1"}))
		if code, _ := doJSON(t, http.MethodGet, server.URL+"/admin/requests/"+foreign, token, false, nil); code != 200 {
			t.Fatalf("mixed read union lost %s: %d", combination.suffix, code)
		}
		if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/refunds", token, true, map[string]any{"request_id": charged}); code != 404 {
			t.Fatalf("mixed role expanded refund scope %s: %d", combination.suffix, code)
		}
		code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/usage/pending/resolve", token, true, map[string]any{"ids": []string{foreign}})
		if combination.canResolve && code != 200 || !combination.canResolve && code != 404 {
			t.Fatalf("mixed reconcile scope %s: %d", combination.suffix, code)
		}
	}
	var chargeState string
	if err := a.DB.Table("billing_customer_charges").Where("request_id = ?", charged).Pluck("status", &chargeState).Error; err != nil || chargeState != "committed" {
		t.Fatalf("mixed role mutated foreign charge %s %v", chargeState, err)
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/usage/pending/resolve", cfg.BootstrapAdmin, true, map[string]any{"ids": []string{foreign}}); code != 200 {
		t.Fatalf("platform original authority lost: %d", code)
	}
}

func TestOverhaulRequestCommissionOnlyOwnBeneficiaries(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("isolated postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "commission-detail-admin"
	cfg.BootstrapUser = "commission-detail-user"
	cfg.BootstrapChannel = "commission-detail-b"
	a := mustApp(t, cfg)
	ctx := context.Background()
	server := httptest.NewServer(a.Router())
	defer server.Close()
	p, err := a.Identity.Authenticate(ctx, "Bearer "+cfg.BootstrapChannel)
	if err != nil || p == nil {
		t.Fatalf("B principal: %+v %v", p, err)
	}
	marker := "commission-request-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := a.DB.Table("identity_role_members").Create(map[string]any{"user_id": p.UserID, "acquisition_role_id": identity.KOL2BRoleID, "created_at": time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	defer a.DB.Table("identity_role_members").Where("user_id = ? AND acquisition_role_id = ?", p.UserID, identity.KOL2BRoleID).Delete(nil)
	if err := a.DB.Table("gateway_requests").Create(map[string]any{"id": "g-" + marker, "request_id": marker, "user_id": "scope-user", "channel_org_id": identity.ResellerChannelID, "public_model_id": "scope/model", "protocol": "chat", "status": "succeeded", "started_at": time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{identity.KOL2BRoleID, identity.KOL1BRoleID} {
		if err := a.DB.Table("commission_entries").Create(map[string]any{"id": marker + "-" + role, "usage_event_id": marker, "request_id": marker, "user_id": "scope-user", "channel_org_id": identity.ResellerChannelID, "beneficiary_role_id": role, "kind": "direct", "policy_version": "scope-v1", "base_amount_minor": 10000, "raw_amount_minor": 100, "amount_minor": 100, "status": "frozen", "idempotency_key": marker + "-" + role, "created_at": time.Now().UTC()}).Error; err != nil {
			t.Fatal(err)
		}
	}
	detail := getAuthJSON(t, server.URL+"/channel/requests/"+marker, cfg.BootstrapChannel)
	entries := detail["commissions"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["beneficiary_role_id"] != identity.KOL2BRoleID {
		t.Fatalf("B saw other beneficiary %+v", detail)
	}
	admin := getAuthJSON(t, server.URL+"/admin/requests/"+marker, cfg.BootstrapAdmin)
	if len(admin["commissions"].([]any)) != 2 {
		t.Fatalf("platform facts lost %+v", admin)
	}
	if err := a.DB.Table("identity_role_members").Where("user_id = ? AND acquisition_role_id = ?", p.UserID, identity.KOL2BRoleID).Delete(nil).Error; err != nil {
		t.Fatal(err)
	}
	empty := getAuthJSON(t, server.URL+"/channel/requests/"+marker, cfg.BootstrapChannel)
	if len(empty["commissions"].([]any)) != 0 {
		t.Fatalf("empty role widened %+v", empty)
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/channel/usage/pending/resolve", cfg.BootstrapChannel, true, map[string]any{"request_ids": []string{marker}}); code < 400 {
		t.Fatalf("B got finance authority %d", code)
	}
}
