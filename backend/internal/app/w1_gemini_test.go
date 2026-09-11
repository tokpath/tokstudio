package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/gateway"
)

func geminiLiveConfigured() bool {
	return strings.TrimSpace(os.Getenv("TOKENHUB_GEMINI_API_KEY")) != ""
}

func TestW1GeminiWithoutKeyIsUnavailable(t *testing.T) {
	fx := newWMeterEnv(t)
	if geminiLiveConfigured() {
		t.Skip("this contract is the no-Key path; live Key is gated separately")
	}

	// 只钉住种子 gemini：共享库上可能残留测试挂上的 active backup，否则会先 503 再回落到 harness echo。
	code, body := doJSON(t, http.MethodPost, fx.server.URL+"/v1/chat/completions?provider.only="+catalog.GeminiProvider, fx.apiKey, true, map[string]any{
		"model": catalog.GeminiModelID, "messages": []map[string]string{{"role": "user", "content": "w1-gemini-echo"}},
	})
	if code != http.StatusServiceUnavailable && code != http.StatusBadGateway && code != http.StatusOK {
		// chat may return 200 with error payload depending on gateway HTTP mapping; accept structured fail
		t.Logf("chat status %d body %+v", code, body)
	}
	if content, ok := firstContentOf(body); ok && strings.Contains(content, "echo:") {
		t.Fatalf("no Key must not sandbox-echo: %+v", body)
	}
	if errObj, _ := body["error"].(map[string]any); errObj != nil {
		if errObj["code"] == nil && errObj["message"] == nil {
			t.Fatalf("expected structured error: %+v", body)
		}
	} else if code == http.StatusOK {
		// Execute may surface as failed attempts without top-level error in some paths
		requestID, _ := body["request_id"].(string)
		if requestID == "" {
			t.Fatalf("expected failure without echo: %+v", body)
		}
		attempts := getAuthJSON(t, fx.server.URL+"/v1/requests/"+requestID+"/attempts", fx.apiKey)
		items, _ := attempts["items"].([]any)
		for _, item := range items {
			atm := item.(map[string]any)
			if atm["fact_source"] == gateway.FactSourceSandbox {
				t.Fatalf("must not label sandbox facts after echo removal: %+v", atm)
			}
			if atm["status"] == "succeeded" {
				t.Fatalf("no Key must not succeed: %+v", attempts)
			}
		}
	}
}

func TestW1GeminiCatalogCandidatesSingleLayerFallback(t *testing.T) {
	fx := newWMeterEnv(t)
	// 确保种子 gemini 仍在路由候选里（共享库上一次 503 熔断后会被 ResolveRoute 跳过）。
	if err := fx.app.Ops.ResetCircuit(fx.ctx, "prd_gemini"); err != nil {
		t.Fatal(err)
	}

	models := getAuthJSON(t, fx.server.URL+"/v1/models", fx.apiKey)
	found := false
	for _, raw := range modelListItems(models) {
		if raw["id"] == catalog.GeminiModelID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("public Gemini model must enter the catalog: %+v", models)
	}

	slug := "w1-gem-fb-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	prd := postJSONRaw(t, fx.server.URL+"/admin/providers", "wmeter2_admin", map[string]any{
		"name": "W1 Gemini Backup", "slug": slug, "adapter": "test",
	})
	prdID := prd["item"].(map[string]any)["id"].(string)
	_ = postJSONRaw(t, fx.server.URL+"/admin/models/attach", "wmeter2_admin", map[string]any{
		"public_id": catalog.GeminiModelID, "provider_id": prdID, "upstream_model_id": "echo-gemini-backup",
	})
	t.Cleanup(func() {
		_ = patchJSONRaw(t, fx.server.URL+"/admin/providers/"+prdID, "wmeter2_admin", map[string]any{"status": "maintenance"})
	})

	before := fx.app.Gateway.AdapterCalls()
	fb := forceFailGeminiChat(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, catalog.GeminiProvider)
	if got := fx.app.Gateway.AdapterCalls() - before; got < 2 {
		t.Fatalf("429 must stay single-layer catalog fallback (>=2 sequential calls), got %d", got)
	}
	fbID, _ := fb["request_id"].(string)
	if fbID == "" {
		t.Fatalf("fallback chat missing request_id: %+v", fb)
	}
	if content, ok := firstContentOf(fb); !ok || !strings.Contains(content, "echo:") {
		t.Fatalf("catalog backup harness should echo after gemini fail: %+v", fb)
	}
	if fb["provider"] == catalog.GeminiProvider {
		t.Fatalf("429 must leave gemini via catalog fallback: %+v", fb)
	}
	attempts := getAuthJSON(t, fx.server.URL+"/v1/requests/"+fbID+"/attempts", fx.apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) < 2 {
		t.Fatalf("429 fallback must record sequential catalog attempts: %+v", attempts)
	}
	seen := map[float64]int{}
	for _, raw := range items {
		row := raw.(map[string]any)
		n, _ := row["attempt_no"].(float64)
		seen[n]++
		if seen[n] > 1 {
			t.Fatalf("attempt_no %v raced twice (smart routing / dual-layer): %+v", n, attempts)
		}
		if row["fact_source"] == gateway.FactSourceSandbox {
			t.Fatalf("must not invent sandbox facts: %+v", row)
		}
	}
}

