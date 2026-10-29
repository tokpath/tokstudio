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
	order, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountMoney: ledger.MustDecimal("10")})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	base := "/v1/operators/" + operator
	input := map[string]any{"order_id": order.ID, "provider_receipt_id": "independent-bank-10", "currency": "USD", "gross_money": "10", "recipient_reference": "merchant bank", "payer_reference": "customer bank", "verification_evidence": "independently verified bank statement", "paid_at": time.Now().UTC()}
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
	if err != nil || receipt.OriginalAmount != "10" || receipt.ObservedAt.IsZero() || receipt.FeeStatus != "pending" || receipt.FeeMoney != nil || receipt.NetMoney != nil || receipt.AllocatedMoney.String() != "10" {
		t.Fatal(receipt, err)
	}
	paid, err := f.a.Commerce.GetOrder(ctx, nil, customer, order.ID)
	if err != nil || paid.Status != "paid" || paid.ExternalAllocatedMoney.String() != "10" {
		t.Fatal(paid, err)
	}
	status, out, _ = f.call(t, "POST", base+"/payments/"+id+"/accounting", map[string]any{"fee_money": "1", "evidence": "verified fee statement"}, uuid.NewString())
	mustOK(t, status, out)
	receipt, err = f.a.Commerce.GetReceipt(ctx, nil, operator, id)
	if err != nil || receipt.FeeStatus != "confirmed" || receipt.NetMoney.String() != "9" || receipt.GrossMoney.String() != "10" || receipt.AllocatedMoney.String() != "10" || receipt.OriginalFeeAmount == nil || receipt.OriginalFeeAmount.String() != "1" || receipt.OriginalNetAmount == nil || receipt.OriginalNetAmount.String() != "9" {
		t.Fatal("fee changed customer value", receipt, err)
	}
	status, _, _ = f.call(t, "POST", base+"/payments/"+id+"/accounting", map[string]any{"fee_money": "1", "original_fee_amount": "2", "evidence": "contradictory same-currency fee"}, uuid.NewString())
	if status != 400 {
		t.Fatal("same-currency fee conflict was accepted", status)
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
			if name == "provider" || name == "net_money" || name == "observed_at" {
				t.Fatal("derived value exposed", rawField)
			}
		}
	}

	stripeID := uuid.NewString()
	_, err = f.a.Payments.SaveMerchant(ctx, owner, payments.Merchant{ID: stripeID, OperatorID: operator, Driver: "stripe", Enabled: true, Credentials: map[string]string{"secret_key": "sk_test_finance", "publishable_key": "pk_test_finance", "webhook_secret": "whsec_finance", "currency": "USD"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unsafe, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountMoney: ledger.MustDecimal("10")})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.DB.Create(&checkoutRow{OrderID: unsafe.ID, CustomerID: customer, MerchantID: &stripeID, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	input["order_id"], input["provider_receipt_id"] = unsafe.ID, "must-not-bypass-online-payment"
	status, out, _ = f.call(t, "POST", base+"/payments", input, uuid.NewString())
	mustOK(t, status, out)
	unmatched, err := f.a.Commerce.GetReceipt(ctx, nil, operator, out["id"].(string))
	if err != nil || !unmatched.AllocatedMoney.IsZero() || !strings.Contains(out["message"].(string), "尚未关联") {
		t.Fatal("real money lost or online checkout bypassed", unmatched, out, err)
	}
	short, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountMoney: ledger.MustDecimal("20")})
	if err != nil {
		t.Fatal(err)
	}
	input["order_id"], input["provider_receipt_id"] = short.ID, "short-payment-kept-for-review"
	shortKey := uuid.NewString()
	status, out, _ = f.call(t, "POST", base+"/payments", input, shortKey)
	mustOK(t, status, out)
	shortReceipt := out["id"].(string)
	status, replay, _ = f.call(t, "POST", base+"/payments", input, shortKey)
	mustOK(t, status, replay)
	if replay["id"] != shortReceipt {
		t.Fatal("unmatched receipt duplicated", replay)
	}
	unmatched, err = f.a.Commerce.GetReceipt(ctx, nil, operator, shortReceipt)
	if err != nil || !unmatched.AllocatedMoney.IsZero() || unmatched.GrossMoney.String() != "10" {
		t.Fatal("short real receipt lost", unmatched, err)
	}
	status, out, _ = f.call(t, "POST", base+"/payments/"+shortReceipt+"/allocate", map[string]any{"order_id": short.ID}, uuid.NewString())
	if status != 409 || out["error"].(map[string]any)["code"] != "receipt_amount_conflict" || out["error"].(map[string]any)["retryable"] != false {
		t.Fatal("known mismatch reported as an uncertain execution", status, out)
	}
}

