package portal

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
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

	status, choices, _ := f.call(t, "GET", "/v1/me/references/receipts?purpose=invoice", nil, "")
	mustOK(t, status, choices)
	values := map[string]string{}
	for _, item := range choices["items"].([]any) {
		row := item.(map[string]any)
		values[row["value"].(string)] = row["label"].(string)
	}
	if _, ok := values[packageOrder]; !ok {
		t.Fatal("completed package was not selectable", choices["items"])
	}
	if _, ok := values[balancePackage]; !ok {
		t.Fatal("package paid from balance was not selectable", choices["items"])
	}
	if _, ok := values[topupOrder]; ok {
		t.Fatal("top-up order was offered for invoicing", choices["items"])
	}
	if _, ok := values[topupReceipt]; ok {
		t.Fatal("top-up receipt was offered for invoicing", choices["items"])
	}
	if label := values[packageOrder]; label == "" || len(label) < len("套餐购买") || label[:len("套餐购买")] != "套餐购买" {
		t.Fatal("package choice was not labeled as a purchase", values[packageOrder])
	}

	status, page, _ := f.call(t, "GET", "/v1/me/invoices/workspace", nil, "")
	mustOK(t, status, page)
	operations := page["operations"].([]any)
	fields := operations[0].(map[string]any)["fields"].([]any)
	var purchase map[string]any
	for _, field := range fields {
		row := field.(map[string]any)
		if row["name"] == "lines.0.receipt_id" {
			purchase = row
		}
	}
	if purchase == nil || purchase["label"] != "购买订单" || purchase["lookup_url"] != "/api/v1/me/references/receipts?purpose=invoice" {
		t.Fatal("invoice form still asks for an original receipt", purchase)
	}

	details := map[string]string{"title": "测试抬头", "tax_id": "TEST-NOT-A-TAX-ID", "email": "invoice-purchase@example.test"}
	status, denied, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"receipt_id": packageOrder, "amount_money": "40"}}, "details": details}, "")
	if status != 409 {
		t.Fatal("invoice request was accepted before the operator opened it", status, denied)
	}

	f.login(t, "staff", "owner@example.test", "owner-password-test")
	status, saved, _ := f.call(t, "POST", "/v1/operators/"+op+"/invoices", map[string]any{"party_kind": "customer", "enabled": true, "stage": "receipt", "maximum_ratio": "1", "rules_reference": "invoice contract", "expected_version": 0}, "")
	mustOK(t, status, saved)
	f.login(t, "customer", "invoice-purchase@example.test", "customer-password-test")

	status, created, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"receipt_id": packageOrder, "amount_money": "40"}}, "details": details}, "")
	mustOK(t, status, created)
	var storedReceipt string
	if err := f.a.DB.Raw("SELECT receipt_id::text FROM operations_invoice_sources WHERE invoice_id=?", created["id"]).Scan(&storedReceipt).Error; err != nil || storedReceipt != packageReceipt {
		t.Fatal("purchase was not invoiced against its funding receipt", storedReceipt, err)
	}

	status, tooMuch, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"receipt_id": packageOrder, "amount_money": "80"}}, "details": details}, "")
	if status != 400 || errorMessage(tooMuch) != errInvoiceAmount.Error() {
		t.Fatal("over-amount purchase was not rejected", status, tooMuch)
	}
	status, topup, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"receipt_id": topupReceipt, "amount_money": "40"}}, "details": details}, "")
	if status != 400 || errorMessage(topup) != errInvoiceTopUp.Error() {
		t.Fatal("top-up receipt was invoiced", status, topup)
	}
	status, balance, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"receipt_id": balancePackage, "amount_money": "40"}}, "details": details}, "")
	if status != 400 || errorMessage(balance) != errInvoiceStage.Error() {
		t.Fatal("balance-only package was invoiced at the receipt stage", status, balance)
	}
}

func errorMessage(out map[string]any) string {
	body, _ := out["error"].(map[string]any)
	message, _ := body["message"].(string)
	return message
}
