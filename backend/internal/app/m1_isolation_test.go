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
	server := httptest.NewServer(application.Router())
	defer server.Close()

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

	oemBrand := getAuthJSON(t, server.URL+"/v1/public/brand?host=oem.localhost", "")
	brand, _ := oemBrand["brand"].(map[string]any)
	if brand["name"] != "Aurora OEM" {
		t.Fatalf("oem brand: %+v", oemBrand)
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
