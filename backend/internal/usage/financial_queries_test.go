package usage

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func TestPostgresFinancialFactsKeepGiftCostWithinTotalAndCorrectionsSeparate(t *testing.T) {
	f, inc, req := correctionFixture(t)
	id := uuid.NewString()
	row := AttemptRecord{ID: id, RequestID: req.ID, UpstreamID: "upstream", ProviderID: "provider", Definition: []byte(`{}`), State: "dispatching", CostStatus: "pending", StartedAt: time.Now().Add(-time.Second)}
	if e := f.db.Create(&row).Error; e != nil {
		t.Fatal(e)
	}
	cost := "0.8"
	result := modelruntime.Result{State: "completed", Usage: measured("400", "400"), Cost: &modelruntime.CostObservation{ReferenceCost: &modelruntime.Money{Currency: "USD", Amount: cost}, ReportedCost: &modelruntime.Money{Currency: "CNY", Amount: "6"}, Source: "bifrost_calculated"}}
	if e := f.s.RecordAttempt(f.ctx, req.ID, id, result); e != nil {
		t.Fatal(e)
	}
	if e := f.s.RecordAttempt(f.ctx, req.ID, id, result); e != nil {
		t.Fatal("same provider fact must remain one expense", e)
	}
	var postings int64
	if e := f.db.Table("ledger_journals").Where("business_key=?", "supplier:"+id).Count(&postings).Error; e != nil || postings != 0 {
		t.Fatal("supplier observations created automatic financial postings", postings, e)
	}
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Minute)
	report, e := f.s.FinancialFacts(f.ctx, nil, f.operator, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if report.CompletedRequests != 1 || report.PaidInvocationMoney.Cmp(d("80")) != 0 || len(report.SupplierCosts) != 2 || report.SupplierCosts[0].Currency != "CNY" || report.SupplierCosts[0].ReportedCost.String() != "6" || report.SupplierCosts[1].ReferenceCost.String() != "0.8" || !report.OwnRetailFulfillmentCostMoney.IsZero() || !report.OwnRetailGiftCostMoney.IsZero() || report.ShortfallATG.Cmp(d("0.1")) != 0 {
		t.Fatalf("financial split: %+v", report)
	}
	if _, e = f.s.CorrectCustomerCharge(f.ctx, nil, ChargeCorrectionInput{OperatorID: f.operator, RequestID: req.ID, BusinessKey: "financial-correction", ActorID: f.owner, Evidence: "confirmed billing mistake", AmountATG: d("0.8")}, inc); e != nil {
		t.Fatal(e)
	}
	report, e = f.s.FinancialFacts(f.ctx, nil, f.operator, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if report.CorrectionPaidMoney.Cmp(d("80")) != 0 || !report.ShortfallATG.IsZero() || len(report.SupplierCosts) != 2 || report.SupplierCosts[0].Currency != "CNY" || report.SupplierCosts[0].ReportedCost.String() != "6" || report.SupplierCosts[1].ReferenceCost.String() != "0.8" {
		t.Fatalf("retail correction must not fabricate supplier refund: %+v", report)
	}
	rows, e := f.s.OperationalRequests(f.ctx, nil, f.operator, f.customer, from, to, 100)
	if e != nil || len(rows) != 1 || rows[0].EstimateATG == nil || rows[0].EstimateATG.Sign() <= 0 || rows[0].ActualATG.String() != "0.8" || rows[0].ChargedATG.String() != "0.7" || rows[0].CorrectedATG.String() != "0.8" || rows[0].CorrectedChargedATG.String() != "0.7" || !rows[0].NetActualATG.IsZero() || !rows[0].NetChargedATG.IsZero() || !rows[0].FulfillmentCostMoney.IsZero() || len(rows[0].SupplierReferenceCosts) != 1 || rows[0].SupplierReferenceCosts[0].Currency != "USD" || rows[0].SupplierReferenceCosts[0].Amount != "0.8" {
		t.Fatal("export lost reservation, original and corrected facts, or rewrote supplier cost", rows, e)
	}
	other, e := f.s.FinancialFacts(f.ctx, nil, uuid.NewString(), from, to)
	if e != nil || other.CompletedRequests != 0 || !other.PaidInvocationMoney.IsZero() || len(other.SupplierCosts) != 0 {
		t.Fatal("cross-operator financial leak", other, e)
	}
}
