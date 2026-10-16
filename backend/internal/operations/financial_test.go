package operations

import (
	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/cooperation"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/usage"
	"testing"
)

func TestOperatingRecognitionDoesNotDoubleCountPrepaymentOrGifts(t *testing.T) {
	r := composeFinancial("operator", ledger.FinancialSnapshot{GrantEvents: []ledger.GrantFinancialFact{{Kind: "expiry", PaidUSD: ledger.FromInt64(5)}}}, commerce.FinancialSnapshot{SalesUSD: ledger.FromInt64(100), WalletTopUpUSD: ledger.FromInt64(200)}, cooperation.FinancialSnapshot{PurchasesUSD: ledger.FromInt64(70), WholesaleSalesUSD: ledger.FromInt64(80), WholesaleEarnedUSD: ledger.FromInt64(12), WholesaleCorrectionUSD: ledger.FromInt64(2), ProcurementCostUSD: ledger.FromInt64(8), ProcurementCorrectionUSD: ledger.FromInt64(1)}, usage.FinancialSnapshot{PaidInvocationUSD: ledger.FromInt64(20), CorrectionPaidUSD: ledger.FromInt64(3), OwnRetailGiftCostUSD: ledger.FromInt64(2)}, RefundFinancialFacts{PendingFees: 1}, false)
	if r.Revenue.TotalUSD.String() != "32" || r.Costs.TotalUSD.String() != "7" || r.OperatingResultUSD.String() != "25" || r.Costs.RetailGiftSubsetUSD.String() != "2" || r.Status != "pending_costs" {
		t.Fatalf("prepayments became income, gift subset deducted twice, or unknown fees hidden: %+v", r)
	}
}
