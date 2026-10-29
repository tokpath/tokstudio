package operations

import (
	"encoding/json"
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"strings"
	"testing"
)

func TestOperatingRecognitionDoesNotDoubleCountPrepaymentOrGifts(t *testing.T) {
	r := composeFinancial("operator", ledger.FinancialSnapshot{GrantEvents: []ledger.GrantFinancialFact{{Kind: "expiry", PaidMoney: ledger.FromInt64(5)}}}, commerce.FinancialSnapshot{SalesMoney: ledger.FromInt64(100), WalletTopUpMoney: ledger.FromInt64(200)}, cooperation.FinancialSnapshot{PurchasesMoney: ledger.FromInt64(70), WholesaleSalesMoney: ledger.FromInt64(80), WholesaleEarnedMoney: ledger.FromInt64(12), WholesaleCorrectionMoney: ledger.FromInt64(2), ProcurementCostMoney: ledger.FromInt64(8), ProcurementCorrectionMoney: ledger.FromInt64(1)}, usage.FinancialSnapshot{PaidInvocationMoney: ledger.FromInt64(20), CorrectionPaidMoney: ledger.FromInt64(3), OwnRetailGiftCostMoney: ledger.FromInt64(2)}, RefundFinancialFacts{PendingFees: 1}, false)
	if r.Revenue.TotalMoney.String() != "32" || r.Costs.TotalMoney.String() != "7" || r.Costs.RetailGiftSubsetMoney.String() != "2" || r.Status != "pending_costs" {
		t.Fatalf("prepayments became income, gift subset deducted twice, or unknown fees hidden: %+v", r)
	}
}

func TestFinancialExportSeparatesNativeCostsAndOmitsProfit(t *testing.T) {
	report := FinancialReport{SettlementCurrency: "CNY", OriginalCurrencyTotals: []OriginalCurrencyTotal{
		{Currency: "USD", SupplierReferenceCost: ledger.MustDecimal("2.5")},
		{Currency: "EUR", SupplierActualCost: ledger.MustDecimal("3")},
		{Currency: "CNY", Income: ledger.MustDecimal("100"), Expense: ledger.MustDecimal("7"), SupplierReportedCost: ledger.MustDecimal("9")},
	}}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "operating_result") || strings.Contains(string(raw), "profit") {
		t.Fatal("profit returned", string(raw))
	}
	rows, err := financialExportRows(report)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, row := range rows {
		var item struct {
			Section, Metric, Unit string
			Value                 json.RawMessage
		}
		if err = json.Unmarshal(row, &item); err != nil {
			t.Fatal(err)
		}
		found[item.Metric+":"+item.Unit] = string(item.Value)
		if strings.Contains(item.Metric, "经营结果") {
			t.Fatal("profit exported", string(row))
		}
	}
	if found["Bifrost 参考成本（观察）:USD"] != "\"2.5\"" || found["登记实际账单成本:EUR"] != "\"3\"" || found["供应商报告成本（观察）:CNY"] != "\"9\"" {
		t.Fatal("original cost currency or basis lost", found)
	}
}
