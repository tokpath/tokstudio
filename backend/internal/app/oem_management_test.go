package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestOEMManagementScope(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("postgres and redis required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Env = "test"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	login := func(name string) map[string]any {
		return postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": name + "@tokenhub.local", "password": identity.BootstrapPassword})
	}
	cLogin, bLogin := login("channel.c"), login("channel.b")
	cToken, bToken := tokenOf(cLogin), tokenOf(bLogin)
	marker := "oem-scope-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	child, err := a.Identity.CreateChannel(context.Background(), identity.Principal{Roles: []string{"channel_admin"}, ChannelOrgID: identity.OEMChannelID}, identity.ChannelInput{Code: marker, Type: "B"})
	if err != nil {
		t.Fatal(err)
	}
	for i, channel := range []string{child.ID, identity.OEMChannelID, identity.ResellerChannelID} {
		key := marker + strconv.Itoa(i)
		if err := a.DB.Exec(`INSERT INTO billing_usage_events (id, request_id, user_id, channel_org_id, public_model_id, unit_usage_json, unit_prices_json, customer_amount_minor, upstream_cost_minor, wholesale_amount_minor, state, idempotency_key) VALUES (?, ?, ?, ?, ?, '{}', '{}', 100, 3, 40, 'confirmed', ?)`, key, key, userIDOf(cLogin), channel, marker, key).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Exec(`INSERT INTO billing_customer_charges (id,request_id,usage_event_id,amount_minor,status) VALUES (?,?,?,100,'committed')`, key, key, key).Error; err != nil {
			t.Fatal(err)
		}
		if err := a.DB.Exec(`INSERT INTO media_jobs (id, request_id, user_id, channel_org_id, public_model_id, job_kind, task_type, status, prompt) VALUES (?, ?, ?, ?, ?, 'video', 'generate', 'failed', 'private prompt')`, key, key, userIDOf(cLogin), channel, marker).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		a.DB.Exec("DELETE FROM billing_customer_charges WHERE usage_event_id IN (SELECT id FROM billing_usage_events WHERE public_model_id=?)", marker)
		a.DB.Exec("DELETE FROM billing_usage_events WHERE public_model_id = ?", marker)
		a.DB.Exec("DELETE FROM media_jobs WHERE public_model_id = ?", marker)
	})
	for _, path := range []string{"/channel/metrics", "/channel/media", "/channel/audit-logs", "/channel/alerts", "/channel/me/2fa"} {
		if status := getStatus(t, server.URL+path, bToken); status != 403 {
			t.Fatalf("B accessed %s: %d", path, status)
		}
		if status := getStatus(t, server.URL+path+"?channel_id="+identity.ResellerChannelID, cToken); status != 403 {
			t.Fatalf("foreign scope accessed %s: %d", path, status)
		}
		if status := getStatus(t, server.URL+path, cToken); status != 200 {
			t.Fatalf("OEM denied %s: %d", path, status)
		}
	}
	for _, path := range []string{"/channel/usage", "/channel/reconciliation", "/channel/users"} {
		if status := getStatus(t, server.URL+path+"?channel_id="+child.ID, cToken); status != 200 {
			t.Fatalf("OEM cannot manage child %s: %d", path, status)
		}
		if status := getStatus(t, server.URL+path+"?channel_id="+identity.ResellerChannelID, cToken); status != 403 {
			t.Fatalf("OEM accessed another brand's %s: %d", path, status)
		}
		if status := getStatus(t, server.URL+path+"?channel_id="+child.ID, bToken); status != 403 {
			t.Fatalf("B accessed foreign %s: %d", path, status)
		}
	}
	for _, path := range []string{"/admin/metrics", "/admin/ops/alerts", "/admin/audit-logs"} {
		if status := getStatus(t, server.URL+path, cToken); status != 403 {
			t.Fatalf("global access granted: %s %d", path, status)
		}
	}
	report := getAuthJSON(t, server.URL+"/channel/metrics?channel_id="+child.ID, cToken)["totals"].(map[string]any)
	if report["requests"] != float64(1) || report["revenue_minor"] != float64(100) || report["cost_minor"] != float64(40) || report["margin_minor"] != float64(60) {
		t.Fatalf("OEM must use wholesale cost and exact child scope: %+v", report)
	}
	media := getAuthJSON(t, server.URL+"/channel/media?q="+marker, cToken)["items"].([]any)
	if len(media) != 2 {
		t.Fatalf("wrong media scope: %+v", media)
	}
	for _, item := range media {
		row := item.(map[string]any)
		if row["channel_org_id"] == identity.ResellerChannelID || row["prompt"] != nil || row["provider"] != nil {
			t.Fatalf("media data leaked: %+v", row)
		}
	}
	_, err = a.Audit.Record(context.Background(), audit.RecordInput{ActorUserID: userIDOf(cLogin), Action: marker, ResourceType: "channel", ResourceID: child.ID, After: map[string]string{"secret": "private"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Audit.Record(context.Background(), audit.RecordInput{ActorUserID: userIDOf(bLogin), Action: marker, ResourceType: "channel", ResourceID: identity.ResellerChannelID})
	if err != nil {
		t.Fatal(err)
	}
	logs := getAuthJSON(t, server.URL+"/channel/audit-logs?q="+marker, cToken)["items"].([]any)
	if len(logs) != 1 {
		t.Fatalf("wrong audit scope: %+v", logs)
	}
	if row := logs[0].(map[string]any); row["after"] != nil || row["before"] != nil || row["actor_email"] == "" {
		t.Fatalf("unsafe or incomplete audit view: %+v", row)
	}
	empty, models, err := a.Billing.BrandReport(context.Background(), nil)
	if err != nil || empty.Requests != 0 || len(models) != 0 {
		t.Fatal("empty scope widened report")
	}
	if _, err := a.Identity.SetupTOTP(context.Background(), identity.Principal{UserID: userIDOf(bLogin), Roles: []string{"channel_admin"}, ChannelOrgID: identity.ResellerChannelID}, cfg.EncryptionKey, "TokenHub"); err == nil {
		t.Fatal("B may not configure OEM security")
	}
	if code, body := doJSON(t, http.MethodPost, server.URL+"/channel/me/2fa/setup", cToken, false, map[string]any{}); code != 200 || body["item"] == nil {
		t.Fatalf("OEM authenticator setup failed: %d", code)
	}
	status := getAuthJSON(t, server.URL+"/channel/me/2fa", cToken)["item"].(map[string]any)
	if status["secret"] != "" || status["status"] != "pending" {
		t.Fatal("TOTP status disclosed the secret or setup was not persisted")
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/channel/me/2fa/setup", bToken, false, map[string]any{}); code != 403 {
		t.Fatal("B accessed OEM security endpoint")
	}
	t.Cleanup(func() { a.DB.Exec("DELETE FROM identity_admin_totp WHERE user_id = ?", userIDOf(cLogin)) })
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/channels", cToken, true, map[string]any{"code": marker + "-nested", "type": "C"}); code < 400 {
		t.Fatal("OEM created a child OEM")
	}
}
