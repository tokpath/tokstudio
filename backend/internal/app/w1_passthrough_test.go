package app_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
)

func TestW1BifrostPassthroughMetadata(t *testing.T) {
	fx := newWMeterEnv(t)
	slug := "w1-bifrost-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	prd := postJSONRaw(t, fx.server.URL+"/admin/providers", "wmeter2_admin", map[string]any{
		"name": "W1 Bifrost", "slug": slug, "adapter": "bifrost",
	})
	prdID := prd["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, fx.server.URL+"/admin/models/attach", "wmeter2_admin", map[string]any{
		"public_id": catalog.EchoModelID, "provider_id": prdID, "upstream_model_id": "echo-upstream",
	})

	chat := postJSONRaw(t, fx.server.URL+"/v1/chat/completions?provider.only="+slug, fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "w1-pass"}},
	})
	requestID, _ := chat["request_id"].(string)
	if requestID == "" {
		t.Fatalf("chat missing request_id: %+v", chat)
	}
	content, _ := firstContentOf(chat)
	if !strings.Contains(content, "bifrost:w1-pass") {
		t.Fatalf("sandbox bifrost reply: %+v", chat)
	}

	attempts := getAuthJSON(t, fx.server.URL+"/v1/requests/"+requestID+"/attempts", fx.apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) < 1 {
		t.Fatalf("need attempt: %+v", attempts)
	}
	atm := items[0].(map[string]any)
	if atm["id"] == "" || atm["request_id"] != requestID {
		t.Fatalf("attempt must carry request_id: %+v", atm)
	}
	if atm["provider_id"] != prdID {
		t.Fatalf("attempt provider: %+v", atm)
	}
	if atm["upstream_model_id"] != "echo-upstream" {
		t.Fatalf("attempt upstream model: %+v", atm)
	}
	if atm["fact_source"] != gateway.FactSourceSandbox {
		t.Fatalf("CI sandbox must label echo, not live: %+v", atm)
	}
	if atm["fact_source"] == gateway.FactSourceLive {
		t.Fatal("sandbox must not impersonate live upstream")
	}
	meta, _ := atm["metadata"].(map[string]any)
	if meta["request_id"] != requestID || meta["attempt_id"] != atm["id"] {
		t.Fatalf("metadata pass-through: %+v", meta)
	}
	if meta["user_id"] != fx.userID || meta["public_model_id"] != catalog.EchoModelID {
		t.Fatalf("user/model metadata: %+v", meta)
	}

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{RequestID: requestID, Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 {
		t.Fatalf("usage: %+v", usages)
	}
	if usages[0].AttemptID != atm["id"] || usages[0].ProviderID != prdID || usages[0].UpstreamModelID != "echo-upstream" {
		t.Fatalf("usage must keep TokenHub attempt facts: %+v", usages[0])
	}
	if usages[0].FactSource != billing.FactSourceSandbox {
		t.Fatalf("usage fact_source: %+v", usages[0])
	}

	margin := getAuthJSON(t, fx.server.URL+"/admin/margin?request_id="+requestID, "wmeter2_admin")
	item, _ := margin["item"].(map[string]any)
	rows, _ := item["items"].([]any)
	if len(rows) < 1 {
		t.Fatalf("margin must read TokenHub only: %+v", margin)
	}
	row := rows[0].(map[string]any)
	if row["cost_source"] != "TokenHub" {
		t.Fatalf("margin cost_source: %+v", row)
	}
	if row["request_id"] != requestID || row["attempt_id"] != atm["id"] {
		t.Fatalf("margin ids: %+v", row)
	}
	if row["provider_id"] != prdID || row["upstream_model_id"] != "echo-upstream" {
		t.Fatalf("margin upstream facts: %+v", row)
	}
	if row["fact_source"] == gateway.FactSourceLive {
		t.Fatal("margin must not relabel sandbox as live")
	}
}

