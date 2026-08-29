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
	"github.com/tokpath/tokstudio/backend/internal/gateway"
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
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	bifrost := httptest.NewServer(gateway.SandboxHandler())
	defer bifrost.Close()
	cfg.BifrostURL = bifrost.URL
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
	dash := getAuthJSON(t, server.URL+"/admin/ops/dashboard", "m7_admin")["dashboard"].(map[string]any)
	totals := dash["totals"].(map[string]any)
	if totals["revenue_minor"] == nil || totals["gross_profit_minor"] == nil || totals["success_rate"] == nil || totals["low_balance_wallets"] == nil {
		t.Fatalf("totals incomplete: %+v", totals)
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

	_ = postJSONRaw(t, server.URL+"/admin/audit-probes", "m7_admin", map[string]any{})
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
	_ = postJSONRaw(t, server.URL+"/admin/ops/alerts/evaluate", "m7_admin", map[string]any{})
	if books := getAuthJSON(t, server.URL+"/admin/ops/runbooks", "m7_admin")["items"].([]any); len(books) == 0 {
		t.Fatal("runbooks missing")
	}

	gemini := postJSONRaw(t, server.URL+"/v1/chat/completions", key, map[string]any{
		"model": catalog.GeminiModelID, "messages": []map[string]string{{"role": "user", "content": "hi-gemini"}},
	})
	if gemini["provider"] != catalog.GeminiProvider {
		t.Fatalf("gemini sandbox: %+v", gemini)
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
	patched := patchJSONRaw(t, server.URL+fmt.Sprintf("/admin/providers/%s", prov["id"]), "m7_admin", map[string]any{"status": "maintenance", "rpm_limit": 30})
	if patched["item"].(map[string]any)["status"] != "maintenance" {
		t.Fatalf("patch provider: %+v", patched)
	}
	cred := postJSONRaw(t, server.URL+fmt.Sprintf("/admin/providers/%s/credentials", prov["id"]), "m7_admin", map[string]any{"secret": "sk-sandbox"})
	if cred["credential_ref"] == nil {
		t.Fatalf("rotate credential: %+v", cred)
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
	issued := postJSONRaw(t, server.URL+"/admin/brands/"+identity.OEMBrandID+"/tls/issue", "m7_admin", map[string]any{})
	if issued["item"].(map[string]any)["tls_status"] != "issued" {
		t.Fatalf("issue oem tls: %+v", issued)
	}
	models := getAuthJSON(t, server.URL+"/admin/models", "m7_admin")["items"].([]any)
	if len(models) == 0 {
		t.Fatal("admin models empty")
	}
	draft := postJSONRaw(t, server.URL+"/admin/models", "m7_admin", map[string]any{
		"public_id": "tokenhub/ops-draft-" + strconv.FormatInt(time.Now().UnixNano(), 10), "vendor": "tokenhub", "display_name": "Ops Draft", "status": "draft",
	})
	if draft["item"].(map[string]any)["status"] != "draft" {
		t.Fatalf("create model: %+v", draft)
	}
	routes := getAuthJSON(t, server.URL+"/admin/routes", "m7_admin")["items"].([]any)
	if len(routes) == 0 {
		t.Fatal("admin routes empty")
	}
	chCode := "ops-lab-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	ch := postJSONRaw(t, server.URL+"/admin/channels", "m7_admin", map[string]any{"code": chCode, "type": "B"})
	if ch["item"].(map[string]any)["code"] != chCode {
		t.Fatalf("create channel: %+v", ch)
	}
	keys := getAuthJSON(t, server.URL+"/admin/api-keys", "m7_admin")["items"].([]any)
	if len(keys) == 0 {
		t.Fatal("admin api keys empty")
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
	items, _ := listed["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != catalog.AccountInvalid {
		t.Fatalf("401 should mark account invalid: %+v", listed)
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
	if !strings.Contains(content, "bifrost:sidecar") {
		t.Fatalf("bifrost sandbox reply: %+v", viaBifrost)
	}

	fromSession := postAccepted(t, server.URL+"/v1/videos", session, "sess-m7", map[string]any{
		"model": catalog.SeedanceModelID, "prompt": "from console",
	})
	if fromSession["id"] == nil {
		t.Fatalf("session media create: %+v", fromSession)
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
