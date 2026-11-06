package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"gorm.io/gorm"
)

func TestCustomerInvoiceChoicesArePurchasesNotTopUps(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "invoice-purchase@example.test")
	f.login(t, "customer", "invoice-purchase@example.test", "customer-password-test")
	var customerID string
	if err := f.a.DB.Raw("SELECT customer_id FROM organization_customer_identities WHERE email_normalized=? AND is_current", "invoice-purchase@example.test").Scan(&customerID).Error; err != nil || customerID == "" {
		t.Fatal(err, customerID)
	}
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	now := time.Now().UTC()
	zero := ledger.MustDecimal("0")
	saveOrder := func(key, kind, status string) string {
		t.Helper()
		id := uuid.NewString()
		paid := zero
		if kind == "package" {
			paid = ledger.MustDecimal("40")
		}
		order := commerce.Order{ID: id, OperatorID: op, OriginalOperatorID: op, OriginalOwnershipVersion: 1, BrandID: f.a.Installation.BrandID, CustomerID: customerID, OwnershipVersion: 1, BusinessKey: key, Fingerprint: key, Kind: kind, DueMoney: ledger.MustDecimal("40"), PaidATG: paid, PackageGiftATG: zero, MerchantGiftATG: zero, KOLGiftATG: zero, InternalMoney: zero, CommissionMoney: zero, ExternalAllocatedMoney: zero, InternalStatus: "none", CoverageStatus: "none", Status: status, Period: ledger.Lifetime, Timezone: "UTC", ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now}
		if err := f.a.DB.Table("commerce_orders").Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	saveReceipt := func(reference string) string {
		t.Helper()
		id := uuid.NewString()
		receipt := commerce.Receipt{ID: id, OperatorID: op, Provider: "manual", ProviderReceiptID: reference, Fingerprint: reference, GrossMoney: ledger.MustDecimal("40"), FeeStatus: "pending", Currency: "USD", OriginalAmount: "40", PayerReference: "synthetic payer", RecipientReference: "synthetic bank", ValuationEvidence: "synthetic", VerificationEvidence: "synthetic", VerifiedBy: owner, ObservedAt: now, CreatedAt: now}
		if err := f.a.DB.Table("commerce_receipts").Create(&receipt).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	allocate := func(receiptID, orderID, key string) {
		t.Helper()
		allocation := commerce.Allocation{ID: uuid.NewString(), OperatorID: op, BusinessKey: key, ReceiptID: receiptID, OrderID: orderID, AmountMoney: ledger.MustDecimal("40"), TimingProof: "observed_before_expiry", CreatedAt: now}
		if err := f.a.DB.Table("commerce_receipt_allocations").Create(&allocation).Error; err != nil {
			t.Fatal(err)
		}
	}
	complete := func(id string) {
		t.Helper()
		if err := f.a.DB.Table("commerce_orders").Where("id=?", id).Update("status", "completed").Error; err != nil {
			t.Fatal(err)
		}
	}

	topupOrder := saveOrder("invoice-topup", "topup", "pending")
	topupReceipt := saveReceipt("invoice-topup-bank")
	allocate(topupReceipt, topupOrder, "invoice-topup-allocation")
	complete(topupOrder)

	packageOrder := saveOrder("invoice-package", "package", "pending")
	packageReceipt := saveReceipt("invoice-package-bank")
	allocate(packageReceipt, packageOrder, "invoice-package-allocation")
	complete(packageOrder)
	balancePackage := saveOrder("invoice-balance-package", "package", "completed")

	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, policy, _ := f.call(t, "POST", "/v1/operators/"+op+"/invoices", map[string]any{"instructions": "Within three working days", "expected_version": 0}, "")
	mustOK(t, status, policy)
	status, page, _ := f.call(t, "GET", "/v1/operators/"+op+"/invoices", nil, "")
	mustOK(t, status, page)
	for _, raw := range page["operations"].([]any) {
		for _, field := range raw.(map[string]any)["fields"].([]any) {
			name := field.(map[string]any)["name"]
			if name == "stage" || name == "maximum_ratio" {
				t.Fatal("obsolete policy setting", name)
			}
		}
	}
	f.login(t, "customer", "invoice-purchase@example.test", "customer-password-test")
	status, choices, _ := f.call(t, "GET", "/v1/me/references/invoice-orders", nil, "")
	mustOK(t, status, choices)
	values := map[string]bool{}
	for _, raw := range choices["items"].([]any) {
		values[raw.(map[string]any)["value"].(string)] = true
	}
	if !values[packageOrder] || !values[balancePackage] || values[topupOrder] || values[topupReceipt] {
		t.Fatal("incorrect invoice order choices", choices)
	}
	status, page, _ = f.call(t, "GET", "/v1/me/invoices/workspace", nil, "")
	mustOK(t, status, page)
	found := false
	for _, raw := range page["operations"].([]any) {
		for _, rawField := range raw.(map[string]any)["fields"].([]any) {
			field := rawField.(map[string]any)
			name := field["name"]
			if name == "lines.0.amount_money" {
				t.Fatal("customer still enters invoice amount")
			}
			if name == "lines.0.order_id" {
				found = true
				if field["lookup_url"] != "/api/v1/me/references/invoice-orders" {
					t.Fatal(field)
				}
			}
		}
	}
	if !found {
		t.Fatal("order picker missing")
	}
	details := map[string]string{"title": "测试抬头", "tax_id": "TEST-TAX-ID", "email": "invoice-purchase@example.test"}
	request := func(order, key, clientAmount string) (int, map[string]any) {
		status, result, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"order_id": order, "amount_money": clientAmount}}, "details": details}, key)
		return status, result
	}
	key := uuid.NewString()
	status, created := request(packageOrder, key, "999")
	mustOK(t, status, created)
	var amount, storedOrder string
	if err := f.a.DB.Raw("SELECT amount_money::text FROM operations_invoices WHERE id=?", created["id"]).Scan(&amount).Error; err != nil || ledger.MustDecimal(amount).String() != "40" {
		t.Fatal("client controlled amount", amount, err)
	}
	if err := f.a.DB.Raw("SELECT order_id::text FROM operations_invoice_orders WHERE invoice_id=?", created["id"]).Scan(&storedOrder).Error; err != nil || storedOrder != packageOrder {
		t.Fatal(storedOrder, err)
	}
	status, replay := request(packageOrder, key, "1")
	mustOK(t, status, replay)
	if replay["id"] != created["id"] {
		t.Fatal("retry created new invoice", replay)
	}
	status, duplicate := request(packageOrder, uuid.NewString(), "40")
	if status != 409 {
		t.Fatal("same order invoiced twice", status, duplicate)
	}
	status, second := request(balancePackage, uuid.NewString(), "1")
	mustOK(t, status, second)
	status, invalid := request(topupOrder, uuid.NewString(), "40")
	if status == 200 {
		t.Fatal("topup invoiced", invalid)
	}
	status, choices, _ = f.call(t, "GET", "/v1/me/references/invoice-orders", nil, "")
	mustOK(t, status, choices)
	if len(choices["items"].([]any)) != 0 {
		t.Fatal("already requested orders still offered", choices)
	}
	// Backfill a legacy document without rewriting its amount, private details,
	// receipt evidence, or file. Existing new order invoices are unaffected.
	legacy := operations.Invoice{ID: uuid.NewString(), OperatorID: op, CustomerID: customerID, PartyKind: "customer", BusinessKey: "legacy-invoice", Fingerprint: "legacy-fingerprint", AmountMoney: ledger.MustDecimal("40"), State: "issued", DetailsCiphertext: "existing-private-details", CreatedAt: now.Add(time.Minute), InvoiceReference: "LEGACY-1"}
	if err := f.a.DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Table("operations_invoice_sources").Create(map[string]any{"invoice_id": legacy.ID, "receipt_id": packageReceipt, "amount_money": "40"}).Error; err != nil {
		t.Fatal(err)
	}
	oldFile := operations.InvoiceFile{ID: uuid.NewString(), InvoiceID: legacy.ID, OperatorID: op, BusinessKey: "legacy-file", Fingerprint: "legacy-content", ObjectKey: "legacy-object", Filename: "legacy.pdf", ContentType: "application/pdf", SHA256: "legacy-hash", SizeBytes: 123, State: "ready", CreatedBy: owner, CreatedAt: now}
	if err := f.a.DB.Create(&oldFile).Error; err != nil {
		t.Fatal(err)
	}
	sql, err := migrations.ReadFile("migrate/0005_invoice_order_links.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(sql)).Error }); err != nil {
		t.Fatal(err)
	}
	var link operations.InvoiceLine
	if err = f.a.DB.Table("operations_invoice_orders").Where("invoice_id=?", legacy.ID).Take(&link).Error; err != nil || link.OrderID != packageOrder || link.AmountMoney.Cmp(ledger.MustDecimal("40")) != 0 {
		t.Fatal(link, err)
	}
	var retained operations.Invoice
	if err = f.a.DB.First(&retained, "id=?", legacy.ID).Error; err != nil || retained.DetailsCiphertext != legacy.DetailsCiphertext || retained.InvoiceReference != "LEGACY-1" || retained.AmountMoney.Cmp(legacy.AmountMoney) != 0 {
		t.Fatal(retained, err)
	}
	var retainedFile operations.InvoiceFile
	if err = f.a.DB.First(&retainedFile, "id=?", oldFile.ID).Error; err != nil || retainedFile.ObjectKey != oldFile.ObjectKey || retainedFile.State != "ready" {
		t.Fatal(retainedFile, err)
	}

}
func errorMessage(out map[string]any) string {
	body, _ := out["error"].(map[string]any)
	message, _ := body["message"].(string)
	return message
}
