package app_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/tokpath/tokstudio/backend/internal/billing"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/platform/config"
)

func TestWalletGiftAndCommissionBuckets(t *testing.T) {
	if os.Getenv("TOKENHUB_DATABASE_URL") == "" || os.Getenv("TOKENHUB_REDIS_URL") == "" {
		t.Skip("integration test requires postgres and redis")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAdmin = "bucket_admin"
	cfg.BootstrapUser = "bucket_user"
	cfg.EncryptionKey = "dev-only-32-byte-key-change-me!!"
	application := mustApp(t, cfg)
	server := httptest.NewServer(application.Router())
	defer server.Close()
	ctx := context.Background()
	prices := []byte(`{"input":"1","output":"1"}`)

	email := "gift-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test"
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{
		"email": email, "password": "password1", "promotion_code": "THA1",
	})
	uid := userIDOf(reg)
	if err := application.Billing.GrantGift(ctx, uid, "gift-"+uid, 3*billing.MinorPerUSD); err != nil {
		t.Fatal(err)
	}
	if err := application.Billing.Credit(ctx, uid, "pay-"+uid, 5*billing.MinorPerUSD, "purchased"); err != nil {
		t.Fatal(err)
	}
	bal, err := application.Billing.Balance(ctx, uid, identity.OfficialChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if bal.GiftMinor != 3*billing.MinorPerUSD || bal.PurchasedMinor != 5*billing.MinorPerUSD {
		t.Fatalf("gift/purchased split: %+v", bal)
	}

	relID := "gift-release-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: uid, ChannelOrgID: identity.OfficialChannelID, RequestID: relID,
		ReserveMinor: 1 * billing.MinorPerUSD, UnitPrices: prices,
	}); err != nil {
		t.Fatal(err)
	}
	heldGift, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if heldGift.GiftMinor != 2*billing.MinorPerUSD {
		t.Fatalf("reserve should consume gift first: %+v", heldGift)
	}
	if err := application.Billing.Release(ctx, relID); err != nil {
		t.Fatal(err)
	}
	restored, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if restored.GiftMinor != 3*billing.MinorPerUSD {
		t.Fatalf("release should restore gift: %+v", restored)
	}

	reqID := "gift-settle-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: uid, ChannelOrgID: identity.OfficialChannelID, RequestID: reqID,
		ReserveMinor: 4 * billing.MinorPerUSD, UnitPrices: prices,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := application.Billing.Settle(ctx, billing.SettleInput{
		RequestID: reqID, UserID: uid, ChannelOrgID: identity.OfficialChannelID,
		Usage: map[string]int{"prompt_tokens": 10, "completion_tokens": 1}, UnitPrices: prices,
	}); err != nil {
		t.Fatal(err)
	}
	afterSettle, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if afterSettle.GiftMinor != 0 {
		t.Fatalf("consumed gift must stay consumed: %+v", afterSettle)
	}
	if _, err := application.Billing.RefundCharge(ctx, reqID); err != nil {
		t.Fatal(err)
	}
	afterRefund, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if afterRefund.GiftMinor != 0 {
		t.Fatalf("gift must not be refunded: %+v", afterRefund)
	}
	if afterRefund.PurchasedMinor != 5*billing.MinorPerUSD {
		t.Fatalf("refund purchased only: %+v", afterRefund)
	}

	if err := application.DB.Transaction(func(tx *gorm.DB) error {
		return application.Billing.CreditCommissionTx(tx, uid, "cme-bucket-"+uid, 20*billing.MinorPerUSD)
	}); err != nil {
		t.Fatal(err)
	}
	cash, err := application.Billing.Balance(ctx, uid, "")
	if err != nil {
		t.Fatal(err)
	}
	if cash.CommissionAvailableMinor != 20*billing.MinorPerUSD {
		t.Fatalf("commission cash: %+v", cash)
	}
	if _, err := application.Billing.Reserve(ctx, billing.ReserveInput{
		UserID: uid, ChannelOrgID: identity.OfficialChannelID,
		RequestID: "comm-cannot-pay-" + uid, ReserveMinor: cash.AvailableMinor + 1, UnitPrices: prices,
	}); !errors.Is(err, billing.ErrInsufficientBalance) {
		t.Fatalf("commission cash must not cover API: %v", err)
	}
}
