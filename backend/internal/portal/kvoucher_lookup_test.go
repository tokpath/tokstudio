package portal

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestKVoucherWaitingErrorsDoNotStopTheBatch(t *testing.T) {
	if !kvoucherStillWaiting(payments.ErrNotFinal) || !kvoucherStillWaiting(payments.ErrProvider) || !kvoucherStillWaiting(payments.ErrAwaitingSettlement) || kvoucherStillWaiting(errors.New("database down")) {
		t.Fatal("only an unfinished channel read should be skipped")
	}
}

func TestPostgresOpenKVoucherChecksSkipFreshClosedAndManualOrders(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "kvoucher-lookup@example.test")
	var customerID string
	if err := f.a.DB.Raw("SELECT customer_id FROM organization_customer_identities WHERE email_normalized=? AND is_current", "kvoucher-lookup@example.test").Scan(&customerID).Error; err != nil || customerID == "" {
		t.Fatal(err, customerID)
	}
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	kvoucher, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "kvoucher", Enabled: true, Credentials: map[string]string{"kvoucher_api_key": "key", "kvoucher_api_secret": "secret", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	manual, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "TEST-ONLY", "instructions": "synthetic transfer"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	zero := ledger.MustDecimal("0")
	create := func(key string, status string, expires time.Time) string {
		t.Helper()
		id := uuid.NewString()
		order := commerce.Order{ID: id, OperatorID: op, OriginalOperatorID: op, OriginalOwnershipVersion: 1, BrandID: f.a.Installation.BrandID, CustomerID: customerID, OwnershipVersion: 1, BusinessKey: key, Fingerprint: key, Kind: "package", DueMoney: ledger.MustDecimal("80"), PaidATG: ledger.MustDecimal("80"), PackageGiftATG: ledger.MustDecimal("20"), MerchantGiftATG: zero, KOLGiftATG: zero, InternalMoney: zero, CommissionMoney: zero, ExternalAllocatedMoney: zero, InternalStatus: "none", CoverageStatus: "none", Status: status, Period: ledger.Month, Timezone: "UTC", ExpiresAt: expires, CreatedAt: now.Add(-time.Hour)}
		if err := f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	intent := func(merchant, orderID string, updated time.Time) {
		t.Helper()
		if err := f.a.DB.Table("payments_intents").Create(map[string]any{"merchant_id": merchant, "order_id": orderID, "operator_id": op, "fingerprint": orderID, "state": "created", "order_ciphertext": "sealed", "provider_id": "42", "created_at": updated, "updated_at": updated}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ready := create("kvoucher-ready", "pending", now.Add(commerce.UnpaidPaymentWindow))
	fresh := create("kvoucher-fresh", "pending", now.Add(commerce.UnpaidPaymentWindow))
	closed := create("kvoucher-closed", "closed", now.Add(commerce.UnpaidPaymentWindow))
	expired := create("kvoucher-expired", "pending", now.Add(-time.Minute))
	bank := create("manual-bank", "pending", now.Add(commerce.UnpaidPaymentWindow))
	intent(kvoucher.ID, ready, now.Add(-time.Minute))
	intent(kvoucher.ID, fresh, now)
	intent(kvoucher.ID, closed, now.Add(-time.Minute))
	intent(kvoucher.ID, expired, now.Add(-time.Minute))
	intent(manual.ID, bank, now.Add(-time.Minute))
	rows, err := f.a.openKVoucherChecks(t.Context(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("%+v", rows)
	}
	seen := map[string]bool{}
	for i := 0; i < 11; i++ {
		id := create("old-unresolved-"+uuid.NewString(), "pending", now.Add(-time.Minute))
		intent(kvoucher.ID, id, now.Add(-2*time.Hour))
	}
	for i := 0; i < 4; i++ {
		batch, err := f.a.openKVoucherChecks(t.Context(), 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range batch {
			if seen[row.OrderID] {
				t.Fatal("a waiting payment starved newer payments")
			}
			seen[row.OrderID] = true
		}
	}
	if len(seen) != 11 {
		t.Fatal("old or expired unresolved payments were omitted", len(seen))
	}

}
