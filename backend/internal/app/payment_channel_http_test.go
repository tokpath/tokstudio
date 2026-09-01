package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestChannelPaymentPluggable(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "pay_admin"
	cfg.BootstrapUser = "pay_user"
	cfg.BootstrapChannel = "pay_channel"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	cfg.PaymentSignKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()

	if mustStatusJSON(t, http.MethodGet, server.URL+"/channel/payments/overview", "", nil) != http.StatusForbidden {
		t.Fatal("anon overview must 403")
	}
	over := getAuthJSON(t, server.URL+"/channel/payments/overview", "pay_channel")
	item := over["item"].(map[string]any)
	lanes := item["lanes"].([]any)
	if len(lanes) < 3 {
		t.Fatalf("expected pluggable user-facing lanes, got %+v", over)
	}
	var ids []string
	for _, raw := range lanes {
		lane := raw.(map[string]any)
		ids = append(ids, lane["adapter"].(string))
		if lane["schema"] == nil {
			t.Fatalf("lane missing schema: %+v", lane)
		}
	}
	joined := strings.Join(ids, ",")
	if !strings.Contains(joined, "alipay") || !strings.Contains(joined, "wechat") || !strings.Contains(joined, "stripe") {
		t.Fatalf("lanes: %s", joined)
	}

	created := postJSONRaw(t, server.URL+"/channel/payments/instances", "pay_channel", map[string]any{
		"adapter": "alipay", "name": "B 支付宝沙箱", "mode": "sandbox",
		"credentials": map[string]string{
			"app_id": "202100000", "app_private_key": "BEGIN SECRET", "alipay_public_key": "BEGIN PUB",
		},
	})
	inst := created["item"].(map[string]any)
	raw, _ := json.Marshal(created)
	if containsText(created, "BEGIN SECRET") || strings.Contains(string(raw), "BEGIN SECRET") {
		t.Fatalf("secret leaked: %+v", created)
	}
	if inst["public_fields"].(map[string]any)["app_id"] != "202100000" {
		t.Fatalf("public app_id: %+v", inst)
	}

	tested := postJSONRaw(t, server.URL+"/channel/payments/instances/"+inst["id"].(string)+"/test", "pay_channel", map[string]any{})
	if tested["item"].(map[string]any)["last_test_ok"] != true {
		t.Fatalf("test: %+v", tested)
	}

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "payb-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test",
		"password": "password1", "promotion_code": "THB1",
	})
	session := tokenOf(reg)
	checkout := getAuthJSON(t, server.URL+"/v1/payments/checkout", session)
	methods := checkout["item"].(map[string]any)["methods"].([]any)
	if len(methods) == 0 {
		t.Fatalf("B user should see alipay after instance: %+v", checkout)
	}

	quote := getAuthJSON(t, server.URL+"/v1/payments/quote?adapter=alipay&pay_major=100", session)
	q := quote["item"].(map[string]any)
	if q["pay_currency"] != "CNY" {
		t.Fatalf("quote: %+v", quote)
	}

	order := postJSONRaw(t, server.URL+"/v1/payments/orders", session, map[string]any{
		"adapter": payment.AdapterAlipay, "pay_major": 100,
	})
	co := order["checkout"].(map[string]any)
	ord := co["order"].(map[string]any)
	if ord["channel_org_id"] != identity.ResellerChannelID {
		t.Fatalf("order channel: %+v", order)
	}

	listed := getAuthJSON(t, server.URL+"/channel/payments/orders", "pay_channel")
	if !hasPlan(listed, ord["id"].(string)) {
		t.Fatalf("channel orders missing: %+v", listed)
	}
	adminPays := getAuthJSON(t, server.URL+"/admin/payments", "pay_admin")
	if !hasPlan(adminPays, ord["id"].(string)) {
		t.Fatalf("admin payments missing channel order: %+v", adminPays)
	}

	lights := getAuthJSON(t, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/payments", "pay_admin")
	found := false
	for _, raw := range lights["item"].(map[string]any)["lanes"].([]any) {
		lane := raw.(map[string]any)
		if lane["adapter"] == "alipay" && (lane["state"] == "sandbox" || lane["instance_count"].(float64) >= 1) {
			found = true
		}
	}
	if !found {
		t.Fatalf("admin lights: %+v", lights)
	}
}
