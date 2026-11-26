package portal

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"github.com/tokpath/tokstudio/backend/internal/platform/db"
	"gorm.io/gorm"
)

func TestPostgresExpiredUnpaidOrderReleasesAndRecordedPaymentStays(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "expire-order@example.test")
	var customerID string
	if err := f.a.DB.Raw("SELECT customer_id FROM organization_customer_identities WHERE email_normalized=? AND is_current", "expire-order@example.test").Scan(&customerID).Error; err != nil || customerID == "" {
		t.Fatal(err, customerID)
	}
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	manual, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "TEST-ONLY", "instructions": "synthetic transfer"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	zero := ledger.MustDecimal("0")
	create := func(key string, expires time.Time) string {
		t.Helper()
		id := uuid.NewString()
		order := commerce.Order{ID: id, OperatorID: op, OriginalOperatorID: op, OriginalOwnershipVersion: 1, BrandID: f.a.Installation.BrandID, CustomerID: customerID, OwnershipVersion: 1, BusinessKey: key, Fingerprint: key, Kind: "package", DueMoney: ledger.MustDecimal("80"), PaidATG: ledger.MustDecimal("80"), PackageGiftATG: ledger.MustDecimal("20"), MerchantGiftATG: zero, KOLGiftATG: zero, InternalMoney: zero, CommissionMoney: zero, ExternalAllocatedMoney: zero, InternalStatus: "none", CoverageStatus: "none", Status: "pending", Period: ledger.Month, Timezone: "UTC", ExpiresAt: expires, CreatedAt: now.Add(-time.Hour)}
		if err := f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.a.DB.Create(&checkoutRow{OrderID: id, CustomerID: customerID, MerchantID: &manual.ID, CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	expired := create("expired-unpaid", now.Add(-time.Minute))
	open := create("still-open", now.Add(commerce.UnpaidPaymentWindow))
	paid := create("receipt-recorded", now.Add(-time.Minute))
	if err := f.a.DB.Table("payments_verified_facts").Create(map[string]any{"id": uuid.NewString(), "merchant_id": manual.ID, "operator_id": op, "order_id": paid, "kind": "receipt", "fingerprint": "recorded-payment", "fact": `{}`, "valuation": `{}`, "state": "verified", "observed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	online, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "kvoucher", Enabled: true, Credentials: map[string]string{"kvoucher_api_key": "fixture", "kvoucher_api_secret": "fixture"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	neverStarted := create("online-never-started", now.Add(-30*time.Second))
	if err := f.a.DB.Model(&checkoutRow{}).Where("order_id=?", neverStarted).Update("merchant_id", online.ID).Error; err != nil {
		t.Fatal(err)
	}
	protected := []string{}
	failedOnline := create("online-proven-failed", now.Add(-2*time.Hour))
	if err := f.a.DB.Model(&checkoutRow{}).Where("order_id=?", failedOnline).Update("merchant_id", online.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Table("payments_intents").Create(map[string]any{"merchant_id": online.ID, "order_id": failedOnline, "operator_id": op, "fingerprint": failedOnline, "state": "failed", "order_ciphertext": "fixture", "created_at": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"executing", "created", "outcome_unknown", "awaiting_settlement", "verified"} {
		id := create("online-"+state, now.Add(-2*time.Hour))
		if err := f.a.DB.Model(&checkoutRow{}).Where("order_id=?", id).Update("merchant_id", online.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.a.DB.Table("payments_intents").Create(map[string]any{"merchant_id": online.ID, "order_id": id, "operator_id": op, "fingerprint": id, "state": state, "order_ciphertext": "fixture", "created_at": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
		protected = append(protected, id)
	}
	if err := f.a.expireUnpaidOrders(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	statusOf := func(id string) string {
		t.Helper()
		var status string
		if err := f.a.DB.Raw("SELECT status FROM commerce_orders WHERE id=?", id).Scan(&status).Error; err != nil {
			t.Fatal(err)
		}
		return status
	}
	// With a one-row batch the oldest proven failed payment is closed first;
	// unresolved older intents must not occupy that slot.
	if statusOf(failedOnline) != "closed" || statusOf(expired) != "pending" || statusOf(open) != "pending" || statusOf(paid) != "pending" {
		t.Fatalf("failed=%s expired=%s open=%s paid=%s", statusOf(failedOnline), statusOf(expired), statusOf(open), statusOf(paid))
	}
	if err := f.a.expireUnpaidOrders(t.Context(), 50); err != nil {
		t.Fatal(err)
	}
	if statusOf(expired) != "closed" || statusOf(neverStarted) != "closed" {
		t.Fatal("expiry left a demonstrably unpaid order open", statusOf(expired), statusOf(neverStarted))
	}
	for _, id := range protected {
		if statusOf(id) != "pending" {
			t.Fatal("unresolved online payment was closed")
		}
	}
	for _, id := range []string{expired, failedOnline, neverStarted} {
		o, err := f.a.Commerce.GetOrder(t.Context(), nil, customerID, id)
		if err != nil || !o.ClosedForPaymentExpiry {
			t.Fatal("worker closure lost its expiry reason", o, err)
		}
	}

	manualCancel := create("legacy-customer-cancel", now.Add(-time.Minute))
	if _, err := f.a.Commerce.CloseOrder(t.Context(), nil, customerID, manualCancel, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: "客户在未记录到付款前取消订单"}); err != nil {
		t.Fatal(err)
	}
	legacyExpiry := create("legacy-expiry", now.Add(-time.Minute))
	if _, err := f.a.Commerce.CloseOrder(t.Context(), nil, customerID, legacyExpiry, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: legacyPaymentExpiryEvidence}); err != nil {
		t.Fatal(err)
	}
	// Reapply the migration in this test schema with the pre-upgrade layout.
	if err := f.a.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DROP INDEX commerce_order_expiry_events").Error; err != nil {
			return err
		}
		if err := tx.Exec("ALTER TABLE commerce_orders DROP COLUMN closed_for_payment_expiry").Error; err != nil {
			return err
		}
		return tx.Exec("DELETE FROM schema_migrations WHERE module='commerce' AND version='0019_order_payment_expiry'").Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Apply(f.a.DB, AllMigrations()); err != nil {
		t.Fatal(err)
	}
	for id, expected := range map[string]bool{expired: true, failedOnline: true, neverStarted: true, manualCancel: false, legacyExpiry: true} {
		o, err := f.a.Commerce.GetOrder(t.Context(), nil, customerID, id)
		if err != nil || o.ClosedForPaymentExpiry != expected {
			t.Fatal("upgrade did not backfill only expiry closures", o, err)
		}
	}
	rollout := create("old-worker-during-upgrade", now.Add(-time.Minute))
	oldWorker, err := f.a.Commerce.CloseOrder(t.Context(), nil, customerID, rollout, commerce.PaymentClosureProof{VerifiedUnpaid: true, EvidenceReference: paymentExpiryEvidence})
	if err != nil || oldWorker.ClosedForPaymentExpiry {
		t.Fatal("expected a closure with only historical expiry evidence", oldWorker, err)
	}
	f.login(t, "customer", "expire-order@example.test", "customer-password-test")
	for id, expected := range map[string]string{expired: "expired", failedOnline: "expired", neverStarted: "expired", manualCancel: "closed", legacyExpiry: "expired", rollout: "expired", paid: "pending", open: "pending"} {
		status, order, _ := f.call(t, "GET", "/v1/me/orders/"+id, nil, "")
		mustOK(t, status, order)
		if order["status"] != expected || order["has_verified_receipt"] != (id == paid) || (id == paid && order["payment_action"] != nil) {
			t.Fatalf("expected %s for %s, got %+v", expected, id, order)
		}
	}
	status, denied, _ := f.call(t, "POST", "/v1/me/orders/"+rollout+"/payment-method", map[string]any{"external_method_id": manual.ID}, "")
	problem, _ := denied["error"].(map[string]any)
	if status != http.StatusConflict || problem["code"] != "order_expired" {
		t.Fatal("old worker closure bypassed the expiry response", status, denied)
	}
	if err := f.a.DB.Table("payments_verified_facts").Create(map[string]any{"id": uuid.NewString(), "merchant_id": manual.ID, "operator_id": op, "order_id": expired, "kind": "receipt", "fingerprint": "late-after-close", "fact": `{}`, "valuation": `{}`, "state": "verified", "observed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	status, order, _ := f.call(t, "GET", "/v1/me/orders/"+expired, nil, "")
	mustOK(t, status, order)
	if order["status"] != "expired" || order["has_verified_receipt"] != true || order["payment_notice"] == nil || order["payment_action"] != nil {
		t.Fatal("the expired order hid its late receipt", order)
	}
}

func TestPostgresExpiredCustomerKVoucherOrderRequiresNewCheckout(t *testing.T) {
	var paid atomic.Bool
	f, merchant, rule, providerOrders := setupCheckoutWithPaymentState(t, "success", &paid)
	ctx := t.Context()
	key := uuid.NewString()
	input := map[string]any{"amount_money": "2", "external_method_id": merchant, "fee_rule_id": rule}
	status, original, _ := f.call(t, "POST", "/v1/me/topups", input, key)
	mustOK(t, status, original)
	id := original["id"].(string)
	path := "/v1/me/orders/" + id
	status, opened, _ := f.call(t, "POST", path+"/payment/open", map[string]any{"expected_generation": 0}, "")
	mustOK(t, status, opened)
	if opened["status"] != "payment_ready" {
		t.Fatal("fixture never received its initial checkout", opened)
	}
	if err := f.a.DB.Exec("UPDATE commerce_orders SET expires_at=clock_timestamp()-interval '1 minute' WHERE id=?", id).Error; err != nil {
		t.Fatal(err)
	}
	assertExpired := func(order map[string]any) {
		t.Helper()
		if order["id"] != id || order["status"] != "expired" || order["payment_state"] != "created" || order["payment_action"] != nil || order["can_reconcile_payment"] != true || order["has_verified_receipt"] != false {
			t.Fatal(order)
		}
	}
	status, detail, _ := f.call(t, "GET", path, nil, "")
	mustOK(t, status, detail)
	assertExpired(detail)
	voucher, err := f.client.Get(f.server.URL + path + "/voucher/order")
	if err != nil {
		t.Fatal(err)
	}
	csv, err := io.ReadAll(voucher.Body)
	voucher.Body.Close()
	if err != nil || voucher.StatusCode != http.StatusOK || !strings.Contains(string(csv), "已过期") {
		t.Fatal("order voucher status diverged from its detail", voucher.StatusCode, string(csv), err)
	}
	status, workspace, _ := f.call(t, "GET", "/v1/me/topups/workspace", nil, "")
	mustOK(t, status, workspace)
	rows := workspace["items"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["status"] != "expired" {
		t.Fatal(rows)
	}
	assertExpired(rows[0].(map[string]any)["values"].(map[string]any))
	for endpoint, body := range map[string]any{
		"/payment/open":   map[string]any{"expected_generation": 1},
		"/payment-method": map[string]any{"external_method_id": merchant, "fee_rule_id": rule},
	} {
		status, denied, _ := f.call(t, "POST", path+endpoint, body, "")
		problem, _ := denied["error"].(map[string]any)
		if status != http.StatusConflict || problem["code"] != "order_expired" || denied["payment_action"] != nil {
			t.Fatal(endpoint, status, denied)
		}
	}
	var checkout checkoutRow
	if err := f.a.DB.Where("order_id=?", id).First(&checkout).Error; err != nil {
		t.Fatal(err)
	}
	var stale payments.Order
	if err := json.Unmarshal(checkout.PaymentOrder, &stale); err != nil || !stale.ExpiresAt.After(time.Now()) {
		t.Fatal("fixture needs a cached checkout with a later deadline", err)
	}
	if _, err := f.a.Payments.Create(ctx, stale.OperatorID, merchant, stale); !errors.Is(err, commerce.ErrOrderExpired) {
		t.Fatal("cached payment request bypassed the order deadline", err)
	}
	status, checked, _ := f.call(t, "POST", path+"/reconcile", map[string]any{}, "")
	mustOK(t, status, checked)
	if checked["status"] != "expired" || checked["message"] != "原通道尚未确认这笔收款。付款期限已过。若已付款，请稍后刷新原订单。" {
		t.Fatal(checked)
	}
	status, replay, _ := f.call(t, "POST", "/v1/me/topups", input, key)
	mustOK(t, status, replay)
	if replay["id"] != id || replay["status"] != "expired" || providerOrders.Load() != 1 {
		t.Fatal(replay, providerOrders.Load())
	}
	assertExpired(replay["order"].(map[string]any))
	status, fresh, _ := f.call(t, "POST", "/v1/me/topups", input, "")
	mustOK(t, status, fresh)
	if fresh["id"] == id || fresh["status"] != "pending" || fresh["order"].(map[string]any)["payment_action"] == nil || providerOrders.Load() != 2 {
		t.Fatal(fresh, providerOrders.Load())
	}

	paid.Store(true)
	status, checked, _ = f.call(t, "POST", path+"/reconcile", map[string]any{}, "")
	mustOK(t, status, checked)
	if checked["status"] != "reconciliation_queued" {
		t.Fatal("expired order rejected its original receipt", checked)
	}
	var receiptID string
	if err := f.a.DB.Raw("SELECT receipt_id FROM payments_verified_facts WHERE order_id=? AND kind='receipt'", id).Scan(&receiptID).Error; err != nil || receiptID == "" {
		t.Fatal("late receipt was lost", receiptID, err)
	}
	if err := f.a.deliverReceipt(ctx, stale.OperatorID, receiptID, id); !errors.Is(err, commerce.ErrLatePayment) {
		t.Fatal("original order accepted late funds for delivery", err)
	}
	receipt, err := f.a.Commerce.GetReceipt(ctx, nil, stale.OperatorID, receiptID)
	if err != nil || !receipt.AllocatedMoney.IsZero() {
		t.Fatal("late funds were allocated", receipt, err)
	}
	status, detail, _ = f.call(t, "GET", path, nil, "")
	mustOK(t, status, detail)
	if detail["has_verified_receipt"] != true || detail["can_reconcile_payment"] != false || detail["payment_action"] != nil {
		t.Fatal("receipt review exposed a second payment action", detail)
	}
}
