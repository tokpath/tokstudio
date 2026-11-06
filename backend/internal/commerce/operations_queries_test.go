package commerce

import (
	"github.com/google/uuid"
	"testing"
)

func TestPostgresInvoicePurchaseUsesFullPaidOrderRegardlessOfFunding(t *testing.T) {
	f := setup(t)
	f.separatelyFundedTopups(t)
	o := f.order(t, "invoice-pooled-package", "60", "0")
	direct := f.receipt(t, "invoice-direct-part", "20", "0", o.CreatedAt)
	f.allocate(t, "invoice-direct-part", direct, o, "20")
	if _, e := f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); e != nil {
		t.Fatal(e)
	}
	order, e := f.s.InvoicePurchase(f.ctx, nil, f.operator, f.customer, o.ID)
	if e != nil || order.DueMoney.Cmp(d("80")) != 0 || order.Status != "completed" {
		t.Fatal(order, e)
	}
	if _, e = f.s.InvoicePurchase(f.ctx, nil, f.operator, uuid.NewString(), o.ID); e == nil {
		t.Fatal("another customer read sale")
	}
}
