package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCustomerSearchProjectionAndTotals(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "customers_admin"
	cfg.BootstrapUser = "customers_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	ctx := context.Background()
	prefix := fmt.Sprintf("customers-%d", time.Now().UnixNano())
	// 160 registrations in one scope; the 120th oldest is outside the former
	// latest-100 window. Fixtures have no credentials and are never real users.
	if err := a.DB.Exec(`INSERT INTO identity_users(id,email,display_name,status,channel_org_id,brand_id,created_at,updated_at) SELECT ? || '-' || n,? || '-' || n || '@example.test','Customer ' || n,'active',?,?,now()-n*interval '1 second',now() FROM generate_series(1,160) AS n`, prefix, prefix, identity.ResellerChannelID, identity.OfficialBrandID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO identity_user_roles(user_id,role_id,scope_type,scope_id) SELECT u.id,r.id,'channel',? FROM identity_users u CROSS JOIN identity_roles r WHERE u.id LIKE ? AND r.code='end_user'`, identity.ResellerChannelID, prefix+"-%").Error; err != nil {
		t.Fatal(err)
	}
	login := func(name string) string {
		return tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": name + "@tokenhub.local", "password": "password1"}))
	}
	admin, fin, audit, oem, b := login("admin"), login("finance"), login("audit"), login("channel.c"), login("channel.b")
	get := func(path, token string) map[string]any {
		t.Helper()
		status, body := doJSON(t, "GET", server.URL+path, token, false, nil)
		if status != 200 {
			t.Fatalf("%s %d %+v", path, status, body)
		}
		return body
	}
	target := prefix + "-120"
	first := get("/admin/customers?q="+url.QueryEscape(prefix), admin)
	if first["total"] != float64(160) || len(first["items"].([]any)) != 25 {
		t.Fatalf("full server count %+v", first)
	}
	found := get("/admin/customers?q="+url.QueryEscape(target), admin)
	if found["total"] != float64(1) || found["items"].([]any)[0].(map[string]any)["id"] != target {
		t.Fatal("old customer is not searchable")
	}
	cursor := first["next_cursor"].(string)
	next := get("/admin/customers?q="+url.QueryEscape(prefix)+"&cursor="+url.QueryEscape(cursor), admin)
	if next["items"].([]any)[0].(map[string]any)["id"] == first["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("same first page")
	}
	for _, path := range []string{"/admin/customers?q=other&cursor=" + url.QueryEscape(cursor), "/admin/customers?q=" + url.QueryEscape(prefix) + "&cursor=" + url.QueryEscape(cursor)} {
		token := admin
		if strings.Contains(path, "q="+prefix) {
			token = fin
		}
		if status, _ := doJSON(t, "GET", server.URL+path, token, false, nil); status != 400 {
			t.Fatalf("stale query/identity cursor %d", status)
		}
	}
	// Literal wildcard characters do not enumerate all customers.
	if get("/admin/customers?q=%25", admin)["total"] != float64(0) {
		t.Fatal("wildcard search widened")
	}
	if status, _ := doJSON(t, "GET", server.URL+"/channel/customers?q="+url.QueryEscape(target), oem, false, nil); status != 200 {
		t.Fatal(status)
	}
	if get("/channel/customers?q="+url.QueryEscape(target), oem)["total"] != float64(0) {
		t.Fatal("cross-brand customer leak")
	}
	if status, _ := doJSON(t, "GET", server.URL+"/channel/customers/"+target, oem, false, nil); status != 404 {
		t.Fatal("cross-brand detail leak")
	}
	if status, _ := doJSON(t, "GET", server.URL+"/channel/customers?channel_id="+identity.OEMChannelID, b, false, nil); status != 403 {
		t.Fatal("channel widened scope")
	}
	if get("/channel/customers?q="+url.QueryEscape(prefix), b)["total"] != float64(160) {
		t.Fatal("B customer range incomplete")
	}
	if err := a.DB.Exec(`INSERT INTO identity_acquisition_roles(id,channel_org_id,type,level,status,created_at) SELECT ? || '-pro-' || n,?,'agent',0,'active',now()-n*interval '1 second' FROM generate_series(1,160) n`, prefix, identity.ResellerChannelID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO identity_role_members(user_id,acquisition_role_id) SELECT ? || '-' || n,? || '-pro-' || n FROM generate_series(1,160) n`, prefix, prefix).Error; err != nil {
		t.Fatal(err)
	}
	professionals := get("/admin/professional-customers/search?q="+url.QueryEscape(prefix), admin)
	if professionals["total"] != float64(160) || len(professionals["items"].([]any)) != 25 {
		t.Fatalf("professional paging %+v", professionals)
	}
	oldProfessional := get("/admin/professional-customers/search?q="+url.QueryEscape(prefix+"-120@example.test"), admin)
	if oldProfessional["total"] != float64(1) {
		t.Fatal("old professional not searchable by customer email")
	}
	readOnlyPro := get("/admin/professional-customers/"+prefix+"-pro-120", audit)
	if readOnlyPro["manage"] != false {
		t.Fatal("audit received professional write action")
	}

	// Aggregate over 250 facts, retaining tiny amounts and separate reversals.
	if err := a.DB.Exec(`INSERT INTO billing_usage_events(id,request_id,user_id,channel_org_id,public_model_id,unit_usage_json,unit_prices_json,customer_amount_minor,upstream_cost_minor,wholesale_amount_minor,state,idempotency_key) SELECT ? || '-usage-' || n,? || '-request-' || n,?,?, 'echo','{}','{}',1,0,0,'confirmed',? || '-usage-' || n FROM generate_series(1,250) n`, prefix, prefix, target, identity.ResellerChannelID, prefix).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO gateway_requests(id,request_id,user_id,channel_org_id,public_model_id,protocol,status,started_at) SELECT ? || '-request-' || n,? || '-request-' || n,?,?,'echo','openai','succeeded',now() FROM generate_series(1,250) n`, prefix, prefix, target, identity.ResellerChannelID).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.DB.Exec(`INSERT INTO billing_customer_charges(id,request_id,usage_event_id,amount_minor,status) VALUES (?,?,?,?,'reversed')`, prefix+"-refund", prefix+"-request-1", prefix+"-usage-1", 7).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{admin, fin, audit, b} {
		surface := "admin"
		if token == b {
			surface = "channel"
		}
		detail := get("/"+surface+"/customers/"+target, token)
		if detail["usage"].(map[string]any)["refunded_minor"] != float64(7) || detail["usage"].(map[string]any)["confirmed_minor"] != float64(250) || detail["activity"].(map[string]any)["count"] != float64(250) {
			t.Fatalf("page used as lifetime total %+v", detail)
		}
		raw, _ := json.Marshal(detail)
		for _, field := range []string{"password_hash", "google_sub", "login_methods", "token_digest", "provider_id", "upstream_model_id"} {
			if strings.Contains(string(raw), field) {
				t.Fatalf("sensitive projection: %s", field)
			}
		}
		if token == fin {
			user := detail["item"].(map[string]any)
			if _, ok := user["roles"]; ok {
				t.Fatal("finance received role projection")
			}
			if detail["permissions"].(map[string]any)["manage"] != false || detail["permissions"].(map[string]any)["read_payments"] != true {
				t.Fatal("finance permission projection")
			}
		}
		if token == b {
			if _, ok := detail["credit"]; ok {
				t.Fatal("B received customer cash projection")
			}
		}
	}
	callback := "fail_customer_credit_" + prefix
	if err := a.DB.Callback().Row().Before("gorm:row").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_wallets" {
			tx.AddError(errors.New("isolated wallet read failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	partial := get("/admin/customers/"+target, fin)
	a.DB.Callback().Row().Remove(callback)
	if partial["errors"].(map[string]any)["credit"] == nil || partial["usage"] == nil || partial["activity"] == nil || partial["credit"] != nil {
		t.Fatalf("read failure disguised as zero or lost other sections %+v", partial)
	}
	// A customer's current attribution never authorizes foreign historical funds.
	if err := a.DB.Exec(`INSERT INTO payment_orders(id,user_id,channel_org_id,payee_channel_org_id,adapter,purpose,amount_minor,credit_minor,status) VALUES (?,?,?,?,'alipay','wallet',1,1,'paid'),(?,?,?,?,'alipay','wallet',99,99,'paid')`, prefix+"-own-order", target, identity.ResellerChannelID, identity.OfficialChannelID, prefix+"-foreign-order", target, identity.OEMChannelID, identity.OEMChannelID).Error; err != nil {
		t.Fatal(err)
	}
	limited := get("/admin/customers/"+target, fin)
	if limited["orders"].(map[string]any)["count"] != float64(1) || limited["credit"] != nil || limited["entitlements"] != nil || limited["errors"].(map[string]any)["credit"] == nil {
		t.Fatalf("foreign historical funds exposed %+v", limited)
	}
	if get("/admin/customers/"+target, admin)["credit"] == nil {
		t.Fatal("platform full-domain legacy credit projection lost")
	}
	// Platform super-admin retains historical cross-OEM read capability, while
	// finance does not receive an OEM customer or payment action.
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "-oem@example.test", "password": "password1", "promotion_code": "THC1"})
	oemID := userIDOf(reg)
	detail := get("/admin/customers/"+oemID, admin)
	if detail["permissions"].(map[string]any)["record_payment"] != false || detail["permissions"].(map[string]any)["manage"] != false || detail["permissions"].(map[string]any)["read_payments"] != false || detail["orders"] != nil {
		t.Fatal("platform cross-OEM write affordance")
	}
	if status, _ := doJSON(t, "GET", server.URL+"/admin/customers/"+oemID, fin, false, nil); status != 404 {
		t.Fatal("finance crossed brand")
	}
	if status, _ := doJSON(t, "GET", server.URL+"/admin/users", fin, false, nil); status != http.StatusForbidden {
		t.Fatal("legacy user route broadened")
	}
	if _, err := a.Identity.SearchCustomers(ctx, identity.Principal{Roles: []string{"end_user"}}, identity.CustomerQuery{}); !errors.Is(err, identity.ErrChannelImmutable) {
		t.Fatal("end user enumerated customer directory")
	}
}

func TestProfessionalCustomerAndChannelOnboarding(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "professional_admin"
	cfg.BootstrapUser = "professional_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	prefix := fmt.Sprintf("professional-%d", time.Now().UnixNano())
	login := func(name string) string {
		return tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": name + "@tokenhub.local", "password": "password1"}))
	}
	admin, oem, b := login("admin"), login("channel.c"), login("channel.b")
	get := func(path, token string) map[string]any {
		t.Helper()
		status, body := doJSON(t, "GET", server.URL+path, token, false, nil)
		if status != 200 {
			t.Fatalf("%s %d %+v", path, status, body)
		}
		return body
	}
	createChannel := func(token, name string) (string, string) {
		t.Helper()
		status, body := doJSON(t, "POST", server.URL+"/admin/channels", token, true, map[string]string{"code": name, "type": "B", "status": "active"})
		if status != 201 {
			t.Fatalf("channel %d %+v", status, body)
		}
		id := body["item"].(map[string]any)["id"].(string)
		onboard := get("/admin/channels/"+id+"/onboarding", token)
		item := onboard["item"].(map[string]any)
		if item["admin_count"] != float64(0) || onboard["model_count"] != float64(0) {
			t.Fatalf("fake ready state %+v", onboard)
		}
		link, err := url.Parse(item["registration_url"].(string))
		if err != nil || link.Path != "/login" || link.Host == "" || link.Query().Get("promotion_code") == "" {
			t.Fatalf("invalid canonical invitation %+v", item)
		}
		return id, link.Query().Get("promotion_code")
	}
	channel, code := createChannel(admin, prefix+"-platform")
	if replayID, _ := createChannel(admin, prefix+"-platform"); replayID != channel {
		t.Fatal("lost channel creation response did not recover original channel")
	}
	user := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "@example.test", "password": "password1", "promotion_code": code})
	userID := userIDOf(user)
	detail := get("/admin/customers/"+userID, admin)
	if detail["item"].(map[string]any)["channel_org_id"] != channel {
		t.Fatal("register intent did not retain channel")
	}
	var before struct {
		PasswordHash string
		ChannelOrgID string
		BrandID      string
		SourceCode   string
	}
	if err := a.DB.Table("identity_users u").Select("u.password_hash,u.channel_org_id,u.brand_id,a.source_code").Joins("JOIN identity_attributions a ON a.user_id=u.id").Where("u.id=?", userID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	create := func(token, id, typ, parent string) (int, map[string]any) {
		return doJSON(t, "POST", server.URL+"/admin/professional-customers", token, true, map[string]string{"user_id": id, "type": typ, "parent_id": parent})
	}
	status, body := create(admin, userID, "agent", "")
	if status != 201 {
		t.Fatalf("professional %d %+v", status, body)
	}
	roleID := body["item"].(map[string]any)["id"].(string)
	status, replay := create(admin, userID, "agent", "")
	if status != 201 || replay["item"].(map[string]any)["id"] != roleID {
		t.Fatal("repeat creation changed original membership")
	}
	if status, _ := create(admin, userID, "kol_l1", roleID); status < 400 {
		t.Fatal("changed existing professional relation")
	}
	pro := get("/admin/professional-customers/"+roleID, admin)["item"].(map[string]any)
	if len(pro["members"].([]any)) != 1 || len(pro["codes"].([]any)) != 1 {
		t.Fatalf("missing bound customer or valid share link %+v", pro)
	}
	var after struct {
		PasswordHash string
		ChannelOrgID string
		BrandID      string
		SourceCode   string
	}
	if err := a.DB.Table("identity_users u").Select("u.password_hash,u.channel_org_id,u.brand_id,a.source_code").Joins("JOIN identity_attributions a ON a.user_id=u.id").Where("u.id=?", userID).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if before != after || before.PasswordHash == "" {
		t.Fatal("professional binding rewrote account or attribution")
	}
	if status, _ := create(b, userID, "agent", ""); status < 400 {
		t.Fatal("B manager widened ownership")
	}
	// OEM can create/read its own direct B customer's relationship, while the
	// platform has read-only cross-brand capability and cannot bind that customer.
	child, childCode := createChannel(oem, prefix+"-oem")
	oemUser := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "-oem@example.test", "password": "password1", "promotion_code": childCode})
	oemID := userIDOf(oemUser)
	if status, _ := create(admin, oemID, "agent", ""); status != 403 {
		t.Fatalf("platform cross-OEM bind %d", status)
	}
	status, oemRole := create(oem, oemID, "agent", "")
	if status != 201 {
		t.Fatalf("OEM child bind %d %+v", status, oemRole)
	}
	oemRoleID := oemRole["item"].(map[string]any)["id"].(string)
	get("/admin/professional-customers/"+oemRoleID, oem)
	if get("/channel/customers/"+oemID, oem)["item"].(map[string]any)["channel_org_id"] != child {
		t.Fatal("OEM child projection")
	}
	unionUser := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "-union@example.test", "password": "password1", "promotion_code": "THC1"})
	unionID := userIDOf(unionUser)
	if err := a.DB.Exec(`INSERT INTO identity_user_roles(user_id,role_id,scope_type,scope_id) SELECT ?,id,'channel',? FROM identity_roles WHERE code IN ('oem_ops','oem_finance')`, unionID, identity.OEMChannelID).Error; err != nil {
		t.Fatal(err)
	}
	unionToken := tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": prefix + "-union@example.test", "password": "password1"}))
	unionDetail := get("/channel/customers/"+oemID, unionToken)
	permissions := unionDetail["permissions"].(map[string]any)
	if permissions["operations"] != true || permissions["finance"] != true || permissions["record_payment"] != true {
		t.Fatalf("multi-role union lost a permitted projection %+v", permissions)
	}
	get("/admin/professional-customers/"+oemRoleID, unionToken)
	// Simulate attribution committing while the creator waits for the user lock.
	raceUser := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "-race@example.test", "password": "password1", "promotion_code": code})
	raceID := userIDOf(raceUser)
	raceCallback := "move_professional_customer_" + prefix
	moved := false
	if err := a.DB.Callback().Query().Before("gorm:query").Register(raceCallback, func(tx *gorm.DB) {
		if !moved && tx.Statement.Table == "identity_users" {
			if _, locked := tx.Statement.Clauses["FOR"]; locked {
				moved = true
				tx.Exec("UPDATE identity_users SET channel_org_id=?,brand_id=? WHERE id=?", identity.OEMChannelID, identity.OEMBrandID, raceID)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	status, _ = create(admin, raceID, "agent", "")
	a.DB.Callback().Query().Remove(raceCallback)
	if !moved || status != 403 {
		t.Fatalf("pre-lock owner authorized changed attribution: moved=%v status=%d", moved, status)
	}
	// An audit insertion failure atomically rolls back role/member/code creation.
	rollbackUser := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": prefix + "-rollback@example.test", "password": "password1", "promotion_code": code})
	rollbackID := userIDOf(rollbackUser)
	callback := "fail_professional_audit_" + prefix
	if err := a.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("isolated audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	status, _ = create(admin, rollbackID, "agent", "")
	a.DB.Callback().Create().Remove(callback)
	if status < 400 {
		t.Fatal("audit failure accepted")
	}
	var count int64
	a.DB.Table("identity_role_members m").Joins("JOIN identity_acquisition_roles r ON r.id=m.acquisition_role_id").Where("m.user_id=? AND r.type IN ?", rollbackID, []string{identity.AcqAgent, identity.AcqKOL1, identity.AcqKOL2}).Count(&count)
	if count != 0 {
		t.Fatal("failed audit retained role membership")
	}
	// True server search and stable pages for 160 direct OEM channels.
	if err := a.DB.Exec(`INSERT INTO identity_channel_orgs(id,code,type,parent_id,status,brand_id,created_at) SELECT ? || '-channel-' || n,? || '-channel-' || n,'B',?,'active',?,now()-n*interval '1 second' FROM generate_series(1,160) n`, prefix, prefix, identity.OEMChannelID, identity.OEMBrandID).Error; err != nil {
		t.Fatal(err)
	}
	first := get("/channel/subchannels?q="+url.QueryEscape(prefix+"-channel-"), oem)
	if first["total"] != float64(160) || len(first["items"].([]any)) != 25 {
		t.Fatalf("subchannels not server paged %+v", first)
	}
	found := get("/channel/subchannels?q="+url.QueryEscape(prefix+"-channel-120"), oem)
	if found["total"] != float64(1) {
		t.Fatal("old channel not searchable")
	}
	next := get("/channel/subchannels?q="+url.QueryEscape(prefix+"-channel-")+"&cursor="+url.QueryEscape(first["next_cursor"].(string)), oem)
	if next["items"].([]any)[0].(map[string]any)["id"] == first["items"].([]any)[0].(map[string]any)["id"] {
		t.Fatal("subchannel paging repeated")
	}
	if status, _ := doJSON(t, "GET", server.URL+"/channel/subchannels", b, false, nil); status != 403 {
		t.Fatal("B enumerated OEM child channels")
	}
}
