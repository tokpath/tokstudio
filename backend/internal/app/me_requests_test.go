package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/id"
)

func TestMeRequestsSplitsResultAndBilling(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "req_admin"
	cfg.BootstrapUser = "req_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "req-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "req"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	_, _ = doJSON(t, http.MethodPost, server.URL+"/v1/topups/redeem", session, true, map[string]any{"code": billing.RedeemCredit10})

	pendingChat := omitChat(t, server.URL, apiKey, "pending-request")
	pendingID := pendingChat["request_id"].(string)
	voidChat := omitChat(t, server.URL, apiKey, "void-request")
	voidID := voidChat["request_id"].(string)
	postJSONRaw(t, server.URL+"/admin/usage/pending/resolve", "req_admin", map[string]any{
		"request_ids": []string{voidID},
	})
	okChat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "ok"}},
	})
	okID := okChat["request_id"].(string)
	failID := upstreamFailChat(t, server.URL, apiKey)

	if code, body := doJSON(t, http.MethodGet, server.URL+"/v1/me/usage?state=failed", session, false, nil); code != http.StatusBadRequest {
		t.Fatalf("usage must reject fake failed state: %d %+v", code, body)
	}

	listed := getAuthJSON(t, server.URL+"/v1/me/requests?limit=100", session)
	okRow := mustRequest(t, listed, okID)
	failRow := mustRequest(t, listed, failID)
	pendingRow := mustRequest(t, listed, pendingID)
	voidRow := mustRequest(t, listed, voidID)
	if okRow["result"] != "succeeded" || okRow["billing_state"] != billing.UsageConfirmed {
		t.Fatalf("success request: %+v", okRow)
	}
	if failRow["result"] != "failed" || failRow["billing_state"] != nil && failRow["billing_state"] != "" {
		t.Fatalf("upstream failure is a request result, not a billing state: %+v", failRow)
	}
	if failRow["error_code"] != "rate_limited" {
		t.Fatalf("failed request must keep attempt error_code: %+v", failRow)
	}
	if pendingRow["result"] != "succeeded" || pendingRow["billing_state"] != billing.UsagePending {
		t.Fatalf("omit usage stays succeeded + pending_reconciliation: %+v", pendingRow)
	}
	if voidRow["result"] != "succeeded" || voidRow["billing_state"] != billing.UsageVoided {
		t.Fatalf("voided is billing, not request failure: %+v", voidRow)
	}

	failedOnly := getAuthJSON(t, server.URL+"/v1/me/requests?result=failed", session)
	mustRequest(t, failedOnly, failID)
	if containsRequest(failedOnly, okID) || containsRequest(failedOnly, voidID) {
		t.Fatalf("result=failed must not include success or voided billing: %+v", failedOnly)
	}
	confirmedOnly := getAuthJSON(t, server.URL+"/v1/me/requests?billing_state=confirmed", session)
	mustRequest(t, confirmedOnly, okID)
	if containsRequest(confirmedOnly, failID) || containsRequest(confirmedOnly, pendingID) || containsRequest(confirmedOnly, voidID) {
		t.Fatalf("billing_state=confirmed leaked non-confirmed rows: %+v", confirmedOnly)
	}
	pendingOnly := getAuthJSON(t, server.URL+"/v1/me/requests?billing_state=pending_reconciliation", session)
	mustRequest(t, pendingOnly, pendingID)
	voidOnly := getAuthJSON(t, server.URL+"/v1/me/requests?billing_state=voided", session)
	mustRequest(t, voidOnly, voidID)
	if containsRequest(voidOnly, failID) {
		t.Fatal("voided must not be treated as a failed request")
	}

	other := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "req-other-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	otherSession := tokenOf(other)
	foreign := getAuthJSON(t, server.URL+"/v1/me/requests?limit=100", otherSession)
	if containsRequest(foreign, okID) || containsRequest(foreign, failID) || containsRequest(foreign, pendingID) || containsRequest(foreign, voidID) {
		t.Fatalf("another user must not read these requests: %+v", foreign)
	}

	_ = application
}

func TestMeRequestsBillingFilterSurvivesRecentMediaUsage(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "req_media_admin"
	cfg.BootstrapUser = "req_media_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "req-media-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "req-media"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "older-confirmed"}},
	})
	okID, _ := chat["request_id"].(string)
	if okID == "" {
		t.Fatalf("chat: %+v", chat)
	}
	userID := getAuthJSON(t, server.URL+"/v1/me", session)["user"].(map[string]any)["id"].(string)
	later := time.Now().UTC().Add(time.Minute)
	for i := 0; i < 201; i++ {
		if err := application.DB.Exec(
			`INSERT INTO billing_usage_events (
				id, request_id, user_id, public_model_id, unit_usage_json, unit_prices_json,
				customer_amount_minor, upstream_cost_minor, wholesale_amount_minor, currency, state, idempotency_key, occurred_at
			) VALUES (?, ?, ?, ?, '{}'::jsonb, '{}'::jsonb, 0, 0, 0, 'USD', ?, ?, ?)`,
			id.New("usg"),
			fmt.Sprintf("media_job_%d", i),
			userID,
			"bytedance/seedance",
			billing.UsageConfirmed,
			id.New("idem"),
			later.Add(time.Duration(i)*time.Second),
		).Error; err != nil {
			t.Fatalf("insert media usage %d: %v", i, err)
		}
	}
	listed := getAuthJSON(t, server.URL+"/v1/me/requests?billing_state=confirmed&limit=20", session)
	mustRequest(t, listed, okID)
	if containsRequest(listed, "media_job_0") || containsRequest(listed, "media_job_200") {
		t.Fatalf("media usage must not appear as gateway requests: %+v", listed)
	}
}

func upstreamFailChat(t *testing.T, base, key string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "upstream-fail"}},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions?provider.only="+catalog.PrimaryProvider, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Force-Fail", catalog.PrimaryProvider)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected upstream failure 503, got %d %+v", resp.StatusCode, out)
	}
	errBody, _ := out["error"].(map[string]any)
	requestID, _ := errBody["request_id"].(string)
	if requestID == "" {
		t.Fatalf("failed chat must return request_id: %+v", out)
	}
	return requestID
}

func mustRequest(t *testing.T, body map[string]any, requestID string) map[string]any {
	t.Helper()
	items, _ := body["items"].([]any)
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if item["request_id"] == requestID {
			return item
		}
	}
	t.Fatalf("missing request %s in %+v", requestID, body)
	return nil
}
