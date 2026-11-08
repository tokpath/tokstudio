package remediation

import (
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"testing"
	"time"
)

func TestPostgresUndeliveredRefundReturnsPrincipalAndPaymentFee(t *testing.T) {
	f := setup(t)
	o, e := f.s.Commerce.CreateOrder(f.ctx, nil, commerce.CreateOrderInput{CustomerID: f.customer, OperatorID: f.operator, QuoteID: f.quote.ID, BusinessKey: uuid.NewString(), ExpectedOwnershipVersion: 1})
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	r, e := f.s.Commerce.RecordReceipt(f.ctx, nil, commerce.VerifiedReceipt{OperatorID: f.operator, Provider: "verified_test", ProviderReceiptID: uuid.NewString(), GrossMoney: d("82"), PaymentFeeMoney: d("2"), FeeStatus: "pending", Currency: "USD", OriginalAmount: "82", RecipientReference: "merchant", PayerReference: "payer", ValuationEvidence: "USD receipt", VerificationEvidence: "verified receipt", VerifiedBy: f.owner, PaidAt: &now, ObservedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	input := commerce.AllocateReceiptInput{ActorStaffID: f.owner, OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: uuid.NewString(), AmountMoney: d("80")}
	for i := 0; i < 2; i++ {
		if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, input); e != nil {
			t.Fatal(e)
		}
	}
	c, e := f.s.RequestPackageException(f.ctx, PackageExceptionInput{ActorStaffID: f.owner, OperatorID: f.operator, CustomerID: f.customer, OrderID: o.ID, BusinessKey: uuid.NewString(), Reason: "unable_to_fulfill"})
	if e != nil || c.AmountMoney.String() != "82" {
		t.Fatal(c, e)
	}
	if _, e = f.s.ReviewRefund(f.ctx, f.owner, f.operator, c.ID, "approve", ""); e != nil {
		t.Fatal(e)
	}
	legs, e := f.s.Legs(f.ctx, nil, f.operator, c.ID)
	if e != nil || len(legs) != 1 || legs[0].AmountMoney.String() != "82" {
		t.Fatal(legs, e)
	}
	f.pay(t, legs[0])
	var income ledger.Decimal
	if e = f.db.Table("ledger_accounts").Where("operator_id=? AND purpose='control:payment_fees'", f.operator).Select("balance").Row().Scan(&income); e != nil || !income.IsZero() {
		t.Fatal("fee not reversed", income, e)
	}
}
