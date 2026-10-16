package remediation

import (
	"errors"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestPostgresReceiptDisputeKeepsVerifiedFactAndAllowsOnlyReleasedValidRemainder(t *testing.T) {
	f := setup(t)
	o, e := f.s.Commerce.CreateTopUpOrder(f.ctx, nil, commerce.CreateTopUpInput{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "pending-dispute-topup", ExpectedOwnershipVersion: 1, AmountUSD: d("20")})
	if e != nil {
		t.Fatal(e)
	}
	r := f.receipt(t, "verified-unallocated-source", "100")
	cb := f.chargeback(t, r, "partial-unallocated-dispute", "50")
	in := commerce.AllocateReceiptInput{OperatorID: f.operator, ReceiptID: r.ID, OrderID: o.ID, BusinessKey: "original-source-allocation", AmountUSD: d("20")}
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, in); !errors.Is(e, commerce.ErrReceiptRestricted) {
		t.Fatal("new assignment escaped recorded source review", e)
	}
	e = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.Commerce.ReserveReceiptFunds(f.ctx, tx, commerce.ReceiptFundsInput{OperatorID: f.operator, ReceiptID: r.ID, DestinationID: uuid.NewString(), BusinessKey: "unapproved-procurement-source", ActorID: f.owner, AmountUSD: d("10"), Deadline: time.Now().Add(time.Hour)})
		return e
	})
	if !errors.Is(e, commerce.ErrReceiptRestricted) {
		t.Fatal("wholesale allocation escaped shared guard", e)
	}
	if current, e := f.s.Commerce.GetReceipt(f.ctx, nil, f.operator, r.ID); e != nil || current.GrossUSD.Cmp(d("100")) != 0 || !current.AllocatedUSD.IsZero() {
		t.Fatal("restricted allocation erased actual receipt", current, e)
	}
	if e = f.s.RestrictChargeback(f.ctx, f.owner, f.operator, cb.ID, "specific disputed half now held"); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commerce.AllocateReceipt(f.ctx, nil, in); e != nil {
		t.Fatal("valid remainder stayed globally frozen", e)
	}
	if _, e = f.s.Commerce.Fulfill(f.ctx, nil, o.ID, f.owner); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ResolveChargeback(f.ctx, f.owner, f.operator, cb.ID, "invalid_payment", "only original disputed half invalid"); e != nil {
		t.Fatal(e)
	}
	if a := f.balance(t); a.Available().Cmp(d("20")) != 0 {
		t.Fatal("new purchase from valid source remainder was seized", a)
	}
}
