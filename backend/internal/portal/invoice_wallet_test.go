package portal

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/operations"
	"testing"
	"time"
)

// One topup funds two independent purchases and leaves cash available for an
// unrelated withdrawal. There are no recharge lots to satisfy these actions.
func pooledInvoiceFixture(t *testing.T) (*testApp, string, []commerce.Order) {
	t.Helper()
	f := setupPortal(t)
	f.openRegistration(t)
	f.registered(t, "pooled-invoice@example.test")
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	var customer string
	if err := f.a.DB.Raw("SELECT customer_id FROM organization_customer_identities WHERE email_normalized=? AND is_current", "pooled-invoice@example.test").Scan(&customer).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.a.DB.Exec("UPDATE organization_operator_statuses SET new_purchase=true WHERE operator_id=?", op).Error; err != nil {
		t.Fatal(err)
	}
	money := ledger.MustDecimal("100")
	zero := ledger.Zero()
	now := time.Now().UTC()
	topup, err := f.a.Commerce.CreateTopUpOrder(ctx, nil, commerce.CreateTopUpInput{OperatorID: op, CustomerID: customer, BusinessKey: "pooled-topup", ExpectedOwnershipVersion: 1, AmountMoney: money})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := f.a.Commerce.RecordReceipt(ctx, nil, commerce.VerifiedReceipt{OperatorID: op, Provider: "isolated_fixture", ProviderReceiptID: "pooled-topup", PayerReference: "synthetic payer", RecipientReference: "synthetic merchant", GrossMoney: money, NetMoney: &money, FeeMoney: &zero, FeeStatus: "confirmed", Currency: "USD", OriginalAmount: "100", VerificationEvidence: "synthetic cash receipt", ValuationEvidence: "USD", VerifiedBy: owner, PaidAt: &now, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Commerce.AllocateReceipt(ctx, nil, commerce.AllocateReceiptInput{ActorStaffID: owner, OperatorID: op, ReceiptID: receipt.ID, OrderID: topup.ID, BusinessKey: "topup-paid", AmountMoney: money}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Commerce.Fulfill(ctx, nil, topup.ID, owner); err != nil {
		t.Fatal(err)
	}
	orders := []commerce.Order{}
	for _, name := range []string{"Purchase A", "Purchase B"} {
		q, err := f.a.Commerce.PublishPackage(ctx, nil, commerce.PublishPackageInput{ActorStaffID: owner, OperatorID: op, Name: name, PriceMoney: ledger.MustDecimal("40"), PaidATG: ledger.MustDecimal("40"), GiftATG: ledger.MustDecimal("10"), Period: ledger.Month, Timezone: "UTC", NewPurchaseEnabled: true, TermsReference: "isolated test"})
		if err != nil {
			t.Fatal(err)
		}
		o, err := f.a.Commerce.CreateOrder(ctx, nil, commerce.CreateOrderInput{OperatorID: op, CustomerID: customer, QuoteID: q.ID, BusinessKey: name, ExpectedOwnershipVersion: 1, InternalMoney: ledger.MustDecimal("40")})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.a.Commerce.Fulfill(ctx, nil, o.ID, owner); err != nil {
			t.Fatal(err)
		}
		orders = append(orders, o)
	}
	if _, err = f.a.Operations.SaveInvoicePolicy(ctx, owner, operations.InvoicePolicy{OperatorID: op, Enabled: true, RulesReference: "Completed purchases at the full paid amount"}, 0); err != nil {
		t.Fatal(err)
	}
	return f, customer, orders
}
func TestPooledWalletInvoicesAndWithdrawalAreIndependent(t *testing.T) {
	f, customer, orders := pooledInvoiceFixture(t)
	ctx := context.Background()
	op, owner := f.a.Installation.OperatorID, f.a.Installation.OwnerStaffID
	f.login(t, "customer", "pooled-invoice@example.test", "customer-password-test")
	request := func(order, key string) (int, map[string]any) {
		code, result, _ := f.call(t, "POST", "/v1/me/invoices", map[string]any{"issuer_id": op, "lines": []map[string]string{{"order_id": order}}, "details": map[string]string{"title": "Pooled buyer", "tax_id": "TEST-123", "email": "billing@example.test"}}, key)
		return code, result
	}
	key := uuid.NewString()
	status, a := request(orders[0].ID, key)
	mustOK(t, status, a)
	status, replay := request(orders[0].ID, key)
	mustOK(t, status, replay)
	if replay["id"] != a["id"] {
		t.Fatal(replay)
	}
	status, duplicate := request(orders[0].ID, uuid.NewString())
	if status != 409 {
		t.Fatal(status, duplicate)
	}
	status, b := request(orders[1].ID, uuid.NewString())
	mustOK(t, status, b)
	var total ledger.Decimal
	if err := f.a.DB.Table("operations_invoices").Select("sum(amount_money)::text").Row().Scan(&total); err != nil || total.Cmp(ledger.MustDecimal("80")) != 0 {
		t.Fatal(total, err)
	}
	policy, err := f.a.Incentives.SetWithdrawalPolicy(ctx, nil, owner, op, incentives.WithdrawalPolicy{Wallet: "balance", MethodID: uuid.NewString(), MethodName: "Cash", PayoutKind: "bank_cn", Enabled: true, MinimumMoney: ledger.FromInt64(1)}, 0)
	if err != nil {
		t.Fatal(err)
	}
	withdrawal, err := f.a.Incentives.RequestWithdrawal(ctx, nil, incentives.WithdrawalInput{OperatorID: op, CustomerID: customer, BusinessKey: "remaining-cash", PolicyID: policy.ID, RecipientReference: "test recipient", AmountMoney: ledger.MustDecimal("20")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Incentives.ApproveWithdrawal(ctx, nil, owner, op, withdrawal.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Incentives.VerifyWithdrawalPaid(ctx, nil, owner, op, withdrawal.ID, incentives.WithdrawalPaymentProof{Currency: "USD", OriginalAmount: "20", PaidMoney: ledger.MustDecimal("20"), PaidAt: time.Now().UTC(), Evidence: "synthetic payment"}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"commerce_wallet_funding_lots", "commerce_wallet_funding_commands", "commerce_wallet_funding_allocations"} {
		var n int64
		if err := f.a.DB.Table(table).Count(&n).Error; err != nil || n != 0 {
			t.Fatal("pooled operations created source allocations", table, n, err)
		}
	}
	status, replay = request(orders[0].ID, key)
	mustOK(t, status, replay)
	if replay["id"] != a["id"] {
		t.Fatal("withdrawal affected purchase invoice", replay)
	}
}
