package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM7OpsHardening(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m7_admin"
	cfg.BootstrapUser = "m7_user"
	cfg.BootstrapChannel = "m7_admin-b"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	if me := getAuthJSON(t, server.URL+"/admin/me", "m7_admin"); me["user_id"] != nil {
		_ = application.Identity.DisableTOTP(context.Background(), me["user_id"].(string))
	}
	defer func() {
		if me := getAuthJSON(t, server.URL+"/admin/me", "m7_admin"); me["user_id"] != nil {
			_ = application.Identity.DisableTOTP(context.Background(), me["user_id"].(string))
		}
	}()

	health := getJSON(t, server.URL+"/healthz", "")
	if ver, _ := health["version"].(string); ver != "0.1.0-m7" {
		t.Fatalf("health: %+v", health)
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "ops-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": identity.PromoKOL2B,
	})
	session := tokenOf(reg)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})
	key := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m7"})["item"].(map[string]any)["key"].(string)
	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "m7-dash"}},
	})
	if chat["request_id"] == nil {
		t.Fatalf("chat: %+v", chat)
	}

	metrics := getAuthJSON(t, server.URL+"/admin/metrics?dimension=model", "m7_admin")
	found := false
	for _, raw := range metrics["items"].([]any) {
		item := raw.(map[string]any)
		if item["key"] == catalog.EchoModelID && asInt(item["requests"]) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("dashboard missing echo model: %+v", metrics)
	}
	series := getAuthJSON(t, server.URL+"/admin/metrics/series?days=7", "m7_admin")
	items, _ := series["items"].([]any)
	if len(items) != 7 {
		t.Fatalf("series days: %+v", series)
	}
	sawToday := false
	today := time.Now().UTC().Format("2006-01-02")
	for _, raw := range items {
		point := raw.(map[string]any)
		if point["day"] == today && asInt(point["requests"]) > 0 {
			sawToday = true
		}
	}
	if !sawToday {
		t.Fatalf("series missing today traffic: %+v", series)
	}
	csvReq, _ := http.NewRequest(http.MethodGet, server.URL+"/admin/metrics/daily?format=csv&days=7", nil)
	csvReq.Header.Set("Authorization", "Bearer m7_admin")
	csvResp, err := http.DefaultClient.Do(csvReq)
	if err != nil {
		t.Fatal(err)
	}
	csvBody, _ := io.ReadAll(csvResp.Body)
	_ = csvResp.Body.Close()
	if csvResp.StatusCode != http.StatusOK || !strings.Contains(string(csvBody), "gross_profit_minor") || !strings.Contains(string(csvBody), today) {
		t.Fatalf("daily csv: %d %s", csvResp.StatusCode, csvBody)
	}
	dash := getAuthJSON(t, server.URL+"/admin/ops/dashboard", "m7_admin")["dashboard"].(map[string]any)
	totals := dash["totals"].(map[string]any)
	if totals["revenue_minor"] == nil || totals["gross_profit_minor"] == nil || totals["success_rate"] == nil || totals["low_balance_wallets"] == nil || totals["latency_p99_ms"] == nil || totals["http_429"] == nil || totals["preauth_failed"] == nil || totals["callback_latency_p95_ms"] == nil || totals["timeouts"] == nil || totals["error_codes"] == nil || totals["prompt_tokens"] == nil || totals["video_seconds"] == nil {
		t.Fatalf("totals incomplete: %+v", totals)
	}
	if asInt(totals["prompt_tokens"]) < 8 {
		t.Fatalf("echo chat should record prompt tokens: %+v", totals)
	}
	if dash["thresholds"] == nil {
		t.Fatalf("dashboard missing thresholds: %+v", dash)
	}
	thr := getAuthJSON(t, server.URL+"/admin/ops/thresholds", "m7_admin")["thresholds"].(map[string]any)
	if thr["success_rate_min"] == nil || asInt(thr["min_requests"]) < 1 {
		t.Fatalf("default thresholds: %+v", thr)
	}
	thrPatched := patchJSONRaw(t, server.URL+"/admin/ops/thresholds", "m7_admin", map[string]any{
		"success_rate_min": 0.8, "min_requests": 10, "pending_count": 2,
	})["thresholds"].(map[string]any)
	if asInt(thrPatched["min_requests"]) != 10 {
		t.Fatalf("patched thresholds: %+v", thrPatched)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/ops/thresholds", "m7_admin", map[string]any{
		"success_rate_min": 0.5, "min_requests": 5, "pending_count": 1,
	})
	_ = patchJSONRaw(t, server.URL+"/admin/providers/prd_echo_primary", "m7_admin", map[string]any{"test_behavior": "timeout"})
	timed := postJSONRaw(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "timeout-probe"}},
	})
	_ = patchJSONRaw(t, server.URL+"/admin/providers/prd_echo_primary", "m7_admin", map[string]any{"test_behavior": "ok"})
	if timed["provider"] != catalog.BackupProvider {
		t.Fatalf("timeout should fallback: %+v", timed)
	}
	afterTO := getAuthJSON(t, server.URL+"/admin/ops/dashboard", "m7_admin")["dashboard"].(map[string]any)["totals"].(map[string]any)
	if asInt(afterTO["timeouts"]) < 1 {
		t.Fatalf("timeout should be counted: %+v", afterTO)
	}
	codes, _ := afterTO["error_codes"].(map[string]any)
	if asInt(codes["timeout"]) < 1 {
		t.Fatalf("error_codes should include timeout: %+v", afterTO)
	}
	agents, _ := dash["dimensions"].(map[string]any)["agent"].([]any)
	sawAgent := false
	for _, raw := range agents {
		if raw.(map[string]any)["key"] == identity.KOL2BRoleID {
			sawAgent = true
		}
	}
	if !sawAgent {
		t.Fatalf("dashboard missing agent dimension: %+v", dash["dimensions"])
	}
	if code := postStatus(t, server.URL+"/admin/refunds", "m7_admin", map[string]any{"request_id": "missing"}); code != http.StatusConflict {
		t.Fatalf("sensitive refund without confirm should be 409, got %d", code)
	}

	limited := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "slow", "rpm_limit": 1})["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/chat/completions", limited, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "first"}},
	})
	if code := postStatus(t, server.URL+"/v1/chat/completions", limited, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "second"}},
	}); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", code)
	}

	_ = postJSONRaw(t, server.URL+"/admin/ops/circuit/prd_echo_primary", "m7_admin", map[string]any{"action": "trip"})
	afterTrip := postJSONRaw(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "after-trip"}},
	})
	if afterTrip["provider"] != catalog.BackupProvider {
		t.Fatalf("tripped primary should fallback: %+v", afterTrip)
	}
	_ = postJSONRaw(t, server.URL+"/admin/ops/circuit/prd_echo_primary", "m7_admin", map[string]any{"action": "reset"})

	_ = postJSONRaw(t, server.URL+"/admin/ops/canary", "m7_admin", map[string]any{"provider_slug": catalog.BackupProvider, "percent": 100})
	forced := doChatHeader(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "canary"}},
	}, map[string]string{"X-Tokenhub-Canary": "1"})
	if forced["provider"] != catalog.BackupProvider {
		t.Fatalf("canary should prefer backup: %+v", forced)
	}
	_ = postJSONRaw(t, server.URL+"/admin/ops/canary", "m7_admin", map[string]any{"provider_slug": catalog.BackupProvider, "percent": 0})

	stats := getAuthJSON(t, server.URL+"/admin/outbox/stats", "m7_admin")["stats"].(map[string]any)
	if stats["pending"] == nil && stats["published"] == nil && stats["failed"] == nil {
		t.Fatalf("outbox stats: %+v", stats)
	}
	probe := postJSONRaw(t, server.URL+"/admin/audit-probes", "m7_admin", map[string]any{})
	if probe["item"].(map[string]any)["action"] != "audit.probe" {
		t.Fatalf("audit probe: %+v", probe)
	}
	searched := getAuthJSON(t, server.URL+"/admin/audit-logs?action=audit.probe", "m7_admin")["items"].([]any)
	if len(searched) == 0 {
		t.Fatal("audit search by action should find probe")
	}

	drill := postJSONRaw(t, server.URL+"/admin/ops/backup-drill", "m7_admin", map[string]any{})
	item := drill["item"].(map[string]any)
	if item["status"] != "passed" || asInt(item["rpo_minutes"]) > 15 || asInt(item["rto_minutes"]) > 60 {
		t.Fatalf("backup drill: %+v", drill)
	}
	if pay := postJSONRaw(t, server.URL+"/admin/ops/drills/payment", "m7_admin", map[string]any{}); pay["item"].(map[string]any)["passed"] != true {
		t.Fatalf("payment drill: %+v", pay)
	}
	if media := postJSONRaw(t, server.URL+"/admin/ops/drills/media", "m7_admin", map[string]any{}); media["item"].(map[string]any)["passed"] != true {
		t.Fatalf("media drill: %+v", media)
	}
	if tls := postJSONRaw(t, server.URL+"/admin/ops/drills/tls", "m7_admin", map[string]any{}); tls["item"].(map[string]any)["passed"] != true {
		t.Fatalf("tls drill: %+v", tls)
	}
	_ = postJSONRaw(t, server.URL+"/admin/ops/alerts/evaluate", "m7_admin", map[string]any{})
	if books := getAuthJSON(t, server.URL+"/admin/ops/runbooks", "m7_admin")["items"].([]any); len(books) == 0 {
		t.Fatal("runbooks missing")
	}

	gemini := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.GeminiModelID, "messages": []map[string]string{{"role": "user", "content": "hi-gemini"}},
	})
	if strings.TrimSpace(os.Getenv("TOKENHUB_GEMINI_API_KEY")) == "" {
		if gemini.status != http.StatusServiceUnavailable && gemini.status != http.StatusBadGateway {
			// chat 可能以 200 + error 或 503 返回；禁止 sandbox echo 成功
			if content, ok := firstContentOf(gemini.body); ok && strings.Contains(content, "echo:") {
				t.Fatalf("gemini without key must not harness/sandbox-echo: %+v", gemini.body)
			}
			if gemini.status == http.StatusOK && gemini.body["provider"] == catalog.GeminiProvider {
				t.Fatalf("gemini without key must not succeed via echo: %+v", gemini.body)
			}
		}
	} else if gemini.body["provider"] != catalog.GeminiProvider {
		t.Fatalf("live gemini: %+v", gemini.body)
	}

	if code := postStatus(t, server.URL+"/admin/providers", "m7_admin", map[string]any{"slug": "ops-echo", "name": "Ops Echo"}); code != http.StatusConflict {
		t.Fatalf("create provider without confirm should be 409, got %d", code)
	}
	created := postJSONRaw(t, server.URL+"/admin/providers", "m7_admin", map[string]any{
		"name": "Ops Echo", "slug": "ops-echo-" + strconv.FormatInt(time.Now().UnixNano(), 10), "adapter": "test",
	})
	prov := created["item"].(map[string]any)
	if prov["slug"] == nil || prov["adapter"] != "test" {
		t.Fatalf("create provider: %+v", created)
	}
	if code := patchStatus(t, server.URL+fmt.Sprintf("/admin/providers/%s", prov["id"]), "m7_admin", map[string]any{"status": "maintenance", "rpm_limit": 30}); code != http.StatusConflict {
		t.Fatalf("patch provider without confirm should be 409, got %d", code)
	}
	patched := patchJSONRaw(t, server.URL+fmt.Sprintf("/admin/providers/%s", prov["id"]), "m7_admin", map[string]any{"status": "maintenance", "rpm_limit": 30})
	patchedItem := patched["item"].(map[string]any)
	if patchedItem["status"] != "maintenance" {
		t.Fatalf("patch provider: %+v", patched)
	}
	if rpm, _ := patchedItem["rpm_limit"].(float64); rpm != 30 {
		t.Fatalf("patch provider rpm: %+v", patched)
	}
	gotProvider := getAuthJSON(t, server.URL+"/admin/providers/"+prov["id"].(string), "m7_admin")["item"].(map[string]any)
	if gotProvider["id"] != prov["id"] || gotProvider["status"] != "maintenance" {
		t.Fatalf("get provider: %+v", gotProvider)
	}
	echoProvider := getAuthJSON(t, server.URL+"/admin/providers/echo-primary", "m7_admin")["item"].(map[string]any)
	if echoProvider["slug"] != catalog.PrimaryProvider {
		t.Fatalf("get provider by slug: %+v", echoProvider)
	}
	echoModels, _ := echoProvider["models"].([]any)
	if len(echoModels) < 2 {
		t.Fatalf("echo-primary should map more than one public model: %+v", echoProvider)
	}
	if code := postStatus(t, server.URL+fmt.Sprintf("/admin/providers/%s/credentials", prov["id"]), "m7_admin", map[string]any{"secret": "sk-no-confirm"}); code != http.StatusConflict {
		t.Fatalf("rotate credential without confirm should be 409, got %d", code)
	}
	cred := postJSONRaw(t, server.URL+fmt.Sprintf("/admin/providers/%s/credentials", prov["id"]), "m7_admin", map[string]any{"secret": "sk-sandbox"})
	if cred["credential_ref"] == nil {
		t.Fatalf("rotate credential: %+v", cred)
	}
	if _, leaked := cred["secret"]; leaked {
		t.Fatalf("rotate must not echo plaintext: %+v", cred)
	}
	if code := postStatusConfirm(t, server.URL+"/admin/providers", "m7_admin", map[string]any{
		"name": "ssrf", "slug": "ssrf-" + strconv.FormatInt(time.Now().UnixNano(), 10), "adapter": "openai", "base_url": "http://169.254.169.254/",
	}); code != http.StatusBadRequest {
		t.Fatalf("metadata base url should be 400, got %d", code)
	}
	tlsOK, err := http.Get(server.URL + "/v1/public/tls-check?domain=oem.localhost")
	if err != nil {
		t.Fatal(err)
	}
	tlsOK.Body.Close()
	if tlsOK.StatusCode != http.StatusOK {
		t.Fatalf("oem host should be known for CNAME/TLS, got %d", tlsOK.StatusCode)
	}
	tlsBad, err := http.Get(server.URL + "/v1/public/tls-check?domain=evil.example")
	if err != nil {
		t.Fatal(err)
	}
	tlsBad.Body.Close()
	if tlsBad.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown host must not get a cert, got %d", tlsBad.StatusCode)
	}
	if code := postStatus(t, server.URL+"/admin/brands/"+identity.OEMBrandID+"/tls/issue", "m7_admin", map[string]any{}); code != http.StatusConflict {
		t.Fatalf("issue tls without confirm should be 409, got %d", code)
	}
	issued := postJSONRaw(t, server.URL+"/admin/brands/"+identity.OEMBrandID+"/tls/issue", "m7_admin", map[string]any{})
	tlsItem := issued["item"].(map[string]any)
	if tlsItem["tls_status"] != "issued" {
		t.Fatalf("issue oem tls: %+v", issued)
	}
	if issuer, _ := tlsItem["tls_issuer"].(string); issuer != identity.IssuerSandbox && issuer != "" {
		t.Fatalf("oem.localhost must stay sandbox issuer, got %q", issuer)
	}
	if code := getStatus(t, server.URL+"/.well-known/acme-challenge/missing", ""); code != http.StatusNotFound {
		t.Fatalf("unknown ACME token should 404, got %d", code)
	}
	models := getAuthJSON(t, server.URL+"/admin/models", "m7_admin")["items"].([]any)
	if len(models) == 0 {
		t.Fatal("admin models empty")
	}
	if code := postStatus(t, server.URL+"/admin/models", "m7_admin", map[string]any{
		"public_id": "tokenhub/ops-noconfirm", "vendor": "tokenhub",
	}); code != http.StatusConflict {
		t.Fatalf("create model without confirm should be 409, got %d", code)
	}
	draft := postJSONRaw(t, server.URL+"/admin/models", "m7_admin", map[string]any{
		"public_id": "tokenhub/ops-draft-" + strconv.FormatInt(time.Now().UnixNano(), 10), "vendor": "tokenhub", "display_name": "Ops Draft", "status": "published",
	})
	if item := draft["item"].(map[string]any); item["status"] != catalog.SyncDraft || item["sync_state"] != catalog.SyncDraft {
		t.Fatalf("create model must stay draft: %+v", draft)
	}
	publicID := draft["item"].(map[string]any)["id"].(string)
	got := getAuthJSON(t, server.URL+"/admin/models/"+publicID, "m7_admin")
	if item, _ := got["item"].(map[string]any); item == nil || item["id"] != publicID {
		t.Fatalf("get admin model: %+v", got)
	}
	echo := getAuthJSON(t, server.URL+"/admin/models/"+catalog.EchoModelID, "m7_admin")
	if item, _ := echo["item"].(map[string]any); item == nil || item["id"] != catalog.EchoModelID {
		t.Fatalf("get echo model by public id with slash: %+v", echo)
	}
	if code := patchStatus(t, server.URL+"/admin/models/"+publicID, "m7_admin", map[string]any{
		"display_name": "Ops Draft Edited",
	}); code != http.StatusConflict {
		t.Fatalf("patch model without confirm should be 409, got %d", code)
	}
	modelPatched := patchJSONRaw(t, server.URL+"/admin/models/"+publicID, "m7_admin", map[string]any{
		"display_name": "Ops Draft Edited",
		"capabilities": map[string]any{"supported_parameters": []string{"stream", "tools"}},
	})
	if modelPatched["item"].(map[string]any)["display_name"] != "Ops Draft Edited" {
		t.Fatalf("patch model: %+v", modelPatched)
	}
	priced := postJSONRaw(t, server.URL+"/admin/price-books", "m7_admin", map[string]any{
		"model": publicID, "input": "0.000003", "output": "0.000006", "currency": "USD",
	})
	if priced["price"] == nil {
		t.Fatalf("publish price on draft model: %+v", priced)
	}
	afterPrice := getAuthJSON(t, server.URL+"/admin/models/"+publicID, "m7_admin")
	sell, _ := afterPrice["item"].(map[string]any)["sell_price"].(map[string]any)
	if fmt.Sprint(sell["input"]) != "0.000003" {
		t.Fatalf("sell price after publish: %+v", afterPrice)
	}
	if code := postStatus(t, server.URL+"/admin/routes", "m7_admin", map[string]any{
		"public_model_id": publicID, "strategy": "priority",
	}); code != http.StatusConflict {
		t.Fatalf("create route without confirm should be 409, got %d", code)
	}
	createdRoute := postJSONRaw(t, server.URL+"/admin/routes", "m7_admin", map[string]any{
		"public_model_id": publicID, "strategy": "priority", "status": "active",
		"candidates": []map[string]any{{"provider_id": "prd_echo_primary", "priority": 1, "weight": 1}},
	})
	routeID := createdRoute["item"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(routeID, "rg_") {
		t.Fatalf("create route: %+v", createdRoute)
	}
	if code := patchStatus(t, server.URL+"/admin/routes/"+routeID, "m7_admin", map[string]any{"strategy": "health"}); code != http.StatusConflict {
		t.Fatalf("patch route without confirm should be 409, got %d", code)
	}
	patchedRoute := patchJSONRaw(t, server.URL+"/admin/routes/"+routeID, "m7_admin", map[string]any{"strategy": "health"})
	if patchedRoute["item"].(map[string]any)["strategy"] != "health" {
		t.Fatalf("patch route: %+v", patchedRoute)
	}
	if code := postStatus(t, server.URL+"/admin/commissions/recalc", "m7_admin", map[string]any{"usage_event_id": "usg_missing"}); code != http.StatusConflict {
		t.Fatalf("recalc without confirm should be 409, got %d", code)
	}
	routes := getAuthJSON(t, server.URL+"/admin/routes", "m7_admin")["items"].([]any)
	if len(routes) == 0 {
		t.Fatal("admin routes empty")
	}
	chCode := "ops-lab-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if code := postStatus(t, server.URL+"/admin/channels", "m7_admin", map[string]any{"code": chCode, "type": "B"}); code != http.StatusConflict {
		t.Fatalf("create channel without confirm should be 409, got %d", code)
	}
	ch := postJSONRaw(t, server.URL+"/admin/channels", "m7_admin", map[string]any{"code": chCode, "type": "B"})
	if ch["item"].(map[string]any)["code"] != chCode {
		t.Fatalf("create channel: %+v", ch)
	}
	chID := ch["item"].(map[string]any)["id"].(string)
	if code := patchStatus(t, server.URL+"/admin/channels/"+chID, "m7_admin", map[string]any{"status": "disabled"}); code != http.StatusConflict {
		t.Fatalf("patch channel without confirm should be 409, got %d", code)
	}
	patchedCh := patchJSONRaw(t, server.URL+"/admin/channels/"+chID, "m7_admin", map[string]any{"status": "disabled"})
	if patchedCh["item"].(map[string]any)["status"] != "disabled" {
		t.Fatalf("patch channel: %+v", patchedCh)
	}
	createdPlan := postJSONRaw(t, server.URL+"/admin/plans", "m7_admin", map[string]any{
		"name": "Ops Lab Plan", "owner_type": "platform", "price_minor": 1_000_000,
		"items": []map[string]any{{"unit_type": "usd_credit", "included_amount": 1_000_000}},
	})
	planID, _ := createdPlan["item"].(map[string]any)["id"].(string)
	if !strings.HasPrefix(planID, "pln_") || createdPlan["item"].(map[string]any)["status"] != "published" {
		t.Fatalf("create platform plan: %+v", createdPlan)
	}
	archived := patchJSONRaw(t, server.URL+"/admin/plans/"+planID, "m7_admin", map[string]any{"status": "archived"})
	if archived["item"].(map[string]any)["status"] != "archived" {
		t.Fatalf("archive plan: %+v", archived)
	}
	if code := postStatus(t, server.URL+"/admin/subscriptions/sub_missing/force-period-end", "m7_admin", map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("force period end missing sub should be 400, got %d", code)
	}
	renewed := postJSONRaw(t, server.URL+"/admin/subscriptions/process-renewals", "m7_admin", map[string]any{})
	if _, ok := renewed["processed"]; !ok {
		t.Fatalf("process renewals: %+v", renewed)
	}
	keys := getAuthJSON(t, server.URL+"/channel/api-keys", "m7_admin-b")["items"].([]any)
	if len(keys) == 0 {
		t.Fatal("channel api keys empty")
	}
	firstKey, _ := keys[0].(map[string]any)
	if firstKey["id"] == nil || firstKey["id"] == "" || firstKey["prefix"] == nil || firstKey["prefix"] == "" || firstKey["status"] == nil || firstKey["status"] == "" {
		t.Fatalf("channel api key list fields incomplete: %+v", firstKey)
	}
	if code := getStatus(t, server.URL+"/admin/api-keys", "m7_admin"); code != http.StatusGone {
		t.Fatalf("admin api-keys should be 410, got %d", code)
	}

	setup := postJSONRaw(t, server.URL+"/admin/me/2fa/setup", "m7_admin", map[string]any{})
	secret, _ := setup["item"].(map[string]any)["secret"].(string)
	if secret == "" {
		t.Fatalf("2fa setup: %+v", setup)
	}
	code := identity.GenerateTOTP(secret)
	enabled := postJSONRaw(t, server.URL+"/admin/me/2fa/enable", "m7_admin", map[string]any{"code": code})
	if enabled["status"] != "enabled" {
		t.Fatalf("2fa enable: %+v", enabled)
	}
	if status := postStatus(t, server.URL+"/admin/refunds", "m7_admin", map[string]any{"request_id": "missing"}); status != http.StatusConflict {
		t.Fatalf("enrolled 2fa without totp should still 409, got %d", status)
	}
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/refunds", bytes.NewReader(mustJSON(map[string]any{"request_id": "missing"})))
	req.Header.Set("Authorization", "Bearer m7_admin")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("confirm without totp after enroll should be 409, got %d", resp.StatusCode)
	}
	req2, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/refunds", bytes.NewReader(mustJSON(map[string]any{"request_id": "missing"})))
	req2.Header.Set("Authorization", "Bearer m7_admin")
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Tokenhub-Confirm", "1")
	req2.Header.Set("X-Tokenhub-TOTP", identity.GenerateTOTP(secret))
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode == http.StatusConflict {
		t.Fatalf("confirm+totp should pass confirm gate, got %d", resp2.StatusCode)
	}
	if code := postStatus(t, server.URL+"/admin/me/2fa/disable", "m7_admin", map[string]any{}); code != http.StatusConflict {
		t.Fatalf("disable 2fa without confirm should be 409, got %d", code)
	}
	reqOff, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/me/2fa/disable", bytes.NewReader(mustJSON(map[string]any{})))
	reqOff.Header.Set("Authorization", "Bearer m7_admin")
	reqOff.Header.Set("Content-Type", "application/json")
	reqOff.Header.Set("X-Tokenhub-Confirm", "1")
	respOff, err := http.DefaultClient.Do(reqOff)
	if err != nil {
		t.Fatal(err)
	}
	_ = respOff.Body.Close()
	if respOff.StatusCode != http.StatusConflict {
		t.Fatalf("disable 2fa without totp after enroll should be 409, got %d", respOff.StatusCode)
	}
	reqOff2, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/me/2fa/disable", bytes.NewReader(mustJSON(map[string]any{})))
	reqOff2.Header.Set("Authorization", "Bearer m7_admin")
	reqOff2.Header.Set("Content-Type", "application/json")
	reqOff2.Header.Set("X-Tokenhub-Confirm", "1")
	reqOff2.Header.Set("X-Tokenhub-TOTP", identity.GenerateTOTP(secret))
	respOff2, err := http.DefaultClient.Do(reqOff2)
	if err != nil {
		t.Fatal(err)
	}
	defer respOff2.Body.Close()
	if respOff2.StatusCode != http.StatusOK {
		t.Fatalf("disable 2fa with confirm+totp should pass, got %d", respOff2.StatusCode)
	}
	totp := getAuthJSON(t, server.URL+"/admin/me/2fa", "m7_admin")["item"].(map[string]any)
	if totp["enabled"] == true || totp["status"] != "disabled" {
		t.Fatalf("2fa should be disabled: %+v", totp)
	}

	createdKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "lifecycle"})
	life := createdKey["item"].(map[string]any)
	oldSecret := life["key"].(string)
	keyID := life["id"].(string)
	rotated := postJSONRaw(t, server.URL+"/v1/me/api-keys/"+keyID+"/rotate", session, map[string]any{})
	newSecret := rotated["item"].(map[string]any)["key"].(string)
	if newSecret == "" || newSecret == oldSecret {
		t.Fatalf("rotate should return a new secret: %+v", rotated)
	}
	if code := postStatus(t, server.URL+"/v1/chat/completions", oldSecret, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "old"}},
	}); code != http.StatusForbidden {
		t.Fatalf("rotated old key should be 403, got %d", code)
	}
	if chat := postJSONRaw(t, server.URL+"/v1/chat/completions", newSecret, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "new"}},
	}); chat["request_id"] == nil {
		t.Fatalf("rotated key should work: %+v", chat)
	}
	_ = postJSONRaw(t, server.URL+"/v1/me/api-keys/"+keyID+"/disable", session, map[string]any{})
	if code := postStatus(t, server.URL+"/v1/chat/completions", newSecret, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "off"}},
	}); code != http.StatusForbidden {
		t.Fatalf("disabled key should be 403, got %d", code)
	}
	adminBan := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "admin-disable"})
	adminBanID := adminBan["item"].(map[string]any)["id"].(string)
	adminBanSecret := adminBan["item"].(map[string]any)["key"].(string)
	if code := postStatus(t, server.URL+"/admin/api-keys/"+adminBanID+"/disable", "m7_admin", map[string]any{}); code != http.StatusGone {
		t.Fatalf("admin disable key should be 410, got %d", code)
	}
	if code := postStatus(t, server.URL+"/channel/api-keys/"+adminBanID+"/disable", "m7_admin-b", map[string]any{}); code != http.StatusConflict {
		t.Fatalf("channel disable key without confirm should be 409, got %d", code)
	}
	banned := postJSONRaw(t, server.URL+"/channel/api-keys/"+adminBanID+"/disable", "m7_admin-b", map[string]any{})
	if banned["item"].(map[string]any)["status"] != "disabled" {
		t.Fatalf("channel disable key: %+v", banned)
	}
	if code := postStatus(t, server.URL+"/v1/chat/completions", adminBanSecret, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "admin-off"}},
	}); code != http.StatusForbidden {
		t.Fatalf("channel-disabled key should be 403, got %d", code)
	}
	expKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "exp"})
	expID := expKey["item"].(map[string]any)["id"].(string)
	expSecret := expKey["item"].(map[string]any)["key"].(string)
	_ = postJSONRaw(t, server.URL+"/v1/me/api-keys/"+expID+"/expire", session, map[string]any{"expires_at": time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)})
	if code := postStatus(t, server.URL+"/v1/chat/completions", expSecret, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "exp"}},
	}); code != http.StatusForbidden {
		t.Fatalf("expired key should be 403, got %d", code)
	}
	reqExp, _ := http.NewRequest(http.MethodGet, server.URL+"/admin/billing/export", nil)
	reqExp.Header.Set("Authorization", "Bearer m7_admin")
	respExp, err := http.DefaultClient.Do(reqExp)
	if err != nil {
		t.Fatal(err)
	}
	defer respExp.Body.Close()
	raw, _ := io.ReadAll(respExp.Body)
	if respExp.StatusCode != http.StatusOK || !strings.Contains(string(raw), "gross_profit") {
		t.Fatalf("billing export: %d %s", respExp.StatusCode, string(raw))
	}

	finance := "m7_admin-finance"
	opsTok := "m7_admin-ops"
	tech := "m7_admin-tech"
	audit := "m7_admin-audit"
	if me := getAuthJSON(t, server.URL+"/admin/me", finance); !hasRole(me, "finance_admin") {
		t.Fatalf("finance bootstrap: %+v", me)
	}
	if getStatus(t, server.URL+"/admin/providers", finance) != http.StatusForbidden {
		t.Fatal("finance must not list providers")
	}
	if getStatus(t, server.URL+"/admin/providers/prd_echo_primary", finance) != http.StatusForbidden {
		t.Fatal("finance must not read a provider")
	}
	if getStatus(t, server.URL+"/admin/providers/prd_echo_primary", tech) != http.StatusOK {
		t.Fatal("tech should read a provider")
	}
	if getStatus(t, server.URL+"/admin/billing/export", finance) != http.StatusOK {
		t.Fatal("finance should export billing")
	}
	if postStatusConfirm(t, server.URL+"/admin/providers/prd_echo_primary/credentials", finance, map[string]any{"secret": "sk-x"}) != http.StatusForbidden {
		t.Fatal("finance must not rotate provider credentials")
	}
	if postStatusConfirm(t, server.URL+"/admin/refunds", opsTok, map[string]any{"request_id": "missing"}) != http.StatusForbidden {
		t.Fatal("ops must not refund")
	}
	if postStatusConfirm(t, server.URL+"/admin/refunds", tech, map[string]any{"request_id": "missing"}) != http.StatusForbidden {
		t.Fatal("tech must not refund")
	}
	if postStatusConfirm(t, server.URL+"/admin/refunds", audit, map[string]any{"request_id": "missing"}) != http.StatusForbidden {
		t.Fatal("audit must not write refunds")
	}
	if getStatus(t, server.URL+"/admin/audit-logs", audit) != http.StatusOK {
		t.Fatal("audit should read logs")
	}
	if probe := postJSONRaw(t, server.URL+"/admin/providers/prd_echo_primary/health-check", tech, map[string]any{}); probe["health"] == nil {
		t.Fatalf("tech health-check: %+v", probe)
	}

	poolSlug := "pool-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	pool := postJSONRaw(t, server.URL+"/admin/providers", tech, map[string]any{
		"name": "Account Pool", "slug": poolSlug, "adapter": "test",
	})
	poolID := pool["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, server.URL+"/admin/models/attach", tech, map[string]any{
		"public_id": catalog.EchoModelID, "provider_id": poolID, "upstream_model_id": "echo-upstream",
	})
	if getStatus(t, server.URL+"/admin/providers/"+poolID+"/accounts", finance) != http.StatusForbidden {
		t.Fatal("finance must not list provider accounts")
	}
	cool := postJSONRaw(t, server.URL+"/admin/providers/"+poolID+"/accounts", tech, map[string]any{
		"secret": "sk-cool", "label": "cooling",
	})
	coolItem := cool["item"].(map[string]any)
	if coolItem["fingerprint"] == nil || coolItem["ciphertext"] != nil {
		t.Fatalf("account must show fingerprint only: %+v", cool)
	}
	coolID := coolItem["id"].(string)
	_ = patchJSONRaw(t, server.URL+"/admin/providers/"+poolID+"/accounts/"+coolID, tech, map[string]any{
		"cooldown_seconds": 120,
	})
	if code := postStatusHeader(t, server.URL+"/v1/chat/completions?provider.only="+poolSlug, key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "cool"}},
	}, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("cooldown-only pool should 503, got %d", code)
	}
	_ = postJSONRaw(t, server.URL+"/admin/providers/"+poolID+"/accounts", tech, map[string]any{
		"secret": "sk-hot", "label": "hot",
	})
	viaPool := postJSONRaw(t, server.URL+"/v1/chat/completions?provider.only="+poolSlug, key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "hot"}},
	})
	if viaPool["provider"] != poolSlug {
		t.Fatalf("active account should route: %+v", viaPool)
	}
	if err := application.Catalog.RecordAccountOutcome(context.Background(), coolID, 401); err != nil {
		t.Fatal(err)
	}
	listed := getAuthJSON(t, server.URL+"/admin/providers/"+poolID+"/accounts?q=cooling", tech)
	items, _ = listed["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != catalog.AccountInvalid {
		t.Fatalf("401 should mark account invalid: %+v", listed)
	}

	own := postJSONRaw(t, server.URL+"/admin/models", "m7_admin", map[string]any{
		"public_id": "tokenhub/review-own-" + strconv.FormatInt(time.Now().UnixNano(), 10), "vendor": "tokenhub", "display_name": "Own Draft",
	})
	ownID := own["item"].(map[string]any)["id"].(string)
	if code := postStatusConfirm(t, server.URL+"/admin/models/review", "m7_admin", map[string]any{
		"public_id": ownID, "action": "approve",
	}); code != http.StatusConflict {
		t.Fatalf("creator must not review own model, got %d", code)
	}
	if code := postStatusConfirm(t, server.URL+"/admin/models/publish", "m7_admin-ops", map[string]any{
		"public_id": ownID,
	}); code != http.StatusConflict {
		t.Fatalf("publish without review should 409, got %d", code)
	}
	pending := getAuthJSON(t, server.URL+"/admin/models?sync_state=draft&q="+ownID, "m7_admin-ops")
	if len(pending["items"].([]any)) == 0 {
		t.Fatalf("draft queue missing model: %+v", pending)
	}
	rej := postJSONRaw(t, server.URL+"/admin/models", "m7_admin", map[string]any{
		"public_id": "tokenhub/review-rej-" + strconv.FormatInt(time.Now().UnixNano(), 10), "vendor": "tokenhub", "display_name": "Reject Me",
	})
	rejID := rej["item"].(map[string]any)["id"].(string)
	rejected := postJSONRaw(t, server.URL+"/admin/models/review", "m7_admin-ops", map[string]any{
		"public_id": rejID, "action": "reject",
	})
	if rejected["item"].(map[string]any)["sync_state"] != catalog.SyncRejected {
		t.Fatalf("reject: %+v", rejected)
	}
	if code := postStatusConfirm(t, server.URL+"/admin/models/publish", "m7_admin-ops", map[string]any{
		"public_id": rejID,
	}); code != http.StatusConflict {
		t.Fatalf("rejected model must not publish, got %d", code)
	}
	approvedOwn := postJSONRaw(t, server.URL+"/admin/models/review", "m7_admin-ops", map[string]any{
		"public_id": ownID, "action": "approve",
	})
	if approvedOwn["item"].(map[string]any)["sync_state"] != catalog.SyncReviewed {
		t.Fatalf("ops review of admin draft: %+v", approvedOwn)
	}
	if code := postStatusConfirm(t, server.URL+"/admin/models/publish", "m7_admin", map[string]any{
		"public_id": ownID,
	}); code != http.StatusConflict {
		t.Fatalf("creator must not publish own model, got %d", code)
	}
	publishedOwn := postJSONRaw(t, server.URL+"/admin/models/publish", "m7_admin-ops", map[string]any{"public_id": ownID})
	if publishedOwn["item"].(map[string]any)["status"] != catalog.SyncPublished {
		t.Fatalf("ops publish after review: %+v", publishedOwn)
	}

	synced := postJSONRaw(t, server.URL+"/admin/providers/"+poolID+"/sync", tech, map[string]any{})
	syncItem := synced["item"].(map[string]any)
	syncModels := syncItem["items"].([]any)
	if len(syncModels) == 0 || syncModels[0].(map[string]any)["status"] != catalog.SyncDraft {
		t.Fatalf("sync should create draft: %+v", synced)
	}
	syncPublic := syncModels[0].(map[string]any)["id"].(string)
	visible := getAuthJSON(t, server.URL+"/v1/models", key)
	for _, raw := range visible["data"].([]any) {
		if raw.(map[string]any)["id"] == syncPublic {
			t.Fatalf("draft model must not be in customer catalog: %+v", visible)
		}
	}
	if code := postStatusConfirm(t, server.URL+"/admin/models/review", tech, map[string]any{
		"public_id": syncPublic, "action": "approve",
	}); code != http.StatusForbidden {
		t.Fatalf("tech must not review models, got %d", code)
	}
	reviewed := postJSONRaw(t, server.URL+"/admin/models/review", "m7_admin-ops", map[string]any{
		"public_id": syncPublic, "action": "approve",
	})
	if reviewed["item"].(map[string]any)["sync_state"] != catalog.SyncReviewed {
		t.Fatalf("review: %+v", reviewed)
	}
	published := postJSONRaw(t, server.URL+"/admin/models/publish", "m7_admin-ops", map[string]any{"public_id": syncPublic})
	if published["item"].(map[string]any)["status"] != catalog.SyncPublished {
		t.Fatalf("publish: %+v", published)
	}
	after := getAuthJSON(t, server.URL+"/v1/models", key)
	foundSync := false
	for _, raw := range after["data"].([]any) {
		if raw.(map[string]any)["id"] == syncPublic {
			foundSync = true
		}
	}
	if !foundSync {
		t.Fatalf("published sync model missing from catalog: %+v", after)
	}
	deprecated := postJSONRaw(t, server.URL+"/admin/models/deprecate", "m7_admin-ops", map[string]any{"public_id": syncPublic})
	if deprecated["item"].(map[string]any)["status"] != "deprecated" {
		t.Fatalf("deprecate: %+v", deprecated)
	}
	gone := getAuthJSON(t, server.URL+"/v1/models", key)
	for _, raw := range gone["data"].([]any) {
		if raw.(map[string]any)["id"] == syncPublic {
			t.Fatalf("deprecated model still visible: %+v", gone)
		}
	}
	adminStill := getAuthJSON(t, server.URL+"/admin/models?q=sync-", "m7_admin-ops")
	if len(adminStill["items"].([]any)) == 0 {
		t.Fatalf("deprecated model must remain in admin list: %+v", adminStill)
	}

	idem := "chat-idem-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	firstChat := doChatHeader(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "idem"}},
	}, map[string]string{"Idempotency-Key": idem})
	secondChat := doChatHeader(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "idem"}},
	}, map[string]string{"Idempotency-Key": idem})
	if firstChat["request_id"] != secondChat["request_id"] {
		t.Fatalf("idempotent chat should replay: %v vs %v", firstChat["request_id"], secondChat["request_id"])
	}
	if code := postStatusHeader(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "other"}},
	}, map[string]string{"Idempotency-Key": idem}); code != http.StatusConflict {
		t.Fatalf("mismatched idempotency body should 409, got %d", code)
	}

	slug := "bifrost-lab-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	prd := postJSONRaw(t, server.URL+"/admin/providers", tech, map[string]any{
		"name": "Bifrost Lab", "slug": slug, "adapter": "bifrost",
	})
	prdID := prd["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, server.URL+"/admin/models/attach", tech, map[string]any{
		"public_id": catalog.EchoModelID, "provider_id": prdID, "upstream_model_id": "echo-upstream",
	})
	viaBifrost := postJSONRaw(t, server.URL+"/v1/chat/completions?provider.only="+slug, key, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "sidecar"}},
	})
	content, _ := firstContentOf(viaBifrost)
	if !strings.Contains(content, "echo:sidecar") {
		t.Fatalf("test harness bifrost reply should echo: %+v", viaBifrost)
	}

	if application.Media.StoreStatus(context.Background()).OK {
		fromSession := postAccepted(t, server.URL+"/v1/videos", session, "sess-m7", map[string]any{
			"model": catalog.SeedanceModelID, "prompt": "from console",
		})
		if fromSession["id"] == nil {
			t.Fatalf("session media create: %+v", fromSession)
		}
	}

	frozenCode := "freeze-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	frozenCh := postJSONRaw(t, server.URL+"/admin/channels", "m7_admin", map[string]any{
		"code": frozenCode, "type": "B", "status": "active",
	})
	frozenID := frozenCh["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, server.URL+"/admin/channel-quotas/grant", "m7_admin", map[string]any{
		"channel_org_id": frozenID, "amount_minor": 100 * billing.MinorPerUSD,
	})
	promo := "THX-FZ-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_ = postJSONRaw(t, server.URL+"/admin/promotion-codes", "m7_admin", map[string]any{
		"channel_org_id": frozenID, "code": promo,
	})
	frozenReg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "freeze-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": promo,
	})
	if channelOf(frozenReg) != frozenID {
		t.Fatalf("freeze user should bind new channel: %+v", frozenReg)
	}
	frozenSession := tokenOf(frozenReg)
	_ = postJSONRaw(t, server.URL+"/v1/topups/redeem", frozenSession, map[string]any{"code": billing.RedeemE2E})
	frozenKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", frozenSession, map[string]any{"name": "freeze"})["item"].(map[string]any)["key"].(string)
	if chat := postJSONRaw(t, server.URL+"/v1/chat/completions", frozenKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "before-disable"}},
	}); chat["request_id"] == nil {
		t.Fatalf("active channel should still chat: %+v", chat)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/channels/"+frozenID, "m7_admin", map[string]any{"status": "disabled"})
	blocked := mustStatusBody(t, http.MethodPost, server.URL+"/v1/chat/completions", frozenKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "after-disable"}},
	})
	if blocked.status != http.StatusForbidden {
		t.Fatalf("disabled channel chat should 403, got %d %+v", blocked.status, blocked.body)
	}
	errBody, _ := blocked.body["error"].(map[string]any)
	if errBody["code"] != "channel_disabled" {
		t.Fatalf("disabled channel error: %+v", blocked.body)
	}
	if getAuthJSON(t, server.URL+"/v1/me/balance", frozenSession)["balance"] == nil {
		t.Fatal("disabled channel must keep readable balance")
	}
	if len(getAuthJSON(t, server.URL+"/v1/me/usage", frozenSession)["items"].([]any)) == 0 {
		t.Fatal("disabled channel must keep usage history")
	}
	mediaBlocked := mustStatusBody(t, http.MethodPost, server.URL+"/v1/videos", frozenSession, map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "frozen",
	})
	if mediaBlocked.status != http.StatusForbidden {
		t.Fatalf("disabled channel media should 403, got %d %+v", mediaBlocked.status, mediaBlocked.body)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/channels/"+frozenID, "m7_admin", map[string]any{"status": "active"})
	if chat := postJSONRaw(t, server.URL+"/v1/chat/completions", frozenKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "reenabled"}},
	}); chat["request_id"] == nil {
		t.Fatalf("re-enabled channel should chat: %+v", chat)
	}
}

