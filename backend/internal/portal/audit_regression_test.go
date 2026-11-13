package portal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
)

func TestPostgresClosedProviderReleasesExpiredRetailOrder(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	const email = "closed-pay@audit.test"
	f.registered(t, email)
	f.login(t, "customer", email, "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	top, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{CustomerID: customer, OperatorID: op, ExpectedOwnershipVersion: 1, BusinessKey: uuid.NewString(), AmountMoney: ledger.FromInt64(20)})
	if err != nil {
		t.Fatal(err)
	}
	paid := time.Now().UTC()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: op, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(20), Currency: "USD", FeeStatus: "pending", OriginalAmount: "20", RecipientReference: "test bank", VerificationEvidence: "synthetic", ValuationEvidence: "USD", PaidAt: &paid, ObservedAt: paid})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Commerce.AllocateReceipt(ctx, nil, commerce.AllocateReceiptInput{OperatorID: op, ActorStaffID: owner, BusinessKey: uuid.NewString(), ReceiptID: receipt.ID, OrderID: top.ID, AmountMoney: ledger.FromInt64(20)}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Commerce.Fulfill(ctx, nil, top.ID, owner); err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{OperatorID: op, ActorStaffID: owner, Name: "Closed checkout plan", PriceMoney: ledger.FromInt64(100), PaidATG: ledger.FromInt64(100), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	var observed atomic.Int32
	var closed atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/v1/payment_intents" {
			fmt.Fprint(w, `{"id":"pi_audit","status":"requires_payment_method","client_secret":"synthetic-only"}`)
			return
		}
		if r.Method == "GET" && r.URL.Path == "/v1/payment_intents/pi_audit" {
			observed.Add(1)
			// Stripe's lookup must match the same business order. The create response is saved by the application.
			var reference struct{ OrderID string }
			if f.a.DB.Table("payments_intents").Select("order_id").Where("provider_id=?", "pi_audit").Take(&reference).Error != nil {
				http.Error(w, "missing fixture", 500)
				return
			}
			state := "requires_payment_method"
			if closed.Load() {
				state = "canceled"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "pi_audit", "status": state, "amount_received": 0, "currency": "usd", "metadata": map[string]string{"order_id": reference.OrderID, "operator_id": op}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	f.a.Payments.Drivers["stripe"] = payments.Stripe{HTTP: &payments.Transport{Client: upstream.Client(), AllowTestLoopback: true}}
	merchant, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "stripe", Enabled: true, APIBaseURL: upstream.URL, Credentials: map[string]string{"currency": "USD", "secret_key": "sk_test_audit", "publishable_key": "pk_test_audit", "webhook_secret": "whsec_audit"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	status, preview, _ := f.call(t, "POST", "/v1/me/orders/quotes", map[string]string{"plan_id": quote.PackageID}, "")
	mustOK(t, status, preview)
	status, order, _ := f.call(t, "POST", "/v1/me/orders", map[string]any{"quote_id": preview["id"], "terms_version": preview["terms_version"], "funding": map[string]string{"money_balance": "20", "commission_money": "0", "external_method_id": merchant.ID}}, "")
	mustOK(t, status, order)
	id := order["id"].(string)
	if err = f.a.DB.Exec("UPDATE commerce_orders SET expires_at=clock_timestamp()-interval '1 minute' WHERE id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	checkExpiry := func() {
		t.Helper()
		if err := f.a.DB.Exec("UPDATE payments_intents SET updated_at=clock_timestamp()-interval '2 minutes' WHERE order_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.a.reconcileExpiredPayments(ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.a.expireUnpaidOrders(ctx, 50); err != nil {
			t.Fatal(err)
		}
	}
	checkExpiry()
	status, pending, _ := f.call(t, "GET", "/v1/me/orders/"+id, nil, "")
	mustOK(t, status, pending)
	if pending["status"] != "pending" || observed.Load() != 1 {
		t.Fatal(pending, observed.Load())
	}
	closed.Store(true)
	checkExpiry()
	status, view, _ := f.call(t, "GET", "/v1/me/orders/"+id, nil, "")
	mustOK(t, status, view)
	status, balance, _ := f.call(t, "GET", "/v1/me/credits", nil, "")
	mustOK(t, status, balance)
	wallet := balance["money_balance"].(map[string]any)
	if observed.Load() != 2 || view["status"] != "closed" || view["payment_state"] != "closed" || view["payment_action"] != nil || wallet["held"] != "0" || wallet["available"] != "20" {
		t.Fatal(view, wallet, observed.Load())
	}
}

func TestPostgresVerifiedProcurementSurvivesStaffSuspension(t *testing.T) {
	s := stockSetup(t)
	f := s.f
	ctx := t.Context()
	owner := f.a.Installation.OwnerStaffID
	status, res := s.buy(t, 80)
	mustOK(t, status, res)
	id := res["id"].(string)
	now := time.Now().UTC()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(80), Currency: "USD", FeeStatus: "pending", OriginalAmount: "80", RecipientReference: "test bank", VerificationEvidence: "synthetic", ValuationEvidence: "USD", PaidAt: &now, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Exec("UPDATE organization_staff_memberships SET status='suspended' WHERE operator_id=? AND staff_id=?", s.channel, owner).Error; err != nil {
		t.Fatal(err)
	}
	err = f.a.deliverProcurementReceipt(ctx, s.oem, id, receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	var order cooperation.PurchaseOrder
	if err = f.a.DB.First(&order, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != "completed" || order.AllocatedMoney.String() != "80" {
		t.Fatal(order.Status, order.AllocatedMoney)
	}
}

func TestPostgresVerifiedProcurementKeepsFundsWhenDeliveryFails(t *testing.T) {
	s := stockSetup(t)
	f := s.f
	ctx := t.Context()
	owner := f.a.Installation.OwnerStaffID
	status, res := s.buy(t, 80)
	mustOK(t, status, res)
	id := res["id"].(string)
	now := time.Now().UTC()
	receipt, err := f.a.Commerce.RecordManualReceipt(ctx, nil, owner, commerce.VerifiedReceipt{OperatorID: s.oem, ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(80), Currency: "USD", FeeStatus: "pending", OriginalAmount: "80", RecipientReference: "test bank", VerificationEvidence: "synthetic", ValuationEvidence: "USD", PaidAt: &now, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.Organization.SetSupplyStatus(ctx, nil, owner, s.channel, "paused", "simulated delivery cannot proceed"); err != nil {
		t.Fatal(err)
	}
	err = f.a.deliverProcurementReceipt(ctx, s.oem, id, receipt.ID)
	if err == nil {
		t.Fatal("expected delivery pause")
	}
	var order cooperation.PurchaseOrder
	if err = f.a.DB.First(&order, "id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	status, view, _ := f.call(t, "GET", "/v1/operators/"+s.oem+"/order-context/"+id, nil, "")
	mustOK(t, status, view)
	if order.Status != "paid" || order.AllocatedMoney.String() != "80" || view["subject"].(map[string]any)["status"] != "paid" {
		t.Fatal(order.Status, order.AllocatedMoney, view["subject"])
	}
	if err = f.a.Organization.SetSupplyStatus(ctx, nil, owner, s.channel, "active", "restore fixture supply"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = f.a.deliverProcurementReceipt(ctx, s.oem, id, receipt.ID); err != nil {
			t.Fatal(err)
		}
	}
	var allocations, lots int64
	if err = f.a.DB.Model(&cooperation.PurchaseAllocation{}).Where("order_id=?", id).Count(&allocations).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Model(&cooperation.ProcurementLot{}).Where("order_id=?", id).Count(&lots).Error; err != nil || allocations != 1 || lots != 1 {
		t.Fatal(allocations, lots, err)
	}
}

func TestPostgresManualFulfillmentKeepsCheckoutRenewalConsent(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	const email = "manual-renewal@audit.test"
	f.registered(t, email)
	f.login(t, "customer", email, "customer-password-test")
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true,renewal=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	merchant, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "synthetic"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{OperatorID: op, ActorStaffID: owner, Name: "Manual renew plan", PriceMoney: ledger.FromInt64(100), PaidATG: ledger.FromInt64(100), Period: ledger.Month, NewPurchaseEnabled: true, RenewalEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	status, preview, _ := f.call(t, "POST", "/v1/me/orders/quotes", map[string]string{"plan_id": quote.PackageID}, "")
	mustOK(t, status, preview)
	status, order, _ := f.call(t, "POST", "/v1/me/orders", map[string]any{"quote_id": preview["id"], "terms_version": preview["terms_version"], "auto_renew": true, "funding": map[string]string{"money_balance": "0", "commission_money": "0", "external_method_id": merchant.ID}}, "")
	mustOK(t, status, order)
	id := order["id"].(string)
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, res, _ := f.call(t, "POST", "/v1/operators/"+op+"/payments", receiptForSalesOrder(id, "100"), "")
	mustOK(t, status, res)
	status, res, _ = f.call(t, "POST", "/v1/operators/"+op+"/orders/"+id+"/fulfill", map[string]any{}, "")
	mustOK(t, status, res)
	f.login(t, "customer", email, "customer-password-test")
	status, orders, _ := f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, status, orders)
	item := orders["items"].([]any)[0].(map[string]any)
	if item["auto_renew"] != true {
		t.Fatal(item)
	}
}

func TestPostgresSuccessfulStreamReturnsBilledRequestNumber(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"synthetic-stream\",\"object\":\"chat.completion.chunk\",\"model\":\"test-chat\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"test\"}}]}\n\n")
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "data: {\"id\":\"synthetic-stream\",\"object\":\"chat.completion.chunk\",\"model\":\"test-chat\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(upstream.Close)
	f := setupPortal(t)
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	const email = "stream-header@audit.test"
	f.registered(t, email)
	model := f.configureExternalCatalog(t, upstream.URL)
	var existing struct{ Definition json.RawMessage }
	if err := f.a.DB.Table("tokenhub_upstreams").Where("public_model_id=?", model).Take(&existing).Error; err != nil {
		t.Fatal(err)
	}
	var route catalog.UpstreamModel
	if err := json.Unmarshal(existing.Definition, &route); err != nil {
		t.Fatal(err)
	}
	route.Capabilities = append(route.Capabilities, modelruntime.Capability{Operation: modelruntime.Chat, Streaming: true})
	if err := f.a.Catalog.SaveUpstream(ctx, op, route, route.Version); err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET model_usage=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	f.login(t, "customer", email, "customer-password-test")
	status, view, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, view)
	customer := view["customer"].(map[string]any)["id"].(string)
	now := time.Now().UTC()
	if _, err := f.a.Ledger.IssueGrant(ctx, nil, ledger.GrantInput{OperatorID: op, CustomerID: customer, OrderID: uuid.NewString(), BusinessKey: uuid.NewString(), ActorID: owner, Reason: "synthetic audit", Sources: []ledger.GrantSource{{Kind: "manual_gift", Amount: ledger.FromInt64(1000)}}, EffectiveAt: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	status, key, _ := f.call(t, "POST", "/v1/me/keys", map[string]any{"name": "Streaming audit", "budget": map[string]string{"period": "lifetime"}}, "")
	mustOK(t, status, key)
	body, _ := json.Marshal(map[string]any{"model": model, "stream": true, "messages": []map[string]string{{"role": "user", "content": "Isolated test"}}, "max_completion_tokens": 20})
	req, err := http.NewRequestWithContext(ctx, "POST", f.server.URL+"/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key["secret"].(string))
	req.Header.Set("Idempotency-Key", uuid.NewString())
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var stream strings.Builder
	if _, err = io.Copy(&stream, response.Body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") || !strings.Contains(stream.String(), "[DONE]") || strings.Contains(stream.String(), "request_interrupted") {
		t.Fatal("stream did not complete", response.StatusCode, response.Header.Get("Content-Type"), stream.String())
	}
	status, res, _ := f.call(t, "GET", "/v1/me/requests", nil, "")
	mustOK(t, status, res)
	items := res["items"].([]any)
	if len(items) != 1 || response.Header.Get("X-TokenHub-Request-ID") != items[0].(map[string]any)["id"] {
		t.Fatal("stream header did not identify the billed request")
	}
}

func TestPostgresCustomerCanFindAndCancelOlderRenewalWhilePaused(t *testing.T) {
	f := setupPortal(t)
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.openRegistration(t)
	const email = "historical-plans@audit.test"
	f.registered(t, email)
	f.login(t, "customer", email, "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	original, err := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: "Monthly audit plan", PriceMoney: ledger.FromInt64(10), PaidATG: ledger.FromInt64(100), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true, RenewalEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	later, err := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: "Lifetime audit plan", PriceMoney: ledger.FromInt64(10), PaidATG: ledger.FromInt64(100), Period: ledger.Lifetime, Timezone: "UTC", NewPurchaseEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Hour)
	expiry := now.AddDate(0, 1, 0)
	order := commerce.Order{ID: uuid.NewString(), OperatorID: op, OriginalOperatorID: op, OriginalOwnershipVersion: 1, BrandID: f.a.Installation.BrandID, CustomerID: customer, BusinessKey: uuid.NewString(), Fingerprint: "audit37 projection", OwnershipVersion: 1, Kind: "package", QuoteID: &original.ID, DueMoney: ledger.FromInt64(10), PaidATG: ledger.FromInt64(100), InternalStatus: "none", CoverageStatus: "none", Status: "completed", Period: ledger.Month, Timezone: "UTC", ExpiresAt: expiry, CreatedAt: now, CompletedAt: &now}
	d := commerce.Delivery{ID: uuid.NewString(), OrderID: order.ID, EffectiveAt: now, ExpiresAt: &expiry, DeliveredAt: now}
	if err = f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Table("commerce_deliveries").Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	subscription, err := f.a.Commerce.EnableAutoRenewal(ctx, nil, customer, order.ID, "synthetic renewal authorization", false)
	if err != nil {
		t.Fatal(err)
	}
	oldStatus, oldView, _ := f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, oldStatus, oldView)
	if oldView["items"].([]any)[0].(map[string]any)["id"] != subscription.ID {
		t.Fatal("renewal not visible before newer orders")
	}
	if err = f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true,renewal=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	business, err := f.a.Organization.GetBusinessStatus(ctx, nil, op)
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, pausedResponse, _ := f.call(t, "POST", "/v1/operators/"+op+"/business-status", map[string]any{"feature": "renewal", "enabled": false, "expected_version": business.Version}, "")
	mustOK(t, status, pausedResponse)
	var notices int64
	if err = f.a.RunMaintenanceJobs(ctx, "renewal-pause-test", 20); err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Table("operations_notifications").Where("recipient_id=? AND title=?", customer, "自动续费已暂停").Count(&notices).Error; err != nil || notices != 1 {
		t.Fatal("missing pause notification", notices, err)
	}
	f.login(t, "customer", email, "customer-password-test")
	status, paused, _ := f.call(t, "GET", "/v1/me/plans", nil, "")
	mustOK(t, status, paused)
	pausedRow := paused["items"].([]any)[0].(map[string]any)
	if pausedRow["auto_renew"] != true || pausedRow["auto_renew_available"] != false {
		t.Fatal("paused renewal should still be revocable", pausedRow)
	}
	if err = f.a.DB.Table("commerce_subscriptions").Where("id=?", subscription.ID).Update("next_due_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.runRenewal(ctx, workerPayload{ID: subscription.ID, CustomerID: customer, OperatorID: op}); err != nil {
		t.Fatal("expected merchant gate to block charge", err)
	}
	if err = f.a.DB.Table("operations_notifications").Where("recipient_id=? AND title=?", customer, "套餐续费条款与资金提醒").Count(&notices).Error; err != nil || notices != 0 {
		t.Fatal("paused renewal sent a reminder", notices, err)
	}
	orders := make([]commerce.Order, 100)
	deliveries := make([]commerce.Delivery, 100)
	for i := range orders {
		recent := now.Add(time.Duration(i+1) * time.Second)
		newer := order
		newer.ID, newer.BusinessKey, newer.QuoteID = uuid.NewString(), uuid.NewString(), &later.ID
		newer.Period, newer.CreatedAt, newer.CompletedAt = ledger.Lifetime, recent, &recent
		orders[i] = newer
		deliveries[i] = commerce.Delivery{ID: uuid.NewString(), OrderID: newer.ID, EffectiveAt: recent, DeliveredAt: recent}
	}
	if err = f.a.DB.Table("commerce_orders").CreateInBatches(&orders, 100).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Table("commerce_deliveries").CreateInBatches(&deliveries, 100).Error; err != nil {
		t.Fatal(err)
	}
	cursor := ""
	for page := 0; page < 6; page++ {
		path := "/v1/me/plans"
		if cursor != "" {
			path += "?cursor=" + cursor
		}
		status, result, _ := f.call(t, "GET", path, nil, "")
		mustOK(t, status, result)
		for _, item := range result["items"].([]any) {
			if item.(map[string]any)["id"] == subscription.ID {
				status, result, _ = f.call(t, "PATCH", "/v1/me/plans/"+subscription.ID+"/renewal", map[string]any{"auto_renew": false, "version": item.(map[string]any)["version"]}, "")
				mustOK(t, status, result)
				after, err := f.a.Commerce.GetSubscription(ctx, nil, customer, subscription.ID)
				if err != nil || after.Status != "cancelled" {
					t.Fatal(after.Status, err)
				}
				return
			}
		}
		cursor, _ = result["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	t.Fatal("earlier subscription could not be reached")

}

func TestPostgresOriginalPaymentPreventsProcurementCancellation(t *testing.T) {
	var paid atomic.Bool
	f, merchant, rule, _ := setupCheckoutWithPaymentState(t, "success", &paid)
	ctx := t.Context()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	child, err := f.a.Organization.CreateOperator(ctx, nil, organization.CreateOperatorInput{ActorStaffID: owner, ParentOperatorID: op, Kind: organization.Channel, Name: "Payment buyer"})
	if err != nil {
		t.Fatal(err)
	}
	invite, err := f.a.Organization.InviteStaff(ctx, nil, owner, child.Operator.ID, "owner@example.test", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Organization.AcceptStaffInvitation(ctx, nil, invite.Token, organization.VerifiedStaff{ID: owner, Email: "owner@example.test", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}
	quote, err := f.a.Cooperation.PublishPurchaseQuote(ctx, nil, owner, cooperation.PurchaseQuote{SupplierID: op, BuyerID: child.Operator.ID, GiftTiers: []cooperation.PurchaseGiftTier{{}}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	order, err := f.a.Cooperation.CreatePurchaseOrder(ctx, nil, owner, child.Operator.ID, quote.ID, uuid.NewString(), ledger.FromInt64(2))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "instructions": "offline fixture"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	nextRule, err := paymentmethods.Current(ctx, f.a.DB, op, offline.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + child.Operator.ID + "/procurement/" + order.ID
	status, result, _ := f.call(t, "POST", base+"/checkout", map[string]any{"merchant_id": merchant, "fee_rule_id": rule}, "")
	mustOK(t, status, result)
	status, result, _ = f.call(t, "POST", base+"/checkout", map[string]any{"merchant_id": offline.ID, "fee_rule_id": nextRule.ID}, "")
	mustOK(t, status, result)
	paid.Store(true)
	if _, err = f.a.Payments.Reconcile(ctx, op, merchant, order.ID); err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		ReceiptID string
		State     string
	}
	if err = f.a.DB.Table("payments_verified_facts").Select("receipt_id,state").Where("operator_id=? AND merchant_id=? AND order_id=? AND kind='receipt'", op, merchant, order.ID).Take(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.ReceiptID == "" || receipt.State != "recorded" {
		t.Fatal("expected an independently verified receipt", receipt.State)
	}
	status, result, _ = f.call(t, "POST", base+"/cancel", map[string]any{}, "")
	if status != http.StatusConflict {
		t.Fatal(status, result)
	}
	if err = f.a.RunDeliveryJobs(ctx, "audit37", 20); err != nil {
		t.Fatal(err)
	}
	after, err := f.a.Cooperation.GetPurchase(ctx, nil, owner, child.Operator.ID, order.ID)
	if err != nil || after.Status != "completed" || after.AllocatedMoney.String() != "2" {
		t.Fatal(after.Status, after.AllocatedMoney, err)
	}

}

func TestPostgresPricingSearchReadsOnlyMatchingModelPrices(t *testing.T) {
	f := setupPortal(t)
	op := f.a.Installation.OperatorID
	const models = 12
	var wanted string
	for i := 0; i < models; i++ {
		model := f.configureExternalCatalog(t, f.server.URL)
		if i == 0 {
			wanted = model
		}
	}
	var reads, otherModels int
	if err := f.a.DB.Callback().Row().Before("gorm:row").Register("test:pricing-query", func(tx *gorm.DB) {
		sql := tx.Statement.SQL.String()
		if !strings.Contains(sql, "SELECT regular.id,regular.definition") {
			return
		}
		reads++
		for _, variable := range tx.Statement.Vars {
			model, ok := variable.(string)
			if ok && strings.HasPrefix(model, "contract-") && model != wanted {
				otherModels++
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, result, _ := f.call(t, "GET", "/v1/operators/"+op+"/pricing?q="+wanted, nil, "")
	mustOK(t, status, result)
	if len(result["items"].([]any)) != 1 || reads != 1 || otherModels != 0 {
		t.Fatal(len(result["items"].([]any)), reads, otherModels)
	}
}