func TestFinanceReceiptRequiresSingleTransferSettlementEvidence(t *testing.T) {
	paid := time.Now().UTC()
	in := manualReceiptInput{Currency: "CNY", GrossMoney: ledger.MustDecimal("10"), OriginalAmount: "70", PaidAt: &paid, FeeStatus: "confirmed"}
	if _, err := in.verified(uuid.NewString(), "EUR"); err == nil {
		t.Fatal("confirmed fee without amount was accepted")
	}
	zero := ledger.Zero()
	in.FeeMoney = &zero
	if _, err := in.verified(uuid.NewString(), "EUR"); err == nil {
		t.Fatal("cross-currency settlement without transfer evidence was accepted")
	}
	in.SettlementEvidence = "verified CNY transfer settles EUR 10 for this order"
	fact, err := in.verified(uuid.NewString(), "EUR")
	if err != nil || fact.Currency != "CNY" || fact.OriginalAmount != "70" || fact.GrossMoney.String() != "10" || fact.ValuationEvidence != in.SettlementEvidence {
		t.Fatal(fact, err)
	}
	in.Currency, in.OriginalAmount, in.SettlementEvidence = " cny ", "", ""
	fact, err = in.verified(uuid.NewString(), "CNY")
	if err != nil || fact.Currency != "CNY" || fact.OriginalAmount != "10" || fact.ValuationEvidence == "" {
		t.Fatal("non-USD native receipt required a conversion", fact, err)
	}
	in.OriginalAmount = "70"
	if _, err = in.verified(uuid.NewString(), "CNY"); err == nil {
		t.Fatal("contradictory same-currency receipt amounts were accepted")
	}
}

func TestFinanceReceiptPreservesOriginalFeesWithoutCrossCurrencyConversion(t *testing.T) {
	paid := time.Now().UTC()
	fee := ledger.FromInt64(2)
	in := manualReceiptInput{Currency: "CNY", SettlementAmount: ledger.FromInt64(10), OriginalAmount: "70", PaidAt: &paid, OriginalFeeAmount: &fee, SettlementEvidence: "verified transfer settles EUR 10"}
	fact, err := in.verified(uuid.NewString(), "EUR")
	if err != nil || fact.OriginalFeeAmount == nil || fact.OriginalFeeAmount.String() != "2" || fact.OriginalNetAmount == nil || fact.OriginalNetAmount.String() != "68" || fact.FeeMoney != nil || fact.NetMoney != nil || fact.FeeStatus != "pending" {
		t.Fatal("original fee was converted into an unverified settlement cost", fact, err)
	}
	settlementFee := ledger.MustDecimal("0.3")
	in.FeeMoney = &settlementFee
	fact, err = in.verified(uuid.NewString(), "EUR")
	if err != nil || fact.OriginalNetAmount.String() != "68" || fact.FeeMoney.String() != "0.3" || fact.NetMoney.String() != "9.7" {
		t.Fatal("explicit fee facts in each currency were conflated", fact, err)
	}
	in.Currency, in.OriginalAmount, in.OriginalFeeAmount = "EUR", "10", nil
	fact, err = in.verified(uuid.NewString(), "EUR")
	if err != nil || fact.OriginalFeeAmount == nil || fact.OriginalFeeAmount.String() != "0.3" || fact.OriginalNetAmount.String() != "9.7" {
		t.Fatal("known same-currency fee was lost from original expense", fact, err)
	}
}

func TestPostgresRetailCashReceiptKeepsOptionalEvidenceAndActualAmounts(t *testing.T) {
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "cash@example.test")
	f.login(t, "customer", "cash@example.test", "customer-password-test")
	status, me, _ := f.call(t, "GET", "/v1/auth/me", nil, "")
	mustOK(t, status, me)
	customer := me["customer"].(map[string]any)["id"].(string)
	operator := f.a.Installation.OperatorID
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", operator).Error; err != nil {
		t.Fatal(err)
	}
	order, err := f.a.Commerce.CreateTopUpOrder(context.Background(), nil, commerce.CreateTopUpInput{OperatorID: operator, CustomerID: customer, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1, AmountMoney: ledger.FromInt64(10)})
	if err != nil {
		t.Fatal(err)
	}
	f.login(t, "staff", "owner@example.test", "owner-password-test")
	input := map[string]any{"order_id": order.ID, "currency": "USD", "gross_money": "10", "paid_at": time.Now().UTC()}
	status, out, _ := f.call(t, "POST", "/v1/operators/"+operator+"/payments", input, uuid.NewString())
	mustOK(t, status, out)
	receipt, err := f.a.Commerce.GetReceipt(context.Background(), nil, operator, out["id"].(string))
	if err != nil || receipt.AllocatedMoney.String() != "10" || receipt.VerificationEvidence != "" {
		t.Fatal("cash payment failed", receipt, err)
	}
	for _, missing := range []string{"paid_at", "gross_money"} {
		invalid := map[string]any{"currency": "USD", "gross_money": "10", "paid_at": time.Now().UTC()}
		delete(invalid, missing)
		status, _, _ = f.call(t, "POST", "/v1/operators/"+operator+"/payments", invalid, uuid.NewString())
		if status != 400 {
			t.Fatal("missing cash fact accepted", missing, status)
		}
	}
	// A verified online-provider record still requires its authenticated facts.
	_, err = f.a.Commerce.RecordReceipt(context.Background(), nil, commerce.VerifiedReceipt{OperatorID: operator, Provider: "stripe", ProviderReceiptID: uuid.NewString(), GrossMoney: ledger.FromInt64(10), FeeStatus: "pending", Currency: "USD", OriginalAmount: "10", ValuationEvidence: "native USD", VerifiedBy: "adapter:stripe"})
	if err == nil {
		t.Fatal("optional manual fields weakened provider validation")
	}
}
