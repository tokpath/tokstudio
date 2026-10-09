package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
	"gorm.io/gorm"
)

func TestKeyLimitsAtomicLifecycle(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "keylimits_admin"
	cfg.BootstrapUser = "keylimits_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	srv := httptest.NewServer(application.Router())
	defer srv.Close()
	ctx := context.Background()
	reg := postBody(t, srv.URL+"/v1/auth/register", "", map[string]string{"email": fmt.Sprintf("keylimits-%d@example.test", time.Now().UnixNano()), "password": "password1", "promotion_code": "THA1"})
	session := tokenOf(reg)
	principal, err := application.Identity.Authenticate(ctx, session)
	if err != nil || principal == nil {
		t.Fatalf("principal %v", err)
	}
	// No account funds are required to create a Key or read model instructions.
	limit := int64(100)
	limits := identity.APIKeyLimits{Name: "finite", ModelMode: "selected", Allowlist: []string{catalog.EchoModelID}, BudgetLimitMinor: &limit}
	key, err := application.Identity.CreateAPIKeyWithLimits(ctx, *principal, cfg.EncryptionKey, limits)
	if err != nil {
		t.Fatal(err)
	}
	if code, body := doJSON(t, "POST", srv.URL+"/v1/me/api-keys", session, false, map[string]any{"name": "bad", "model_mode": "selected", "allowlist": []string{}}); code != 400 {
		t.Fatalf("empty selected %d %+v", code, body)
	}
	if err := application.Billing.Credit(ctx, principal.UserID, id.New("credit"), 1_000_000, "isolated test funds"); err != nil {
		t.Fatal(err)
	}
	prices := json.RawMessage(`{"currency":"USD","input":"0.000001","output":"0.000002"}`)
	reserve := func(request string, amount int64) (*billing.Reservation, error) {
		return application.Billing.Reserve(ctx, billing.ReserveInput{UserID: principal.UserID, ChannelOrgID: principal.ChannelOrgID, APIKeyID: key.ID, RequestID: request, PublicModelID: catalog.EchoModelID, ReserveMinor: amount, BudgetBounded: true, UnitPrices: prices})
	}
	var wg sync.WaitGroup
	results := make(chan struct {
		request string
		err     error
	}, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			request := id.New("req")
			_, err := reserve(request, 60)
			results <- struct {
				request string
				err     error
			}{request, err}
		}()
	}
	wg.Wait()
	close(results)
	accepted := ""
	failures := 0
	for result := range results {
		if result.err == nil {
			if accepted != "" {
				t.Fatal("concurrent requests overspent limit")
			}
			accepted = result.request
		} else {
			if !errors.Is(result.err, identity.ErrKeyBudgetExceeded) {
				t.Fatal(result.err)
			}
			failures++
		}
	}
	if accepted == "" || failures != 1 {
		t.Fatal("expected one atomic reserve")
	}
	low := int64(59)
	edited := limits
	edited.BudgetLimitMinor = &low
	if _, err := application.Identity.UpdateAPIKeyLimits(ctx, *principal, key.ID, edited); !errors.Is(err, identity.ErrKeyBudgetExceeded) {
		t.Fatalf("lower than occupancy: %v", err)
	}
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{UserID: principal.UserID, APIKeyID: key.ID, RequestID: accepted, PublicModelID: catalog.OEMModelID, ReserveMinor: 60, UnitPrices: prices, BudgetBounded: true}); !errors.Is(err, billing.ErrConflict) {
		t.Fatalf("idempotency binding: %v", err)
	}
	// Unknown results remain held across duplicate callbacks, edits, and reaping.
	for i := 0; i < 2; i++ {
		if _, err := application.Billing.Settle(ctx, billing.SettleInput{RequestID: accepted, APIKeyID: key.ID, PublicModelID: catalog.EchoModelID, MissingUsage: true}); err != nil {
			t.Fatal(err)
		}
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 0, 60)
	if _, err := application.Identity.DisableAPIKey(ctx, *principal, key.ID); err != nil {
		t.Fatal(err)
	}
	rotated, err := application.Identity.RotateAPIKey(ctx, *principal, key.ID, cfg.EncryptionKey)
	if err != nil || rotated.Status != "disabled" {
		t.Fatalf("rotation reenabled disabled Key: %+v %v", rotated, err)
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 0, 60)
	if err := application.Billing.Release(ctx, accepted); err != nil {
		t.Fatal(err)
	}
	if err := application.Billing.Release(ctx, accepted); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Identity.EnableAPIKey(ctx, *principal, key.ID); err != nil {
		t.Fatal(err)
	}
	request := id.New("req")
	if _, err := reserve(request, 60); err != nil {
		t.Fatal(err)
	}
	settle := billing.SettleInput{RequestID: request, APIKeyID: key.ID, PublicModelID: catalog.EchoModelID, Usage: map[string]int{"prompt_tokens": 10, "completion_tokens": 5}}
	for i := 0; i < 2; i++ {
		if _, err := application.Billing.Settle(ctx, settle); err != nil {
			t.Fatal(err)
		}
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 20, 0)
	for i := 0; i < 2; i++ {
		if _, err := application.Billing.RefundCharge(ctx, request); err != nil {
			t.Fatal(err)
		}
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 0, 0)
	// A measured excess stays pending with its full usage and occupied budget.
	excess := id.New("req")
	if _, err := reserve(excess, 60); err != nil {
		t.Fatal(err)
	}
	var walletBefore struct{ AvailableMinor, GiftMinor, ReservedMinor int64 }
	application.DB.Table("billing_wallets").Where("user_id=?", principal.UserID).First(&walletBefore)
	excessInput := billing.SettleInput{RequestID: excess, APIKeyID: key.ID, PublicModelID: catalog.EchoModelID, Usage: map[string]int{"prompt_tokens": 120}}
	result, err := application.Billing.Settle(ctx, excessInput)
	if err != nil || result.State != billing.UsagePending || result.AmountMinor != 120 {
		t.Fatalf("excess clipped: %+v %v", result, err)
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 0, 120)
	if replay, err := application.Billing.Settle(ctx, excessInput); err != nil || replay.UsageEventID != result.UsageEventID {
		t.Fatalf("excess replay %+v %v", replay, err)
	}
	excessInput.Usage = map[string]int{"prompt_tokens": 121}
	if _, err := application.Billing.Settle(ctx, excessInput); !errors.Is(err, billing.ErrConflict) {
		t.Fatalf("conflicting measured fact accepted: %v", err)
	}
	var walletAfter struct{ AvailableMinor, GiftMinor, ReservedMinor int64 }
	application.DB.Table("billing_wallets").Where("user_id=?", principal.UserID).First(&walletAfter)
	if walletBefore != walletAfter {
		t.Fatalf("excess added an unapproved wallet debit: %+v -> %+v", walletBefore, walletAfter)
	}
	var pendingCount int64
	application.DB.Table("outbox_events").Where("event_type=? AND aggregate_id=?", "usage.reconciliation.requested", result.UsageEventID).Count(&pendingCount)
	if pendingCount != 1 {
		t.Fatalf("excess reconciliation count %d", pendingCount)
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 0, 120)
	if _, err := reserve(id.New("req"), 1); !errors.Is(err, identity.ErrKeyBudgetExceeded) {
		t.Fatalf("over reserve must block new calls: %v", err)
	}
	if err := application.Billing.Release(ctx, excess); err != nil {
		t.Fatal(err)
	}
	// Expiry becomes visible pending usage and keeps occupancy; an explicit known
	// failure disposition releases it later.
	expiredRequest := id.New("req")
	if _, err := reserve(expiredRequest, 60); err != nil {
		t.Fatal(err)
	}
	if err := application.DB.Exec("UPDATE billing_authorizations SET expires_at=NOW()-INTERVAL '1 minute' WHERE request_id=?", expiredRequest).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := application.Billing.ReapExpired(ctx); err != nil {
		t.Fatal(err)
	}
	usages, err := application.Billing.QueryUsage(ctx, billing.QueryUsageInput{RequestID: expiredRequest})
	if err != nil || len(usages) != 1 || usages[0].State != billing.UsagePending {
		t.Fatalf("expired reserve invisible: %+v %v", usages, err)
	}
	assertBudget(t, application.Identity, ctx, *principal, cfg.EncryptionKey, key.ID, 0, 60)
	if err := application.Billing.Release(ctx, expiredRequest); err != nil {
		t.Fatal(err)
	}
	// Policy failures cannot produce unrestricted principals or half-created Keys.
	callback := "keylimits-policy-read"
	if err := application.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_api_key_model_policies" {
			tx.AddError(errors.New("injected unavailable policy"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Identity.AuthenticateAPIKey(ctx, rotated.Secret); err == nil {
		t.Fatal("policy read failed open")
	}
	application.DB.Callback().Query().Remove(callback)
	callback = "keylimits-policy-write"
	application.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_api_key_model_policies" {
			tx.AddError(errors.New("injected policy failure"))
		}
	})
	broken := limits
	broken.Name = "atomic-failure"
	if _, err := application.Identity.CreateAPIKeyWithLimits(ctx, *principal, cfg.EncryptionKey, broken); err == nil {
		t.Fatal("policy write failure ignored")
	}
	application.DB.Callback().Create().Remove(callback)
	var count int64
	application.DB.Table("identity_api_keys").Where("user_id=? AND name=?", principal.UserID, "atomic-failure").Count(&count)
	if count != 0 {
		t.Fatal("half-created Key survived rollback")
	}
	// Staff roles cannot bypass ownership through the ordinary user endpoint.
	admin, err := application.Identity.Authenticate(ctx, "keylimits_admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Identity.ConfirmOwnedKey(ctx, *admin, key.ID); err == nil {
		t.Fatal("platform role bypassed own-Key scope")
	}
	past := time.Now().Add(-time.Minute)
	edited = limits
	edited.ExpiresAt = &past
	if _, err := application.Identity.UpdateAPIKeyLimits(ctx, *principal, key.ID, edited); err != nil {
		t.Fatal(err)
	}
	if _, err := reserve(id.New("req"), 1); !errors.Is(err, identity.ErrKeyNotUsable) {
		t.Fatalf("expired key accepted: %v", err)
	}
}
func assertBudget(t *testing.T, service *identity.Service, ctx context.Context, p identity.Principal, enc, keyID string, used, reserved int64) {
	t.Helper()
	items, err := service.ListAPIKeys(ctx, p, enc)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == keyID {
			if item.BudgetUsedMinor != used || item.BudgetReservedMinor != reserved {
				t.Fatalf("budget %+v want used=%d reserve=%d", item, used, reserved)
			}
			return
		}
	}
	t.Fatal("key missing")
}

