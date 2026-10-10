package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestWalletPurchaseDurableRecovery(t *testing.T) {
	a, server := newOAuthEnv(t)
	ctx := context.Background()
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": "wallet-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1"})
	userID := userIDOf(reg)
	in := payment.CreateOrderInput{UserID: userID, ChannelOrgID: identity.OfficialChannelID, Adapter: "stripe", Purpose: payment.PurposeWallet, PayMajor: 100}
	op := "wallet-" + userID
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order, err := a.Payment.CreateWalletPurchase(ctx, in, op)
			if err == nil {
				ids <- order.ID
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for value := range ids {
		if id != "" && id != value {
			t.Fatal("concurrent wallet operation created multiple orders")
		}
		id = value
	}
	var count int64
	if err := a.DB.Table("billing_topups").Where("user_id = ?", userID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("topup count %d %v", count, err)
	}
	if order, err := a.Payment.GetWalletPurchase(ctx, userID, op); err != nil || order.ID != id {
		t.Fatalf("durable recovery %+v %v", order, err)
	}
	if body := getAuthJSON(t, server.URL+"/v1/me/wallet-purchases/"+op, "oauth_user"); body["item"] != nil {
		t.Fatal("wallet operation leaked")
	}
	changed := in
	changed.PayMajor = 300
	if _, err := a.Payment.CreateWalletPurchase(ctx, changed, op); !errors.Is(err, payment.ErrPurchaseConflict) {
		t.Fatalf("changed amount accepted %v", err)
	}
	changed = in
	changed.ChannelOrgID = identity.ResellerChannelID
	if _, err := a.Payment.CreateWalletPurchase(ctx, changed, op); !errors.Is(err, payment.ErrPurchaseConflict) {
		t.Fatalf("changed scope accepted %v", err)
	}
	if order, err := a.Payment.CreateWalletPurchase(ctx, in, op+"-new"); err != nil || order.ID == id {
		t.Fatalf("new transaction blocked %+v %v", order, err)
	}
	callback := "wallet-test-order-failure"
	a.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "payment_orders" {
			tx.AddError(errors.New("injected wallet order failure"))
		}
	})
	_, failed := a.Payment.CreateWalletPurchase(ctx, in, op+"-rollback")
	a.DB.Callback().Create().Remove(callback)
	if failed == nil {
		t.Fatal("injected wallet failure not returned")
	}
	if err := a.DB.Table("billing_topups").Where("user_id = ?", userID).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("partial topup persisted %d %v", count, err)
	}
	if _, err := a.Payment.CreateWalletPurchase(ctx, in, op+"-rollback"); err != nil {
		t.Fatal(err)
	}
	original := a.Payment
	registry := payment.NewRegistry()
	for _, driver := range original.Registry().List() {
		registry.MustRegister(driver)
	}
	stripe, _ := registry.Get("stripe")
	timeout := &timeoutCheckoutAdapter{Adapter: stripe}
	registry.MustRegister(timeout)
	a.Payment = payment.NewWithRegistry(a.DB, a.Outbox, a.Plans, a.Billing, original.SignKey(), registry)
	t.Cleanup(func() { a.Payment = original })
	httpOp := op + "-timeout"
	timeout.committed = func() bool {
		order, err := a.Payment.GetWalletPurchase(ctx, userID, httpOp)
		return err == nil && order != nil
	}
	post := func() (int, map[string]any) {
		raw, _ := json.Marshal(map[string]any{"adapter": "stripe", "pay_major": 100})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/payments/orders", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+tokenOf(reg))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", httpOp)
		res, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body map[string]any
		json.NewDecoder(res.Body).Decode(&body)
		return res.StatusCode, body
	}
	if code, _ := post(); code < 400 {
		t.Fatal("provider timeout unexpectedly succeeded")
	}
	if code, body := post(); code != 201 {
		t.Fatalf("provider timeout recovery %d %+v", code, body)
	}
	if len(timeout.orderIDs) != 2 || timeout.orderIDs[0] != timeout.orderIDs[1] {
		t.Fatal("provider timeout changed original wallet order")
	}
}
