package app_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func grantBonusRequest(t *testing.T, endpoint, token, key string, body map[string]any) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(mustJSON(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tokenhub-Confirm", "1")
	req.Header.Set("Idempotency-Key", key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

func TestBonusRecipientJourney(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "bonus_admin"
	cfg.BootstrapUser = "bonus_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	email := "bonus-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": email, "password": "password1", "promotion_code": "THA1"})
	userID := userIDOf(reg)
	for _, token := range []string{"bonus_admin", "bonus_admin-finance", "bonus_admin-ops"} {
		result := getAuthJSON(t, server.URL+"/admin/billing/users?q="+url.QueryEscape(email), token)
		items := result["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("search: %+v", result)
		}
		row := items[0].(map[string]any)
		if row["id"] != userID || row["email"] != email || len(row) != 5 {
			t.Fatalf("minimal recipient projection: %+v", row)
		}
	}
	for _, token := range []string{"", tokenOf(reg), "bonus_admin-tech", "bonus_admin-audit"} {
		code, _ := doJSON(t, http.MethodGet, server.URL+"/admin/billing/users?q=bonus", token, false, nil)
		if code != 401 && code != 403 {
			t.Fatalf("unauthorized search %q: %d", token, code)
		}
	}
	for _, token := range []string{"bonus_admin-finance", "bonus_admin-ops"} {
		code, _ := doJSON(t, http.MethodGet, server.URL+"/admin/users", token, false, nil)
		if code != 403 {
			t.Fatalf("must not grant user management: %d", code)
		}
	}
	for _, q := range []string{"", "a"} {
		code, _ := doJSON(t, http.MethodGet, server.URL+"/admin/billing/users?q="+q, "bonus_admin-finance", false, nil)
		if code != 400 {
			t.Fatalf("short search: %d", code)
		}
	}
	if items := getAuthJSON(t, server.URL+"/admin/billing/users?q=%25%25", "bonus_admin-finance")["items"].([]any); len(items) != 0 {
		t.Fatal("wildcard enumeration")
	}
	endpoint := server.URL + "/admin/entitlements/bonus"
	body := map[string]any{"user_id": userID, "unit_type": "usd_credit", "amount": 2500000, "expires_in_seconds": 86400}
	if code, _ := grantBonusRequest(t, endpoint, "bonus_admin-finance", "", body); code != 400 {
		t.Fatalf("missing key: %d", code)
	}
	key := "journey-" + userID
	var wg sync.WaitGroup
	ids := make(chan string, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, res := grantBonusRequest(t, endpoint, "bonus_admin-finance", key, body)
			if code != 201 {
				t.Errorf("grant: %d %+v", code, res)
				return
			}
			ids <- res["item"].(map[string]any)["id"].(string)
		}()
	}
	wg.Wait()
	close(ids)
	entID := ""
	for got := range ids {
		if entID != "" && entID != got {
			t.Fatal("concurrent duplicate grant")
		}
		entID = got
	}
	if entID == "" {
		t.Fatal("no grant")
	}
	var n int64
	application.DB.Table("plans_entitlement_ledger").Where("account_id = ?", entID).Count(&n)
	if n != 1 {
		t.Fatalf("duplicate ledger: %d", n)
	}
	// The recipient can inspect the amount and expiry; wallet balance is not altered.
	ents := getAuthJSON(t, server.URL+"/v1/me/entitlements", tokenOf(reg))["items"].([]any)
	if len(ents) != 1 {
		t.Fatalf("recipient entitlements: %+v", ents)
	}
	entitlement := ents[0].(map[string]any)
	if asInt(entitlement["remaining"]) != 2500000 || entitlement["source_type"] != "bonus" || entitlement["expires_at"] == nil {
		t.Fatalf("receipt: %+v", entitlement)
	}
	replayCode, replay := grantBonusRequest(t, endpoint, "bonus_admin-finance", key, body)
	if replayCode != 201 || replay["item"].(map[string]any)["expires_at"] != entitlement["expires_at"] {
		t.Fatalf("retry changed expiry: %+v", replay)
	}

	// A ledger failure rolls back the entitlement; retrying the same key may safely complete.
	callback := "bonus-test-fail-ledger"
	if err := application.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "plans_entitlement_ledger" {
			tx.AddError(errors.New("injected ledger failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	failedCode, _ := grantBonusRequest(t, endpoint, "bonus_admin-finance", "rollback-"+key, body)
	application.DB.Callback().Create().Remove(callback)
	if failedCode != 500 {
		t.Fatalf("ledger failure status: %d", failedCode)
	}
	application.DB.Table("plans_entitlement_accounts").Where("user_id = ?", userID).Count(&n)
	if n != 1 {
		t.Fatalf("partial grant persisted: %d", n)
	}
	if code, _ := grantBonusRequest(t, endpoint, "bonus_admin-finance", "rollback-"+key, body); code != 201 {
		t.Fatalf("retry after rollback: %d", code)
	}
	body["amount"] = 3000000
	if code, _ := grantBonusRequest(t, endpoint, "bonus_admin-finance", key, body); code != 409 {
		t.Fatalf("changed payload: %d", code)
	}
	body["user_id"] = "usr_missing"
	if code, _ := grantBonusRequest(t, endpoint, "bonus_admin-finance", "missing-"+key, body); code != 400 {
		t.Fatalf("missing recipient: %d", code)
	}
	body["user_id"] = userID
	if err := application.DB.Exec("UPDATE identity_users SET status = 'banned' WHERE id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	if code, _ := grantBonusRequest(t, endpoint, "bonus_admin-finance", "banned-"+key, body); code != 400 {
		t.Fatalf("banned recipient: %d", code)
	}
}
