package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestChannelAdministratorHandoff(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" {
		t.Skip("postgres required")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "handoff_admin"
	cfg.BootstrapUser = "handoff_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	a := mustApp(t, cfg)
	server := httptest.NewServer(a.Router())
	defer server.Close()
	email := "handoff-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": email, "password": "password1", "promotion_code": identity.PromoKOL2B})
	uid := userIDOf(reg)
	userToken := tokenOf(reg)
	login := func(name string) string {
		return tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{"email": name + "@tokenhub.local", "password": "password1"}))
	}
	admin := login("admin")
	parent, err := a.Identity.CreateChannel(context.Background(), identity.Principal{Roles: []string{"platform_admin"}}, identity.ChannelInput{Code: "handoff-c-" + strconv.FormatInt(time.Now().UnixNano(), 10), Type: "C", BrandID: identity.OEMBrandID})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Catalog.SetChannelModels(context.Background(), parent.ID, []catalog.ChannelModelGrant{{PublicID: "tokenhub/echo-1", Enabled: true, Wholesale: map[string]string{"input": "0.0000007", "output": "0.0000014"}}}); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]string{"code": "handoff-b-" + strconv.FormatInt(time.Now().UnixNano(), 10), "type": "B", "parent_id": parent.ID})
	createReq, _ := http.NewRequest("POST", server.URL+"/admin/channels", bytes.NewReader(payload))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", "Bearer "+admin)
	createReq.Header.Set("X-Tokenhub-Confirm", "1")
	createRes, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var child map[string]any
	json.NewDecoder(createRes.Body).Decode(&child)
	createRes.Body.Close()
	if createRes.StatusCode != 201 {
		t.Fatalf("child %d %+v", createRes.StatusCode, child)
	}
	item := child["item"].(map[string]any)
	if item["brand_id"] != identity.OEMBrandID {
		t.Fatal("child lost OEM brand")
	}
	granted, err := a.Catalog.ListChannelModels(context.Background(), item["id"].(string), false)
	if err != nil || len(granted) != 1 {
		t.Fatalf("child model grants %+v %v", granted, err)
	}

	brandChannel, err := a.Identity.ChannelIDByBrand(context.Background(), identity.OEMBrandID)
	if err != nil || brandChannel != identity.OEMChannelID {
		t.Fatalf("OEM public catalog switched to reseller: %s %v", brandChannel, err)
	}
	channel := login("channel.b")
	finance := login("finance")
	tech := login("tech")
	auditToken := login("audit")
	call := func(token, ch, target string, enabled, confirm bool) int {
		t.Helper()
		payload, _ := json.Marshal(map[string]any{"email": target, "enabled": enabled, "reason": "local test"})
		req, _ := http.NewRequest("POST", server.URL+"/admin/channels/"+ch+"/admins", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		if confirm {
			req.Header.Set("X-Tokenhub-Confirm", "1")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		return res.StatusCode
	}
	for _, token := range []string{userToken, channel, finance, tech, auditToken} {
		if code := call(token, identity.ResellerChannelID, email, true, true); code != 403 {
			t.Fatalf("unauthorized %d", code)
		}
	}
	if code := call(admin, identity.ResellerChannelID, email, true, false); code < 400 {
		t.Fatal("confirmation missing")
	}
	if code := call(admin, identity.OEMChannelID, email, true, true); code != 409 {
		t.Fatalf("cross-channel %d", code)
	}
	if err := a.DB.Callback().Create().Before("gorm:create").Register("fail_handoff_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_logs" {
			tx.AddError(errors.New("injected audit failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	code := call(admin, identity.ResellerChannelID, email, true, true)
	_ = a.DB.Callback().Create().Remove("fail_handoff_audit")
	if code != 500 {
		t.Fatalf("audit failure %d", code)
	}
	count := func() int64 {
		var n int64
		a.DB.Table("identity_user_roles ur").Joins("JOIN identity_roles r ON r.id=ur.role_id").Where("ur.user_id=? AND r.code=?", uid, "channel_admin").Count(&n)
		return n
	}
	if count() != 0 {
		t.Fatal("permission survived failed audit")
	}
	for i := 0; i < 2; i++ {
		if code := call(admin, identity.ResellerChannelID, email, true, true); code != 200 {
			t.Fatalf("grant %d", code)
		}
	}
	if count() != 1 {
		t.Fatal("duplicate/missing grant")
	}
	req, _ := http.NewRequest("GET", server.URL+"/channel/me", nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("existing session failed to gain channel access")
	}
	if code := call(admin, identity.ResellerChannelID, email, false, true); code != 200 {
		t.Fatalf("revoke %d", code)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("revoked session retained channel access")
	}
	if count() != 0 {
		t.Fatal("role remains")
	}
}
