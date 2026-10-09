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
		if err := a.DB.Table("billing_usage_events").Create(map[string]any{"id": req, "request_id": req, "user_id": user.User.ID, "api_key_id": "key_scope", "channel_org_id": identity.OfficialChannelID, "public_model_id": "scope/model", "unit_usage_json": string(usage), "unit_prices_json": "{}", "customer_amount_minor": 100, "upstream_cost_minor": 17, "wholesale_amount_minor": 21, "state": state, "idempotency_key": req, "occurred_at": at}).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Table("gateway_requests").Create(map[string]any{"id": "g-" + req, "request_id": req, "user_id": user.User.ID, "api_key_id": "key_scope", "channel_org_id": identity.OfficialChannelID, "public_model_id": "scope/model", "protocol": "chat", "status": "succeeded", "started_at": at}).Error; err != nil {
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
