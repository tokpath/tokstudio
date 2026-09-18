package app_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

type countingRefund struct {
	payment.Adapter
	calls atomic.Int32
	fail  atomic.Bool
}

func (a *countingRefund) Refund(ctx context.Context, in payment.RefundRequest) (*payment.RefundResult, error) {
	a.calls.Add(1)
	if a.fail.Load() {
		return nil, payment.ErrProviderFailed
	}
	return a.Adapter.Refund(ctx, in)
}

func TestPaymentFinanceJourney(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "payjourney_admin"
	cfg.BootstrapUser = "payjourney_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()
	email := "payjourney-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": email, "password": "password1", "promotion_code": "THA1"})
	userID := userIDOf(reg)
	original, _ := application.Payment.Registry().Get(payment.AdapterManual)
	adapter := &countingRefund{Adapter: original}
	application.Payment.Registry().MustRegister(adapter)
	makeOrder := func() *payment.OrderView {
		item, err := application.Payment.CreateOrder(ctx, payment.CreateOrderInput{UserID: userID, ChannelOrgID: identity.OfficialChannelID, Adapter: payment.AdapterManual, Purpose: payment.PurposeWallet, AmountMinor: 2500000, Currency: "USD"})
		if err != nil {
			t.Fatal(err)
		}
		return item
	}
	order := makeOrder()
	if _, err := application.Payment.Refund(ctx, order.ID); !errors.Is(err, payment.ErrOrderNotPending) {
		t.Fatalf("pending refund: %v", err)
	}
	if adapter.calls.Load() != 0 {
		t.Fatal("called provider before validating status")
	}
	confirmed := postJSONRaw(t, server.URL+"/admin/payments/"+order.ID+"/confirm", "payjourney_admin-finance", map[string]any{})["item"].(map[string]any)
	if confirmed["fulfilled_at"] == nil {
		t.Fatal("missing fulfillment")
	}
	// Provider failures must roll back local credit reversal and refunded status together.
	adapter.fail.Store(true)
	if _, err := application.Payment.Refund(ctx, order.ID); err == nil {
		t.Fatal("expected provider failure")
	}
	current, _ := application.Payment.GetOrder(ctx, order.ID, "")
	top, _ := application.Billing.GetTopup(ctx, order.ReferenceID, "")
	if current.Status != "paid" || top.Status != "paid" {
		t.Fatalf("partial refund persisted: %+v %+v", current, top)
	}
	adapter.fail.Store(false)
	before := adapter.calls.Load()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := application.Payment.Refund(ctx, order.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if adapter.calls.Load() != before+1 {
		t.Fatal("concurrent/repeated refund reached provider more than once")
	}
	top, _ = application.Billing.GetTopup(ctx, order.ReferenceID, "")
	if top.Status != "refunded" {
		t.Fatalf("topup: %+v", top)
	}
	if _, err := application.Payment.Refund(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	if adapter.calls.Load() != before+1 {
		t.Fatal("repeat reached provider")
	}
	// Insufficient local balance must neither call the provider nor claim a refund.
	second := makeOrder()
	if _, err := application.Payment.ConfirmManual(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := application.DB.Exec("UPDATE billing_wallets SET available_minor = 0 WHERE user_id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}
	before = adapter.calls.Load()
	if _, err := application.Payment.Refund(ctx, second.ID); err == nil {
		t.Fatal("insufficient refund accepted")
	}
	current, _ = application.Payment.GetOrder(ctx, second.ID, "")
	if current.Status != "paid" || adapter.calls.Load() != before {
		t.Fatal("failed refund changed status or called provider")
	}
	// Put the target outside the old 200-row window, then search by ID and email.
	if err := application.DB.Exec("UPDATE payment_orders SET created_at = '2000-01-01' WHERE id = ?", order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := application.DB.Exec(`INSERT INTO payment_orders(id,user_id,channel_org_id,adapter,purpose,amount_minor,credit_minor,currency,status) SELECT ? || g::text, 'usr_unrelated', ?, 'manual','wallet',1000000,1000000,'USD','pending' FROM generate_series(1,205) AS g`, "search-"+userID, identity.OfficialChannelID).Error; err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{order.ID, email} {
		response := getAuthJSON(t, server.URL+"/admin/payments?q="+url.QueryEscape(query), "payjourney_admin-finance")
		found := false
		for _, raw := range response["items"].([]any) {
			item := raw.(map[string]any)
			if item["id"] == order.ID {
				found = true
				if item["user_email"] != email || item["channel_code"] != "official-a" {
					t.Fatalf("missing human context: %+v", item)
				}
			}
		}
		if !found {
			t.Fatalf("old order missing for %s", query)
		}
	}
}
