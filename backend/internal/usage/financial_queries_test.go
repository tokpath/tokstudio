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
	result := modelruntime.Result{State: "completed", Usage: measured("400", "400")}
	if e := f.s.RecordAttempt(f.ctx, req.ID, id, result); e != nil {
		t.Fatal(e)
	}
	if e := f.s.RecordAttempt(f.ctx, req.ID, id, result); e != nil {
		t.Fatal("same provider fact must remain one expense", e)
	}
	from, to := time.Now().Add(-time.Hour), time.Now().Add(time.Minute)
	report, e := f.s.FinancialFacts(f.ctx, nil, f.operator, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if report.CompletedRequests != 1 || report.PaidInvocationUSD.Cmp(d("80")) != 0 || report.SupplierCostUSD.Cmp(d("0.8")) != 0 || report.OwnRetailFulfillmentCostUSD.Cmp(d("0.8")) != 0 || report.OwnRetailGiftCostUSD.Cmp(d("0.2")) != 0 || report.ShortfallATG.Cmp(d("0.1")) != 0 {
		t.Fatalf("financial split: %+v", report)
	}
	if _, e = f.s.CorrectCustomerCharge(f.ctx, nil, ChargeCorrectionInput{OperatorID: f.operator, RequestID: req.ID, BusinessKey: "financial-correction", ActorID: f.owner, Evidence: "confirmed billing mistake", AmountATG: d("0.8")}, inc); e != nil {
		t.Fatal(e)
	}
	report, e = f.s.FinancialFacts(f.ctx, nil, f.operator, from, to)
	if e != nil {
		t.Fatal(e)
	}
	if report.CorrectionPaidUSD.Cmp(d("80")) != 0 || !report.ShortfallATG.IsZero() || report.SupplierCostUSD.Cmp(d("0.8")) != 0 {
		t.Fatalf("retail correction must not fabricate supplier refund: %+v", report)
	}
	rows, e := f.s.OperationalRequests(f.ctx, nil, f.operator, f.customer, from, to, 100)
	if e != nil || len(rows) != 1 || rows[0].ActualATG.String() != "0.8" || rows[0].ChargedATG.String() != "0.7" || rows[0].CorrectedATG.String() != "0.8" || rows[0].CorrectedChargedATG.String() != "0.7" || !rows[0].NetActualATG.IsZero() || !rows[0].NetChargedATG.IsZero() || rows[0].FulfillmentCostUSD.String() != "0.8" {
		t.Fatal("export lost original and corrected facts or rewrote supplier cost", rows, e)
	}
	other, e := f.s.FinancialFacts(f.ctx, nil, uuid.NewString(), from, to)
	if e != nil || other.CompletedRequests != 0 || !other.PaidInvocationUSD.IsZero() || !other.SupplierCostUSD.IsZero() {
		t.Fatal("cross-operator financial leak", other, e)
	}
}
