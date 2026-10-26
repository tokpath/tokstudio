package portal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
)

func TestPostgresFinanceReceiptLinksOneOrderAndDerivesOnlyKnownValues(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "finance-simple@example.test")
	f.login(t, "customer", "finance-simple@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	ctx := context.Background()
	operator, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", operator).Error; err != nil {
		t.Fatal(err)
	}
	order, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountUSD: ledger.MustDecimal("10")})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + operator
	input := map[string]any{"order_id": order.ID, "provider_receipt_id": "independent-bank-10", "currency": "USD", "gross_usd": "10", "recipient_reference": "merchant bank", "payer_reference": "customer bank", "verification_evidence": "independently verified bank statement", "paid_at": time.Now().UTC()}
	key := uuid.NewString()
	status, out, _ := f.call(t, "POST", base+"/payments", input, key)
	mustOK(t, status, out)
	id := out["id"].(string)
	status, replay, _ := f.call(t, "POST", base+"/payments", input, key)
	mustOK(t, status, replay)
	if replay["id"] != id {
		t.Fatal("receipt duplicated", replay)
	}
	receipt, err := f.a.Commerce.GetReceipt(ctx, nil, operator, id)
	if err != nil || receipt.OriginalAmount != "10" || receipt.ObservedAt.IsZero() || receipt.FeeStatus != "pending" || receipt.FeeUSD != nil || receipt.NetUSD != nil || receipt.AllocatedUSD.String() != "10" {
		t.Fatal(receipt, err)
	}
	paid, err := f.a.Commerce.GetOrder(ctx, nil, customer, order.ID)
	if err != nil || paid.Status != "paid" || paid.ExternalAllocatedUSD.String() != "10" {
		t.Fatal(paid, err)
	}
	status, out, _ = f.call(t, "POST", base+"/payments/"+id+"/accounting", map[string]any{"fee_usd": "1", "evidence": "verified fee statement"}, uuid.NewString())
	mustOK(t, status, out)
	receipt, err = f.a.Commerce.GetReceipt(ctx, nil, operator, id)
	if err != nil || receipt.FeeStatus != "confirmed" || receipt.NetUSD.String() != "9" || receipt.GrossUSD.String() != "10" || receipt.AllocatedUSD.String() != "10" {
		t.Fatal("fee changed customer value", receipt, err)
	}
	status, out, _ = f.call(t, "GET", base+"/ledger?view=journals", nil, "")
	mustOK(t, status, out)
	if len(out["items"].([]any)) == 0 {
		t.Fatal("ledger has no postings", out)
	}
	for _, raw := range out["items"].([]any) {
		if len(raw.(map[string]any)["values"].(map[string]any)["entries"].([]any)) == 0 {
			t.Fatal("posting missing entries", raw)
		}
	}
	for _, removed := range []string{"assets", "evidence"} {
		status, _, _ = f.call(t, "GET", base+"/"+removed, nil, "")
		if status != 404 {
			t.Fatal("duplicate workspace remains", removed, status)
		}
	}
	status, out, _ = f.call(t, "GET", base+"/payments", nil, "")
	mustOK(t, status, out)
	for _, raw := range out["operations"].([]any) {
		for _, rawField := range raw.(map[string]any)["fields"].([]any) {
			name := rawField.(map[string]any)["name"]
			if name == "provider" || name == "net_usd" || name == "observed_at" {
				t.Fatal("derived value exposed", rawField)
			}
		}
	}

	stripeID := uuid.NewString()
	_, err = f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: stripeID, OperatorID: operator, Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_finance", "publishable_key": "pk_test_finance", "webhook_secret": "whsec_finance", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unsafe, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountUSD: ledger.MustDecimal("10")})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Create(&checkoutRow{OrderID: unsafe.ID, CustomerID: customer, MerchantID: &stripeID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	input["order_id"], input["provider_receipt_id"] = unsafe.ID, "must-not-bypass-online-payment"
	status, out, _ = f.call(t, "POST", base+"/payments", input, uuid.NewString())
	if status != 409 || out["error"].(map[string]any)["code"] != "original_payment_pending" {
		t.Fatal(status, out)
	}
}

func TestFinanceReceiptForeignCurrencyStillNeedsActualValuation(t *testing.T) {
	paid := time.Now().UTC()
	in := manualReceiptInput{Currency: "CNY", GrossUSD: ledger.MustDecimal("10"), OriginalAmount: "70", PaidAt: &paid, FeeStatus: "confirmed"}
	if _, err := in.verified(uuid.NewString()); err == nil {
		t.Fatal("confirmed fee without amount was accepted")
	}
	zero := ledger.Zero()
	in.FeeUSD = &zero
	fact, err := in.verified(uuid.NewString())
	if err != nil || fact.OriginalAmount != "70" || fact.ValuationEvidence != "" {
		t.Fatal("foreign valuation fabricated", fact, err)
	}
	in.Currency = " usd "
	fact, err = in.verified(uuid.NewString())
	if err != nil || fact.OriginalAmount != "10" || !strings.Contains(fact.ValuationEvidence, "Native USD") {
		t.Fatal(fact, err)
	}
}