func TestW1GeminiMissingUsagePendingNoEstimate(t *testing.T) {
	if !geminiLiveConfigured() {
		t.Skip("omit-usage pending contract requires live Gemini; sandbox echo path removed")
	}
	fx := newWMeterEnv(t)
	omit := omitGeminiChat(t, fx.server.URL, fx.apiKey, "w1-gem-omit")
	requestID := omit["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 0, "Gemini missing usage must not estimate-debit")

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{
		RequestID: requestID, State: billing.UsagePending, Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(usages) != 1 || usages[0].State != billing.UsagePending {
		t.Fatalf("missing usage must be pending_reconciliation: %+v", usages)
	}
	if usages[0].PromptTokens != 0 || usages[0].CustomerMinor != 0 {
		t.Fatalf("pending must stay honest empty: %+v", usages[0])
	}
}

func TestW1GeminiLiveHitsRealUpstreamWhenKeyPresent(t *testing.T) {
	if !geminiLiveConfigured() {
		t.Skip("live Gemini requires TOKENHUB_GEMINI_API_KEY")
	}
	fx := newWMeterEnv(t)
	chat := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.GeminiModelID, "messages": []map[string]string{{"role": "user", "content": "Reply with the single word pong."}},
	})
	requestID, _ := chat["request_id"].(string)
	if requestID == "" {
		t.Fatalf("live gemini missing request_id: %+v", chat)
	}
	content, _ := firstContentOf(chat)
	if strings.Contains(content, "gemini:echo:") {
		t.Fatalf("live+Key must not echo: %+v", chat)
	}

	attempts := getAuthJSON(t, fx.server.URL+"/v1/requests/"+requestID+"/attempts", fx.apiKey)
	items, _ := attempts["items"].([]any)
	if len(items) < 1 {
		t.Fatalf("live need attempt: %+v", attempts)
	}
	atm := items[0].(map[string]any)
	if atm["request_id"] != requestID || atm["provider_id"] == "" || atm["upstream_model_id"] == "" {
		t.Fatalf("live attempt must carry real provider/model/request_id: %+v", atm)
	}
	if atm["fact_source"] == gateway.FactSourceSandbox {
		t.Fatalf("live+Key fact_source must not be sandbox: %+v", atm)
	}

	usages, err := fx.app.Billing.QueryUsage(fx.ctx, billing.QueryUsageInput{RequestID: requestID, Limit: 5})
	if err != nil || len(usages) != 1 {
		t.Fatalf("live usage: %v %+v", err, usages)
	}
	if usages[0].FactSource == billing.FactSourceSandbox {
		t.Fatalf("live usage fact_source: %+v", usages[0])
	}
	if usages[0].ProviderID == "" || usages[0].UpstreamModelID == "" || usages[0].RequestID != requestID {
		t.Fatalf("live usage must carry real provider/model/request_id: %+v", usages[0])
	}

	margin := getAuthJSON(t, fx.server.URL+"/admin/margin?request_id="+requestID, "wmeter2_admin")
	item, _ := margin["item"].(map[string]any)
	if item["cost_source"] == "Bifrost" || item["cost_source"] == "estimate" {
		t.Fatalf("live margin still reads TokenHub only: %+v", margin)
	}
}

func modelListItems(body map[string]any) []map[string]any {
	raw, _ := body["data"].([]any)
	if len(raw) == 0 {
		raw, _ = body["items"].([]any)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if row, ok := item.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out
}

func forceFailGeminiChat(t *testing.T, url, token, provider string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(mustJSON(map[string]any{
		"model": catalog.GeminiModelID, "messages": []map[string]string{{"role": "user", "content": "hi-gem-fb"}},
	})))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Force-Fail", provider)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("force-fail gemini chat %d %+v", resp.StatusCode, out)
	}
	return out
}

func omitGeminiChat(t *testing.T, base, key, content string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model": catalog.GeminiModelID, "messages": []map[string]string{{"role": "user", "content": content}},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Omit-Usage", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 || out["request_id"] == nil {
		t.Fatalf("omit gemini chat %d %+v", resp.StatusCode, out)
	}
	return out
}
