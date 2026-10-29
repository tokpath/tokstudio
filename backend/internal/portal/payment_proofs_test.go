package portal

import (
	"testing"

	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

func TestPaymentProofsUseConfiguredCurrencyAndRejectContradictoryAmounts(t *testing.T) {
	proof := remediation.ExecutionProof{Currency: " cny ", Amount: "70"}
	got, err := normalizeExecutionProof(proof, "CNY")
	if err != nil || got.Currency != "CNY" || got.ActualMoney.String() != "70" {
		t.Fatal(got, err)
	}
	proof.ActualMoney = ledger.FromInt64(10)
	if _, err = normalizeExecutionProof(proof, "CNY"); err == nil {
		t.Fatal("same-currency contradictory proof was rewritten")
	}
	proof.Currency = "USD"
	got, err = normalizeExecutionProof(proof, "CNY")
	if err != nil || got.ActualMoney.String() != "10" {
		t.Fatal("foreign USD was treated as native", got, err)
	}
	withdrawal := incentives.WithdrawalPaymentProof{Currency: " cny ", PaidMoney: ledger.FromInt64(70)}
	paid, err := normalizeWithdrawalPaymentProof(withdrawal, "CNY")
	if err != nil || paid.Currency != "CNY" || paid.OriginalAmount != "70" || paid.ValuationEvidence == "" {
		t.Fatal(paid, err)
	}
	withdrawal.OriginalAmount = "10"
	if _, err = normalizeWithdrawalPaymentProof(withdrawal, "CNY"); err == nil {
		t.Fatal("same-currency contradictory withdrawal proof was rewritten")
	}
	withdrawal.Currency, withdrawal.ValuationEvidence = "USD", "verified one transfer"
	paid, err = normalizeWithdrawalPaymentProof(withdrawal, "CNY")
	if err != nil || paid.OriginalAmount != "10" || paid.ValuationEvidence != withdrawal.ValuationEvidence {
		t.Fatal("foreign USD was treated as native withdrawal", paid, err)
	}
}
