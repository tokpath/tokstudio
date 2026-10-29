package cooperation

import (
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestFinancialProcurementUsesHistoricalCostAndDirectSupplyOnly(t *testing.T) {
	f := setup(t)
	f.purchase(t, "first", "80", "80", "20")
	h, e := f.s.ReserveProcurement(f.ctx, nil, f.channel, "report-consumption", amount("30"), time.Now().Add(time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.SettleProcurement(f.ctx, nil, f.channel, "report-consumption", h.ID, amount("30"), time.Now()); e != nil {
		t.Fatal(e)
	}
	f.purchase(t, "second", "60", "100", "0")
	e = f.db.Transaction(func(tx *gorm.DB) error {
		_, e := f.s.ReverseProcurementUsage(f.ctx, tx, f.channel, "report-consumption", "report-correction", f.owner, "verified incorrect meter", amount("10"), time.Now())
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Second)
	buyer, e := f.s.FinancialFacts(f.ctx, nil, f.channel, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if buyer.ProcurementCostMoney.String() != "24" || buyer.ProcurementCorrectionMoney.String() != "8" || buyer.ClosingInventoryCostMoney.String() != "124" || !buyer.WholesaleEarnedMoney.IsZero() {
		t.Fatalf("purchase updated old weighted cost or buyer counted supplier revenue: %+v", buyer)
	}
	supplier, e := f.s.FinancialFacts(f.ctx, nil, f.platform, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if supplier.WholesaleEarnedMoney.String() != "24" || supplier.WholesaleCorrectionMoney.String() != "8" || supplier.ClosingWholesalePrepaymentMoney.String() != "124" || !supplier.ProcurementCostMoney.IsZero() {
		t.Fatalf("direct wholesale recognition or deferred obligation wrong: %+v", supplier)
	}
}
