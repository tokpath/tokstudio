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
	if err := f.a.expireUnpaidOrders(t.Context(), 50); err != nil {
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
	if statusOf(expired) != "closed" || statusOf(open) != "pending" || statusOf(paid) != "pending" {
		t.Fatalf("expired=%s open=%s paid=%s", statusOf(expired), statusOf(open), statusOf(paid))
	}
	if err := f.a.expireUnpaidOrders(t.Context(), 50); err != nil {
		t.Fatal(err)
	}
	if statusOf(expired) != "closed" {
		t.Fatal("repeat expiry changed the closed order", statusOf(expired))
	}
}