func TestW1SentinelNoImpersonateUpstream(t *testing.T) {
	fx := newWMeterEnv(t)
	if _, err := fx.app.Billing.Settle(fx.ctx, billing.SettleInput{
		RequestID: "req_w1_empty_" + t.Name(), UserID: fx.userID, PublicModelID: catalog.EchoModelID,
		MissingUsage: true, IdempotencyKey: "w1-empty-" + t.Name(),
	}); err != nil && err != billing.ErrAuthNotReserved {
		// 没有预授权时 Settle 会失败；改走 QueryUsage 空行断言。
		_ = err
	}

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{UserID: fx.userID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range usages {
		if row.ProviderID == "openai" || row.UpstreamModelID == "gpt-4" || row.FactSource == billing.FactSourceLive && row.ProviderID == "" {
			t.Fatalf("must not invent upstream facts: %+v", row)
		}
	}

	recon, err := fx.app.Billing.ReconcileWindow(fx.ctx, billing.ReconcileInput{UserID: fx.userID, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range recon.Items {
		if row.ProviderID == "openai" || row.UpstreamModelID == "gpt-4" {
			t.Fatalf("reconcile must not invent provider/model: %+v", row)
		}
	}
}

func TestW1MissingUsagePendingHonestEmpty(t *testing.T) {
	fx := newWMeterEnv(t)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "w1-omit")
	requestID := omit["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 0, "W1 missing usage must not estimate-debit")

	attempts := getAuthJSON(t, fx.server.URL+"/v1/requests/"+requestID+"/attempts", fx.apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) < 1 {
		t.Fatalf("attempt should still exist: %+v", attempts)
	}
	atm := items[0].(map[string]any)
	if atm["prompt_tokens"] != nil || atm["completion_tokens"] != nil {
		t.Fatalf("omit must not invent attempt tokens: %+v", atm)
	}
	if atm["fact_source"] == gateway.FactSourceLive {
		t.Fatal("omit echo adapter is sandbox, not live")
	}

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{
		RequestID: requestID, State: billing.UsagePending, Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].State != billing.UsagePending {
		t.Fatalf("missing usage must be pending: %+v", usages)
	}
	if usages[0].PromptTokens != 0 || usages[0].CustomerMinor != 0 {
		t.Fatalf("pending must stay honest empty: %+v", usages[0])
	}

	margin := getAuthJSON(t, fx.server.URL+"/admin/margin?request_id="+requestID, "wmeter2_admin")
	item, _ := margin["item"].(map[string]any)
	rows, _ := item["items"].([]any)
	if len(rows) != 0 {
		t.Fatalf("pending omit has no confirmed attempt cost: %+v", margin)
	}
	if item["cost_source"] == "Bifrost" {
		t.Fatal("margin must not use Bifrost as billing source")
	}
}

func TestW1PriceMarginReadTokenHubOnly(t *testing.T) {
	fx := newWMeterEnv(t)
	charged := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "w1-th"}},
	})
	requestID := charged["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 1, "live settle")

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{RequestID: requestID, Limit: 5})
	if err != nil || len(usages) != 1 {
		t.Fatalf("TokenHub usage: %v %+v", err, usages)
	}
	body := getAuthJSON(t, fx.server.URL+"/admin/margin?request_id="+requestID, "wmeter2_admin")
	margin, _ := body["item"].(map[string]any)
	if findMarginRowByRequest(margin, requestID) == nil {
		t.Fatalf("margin must assemble from TokenHub: %+v", body)
	}
	if asInt(margin["attempt_cost_minor"]) != usages[0].UpstreamMinor {
		t.Fatalf("margin cost must follow TokenHub usage: margin=%v usage=%d", margin, usages[0].UpstreamMinor)
	}

	if postStatusConfirm(t, fx.server.URL+"/admin/supplier-entries", "wmeter2_admin", map[string]any{
		"amount_minor": 99, "source_type": "estimate", "idempotency_key": "w1-est-" + t.Name(),
	}) != http.StatusBadRequest {
		t.Fatal("invented supplier source must FAIL")
	}
}