func TestPublicGatewayRejectsInternalControls(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires isolated postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "control_admin"
	cfg.BootstrapUser = "control_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	srv := httptest.NewServer(application.Router())
	defer srv.Close()
	reg := postBody(t, srv.URL+"/v1/auth/register", "", map[string]string{"email": fmt.Sprintf("controls-%d@example.test", time.Now().UnixNano()), "password": "password1", "promotion_code": "THA1"})
	session := tokenOf(reg)
	key := postJSONRaw(t, srv.URL+"/v1/me/api-keys", session, map[string]any{"name": "controls"})["item"].(map[string]any)["key"].(string)
	for _, contentType := range []string{"", "text/plain", "Application/JSON; charset=UTF-8"} {
		for _, field := range []string{"provider", "provider.ignore", "provider_order", "RoUtE_Id", "force_fail", "omit_usage"} {
			raw, _ := json.Marshal(map[string]any{"model": catalog.EchoModelID, field: []string{"private"}})
			req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", bytes.NewReader(raw))
			req.Header.Set("Authorization", "Bearer "+key)
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if resp.StatusCode != 400 || body["error"].(map[string]any)["code"] != "unsupported_control_parameter" {
				t.Fatalf("%s / %s accepted: %+v", contentType, field, body)
			}
		}
	}
	foreignDiagnostic, _ := http.NewRequest("POST", srv.URL+"/admin/diagnostics/chat/completions", bytes.NewBufferString(`{"model":"echo-1"}`))
	foreignDiagnostic.Header.Set("Authorization", "Bearer control_admin")
	foreignDiagnostic.Header.Set("X-Tokenhub-Test-Key", key)
	foreignDiagnostic.Header.Set("Content-Type", "application/json")
	foreignResponse, err := http.DefaultClient.Do(foreignDiagnostic)
	if err != nil {
		t.Fatal(err)
	}
	foreignResponse.Body.Close()
	if foreignResponse.StatusCode != 403 {
		t.Fatalf("staff charged another user's test Key: %d", foreignResponse.StatusCode)
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/videos", "/v1/images/generations", "/v1/images/edits"} {
		for _, control := range []string{"query", "header", "body"} {
			t.Run(path+"/"+control, func(t *testing.T) {
				body := map[string]any{"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "provider.only is ordinary content"}}, "prompt": "hello"}
				suffix := ""
				if control == "query" {
					suffix = "?PrOvIdEr.OnLy=private"
				}
				if control == "body" {
					body["routing"] = map[string]any{"order": []string{"private"}}
				}
				raw, _ := json.Marshal(body)
				req, _ := http.NewRequest("POST", srv.URL+path+suffix, bytes.NewReader(raw))
				req.Header.Set("Authorization", "Bearer "+key)
				req.Header.Set("Content-Type", "application/json")
				if control == "header" {
					req.Header.Set("x-tokenhub-omit-usage", "1")
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				var out map[string]any
				json.NewDecoder(resp.Body).Decode(&out)
				if resp.StatusCode != 400 || out["error"].(map[string]any)["code"] != "unsupported_control_parameter" {
					t.Fatalf("public control accepted: %d %+v", resp.StatusCode, out)
				}
			})
		}
	}
	before := application.Gateway.AdapterCalls()
	code, body := doJSON(t, "POST", srv.URL+"/v1/chat/completions", key, false, map[string]any{"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "provider.only"}}, "tools": []map[string]any{{"type": "function", "function": map[string]any{"name": "routing", "parameters": map[string]any{"provider": map[string]string{"type": "string"}}}}}})
	if code != 402 || body["error"].(map[string]any)["code"] != "insufficient_balance" || application.Gateway.AdapterCalls() != before {
		t.Fatalf("ordinary tool content wrongly rejected: %d %+v", code, body)
	}
	code, _ = doJSON(t, "POST", srv.URL+"/admin/diagnostics/chat/completions", key, false, map[string]any{})
	if code != 401 && code != 403 {
		t.Fatalf("public key authorized diagnostics: %d", code)
	}
}

func TestKeyConcurrentOperationAndOriginalSettlement(t *testing.T) {
	fx := newWMeterEnv(t)
	ctx := fx.ctx
	p, err := fx.app.Identity.Authenticate(ctx, fx.session)
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(200)
	limits := identity.APIKeyLimits{Name: "same operation", OperationID: id.New("operation"), ModelMode: "all", BudgetLimitMinor: &limit}
	var wg sync.WaitGroup
	created := make(chan *identity.APIKeyView, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			key, err := fx.app.Identity.CreateAPIKeyWithLimits(ctx, *p, fx.app.Config.EncryptionKey, limits)
			if err != nil {
				failures <- err
				return
			}
			created <- key
		}()
	}
	wg.Wait()
	close(created)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var key *identity.APIKeyView
	for item := range created {
		if key != nil && (key.ID != item.ID || key.Secret != item.Secret) {
			t.Fatal("same creation produced multiple Keys")
		}
		key = item
	}
	changed := limits
	changed.Name = "changed"
	if _, err := fx.app.Identity.CreateAPIKeyWithLimits(ctx, *p, fx.app.Config.EncryptionKey, changed); !errors.Is(err, identity.ErrKeyCreationConflict) {
		t.Fatalf("changed creation accepted %v", err)
	}
	if _, err := fx.app.Plans.GrantBonus(ctx, "key-test", id.New("bonus"), p.UserID, "usd_credit", 100, time.Hour); err != nil {
		t.Fatal(err)
	}
	prices := json.RawMessage(`{"input":"0.000001","output":"0.000002","wholesale_input":"0.0000005"}`)
	request := id.New("req")
	reserve := billing.ReserveInput{UserID: p.UserID, ChannelOrgID: p.ChannelOrgID, APIKeyID: key.ID, PublicModelID: catalog.EchoModelID, RequestID: request, ReserveMinor: 60, BudgetBounded: true, UnitPrices: prices}
	results := make(chan *billing.Reservation, 2)
	failures = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := fx.app.Billing.Reserve(ctx, reserve)
			if err != nil {
				failures <- err
				return
			}
			results <- r
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	replays := 0
	for r := range results {
		if r.Replayed {
			replays++
		}
	}
	if replays != 1 {
		t.Fatalf("concurrent reserve replay count %d", replays)
	}
	available, _ := fx.app.Plans.AvailableUSD(ctx, p.UserID)
	if available != 40 {
		t.Fatalf("same request consumed/reversed another entitlement: %d", available)
	}
	assertBudget(t, fx.app.Identity, ctx, *p, fx.app.Config.EncryptionKey, key.ID, 0, 60)
	poolBefore, _ := fx.app.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	// A changed current membership or late payload cannot retarget the authorization.
	if err := fx.app.DB.Exec("UPDATE identity_users SET channel_org_id=? WHERE id=?", identity.OEMChannelID, p.UserID).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		fx.app.DB.Exec("UPDATE identity_users SET channel_org_id=? WHERE id=?", p.ChannelOrgID, p.UserID)
	})
	settled, err := fx.app.Billing.Settle(ctx, billing.SettleInput{RequestID: request, UserID: "late-other-user", ChannelOrgID: identity.OEMChannelID, APIKeyID: "late-other-key", PublicModelID: catalog.OEMModelID, Usage: map[string]int{"prompt_tokens": 10, "completion_tokens": 5}, UnitPrices: json.RawMessage(`{"input":"100","output":"100","wholesale_input":"100","upstream_cost_input":"0.0000003"}`)})
	if err != nil || settled.AmountMinor != 20 {
		t.Fatalf("authorization price changed %+v %v", settled, err)
	}
	facts, err := fx.app.Billing.QueryUsage(ctx, billing.QueryUsageInput{RequestID: request})
	if err != nil || len(facts) != 1 || facts[0].UserID != p.UserID || facts[0].APIKeyID != key.ID || facts[0].ChannelOrgID != p.ChannelOrgID || facts[0].PublicModelID != catalog.EchoModelID || facts[0].WholesaleMinor != 5 || facts[0].UpstreamMinor != 3 {
		t.Fatalf("original refs/prices not preserved %+v %v", facts, err)
	}
	poolAfter, _ := fx.app.Billing.ChannelQuota(ctx, identity.OEMChannelID)
	if poolBefore.AvailableMinor != poolAfter.AvailableMinor {
		t.Fatal("late settlement moved another brand pool")
	}
	available, _ = fx.app.Plans.AvailableUSD(ctx, p.UserID)
	if available != 80 {
		t.Fatalf("settlement coverage %d", available)
	}
	if _, err := fx.app.Billing.Reserve(ctx, reserve); !errors.Is(err, billing.ErrConflict) {
		t.Fatalf("settled request authorization reused %v", err)
	}
	assertBudget(t, fx.app.Identity, ctx, *p, fx.app.Config.EncryptionKey, key.ID, 20, 0)
}

func TestPublicModelsHTTPPagination(t *testing.T) {
	fx := newWMeterEnv(t)
	prefix := id.New("page")
	// Use 103 actual published, priced and routed models. A mocked next_cursor
	// cannot establish that the public HTTP endpoint returns the complete catalog.
	err := fx.app.DB.Transaction(func(tx *gorm.DB) error {
		queries := []string{
			`INSERT INTO catalog_public_models (id, public_id, vendor, display_name, capabilities_json, status, sync_state)
			 SELECT ? || '_' || n, ? || '/' || LPAD(n::text,3,'0'), ?, m.display_name, m.capabilities_json, m.status, m.sync_state
			 FROM catalog_public_models m CROSS JOIN generate_series(1,103) n WHERE m.public_id=?`,
			`INSERT INTO catalog_provider_model_mappings (id, public_model_id, provider_id, upstream_model_id, capabilities_json, sync_state, status)
			 SELECT ? || '_' || n || '_' || mm.id, ? || '_' || n, mm.provider_id, mm.upstream_model_id, mm.capabilities_json, mm.sync_state, mm.status
			 FROM catalog_provider_model_mappings mm JOIN catalog_public_models m ON m.id=mm.public_model_id CROSS JOIN generate_series(1,103) n WHERE m.public_id=?`,
			`INSERT INTO catalog_price_versions (id, public_model_id, provider_id, unit_prices_json, effective_at, status)
			 SELECT ? || '_' || n || '_' || pv.id, ? || '_' || n, pv.provider_id, pv.unit_prices_json, pv.effective_at, pv.status
			 FROM catalog_price_versions pv JOIN catalog_public_models m ON m.id=pv.public_model_id CROSS JOIN generate_series(1,103) n WHERE m.public_id=?`,
			`INSERT INTO catalog_route_groups (id, public_model_id, strategy, fallback_policy, status)
			 SELECT ? || '_' || n || '_' || rg.id, ? || '_' || n, rg.strategy, rg.fallback_policy, rg.status
			 FROM catalog_route_groups rg JOIN catalog_public_models m ON m.id=rg.public_model_id CROSS JOIN generate_series(1,103) n WHERE m.public_id=?`,
			`INSERT INTO catalog_route_candidates (route_group_id, provider_id, priority, weight)
			 SELECT ? || '_' || n || '_' || rc.route_group_id, rc.provider_id, rc.priority, rc.weight
			 FROM catalog_route_candidates rc JOIN catalog_route_groups rg ON rg.id=rc.route_group_id JOIN catalog_public_models m ON m.id=rg.public_model_id CROSS JOIN generate_series(1,103) n WHERE m.public_id=?`,
			`INSERT INTO catalog_channel_model_policies (channel_org_id, public_model_id, enabled, self_enabled)
			 SELECT cp.channel_org_id, ? || '_' || n, cp.enabled, cp.self_enabled
			 FROM catalog_channel_model_policies cp JOIN catalog_public_models m ON m.id=cp.public_model_id CROSS JOIN generate_series(1,103) n WHERE m.public_id=?`,
		}
		for i, query := range queries {
			args := []any{prefix, prefix, catalog.EchoModelID}
			if i == 0 {
				args = []any{prefix, prefix, prefix, catalog.EchoModelID}
			}
			if i >= 4 {
				args = []any{prefix, catalog.EchoModelID}
			}
			if err := tx.Exec(query, args...).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM catalog_route_candidates WHERE route_group_id IN (SELECT id FROM catalog_route_groups WHERE public_model_id IN (SELECT id FROM catalog_public_models WHERE vendor=?))`,
			`DELETE FROM catalog_route_groups WHERE public_model_id IN (SELECT id FROM catalog_public_models WHERE vendor=?)`,
			`DELETE FROM catalog_provider_model_mappings WHERE public_model_id IN (SELECT id FROM catalog_public_models WHERE vendor=?)`,
			`DELETE FROM catalog_price_versions WHERE public_model_id IN (SELECT id FROM catalog_public_models WHERE vendor=?)`,
			`DELETE FROM catalog_channel_model_policies WHERE public_model_id IN (SELECT id FROM catalog_public_models WHERE vendor=?)`,
			`DELETE FROM catalog_public_models WHERE vendor=?`,
		} {
			if err := fx.app.DB.Exec(query, prefix).Error; err != nil {
				t.Error(err)
			}
		}
	})
	seen := map[string]bool{}
	cursor := ""
	for page := 0; page < 2; page++ {
		params := url.Values{"host": {"localhost"}, "vendor": {prefix}, "limit": {"100"}, "cursor": {cursor}}
		body := getAuthJSON(t, fx.server.URL+"/v1/public/models?"+params.Encode(), "")
		items := body["items"].([]any)
		want := 100
		if page == 1 {
			want = 3
		}
		if len(items) != want || body["total"] != float64(103) {
			t.Fatalf("page %d: %+v", page, body)
		}
		for _, raw := range items {
			item := raw.(map[string]any)
			modelID := item["id"].(string)
			if seen[modelID] {
				t.Fatalf("duplicate %s", modelID)
			}
			seen[modelID] = true
		}
		cursor = body["next_cursor"].(string)
		if (page == 0 && cursor == "") || (page == 1 && cursor != "") {
			t.Fatalf("cursor on page %d: %q", page, cursor)
		}
	}
	if len(seen) != 103 {
		t.Fatalf("lost models %d", len(seen))
	}
}

func TestBrandDocsRejectMissingEndpointAndUseRegisteredProtocols(t *testing.T) {
	fx := newWMeterEnv(t)
	var originalDomain string
	if err := fx.app.DB.Raw("SELECT api_domain FROM identity_brands WHERE id=?", identity.OEMBrandID).Scan(&originalDomain).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fx.app.DB.Exec("UPDATE identity_brands SET api_domain=? WHERE id=?", originalDomain, identity.OEMBrandID).Error; err != nil {
			t.Error(err)
		}
	})
	for _, domain := range []string{"", "https://api.customer.example", "user@api.customer.example", "api.customer.example/path", "api.customer.example?key=value", "api.customer.example#part", "api.customer.example", "api.oem.localhost"} {
		if err := fx.app.DB.Exec("UPDATE identity_brands SET api_domain=? WHERE id=?", domain, identity.OEMBrandID).Error; err != nil {
			t.Fatal(err)
		}
		code, body := doJSON(t, http.MethodGet, fx.server.URL+"/v1/public/docs-context?host=oem.localhost&model="+url.QueryEscape(catalog.OEMModelID), "", false, nil)
		if domain == "api.customer.example" || domain == "api.oem.localhost" {
			if code != 200 || !strings.Contains(body["api_base_url"].(string), domain) {
				t.Fatalf("brand %q lost its own endpoint: %d %+v", domain, code, body)
			}
		} else if code != 503 || body["error"].(map[string]any)["code"] != "brand_api_unavailable" || body["examples"] != nil {
			t.Fatalf("invalid brand endpoint generated copyable docs: %q %d %+v", domain, code, body)
		}
	}
	var originalAdapters []struct{ ID, Adapter string }
	if err := fx.app.DB.Table("catalog_providers").Select("id, adapter").Where("id IN ?", []string{"prd_echo_primary", "prd_echo_backup"}).Scan(&originalAdapters).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, row := range originalAdapters {
			if err := fx.app.DB.Exec("UPDATE catalog_providers SET adapter=? WHERE id=?", row.Adapter, row.ID).Error; err != nil {
				t.Error(err)
			}
		}
	})
	for _, adapter := range []string{"openai", "openrouter"} {
		if err := fx.app.DB.Exec("UPDATE catalog_providers SET adapter=? WHERE id IN ('prd_echo_primary','prd_echo_backup')", adapter).Error; err != nil {
			t.Fatal(err)
		}
		model, err := fx.app.Catalog.GetAdminModel(fx.ctx, catalog.EchoModelID)
		if err != nil {
			t.Fatal(err)
		}
		endpoints, ok := model.Capabilities["supported_endpoints"].([]string)
		if !ok || len(endpoints) != 3 || endpoints[0] != "/v1/chat/completions" {
			t.Fatalf("actual modelView adapter %q lacks implemented protocols: %+v", adapter, model.Capabilities)
		}
	}
}
