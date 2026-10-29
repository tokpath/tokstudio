package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestPostgresCustomerCanCancelUnpaidManualOrderButNotAnOpenOnlineCheckout(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "cancel-order@example.test")
	f.login(t, "customer", "cancel-order@example.test", "customer-password-test")
	var customerID string
	if err := f.a.DB.Raw("SELECT customer_id FROM organization_customer_identities WHERE email_normalized=? AND is_current", "cancel-order@example.test").Scan(&customerID).Error; err != nil || customerID == "" {
		t.Fatal(err, customerID)
	}
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	manual, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "manual", Enabled: true, Credentials: map[string]string{"currency": "USD", "recipient_reference": "TEST-ONLY", "instructions": "synthetic transfer"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	online, err := f.a.Payments.SaveMerchant(t.Context(), owner, payments.Merchant{ID: uuid.NewString(), OperatorID: op, Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_cancel", "publishable_key": "pk_test_cancel", "webhook_secret": "whsec_cancel", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	create := func(key, merchant string) string {
		t.Helper()
		id := uuid.NewString()
		zero := ledger.MustDecimal("0")
		order := commerce.Order{ID: id, OperatorID: op, OriginalOperatorID: op, OriginalOwnershipVersion: 1, BrandID: f.a.Installation.BrandID, CustomerID: customerID, OwnershipVersion: 1, BusinessKey: key, Fingerprint: key, Kind: "package", DueMoney: ledger.MustDecimal("80"), PaidATG: ledger.MustDecimal("80"), PackageGiftATG: ledger.MustDecimal("20"), MerchantGiftATG: zero, KOLGiftATG: zero, InternalMoney: zero, CommissionMoney: zero, ExternalAllocatedMoney: zero, InternalStatus: "none", CoverageStatus: "none", Status: "pending", Period: ledger.Month, Timezone: "UTC", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
		if err := f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		if err := f.a.DB.Create(&checkoutRow{OrderID: id, CustomerID: customerID, MerchantID: &merchant, CreatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	manualOrder := create("manual-cancel", manual.ID)
	status, out, _ := f.call(t, "GET", "/v1/me/orders/"+manualOrder, nil, "")
	mustOK(t, status, out)
	if out["cancel_allowed"] != true {
		t.Fatal("manual instructions were not cancellable", out["cancel_allowed"])
	}
	status, out, _ = f.call(t, "POST", "/v1/me/orders/"+manualOrder+"/cancel", map[string]any{}, "")
	mustOK(t, status, out)
	if out["status"] != "closed" || out["cancel_allowed"] != false {
		t.Fatal("cancel did not close the unpaid order", out["status"], out["cancel_allowed"])
	}
	status, out, _ = f.call(t, "POST", "/v1/me/orders/"+manualOrder+"/cancel", map[string]any{}, "")
	mustOK(t, status, out)
	if out["status"] != "closed" {
		t.Fatal("repeat cancel changed the closed order", out["status"])
	}
	onlineOrder := create("online-cancel", online.ID)
	status, out, _ = f.call(t, "GET", "/v1/me/orders/"+onlineOrder, nil, "")
	mustOK(t, status, out)
	if out["cancel_allowed"] != false {
		t.Fatal("unconfirmed online checkout offered cancellation", out["cancel_allowed"])
	}
	status, out, _ = f.call(t, "POST", "/v1/me/orders/"+onlineOrder+"/cancel", map[string]any{}, "")
	if status != 409 {
		t.Fatal("open online checkout was cancelled", status, out)
	}
}
