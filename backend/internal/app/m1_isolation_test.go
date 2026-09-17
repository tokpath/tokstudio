package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/app"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"github.com/tokpath/tokstudio/backend/internal/platform/logx"
	"github.com/tokpath/tokstudio/backend/internal/platform/redisx"
	"net/http/httptest"
)

func TestM1IdentityIsolation(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires TOKENHUB_DATABASE_URL and TOKENHUB_REDIS_URL")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m1_admin_token"
	cfg.BootstrapUser = "m1_user_token"
	cfg.BootstrapChannel = "m1_channel_token"
	cfg.AllowDemoProbes = true
	cfg.Env = "test"

	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	rdb, err := redisx.Open(cfg.RedisURL)
	if err != nil {
		t.Fatal(err)
	}
	application := app.New(cfg, gdb, rdb, logx.New("error", os.Stdout))
	if err := application.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := application.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer application.Close()
	server := httptest.NewServer(application.Router())
	defer server.Close()
	channelLogin := postBody(t, server.URL+"/v1/auth/login", "", map[string]string{
		"email": "channel.b@tokenhub.local", "password": identity.BootstrapPassword,
	})
	if channelLogin["session"] == nil {
		t.Fatalf("channel bootstrap should login with password: %+v", channelLogin)
	}

	suffix := time.Now().UTC().Format("150405.000000")
	regA := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "alice-a+" + suffix + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	regB := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "bob-b+" + suffix + "@example.test", "password": "password1", "promotion_code": "THB1",
	})
	if channelOf(regA) == channelOf(regB) {
		t.Fatal("A and B users must belong to different channels")
	}

	if code := mustStatusJSON(t, http.MethodPost, server.URL+"/v1/auth/otp/request", "", map[string]string{
		"email": "alice-a+" + suffix + "@example.test", "purpose": "login",
	}); code != http.StatusNotFound {
		t.Fatalf("otp request should be 404, got %d", code)
	}
	if code := mustStatusJSON(t, http.MethodPost, server.URL+"/v1/auth/otp/verify", "", map[string]string{
		"email": "alice-a+" + suffix + "@example.test", "code": "000000",
	}); code != http.StatusNotFound {
		t.Fatalf("otp verify should be 404, got %d", code)
	}

	switchCode := mustStatusJSON(t, http.MethodPost, server.URL+"/v1/me/channel/switch", tokenOf(regA), map[string]string{
		"channel_org_id": identity.ResellerChannelID,
	})
	if switchCode != http.StatusForbidden {
		t.Fatalf("self switch expected 403, got %d", switchCode)
	}

	if mustStatusJSON(t, http.MethodGet, server.URL+"/channel/users", "", nil) != http.StatusForbidden {
		t.Fatal("unauth channel users must be 403")
	}

	channelUsers := getAuthJSON(t, server.URL+"/channel/users", "m1_channel_token")
	items, _ := channelUsers["items"].([]any)
	for _, item := range items {
		row := item.(map[string]any)
		if row["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("channel admin leaked another channel: %+v", row)
		}
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/channel/plans", "", nil) != http.StatusForbidden {
		t.Fatal("unauth channel plans must be 403")
	}
	promos := getAuthJSON(t, server.URL+"/channel/promotion-codes", "m1_channel_token")
	foundTHB := false
	for _, raw := range promos["items"].([]any) {
		row := raw.(map[string]any)
		if row["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("promo leaked another channel: %+v", row)
		}
		if row["code"] == "THB1" {
			foundTHB = true
		}
	}
	if !foundTHB {
		t.Fatalf("channel B promos missing THB1: %+v", promos)
	}
	if mustStatusJSON(t, http.MethodPost, server.URL+"/admin/acquisition-roles", "m1_admin_token", map[string]string{
		"channel_org_id": identity.ResellerChannelID, "type": identity.AcqKOL2, "parent_id": identity.KOL1BRoleID,
	}) != http.StatusConflict {
		t.Fatal("create acquisition role without confirm must be 409")
	}
	if mustStatusJSON(t, http.MethodPost, server.URL+"/admin/promotion-codes", "m1_admin_token", map[string]string{
		"channel_org_id": identity.ResellerChannelID, "code": "THB-NOCONFIRM",
	}) != http.StatusConflict {
		t.Fatal("create promotion code without confirm must be 409")
	}
	role := postJSONRaw(t, server.URL+"/admin/acquisition-roles", "m1_admin_token", map[string]any{
		"channel_org_id": identity.ResellerChannelID, "type": identity.AcqKOL2, "parent_id": identity.KOL1BRoleID,
	})
	if role["item"].(map[string]any)["type"] != identity.AcqKOL2 {
		t.Fatalf("create role: %+v", role)
	}
	roleID := role["item"].(map[string]any)["id"].(string)
	gotRole := getAuthJSON(t, server.URL+"/admin/acquisition-roles/"+roleID, "m1_admin_token")
	if gotRole["item"].(map[string]any)["id"] != roleID {
		t.Fatalf("get role: %+v", gotRole)
	}
	if mustStatusJSON(t, http.MethodPatch, server.URL+"/admin/acquisition-roles/"+roleID, "m1_admin_token", map[string]string{
		"status": "disabled",
	}) != http.StatusConflict {
		t.Fatal("patch acquisition role without confirm must be 409")
	}
	patchedRole := patchJSONRaw(t, server.URL+"/admin/acquisition-roles/"+roleID, "m1_admin_token", map[string]any{"status": "disabled"})
	if patchedRole["item"].(map[string]any)["status"] != "disabled" {
		t.Fatalf("patch role: %+v", patchedRole)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/acquisition-roles/"+roleID, "m1_admin_token", map[string]any{"status": "active"})
	agents := getAuthJSON(t, server.URL+"/admin/acquisition-roles?type=agent", "m1_admin_token")
	for _, raw := range agents["items"].([]any) {
		if raw.(map[string]any)["type"] != identity.AcqAgent {
			t.Fatalf("type=agent leaked: %+v", raw)
		}
	}
	kols := getAuthJSON(t, server.URL+"/admin/acquisition-roles?type=kol", "m1_admin_token")
	for _, raw := range kols["items"].([]any) {
		typ := raw.(map[string]any)["type"]
		if typ != identity.AcqPromoter && typ != identity.AcqKOL1 && typ != identity.AcqKOL2 {
			t.Fatalf("type=kol leaked: %+v", raw)
		}
	}
	ch := getAuthJSON(t, server.URL+"/admin/channels/"+identity.ResellerChannelID, "m1_admin_token")
	if ch["item"].(map[string]any)["code"] != identity.ResellerChannelCode {
		t.Fatalf("get channel: %+v", ch)
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/admin/channels/chn_missing", "m1_admin_token", nil) != http.StatusNotFound {
		t.Fatal("missing channel must be 404")
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/admin/channels/"+identity.OfficialChannelID, "m1_channel_token", nil) != http.StatusForbidden {
		t.Fatal("channel admin must not read another tenant")
	}
	own := getAuthJSON(t, server.URL+"/admin/channels/"+identity.ResellerChannelID, "m1_channel_token")
	if own["item"].(map[string]any)["id"] != identity.ResellerChannelID {
		t.Fatalf("channel admin own tenant: %+v", own)
	}
	models := getAuthJSON(t, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/models", "m1_admin_token")
	if models["items"] == nil {
		t.Fatalf("channel models missing: %+v", models)
	}
	foundTenantEcho := false
	for _, raw := range models["items"].([]any) {
		row := raw.(map[string]any)
		if _, ok := row["credential_ref"]; ok {
			t.Fatalf("tenant models must not leak provider credentials: %+v", row)
		}
		if row["public_id"] == "tokenhub/echo-1" {
			foundTenantEcho = true
		}
	}
	if !foundTenantEcho {
		t.Fatalf("reseller tenant should inherit platform echo model: %+v", models)
	}
	if mustStatusJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/models", "m1_admin_token", nil) != http.StatusConflict {
		t.Fatal("patch channel models without confirm must be 409")
	}
	if mustStatusJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/models", "m1_channel_token", nil) != http.StatusForbidden {
		t.Fatal("channel admin must not grant tenant models")
	}
	unknownStatus, unknown := doJSON(t, http.MethodPatch, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/models", "m1_admin_token", true, map[string]any{
		"items": []map[string]any{{"public_id": "tenant/custom-model", "enabled": true}},
	})
	if unknownStatus != http.StatusBadRequest {
		t.Fatalf("unknown catalog model must be 400, got %d %+v", unknownStatus, unknown)
	}
	if errObj, _ := unknown["error"].(map[string]any); errObj["code"] != "invalid_request" {
		t.Fatalf("unknown catalog model code: %+v", unknown)
	}
	granted := patchJSONRaw(t, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/models", "m1_admin_token", map[string]any{
		"items": []map[string]any{{"public_id": "tokenhub/echo-1", "enabled": true}, {"public_id": "google/gemini-flash", "enabled": false}},
	})
	foundEchoEnabled, foundGeminiDisabled := false, false
	for _, raw := range granted["items"].([]any) {
		row := raw.(map[string]any)
		if row["public_id"] == "tokenhub/echo-1" && row["enabled"] == true {
			foundEchoEnabled = true
		}
		if row["public_id"] == "google/gemini-flash" && row["enabled"] == false {
			foundGeminiDisabled = true
		}
	}
	if !foundEchoEnabled || !foundGeminiDisabled {
		t.Fatalf("platform grant from catalog: %+v", granted)
	}
	_ = patchJSONRaw(t, server.URL+"/admin/channels/"+identity.ResellerChannelID+"/models", "m1_admin_token", map[string]any{
		"items": []map[string]any{{"public_id": "tokenhub/echo-1", "enabled": true}, {"public_id": "google/gemini-flash", "enabled": true}},
	})
	ownModels := getAuthJSON(t, server.URL+"/channel/models", "m1_channel_token")
	if ownModels["items"] == nil {
		t.Fatalf("channel console models missing: %+v", ownModels)
	}
	promoCode := "THB-ADM-" + time.Now().UTC().Format("150405000")
	createdPromo := postJSONRaw(t, server.URL+"/admin/promotion-codes", "m1_admin_token", map[string]any{
		"channel_org_id": identity.ResellerChannelID, "acquisition_role_id": role["item"].(map[string]any)["id"], "code": promoCode,
	})
	if createdPromo["item"].(map[string]any)["code"] != promoCode {
		t.Fatalf("create promo: %+v", createdPromo)
	}
	created := postJSONRaw(t, server.URL+"/channel/plans", "m1_channel_token", map[string]any{
		"name": "Channel Console Cheap", "price_minor": 1000, "owner_id": identity.OfficialChannelID,
		"items": []map[string]any{{"unit_type": "usd_credit", "included_amount": 1}},
	})
	createdItem := created["item"].(map[string]any)
	if createdItem["status"] != "pending_review" || createdItem["owner_id"] != identity.ResellerChannelID {
		t.Fatalf("channel create plan should stay on reseller and enter review: %+v", created)
	}
	plans := getAuthJSON(t, server.URL+"/channel/plans", "m1_channel_token")
	for _, raw := range plans["items"].([]any) {
		row := raw.(map[string]any)
		if row["owner_type"] == "channel" && row["owner_id"] != identity.ResellerChannelID {
			t.Fatalf("channel plans leaked another owner: %+v", row)
		}
	}
	usage := getAuthJSON(t, server.URL+"/channel/usage", "m1_channel_token")
	if usage["usage"] == nil {
		t.Fatalf("channel usage missing: %+v", usage)
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/channel/attribution", "", nil) != http.StatusForbidden {
		t.Fatal("unauth channel attribution must be 403")
	}
	attr := getAuthJSON(t, server.URL+"/channel/attribution", "m1_channel_token")
	foundAttrTHB := false
	for _, raw := range attr["items"].([]any) {
		row := raw.(map[string]any)
		if row["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("attribution leaked another channel: %+v", row)
		}
		if row["source_code"] == "THA1" {
			t.Fatalf("attribution leaked official promo: %+v", row)
		}
		if row["source_code"] == "THB1" {
			foundAttrTHB = true
		}
	}
	if !foundAttrTHB {
		t.Fatalf("channel B attribution missing THB1: %+v", attr)
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/channel/settlements", "", nil) != http.StatusForbidden {
		t.Fatal("unauth channel settlements must be 403")
	}
	settlements := getAuthJSON(t, server.URL+"/channel/settlements", "m1_channel_token")
	if settlements["items"] == nil {
		t.Fatalf("channel settlements missing: %+v", settlements)
	}
	for _, raw := range settlements["items"].([]any) {
		row := raw.(map[string]any)
		if row["channel_org_id"] != identity.ResellerChannelID {
			t.Fatalf("settlement leaked another channel: %+v", row)
		}
	}

	if mustStatusJSON(t, http.MethodGet, server.URL+"/v1/public/models", "", nil) != http.StatusOK {
		t.Fatal("public models must be readable without auth")
	}
	publicModels := getAuthJSON(t, server.URL+"/v1/public/models", "")
	foundEcho := false
	for _, raw := range publicModels["items"].([]any) {
		row := raw.(map[string]any)
		if _, ok := row["providers"]; ok {
			t.Fatalf("public models must not leak providers: %+v", row)
		}
		if row["id"] == "tokenhub/echo-1" {
			foundEcho = true
		}
	}
	if !foundEcho {
		t.Fatalf("official public models missing echo-1: %+v", publicModels)
	}
	oemModels := getAuthJSON(t, server.URL+"/v1/public/models?host=oem.localhost", "")
	foundOEM := false
	for _, raw := range oemModels["items"].([]any) {
		row := raw.(map[string]any)
		if _, ok := row["providers"]; ok {
			t.Fatalf("oem public models leaked providers: %+v", row)
		}
		if row["id"] == "tokenhub/oem-demo" {
			foundOEM = true
		}
	}
	if !foundOEM {
		t.Fatalf("oem public models missing oem-demo: %+v", oemModels)
	}
	oemBrand := getAuthJSON(t, server.URL+"/v1/public/brand?host=oem.localhost", "")
	brand, _ := oemBrand["brand"].(map[string]any)
	if brand["name"] != "Aurora OEM" {
		t.Fatalf("oem brand: %+v", oemBrand)
	}
	docs := getAuthJSON(t, server.URL+"/v1/public/docs-context?host=oem.localhost", "")
	if !containsText(docs, "Aurora OEM") || !containsText(docs, "${TOKENHUB_API_KEY}") || !containsText(docs, "/v1/messages") {
		t.Fatalf("oem docs examples: %+v", docs)
	}

	me := getAuthJSON(t, server.URL+"/v1/me", tokenOf(regA))["user"].(map[string]any)
	if me["locale"] != "zh" || me["display_name"] != "" {
		t.Fatalf("default profile: %+v", me)
	}
	updated := patchJSONRaw(t, server.URL+"/v1/me", tokenOf(regA), map[string]any{
		"display_name": "Alice A", "locale": "en",
	})["user"].(map[string]any)
	if updated["display_name"] != "Alice A" || updated["locale"] != "en" {
		t.Fatalf("updated profile: %+v", updated)
	}
	if updated["channel_org_id"] != identity.OfficialChannelID {
		t.Fatalf("settings must not change channel: %+v", updated)
	}
	if code := mustStatusJSON(t, http.MethodPatch, server.URL+"/v1/me", tokenOf(regA), map[string]string{"locale": "fr"}); code != http.StatusBadRequest {
		t.Fatalf("unsupported locale expected 400, got %d", code)
	}
	_ = postJSONRaw(t, server.URL+"/v1/me/password", tokenOf(regA), map[string]any{
		"current_password": "password1", "new_password": "password2",
	})
	emailA := "alice-a+" + suffix + "@example.test"
	if mustStatusJSON(t, http.MethodPost, server.URL+"/v1/auth/login", "", map[string]string{
		"email": emailA, "password": "password1",
	}) != http.StatusForbidden {
		t.Fatal("old password must fail after change")
	}
	login := postBody(t, server.URL+"/v1/auth/login", "", map[string]string{
		"email": emailA, "password": "password2",
	})
	if tokenOf(login) == "" {
		t.Fatalf("new password login: %+v", login)
	}

	userA := login["session"].(map[string]any)["user"].(map[string]any)
	userAID := userA["id"].(string)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", tokenOf(login), map[string]any{"name": "ban-key"})["item"].(map[string]any)["key"].(string)
	if mustStatusJSON(t, http.MethodPost, server.URL+"/admin/users/"+userAID+"/ban", "m1_admin_token", map[string]string{"reason": "abuse"}) != http.StatusConflict {
		t.Fatal("ban without confirm must be 409")
	}
	adminID := getAuthJSON(t, server.URL+"/v1/me", "m1_admin_token")["user"].(map[string]any)["id"].(string)
	selfBan := mustStatusJSONConfirm(t, http.MethodPost, server.URL+"/admin/users/"+adminID+"/ban", "m1_admin_token", map[string]string{"reason": "self"})
	if selfBan != http.StatusForbidden {
		t.Fatalf("self ban expected 403, got %d", selfBan)
	}
	banned := postJSONRaw(t, server.URL+"/admin/users/"+userAID+"/ban", "m1_admin_token", map[string]any{"reason": "abuse"})
	if banned["item"].(map[string]any)["status"] != identity.UserStatusBanned {
		t.Fatalf("ban status: %+v", banned)
	}
	if mustStatusJSON(t, http.MethodPost, server.URL+"/v1/auth/login", "", map[string]string{
		"email": emailA, "password": "password2",
	}) != http.StatusForbidden {
		t.Fatal("banned user login must fail")
	}
	if mustStatusJSON(t, http.MethodGet, server.URL+"/v1/me", tokenOf(login), nil) != http.StatusForbidden {
		t.Fatal("banned session must be 403")
	}
	if mustStatusJSON(t, http.MethodPost, server.URL+"/v1/chat/completions", apiKey, map[string]string{"model": "tokenhub/echo-1"}) != http.StatusForbidden {
		t.Fatal("banned api key must be 403")
	}
	audit := getAuthJSON(t, server.URL+"/admin/audit-logs?action=identity.user.ban", "m1_admin_token")
	if !containsText(audit, userAID) {
		t.Fatalf("ban audit missing: %+v", audit)
	}
	unbanned := postJSONRaw(t, server.URL+"/admin/users/"+userAID+"/unban", "m1_admin_token", map[string]any{"reason": "appeal"})
	if unbanned["item"].(map[string]any)["status"] != identity.UserStatusActive {
		t.Fatalf("unban status: %+v", unbanned)
	}
	if tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{
		"email": emailA, "password": "password2",
	})) == "" {
		t.Fatal("unban must restore login")
	}
	moved := postJSONRaw(t, server.URL+"/admin/users/"+userAID+"/attribution", "m1_admin_token", map[string]any{
		"promotion_code": "THB1", "reason": "manual move",
	})
	if moved["status"] != "updated" {
		t.Fatalf("reattribute: %+v", moved)
	}
	movedMe := getAuthJSON(t, server.URL+"/v1/me", tokenOf(postBody(t, server.URL+"/v1/auth/login", "", map[string]string{
		"email": emailA, "password": "password2",
	})))["user"].(map[string]any)
	if movedMe["channel_org_id"] != identity.ResellerChannelID || movedMe["source_code"] != "THB1" {
		t.Fatalf("reattribute user: %+v", movedMe)
	}
}

func postBody(t *testing.T, url, token string, payload map[string]string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("POST %s status %d body=%v", url, resp.StatusCode, out)
	}
	return out
}

func getAuthJSON(t *testing.T, url, token string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func doJSON(t *testing.T, method, url, token string, confirm bool, payload any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		reader = bytes.NewReader(mustJSON(payload))
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if confirm {
		req.Header.Set("X-Tokenhub-Confirm", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func mustStatusJSON(t *testing.T, method, url, token string, payload map[string]string) int {
	t.Helper()
	var reader *bytes.Reader
	if payload != nil {
		body, _ := json.Marshal(payload)
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func mustStatusJSONConfirm(t *testing.T, method, url, token string, payload map[string]string) int {
	t.Helper()
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(method, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func tokenOf(body map[string]any) string {
	session, _ := body["session"].(map[string]any)
	token, _ := session["token"].(string)
	return token
}

func channelOf(body map[string]any) string {
	session, _ := body["session"].(map[string]any)
	user, _ := session["user"].(map[string]any)
	channel, _ := user["channel_org_id"].(string)
	return channel
}
