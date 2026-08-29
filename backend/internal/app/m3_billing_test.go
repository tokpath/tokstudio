package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestM3BillingInvariants(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "m3_admin"
	cfg.BootstrapUser = "m3_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()

	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "bill-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	session := tokenOf(reg)
	apiKey := postJSONRaw(t, server.URL+"/v1/me/api-keys", session, map[string]any{"name": "m3"})["item"].(map[string]any)["key"].(string)

	before := application.Gateway.AdapterCalls()
	if code := chatStatus(t, server.URL, apiKey, "hello-no-money", ""); code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d", code)
	}
	if application.Gateway.AdapterCalls() != before {
		t.Fatal("adapter must not be called when balance is insufficient")
	}
	risk, err := application.Billing.Risk(ctx)
	if err != nil || risk == nil || risk.PreauthFailed < 1 {
		t.Fatalf("402 must record a preauth failure: %+v %v", risk, err)
	}

	if postJSONRaw(t, server.URL+"/v1/topups/redeem", session, map[string]any{"code": billing.RedeemE2E})["item"] == nil {
		t.Fatal("redeem failed")
	}
	uid := userIDOf(reg)
	beforeReap := getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"]
	reapID := "reap-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: uid, ChannelOrgID: identity.OfficialChannelID, RequestID: reapID, ReserveMinor: 100000,
		UnitPrices: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	held := getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"]
	if held == beforeReap {
		t.Fatal("reserve should reduce available balance")
	}
	if err := application.DB.Exec(`UPDATE billing_authorizations SET expires_at = NOW() - INTERVAL '1 minute' WHERE request_id = ?`, reapID).Error; err != nil {
		t.Fatal(err)
	}
	n, err := application.Billing.ReapExpired(ctx)
	if err != nil || n < 1 {
		t.Fatalf("reap expired: n=%d err=%v", n, err)
	}
	afterReap := getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"]
	if afterReap != beforeReap {
		t.Fatalf("expired reservation should return balance: before=%v after=%v", beforeReap, afterReap)
	}

	chat := postJSONRaw(t, server.URL+"/v1/chat/completions", apiKey, map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": "hi"}},
	})
	requestID := chat["request_id"].(string)
	usage := getAuthJSON(t, server.URL+"/v1/me/usage", session)
	items, _ := usage["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected usage, got %+v", usage)
	}
	first := items[0].(map[string]any)
	oldAmount := first["customer_amount_minor"]
	usageID := first["id"].(string)

	firstReplay := postJSONRaw(t, server.URL+"/admin/usage/replay", "m3_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	secondReplay := postJSONRaw(t, server.URL+"/admin/usage/replay", "m3_admin", map[string]any{
		"request_id": requestID, "usage": map[string]int{"prompt_tokens": 8, "completion_tokens": 4},
	})
	if firstReplay["item"].(map[string]any)["charge_id"] != secondReplay["item"].(map[string]any)["charge_id"] {
		t.Fatalf("duplicate usage created a second charge")
	}

	_ = postJSONRaw(t, server.URL+"/admin/price-books", "m3_admin", map[string]any{
		"model": catalog.EchoModelID, "input": "0.01", "output": "0.02", "currency": "USD",
	})
	again := getAuthJSON(t, server.URL+"/v1/me/usage", session)["items"].([]any)[0].(map[string]any)
	if again["customer_amount_minor"] != oldAmount {
		t.Fatalf("price change rewrote old bill: %v -> %v", oldAmount, again["customer_amount_minor"])
	}
	if _, err := application.Billing.RecalcCommission(ctx, usageID); err != nil {
		t.Fatal(err)
	}

	beforeBal := getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"]
	_ = postJSONRaw(t, server.URL+"/admin/refunds", "m3_admin", map[string]any{"request_id": requestID})
	afterBal := getAuthJSON(t, server.URL+"/v1/me/balance", session)["balance"].(map[string]any)["available_minor"]
	if afterBal == beforeBal {
		t.Fatal("refund should credit wallet")
	}
	comms, err := application.Billing.ListCommissions(ctx, usageID)
	if err != nil {
		t.Fatal(err)
	}
	sawReverse := false
	for _, item := range comms {
		if item.Status == billing.CommissionReversed {
			sawReverse = true
		}
	}
	if !sawReverse {
		t.Fatalf("expected reversed commission, got %+v", comms)
	}

	reg2 := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": "race-" + t.Name() + "-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1", "promotion_code": "THA1",
	})
	user2 := userIDOf(reg2)
	if _, err := application.Billing.EnsureWallet(ctx, user2); err != nil {
		t.Fatal(err)
	}
	top, err := application.Billing.CreateTopup(ctx, user2, "chn_official_a", 800, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Billing.ConfirmTopup(ctx, top.ID, "m3_admin"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := application.Billing.Reserve(ctx, billing.ReserveInput{
				UserID: user2, RequestID: "race-" + t.Name() + "-" + stamp + "-" + strconv.Itoa(i),
				PublicModelID: catalog.EchoModelID, ReserveMinor: 700,
				UnitPrices: []byte(`{"input":"0.000001","output":"0.000002"}`),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	ok, fail := 0, 0
	for err := range errs {
		if err == nil {
			ok++
		} else if errors.Is(err, billing.ErrInsufficientBalance) {
			fail++
		} else {
			t.Fatalf("unexpected reserve error: %v", err)
		}
	}
	if ok != 1 || fail != 1 {
		t.Fatalf("expected 1 success and 1 insufficient, got ok=%d fail=%d", ok, fail)
	}
	bal, err := application.Billing.Balance(ctx, user2, "")
	if err != nil {
		t.Fatal(err)
	}
	if bal.AvailableMinor+bal.ReservedMinor != 800 {
		t.Fatalf("wallet invariant broken: %+v", bal)
	}
	_ = application
}

func chatStatus(t *testing.T, base, key, content, omit string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"model": catalog.EchoModelID, "messages": []map[string]string{{"role": "user", "content": content}},
	})
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	if omit != "" {
		req.Header.Set("X-Tokenhub-Omit-Usage", omit)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func userIDOf(body map[string]any) string {
	session, _ := body["session"].(map[string]any)
	user, _ := session["user"].(map[string]any)
	id, _ := user["id"].(string)
	return id
}
