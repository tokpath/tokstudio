package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
)

func TestStaffPermissionsLifecycle(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "staff_admin"
	cfg.BootstrapUser = "staff_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	login := func(email string) string {
		return tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": email, "password": "password1"}))
	}
	admin, oem, b := login("admin@tokenhub.local"), login("channel.c@tokenhub.local"), login("channel.b@tokenhub.local")
	uid := func(token string) string {
		return getAuthJSON(t, server.URL+"/v1/me", token)["user"].(map[string]any)["id"].(string)
	}
	adminID, oemID := uid(admin), uid(oem)
	prefix := "staff-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	create := func(path, token, suffix string, roles []string) (string, string) {
		t.Helper()
		email := prefix + suffix + "@example.test"
		code, body := doJSON(t, http.MethodPost, server.URL+path, token, true, map[string]any{"email": email, "display_name": "Staff test", "password": "password1", "roles": roles})
		if code != 201 {
			t.Fatalf("create %s: %d %+v", suffix, code, body)
		}
		return body["item"].(map[string]any)["user_id"].(string), email
	}
	platformID, platformEmail := create("/admin/staff", admin, "-platform", []string{"ops_admin"})
	oemOpsID, oemOpsEmail := create("/channel/staff", oem, "-ops", []string{"oem_ops"})
	financeID, financeEmail := create("/channel/staff", oem, "-finance", []string{"oem_finance"})
	_, auditEmail := create("/channel/staff", oem, "-audit", []string{"oem_audit"})
	opToken, finToken, auditToken := login(oemOpsEmail), login(financeEmail), login(auditEmail)
	for _, token := range []string{b, opToken, finToken, auditToken, login(platformEmail)} {
		for _, path := range []string{"/admin/staff", "/channel/staff"} {
			if code := getStatus(t, server.URL+path, token); code != 403 {
				t.Fatalf("employee managed staff %s %d", path, code)
			}
		}
	}
	for _, role := range []string{"oem_ops", "oem_finance", "oem_audit"} {
		token := map[string]string{"oem_ops": opToken, "oem_finance": finToken, "oem_audit": auditToken}[role]
		p, err := a.Identity.Authenticate(context.Background(), "Bearer "+token)
		if err != nil || p.VisibleChannelID() != identity.OEMChannelID || p.HasRole("channel_admin") {
			t.Fatalf("role scope broadened: %+v %v", p, err)
		}
		for _, path := range []string{"/admin/metrics", "/admin/users", "/admin/providers", "/admin/audit-logs"} {
			if code := getStatus(t, server.URL+path, token); code != 403 {
				t.Fatalf("%s global %s %d", role, path, code)
			}
		}
		for _, path := range []string{"/channel/metrics", "/channel/reconciliation", "/channel/usage", "/channel/payments/orders"} {
			if code := getStatus(t, server.URL+path, token); code != 200 {
				t.Fatalf("%s denied %s %d", role, path, code)
			}
		}
		for _, path := range []string{"/channel/metrics?channel_id=" + identity.ResellerChannelID, "/channel/usage?channel_id=" + identity.ResellerChannelID, "/admin/channels/" + identity.ResellerChannelID, "/admin/channel-quotas/" + identity.ResellerChannelID} {
			if code := getStatus(t, server.URL+path, token); code < 400 {
				t.Fatalf("foreign data %s %d", path, code)
			}
		}
		if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/channels", token, true, map[string]any{"type": "C", "code": prefix + "-bad"}); code < 400 {
			t.Fatal("employee created child OEM")
		}
	}
	if code := getStatus(t, server.URL+"/channel/users", finToken); code != 403 {
		t.Fatal("finance accessed customers")
	}
	if code := getStatus(t, server.URL+"/channel/users", opToken); code != 200 {
		t.Fatal("ops cannot manage customers")
	}
	if code := getStatus(t, server.URL+"/channel/audit-logs", auditToken); code != 200 {
		t.Fatal("audit cannot read scoped audit")
	}
	for _, token := range []string{auditToken, finToken} {
		if code, _ := doJSON(t, http.MethodPost, server.URL+"/channel/plans", token, true, map[string]any{}); code != 403 {
			t.Fatal("unauthorized plan mutation")
		}
	}
	for _, test := range []struct {
		path, token string
		body        map[string]any
		want        int
	}{
		{"/channel/staff", oem, map[string]any{"email": prefix + "-global@example.test", "password": "password1", "roles": []string{"platform_admin"}}, 400},
		{"/channel/staff", oem, map[string]any{"email": "user@tokenhub.local", "roles": []string{"oem_ops"}}, 409},
		{"/admin/staff", admin, map[string]any{"email": oemOpsEmail, "roles": []string{"ops_admin"}}, 409},
	} {
		if code, _ := doJSON(t, http.MethodPost, server.URL+test.path, test.token, true, test.body); code != test.want {
			t.Fatalf("invalid addition %s %d", test.path, code)
		}
	}
	for _, test := range []struct {
		path, token string
		want        int
	}{
		{"/channel/staff/" + platformID, oem, 404}, {"/admin/staff/" + oemOpsID, admin, 404},
		{"/channel/staff/" + oemID, oem, 403}, {"/admin/staff/" + adminID, admin, 403},
	} {
		if code, _ := doJSON(t, http.MethodPatch, server.URL+test.path, test.token, true, map[string]any{"status": "disabled"}); code != test.want {
			t.Fatalf("invalid update %s %d", test.path, code)
		}
	}
	// Permission changes affect old sessions immediately.
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/channel/staff/"+oemOpsID, oem, true, map[string]any{"roles": []string{"oem_audit"}}); code != 200 {
		t.Fatal("change role")
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/channel/plans", opToken, true, map[string]any{}); code != 403 {
		t.Fatal("old session retained ops")
	}
	financePrincipal, err := a.Identity.Authenticate(context.Background(), "Bearer "+finToken)
	if err != nil {
		t.Fatal(err)
	}
	staffKey, err := a.Identity.CreateAPIKey(context.Background(), *financePrincipal, "staff-test", cfg.EncryptionKey, nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Audit failure rolls back roles and status together.
	if err := a.DB.Callback().Create().Before("gorm:create").Register("staff_fail_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	code, _ := doJSON(t, http.MethodPatch, server.URL+"/channel/staff/"+financeID, oem, true, map[string]any{"status": "disabled"})
	a.DB.Callback().Create().Remove("staff_fail_audit")
	if code != 500 || getStatus(t, server.URL+"/channel/metrics", finToken) != 200 {
		t.Fatal("partial disable after audit failure")
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/channel/staff/"+financeID, oem, true, map[string]any{"status": "disabled"}); code != 200 {
		t.Fatal("disable")
	}
	if getStatus(t, server.URL+"/channel/metrics", finToken) != 403 {
		t.Fatal("disabled session active")
	}
	if key, err := a.Identity.AuthenticateAPIKey(context.Background(), staffKey.Secret); (err != nil && !errors.Is(err, identity.ErrKeyNotUsable)) || key != nil {
		t.Fatal("disabled staff API key active")
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/v1/auth/login", "", false, map[string]string{"email": financeEmail, "password": "password1"}); code != 403 {
		t.Fatal("disabled staff can login")
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/channel/staff/"+financeID, oem, true, map[string]any{"status": "active"}); code != 200 {
		t.Fatal("reactivate")
	}
	if getStatus(t, server.URL+"/channel/metrics", finToken) != 403 {
		t.Fatal("reactivation resurrected session")
	}
	if getStatus(t, server.URL+"/channel/metrics", login(financeEmail)) != 200 {
		t.Fatal("reactivated login fails")
	}
	if key, err := a.Identity.AuthenticateAPIKey(context.Background(), staffKey.Secret); (err != nil && !errors.Is(err, identity.ErrKeyNotUsable)) || key != nil {
		t.Fatal("reactivation resurrected API key")
	}
	history := getAuthJSON(t, server.URL+"/channel/staff/"+financeID+"/history", oem)
	raw, _ := json.Marshal(history)
	if strings.Contains(string(raw), "password") || len(history["items"].([]any)) != 3 {
		t.Fatalf("unsafe/missing history: %s", raw)
	}
	if getStatus(t, server.URL+"/admin/staff/"+financeID+"/history", admin) != 404 {
		t.Fatal("history leaked across companies")
	}
	// Legacy OEM administrator handoff cannot remove its last administrator.
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/admin/channels/"+identity.OEMChannelID+"/admins", admin, true, map[string]any{"email": "channel.c@tokenhub.local", "enabled": false, "reason": "test"}); code != 409 {
		t.Fatalf("last administrator removed: %d", code)
	}
	customers := getAuthJSON(t, server.URL+"/channel/users", oem)["items"].([]any)
	for _, item := range customers {
		if item.(map[string]any)["id"] == financeID {
			t.Fatal("staff mixed with customers")
		}
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/channel/users/"+financeID+"/ban", oem, true, map[string]any{"reason": "test"}); code != 403 {
		t.Fatal("employee banned via customer API")
	}
	// Developer seed must preserve manual assignments and disabled state.
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/admin/staff/"+platformID, admin, true, map[string]any{"roles": []string{"audit_readonly"}}); code != 200 {
		t.Fatal("platform update")
	}
	seededFinance := getAuthJSON(t, server.URL+"/admin/staff", admin)["items"].([]any)
	var seededID string
	for _, item := range seededFinance {
		v := item.(map[string]any)
		if v["email"] == "finance@tokenhub.local" {
			seededID = v["user_id"].(string)
		}
	}
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/admin/staff/"+seededID, admin, true, map[string]any{"roles": []string{"ops_admin"}, "status": "disabled"}); code != 200 {
		t.Fatal("seeded update")
	}
	if err := a.Identity.Bootstrap(context.Background(), cfg.BootstrapAdmin, cfg.BootstrapUser, ""); err != nil {
		t.Fatal(err)
	}
	if code, _ := doJSON(t, http.MethodPost, server.URL+"/v1/auth/login", "", false, map[string]string{"email": "finance@tokenhub.local", "password": "password1"}); code != 403 {
		t.Fatal("bootstrap reactivated staff")
	}
	// Restore seeded fixture for other tests.
	if code, _ := doJSON(t, http.MethodPatch, server.URL+"/admin/staff/"+seededID, admin, true, map[string]any{"roles": []string{"finance_admin"}, "status": "active"}); code != 200 {
		t.Fatal("restore fixture")
	}
}
