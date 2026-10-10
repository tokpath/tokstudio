package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/audit"
	"github.com/tokpath/tokstudio/backend/internal/identity"
	"github.com/tokpath/tokstudio/backend/internal/payment"
	"github.com/tokpath/tokstudio/backend/internal/plans"
	"gorm.io/gorm"
)

func TestSubscriptionPurchaseDurableIdempotency(t *testing.T) {
	application, server := newOAuthEnv(t)
	ctx := context.Background()
	reg := postBody(t, server.URL+"/v1/auth/register", "", map[string]string{"email": "purchase-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "@example.test", "password": "password1"})
	userID := userIDOf(reg)
	plan, err := application.Plans.CreatePlan(ctx, plans.CreatePlanInput{OwnerType: plans.OwnerPlatform, OwnerID: identity.OfficialChannelID, Name: "Purchase test", PriceMinor: 2000000, Currency: "USD", BillingPeriod: plans.PeriodMonthly, AutoRenew: true, ChannelScope: plans.ChannelScopeAll, Items: []plans.PlanItemInput{{UnitType: plans.UnitUSDCredit, Included: 2000000}}}, audit.RecordInput{ActorUserID: userID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Plans.ChangePlanStatus(ctx, plan.ID, "approve", audit.RecordInput{ActorUserID: userID}); err != nil {
		t.Fatal(err)
	}
	in := payment.SubscriptionPurchaseInput{UserID: userID, ChannelID: identity.OfficialChannelID, BrandOwnerID: identity.OfficialChannelID, PlanID: plan.ID, Adapter: "stripe", OperationID: "op-" + userID}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	ids := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, order, err := application.Payment.CreateSubscriptionPurchase(ctx, in)
			if err == nil {
				if sub.RenewalPolicy != plans.RenewManual {
					err = errors.New("auto renew must be opt-in")
				}
				ids <- order.ID
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(ids)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for value := range ids {
		if id != "" && id != value {
			t.Fatalf("duplicate order %s != %s", id, value)
		}
		id = value
	}
	var n int64
	if err := application.DB.Table("plans_subscriptions").Where("user_id = ? AND plan_id = ?", userID, plan.ID).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("subscription count=%d err=%v", n, err)
	}
	// A dropped response or an absent Redis entry still recovers the persisted order.
	again, err := application.Payment.GetSubscriptionPurchase(ctx, userID, in.OperationID)
	if err != nil || again.ID != id {
		t.Fatalf("recovery: %+v %v", again, err)
	}
	body := getAuthJSON(t, server.URL+"/v1/me/subscription-purchases/"+in.OperationID, tokenOf(reg))
	if body["item"].(map[string]any)["id"] != id {
		t.Fatalf("HTTP recovery: %+v", body)
	}
	if other := getAuthJSON(t, server.URL+"/v1/me/subscription-purchases/"+in.OperationID, "oauth_user"); other["item"] != nil {
		t.Fatal("purchase operation leaked to another account")
	}
	changed := in
	changed.AutoRenew = true
	if _, _, err := application.Payment.CreateSubscriptionPurchase(ctx, changed); !errors.Is(err, payment.ErrPurchaseConflict) {
		t.Fatalf("changed payload accepted: %v", err)
	}
	changed = in
	changed.ChannelID = identity.ResellerChannelID
	if _, _, err := application.Payment.CreateSubscriptionPurchase(ctx, changed); !errors.Is(err, payment.ErrPurchaseConflict) {
		t.Fatalf("changed brand/channel accepted: %v", err)
	}
	// A genuinely new purchase of the same plan gets its own transaction.
	fresh := in
	fresh.OperationID += "-new"
	fresh.AutoRenew = true
	if _, _, err := application.Payment.CreateSubscriptionPurchase(ctx, fresh); !errors.Is(err, payment.ErrNoAutoRenew) {
		t.Fatalf("unverified renewal authorization accepted: %v", err)
	}
	fresh.AutoRenew = false
	sub, newOrder, err := application.Payment.CreateSubscriptionPurchase(ctx, fresh)
	if err != nil || newOrder.ID == id || sub.RenewalPolicy != plans.RenewManual {
		t.Fatalf("new purchase/renewal: %+v %+v %v", sub, newOrder, err)
	}
	// Failure between subscription and order must leave neither the subscription
	// nor the durable operation behind; a retry can create one complete purchase.
	fail := in
	fail.OperationID += "-rollback"
	callback := "purchase-test-order-failure"
	if err := application.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "payment_orders" {
			tx.AddError(errors.New("injected order failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, _, failed := application.Payment.CreateSubscriptionPurchase(ctx, fail)
	application.DB.Callback().Create().Remove(callback)
	if failed == nil {
		t.Fatal("order creation failure must roll back")
	}
	if err := application.DB.Table("plans_subscriptions").Where("user_id = ? AND plan_id = ?", userID, plan.ID).Count(&n).Error; err != nil || n != 2 {
		t.Fatalf("partial subscription committed: %d %v", n, err)
	}
	if _, _, err := application.Payment.CreateSubscriptionPurchase(ctx, fail); err != nil {
		t.Fatalf("rollback retry: %v", err)
	}
	if status, _ := doJSON(t, http.MethodGet, server.URL+"/v1/me/subscription-purchases/"+in.OperationID, "", false, nil); status != 401 && status != 403 {
		t.Fatalf("anonymous purchase %d", status)
	}
	// A provider timeout happens after the transaction has committed. The HTTP
	// retry must use the original order and the already committed subscription.
	originalPayment := application.Payment
	registry := payment.NewRegistry()
	for _, adapter := range originalPayment.Registry().List() {
		registry.MustRegister(adapter)
	}
	stripe, _ := registry.Get(payment.AdapterStripe)
	driver := &timeoutCheckoutAdapter{Adapter: stripe}
	registry.MustRegister(driver)
	application.Payment = payment.NewWithRegistry(application.DB, application.Outbox, application.Plans, application.Billing, originalPayment.SignKey(), registry)
	t.Cleanup(func() { application.Payment = originalPayment })
	httpOperation := in.OperationID + "-timeout"
	driver.committed = func() bool {
		order, err := application.Payment.GetSubscriptionPurchase(ctx, userID, httpOperation)
		return err == nil && order != nil
	}
	post := func() (int, map[string]any) {
		raw, _ := json.Marshal(map[string]any{"plan_id": plan.ID, "adapter": "stripe"})
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/me/subscriptions", bytes.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+tokenOf(reg))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", httpOperation)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var body map[string]any
		json.NewDecoder(response.Body).Decode(&body)
		return response.StatusCode, body
	}
	if status, _ := post(); status < 400 {
		t.Fatalf("provider timeout must surface unknown result: %d", status)
	}
	status, result := post()
	if status != 201 {
		t.Fatalf("provider recovery %d %+v", status, result)
	}
	if len(driver.orderIDs) != 2 || driver.orderIDs[0] != driver.orderIDs[1] {
		t.Fatal("timeout retry created a different provider order")
	}
}

type timeoutCheckoutAdapter struct {
	payment.Adapter
	orderIDs  []string
	committed func() bool
}

func (d *timeoutCheckoutAdapter) CreateCheckout(ctx context.Context, in payment.CheckoutRequest) (*payment.CheckoutSession, error) {
	if !d.committed() {
		return nil, errors.New("provider invoked before durable purchase committed")
	}
	d.orderIDs = append(d.orderIDs, in.Order.ID)
	if len(d.orderIDs) == 1 {
		return nil, context.DeadlineExceeded
	}
	return &payment.CheckoutSession{Mode: payment.CheckoutSandbox, Sandbox: true}, nil
}
