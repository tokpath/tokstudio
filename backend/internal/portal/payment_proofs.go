package portal

import (
	"strings"

	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/remediation"
)

// Same-currency payments have one amount. Preserve any independently supplied
// amount so contradictory evidence cannot be silently rewritten by the form.
func normalizeExecutionProof(proof remediation.ExecutionProof, settlementCurrency string) (remediation.ExecutionProof, error) {
	proof.Currency = strings.ToUpper(strings.TrimSpace(proof.Currency))
	if proof.Currency != settlementCurrency {
		return proof, nil
	}
	amount, err := ledger.ParseDecimal(proof.Amount)
	if err != nil || amount.Sign() <= 0 || (!proof.ActualMoney.IsZero() && proof.ActualMoney.Cmp(amount) != 0) {
		return proof, remediation.ErrInvalid
	}
	proof.ActualMoney = amount
	return proof, nil
}

func normalizeWithdrawalPaymentProof(proof incentives.WithdrawalPaymentProof, settlementCurrency string) (incentives.WithdrawalPaymentProof, error) {
	proof.Currency = strings.ToUpper(strings.TrimSpace(proof.Currency))
	if proof.Currency != settlementCurrency {
		return proof, nil
	}
	if strings.TrimSpace(proof.OriginalAmount) != "" {
		amount, err := ledger.ParseDecimal(proof.OriginalAmount)
		if err != nil || amount.Cmp(proof.PaidMoney) != 0 {
			return proof, incentives.ErrInvalidInput
		}
	}
	proof.OriginalAmount = proof.PaidMoney.String()
	if strings.TrimSpace(proof.ValuationEvidence) == "" {
		proof.ValuationEvidence = "Verified payment in the settlement currency"
	}
	return proof, nil
}
