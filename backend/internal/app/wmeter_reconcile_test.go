package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func newWMeter3Env(t *testing.T) *wmeterEnv {
	t.Helper()
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "wmeter3_admin"
	cfg.BootstrapUser = "wmeter3_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	t.Cleanup(server.Close)

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email":    "wmeter3-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "wmeter3"})["item"].(map[string]any)["key"].(string)
	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	return &wmeterEnv{
		t: t, app: application, server: server, ctx: context.Background(),
		session: session, apiKey: apiKey, userID: userIDOf(reg),
	}
}

// TestWMeter3ReconcileMatch：已结算且 charge == usage 的行走绿色匹配。
func TestWMeter3ReconcileMatch(t *testing.T) {
	fx := newWMeter3Env(t)
	charged := postJSONRaw(t, fx.server.URL+"/v1/chat/completions", fx.apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "wmeter3-match"}},
	})
	requestID := charged["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 1, "match path must keep one customer charge")

	body := getAuthJSON(t, fx.server.URL+"/v1/me/reconciliation", fx.session)
	item := body["item"].(map[string]any)
	buckets := item["buckets"].(map[string]any)
	if buckets["available_minor"] == nil || buckets["reserved_minor"] == nil || buckets["withdrawable_minor"] == nil {
		t.Fatalf("three-bucket cards missing: %+v", buckets)
	}
	row := findDiffRow(item, requestID)
	if row == nil {
		t.Fatalf("match row missing: %+v", item)
	}
	if row["match"] != true || row["status"] != billing.DiffMatch {
		t.Fatalf("confirmed usage must be the green match path: %+v", row)
	}
	if postStatus(t, fx.server.URL+"/v1/me/reconciliation/flag", fx.session, map[string]any{
		"request_id": requestID,
	}) != http.StatusConflict {
		t.Fatal("match row must refuse flag without becoming pending")
	}
	requireChargeCount(t, fx.app.Billing, requestID, 1, "flagging a match must not mint a charge")
}

// TestWMeter3ReconcileMismatchToPending：差异行必须露出并送进 pending_reconciliation，禁止静默丢弃。
func TestWMeter3ReconcileMismatchToPending(t *testing.T) {
	fx := newWMeter3Env(t)
	before := balanceMinor(t, fx.server.URL, fx.session)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter3-omit")
	requestID := omit["request_id"].(string)
	requireChargeCount(t, fx.app.Billing, requestID, 0, "omit must NEVER estimate-debit")

	body := getAuthJSON(t, fx.server.URL+"/v1/me/reconciliation", fx.session)
	item := body["item"].(map[string]any)
	row := findDiffRow(item, requestID)
	if row == nil {
		t.Fatalf("mismatch must not be silently dropped: %+v", item)
	}
	if row["match"] == true || row["status"] != billing.DiffMismatch {
		t.Fatalf("omit must be a mismatch row: %+v", row)
	}
	if row["already_pending"] != true {
		t.Fatalf("omit should already sit in pending_reconciliation: %+v", row)
	}
	if minorOf(row["charge_minor"]) != 0 || minorOf(row["usage_minor"]) != 0 {
		t.Fatalf("mismatch row must not carry an estimated bill: %+v", row)
	}

	pending, _ := item["pending"].([]any)
	if !containsRequest(map[string]any{"items": pending}, requestID) {
		t.Fatalf("pending queue section must include the omit: %+v", pending)
	}

	if postStatus(t, fx.server.URL+"/v1/me/reconciliation/flag", fx.session, map[string]any{
		"request_id": requestID,
	}) != http.StatusConflict {
		t.Fatal("flag without seal confirm must be 409")
	}
	requireChargeCount(t, fx.app.Billing, requestID, 0, "409 flag must not estimate-debit")

	flagged := postJSONRaw(t, fx.server.URL+"/v1/me/reconciliation/flag", fx.session, map[string]any{
		"request_id": requestID,
	})
	gap := flagged["item"].(map[string]any)
	if gap["state"] != billing.UsagePending {
		t.Fatalf("flag must keep/enqueue pending_reconciliation: %+v", gap)
	}
	requireChargeCount(t, fx.app.Billing, requestID, 0, "flag must NEVER estimate-debit")
	if usageDebitCount(t, fx.app.Billing, fx.userID, requestID) != 0 {
		t.Fatal("flag must not write usage_debit")
	}
	after := balanceMinor(t, fx.server.URL, fx.session)
	if after >= before {
		t.Fatalf("omit hold should remain: before=%d after=%d", before, after)
	}

	again := postJSONRaw(t, fx.server.URL+"/v1/me/reconciliation/flag", fx.session, map[string]any{
		"request_id": requestID,
	})
	if again["item"].(map[string]any)["request_id"] != requestID {
		t.Fatalf("flag must be idempotent: %+v", again)
	}
	requireChargeCount(t, fx.app.Billing, requestID, 0, "idempotent flag must still not estimate-debit")
}

// TestWMeter3ChannelReconcileIsomorphic：渠道台走同一套 billing 对账，三桶 + usage 窗口。
func TestWMeter3ChannelReconcileIsomorphic(t *testing.T) {
	fx := newWMeter3Env(t)
	omit := omitChat(t, fx.server.URL, fx.apiKey, "wmeter3-channel")
	requestID := omit["request_id"].(string)

	body := getAuthJSON(t, fx.server.URL+"/channel/reconciliation", "wmeter3_admin")
	item := body["item"].(map[string]any)
	if item["buckets"] == nil || item["usage_totals"] == nil {
		t.Fatalf("channel page must expose buckets + usage totals: %+v", item)
	}
	if findDiffRow(item, requestID) == nil {
		t.Fatalf("channel mismatch must not be dropped: %+v", item)
	}
	if postStatus(t, fx.server.URL+"/channel/reconciliation/flag", "wmeter3_admin", map[string]any{
		"request_id": requestID,
	}) != http.StatusConflict {
		t.Fatal("channel flag without confirm must be 409")
	}
	flagged := postJSONRaw(t, fx.server.URL+"/channel/reconciliation/flag", "wmeter3_admin", map[string]any{
		"request_id": requestID,
	})
	if flagged["item"].(map[string]any)["state"] != billing.UsagePending {
		t.Fatalf("channel flag must enqueue pending: %+v", flagged)
	}
	requireChargeCount(t, fx.app.Billing, requestID, 0, "channel flag must NEVER estimate-debit")
}

func findDiffRow(item map[string]any, requestID string) map[string]any {
	rows, _ := item["items"].([]any)
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row["request_id"] == requestID {
			return row
		}
	}
	return nil
}
