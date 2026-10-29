package commerce

import (
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"testing"
	"time"
)

func TestPostgresInvoiceStagesFollowOriginalWalletFundingAndActualConsumption(t *testing.T) {
	f := setup(t)
	_, receipts := f.separatelyFundedTopups(t)
	o := f.order(t, "invoice-funded-package", "60", "0")
	direct := f.receipt(t, "invoice-direct-part", "20", "0", o.CreatedAt)
	f.allocate(t, "invoice-direct-part", direct, o, "20")
	if _, e := f.s.Fulfill(f.ctx, nil, o.ID, "test:delivery"); e != nil {
		t.Fatal(e)
	}
	first, e := f.s.InvoiceReceiptBasis(f.ctx, nil, f.operator, f.customer, receipts[0].ID)
	if e != nil || first.TopUpMoney.String() != "30" || first.PurchaseMoney.String() != "30" || !first.ConsumptionMoney.IsZero() {
		t.Fatalf("stage funding %+v %v", first, e)
	}
	second, e := f.s.InvoiceReceiptBasis(f.ctx, nil, f.operator, f.customer, receipts[1].ID)
	if e != nil || second.TopUpMoney.String() != "70" || second.PurchaseMoney.String() != "30" {
		t.Fatalf("split funding %+v %v", second, e)
	}
	external, e := f.s.InvoiceReceiptBasis(f.ctx, nil, f.operator, f.customer, direct.ID)
	if e != nil || !external.TopUpMoney.IsZero() || external.PurchaseMoney.String() != "20" {
		t.Fatalf("direct purchase stage %+v %v", external, e)
	}
	// Production delivery starts the grant at the PostgreSQL business instant.
	// Use that clock for actual consumption as well: a slightly slower host
	// clock would make a newly delivered grant correctly appear not yet active.
	var now time.Time
	if e = f.db.Raw("SELECT clock_timestamp()").Row().Scan(&now); e != nil {
		t.Fatal(e)
	}
	h, e := f.ledger.ReserveCredits(f.ctx, nil, ledger.CreditReservation{OperatorID: f.operator, CustomerID: f.customer, BusinessKey: "invoice-metering", Estimate: d("60"), At: now, ExpiresAt: now.Add(time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.ledger.SettleCredits(f.ctx, nil, f.operator, f.customer, h.ID, "usage:invoice-metering", d("60"), now); e != nil {
		t.Fatal(e)
	}
	first, e = f.s.InvoiceReceiptBasis(f.ctx, nil, f.operator, f.customer, receipts[0].ID)
	if e != nil || first.ConsumptionMoney.String() != "15" {
		t.Fatalf("actual proportional paid consumption %+v %v", first, e)
	}
	external, e = f.s.InvoiceReceiptBasis(f.ctx, nil, f.operator, f.customer, direct.ID)
	if e != nil || external.ConsumptionMoney.String() != "10" {
		t.Fatalf("direct actual consumption %+v %v", external, e)
	}
}