func postStatus(t *testing.T, url, token string, payload map[string]any) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func patchStatus(t *testing.T, url, token string, payload map[string]any) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPatch, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func postStatusConfirm(t *testing.T, url, token string, payload map[string]any) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func getStatus(t *testing.T, url, token string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func postStatusHeader(t *testing.T, url, token string, payload map[string]any, headers map[string]string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func hasRole(me map[string]any, code string) bool {
	roles, _ := me["roles"].([]any)
	for _, role := range roles {
		if role == code {
			return true
		}
	}
	return false
}

func TestACMELocalhostStaysSandbox(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "acme_sandbox_admin"
	cfg.BootstrapUser = "acme_sandbox_user"
	cfg.ACMEDirectory = "https://127.0.0.1:1/dir"
	cfg.ACMEInsecureSkipVerify = true
	cfg.ACMEForce = false
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	issued := postJSONRaw(t, server.URL+"/admin/brands/"+identity.OEMBrandID+"/tls/issue", "acme_sandbox_admin", map[string]any{})
	item := issued["item"].(map[string]any)
	if item["tls_status"] != "issued" {
		t.Fatalf("sandbox issue: %+v", issued)
	}
	if issuer, _ := item["tls_issuer"].(string); issuer != identity.IssuerSandbox && issuer != "" {
		t.Fatalf("directory set must not ACME .localhost without FORCE, got %q", issuer)
	}
}

func TestACMEIssueFailsBadDirectory(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "acme_fail_admin"
	cfg.BootstrapUser = "acme_fail_user"
	cfg.ACMEDirectory = "https://127.0.0.1:1/dir"
	cfg.ACMEInsecureSkipVerify = true
	cfg.ACMEForce = true
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	code := postStatusConfirm(t, server.URL+"/admin/brands/"+identity.OEMBrandID+"/tls/issue", "acme_fail_admin", map[string]any{})
	if code != http.StatusBadGateway {
		t.Fatalf("forced ACME against a dead directory should 502, got %d", code)
	}
}

func firstContentOf(body map[string]any) (string, bool) {
	choices, _ := body["choices"].([]any)
	if len(choices) == 0 {
		return "", false
	}
	choice, _ := choices[0].(map[string]any)
	msg, _ := choice["message"].(map[string]any)
	text, _ := msg["content"].(string)
	return text, text != ""
}

func doChatHeader(t *testing.T, url, token string, payload map[string]any, headers map[string]string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(payload)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
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
