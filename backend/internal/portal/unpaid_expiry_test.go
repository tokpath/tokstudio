package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
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
	if statusOf(expired) != "closed" {
		t.Fatal("repeat expiry changed the closed order", statusOf(expired))
	}
	for _, id := range protected {
		if statusOf(id) != "pending" {
			t.Fatal("unresolved online payment was closed")
		}
	}

}
