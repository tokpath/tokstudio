package ledger

import (
	"context"
	"errors"
	"math/big"

	"gorm.io/gorm"
)

type roundingResult struct {
	AccountID, SourceKey, OperationKey, ExactNumerator, ExactDenominator string
	Amount                                                               Decimal
	RemainderNumerator, RemainderDenominator                             string
}

func (roundingResult) TableName() string { return "ledger_rounding_results" }

type carryRow struct {
	AccountID, SourceKey, Numerator, Denominator string
	Version                                      int64
}

func (carryRow) TableName() string { return "ledger_carries" }

// AccumulateCharge preserves sub-atom charges across requests. It must commit in
// the same transaction as the resulting monetary debit and settlement record.
func (s *Service) AccumulateCharge(ctx context.Context, tx *gorm.DB, account, source, operation string, value *big.Rat) (Decimal, error) {
	if tx == nil || value == nil || value.Sign() < 0 || source == "" || operation == "" {
		return Zero(), ErrAmount
	}
	var out Decimal
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		if _, e := lockAccounts(t, []string{account}); e != nil {
			return e
		}
		var result roundingResult
		e := t.First(&result, "account_id=? AND source_key=? AND operation_key=?", account, source, operation).Error
		if e == nil {
			if result.ExactNumerator != value.Num().String() || result.ExactDenominator != value.Denom().String() {
				return ErrConflict
			}
			out = result.Amount
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		row := carryRow{AccountID: account, SourceKey: source, Numerator: "0", Denominator: "1"}
		// The account row above serializes all writes to its carry. A new
		// carry can use zero in memory until its first result is recorded.
		e = t.First(&row, "account_id=? AND source_key=?", account, source).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		carry, ok := new(big.Rat).SetString(row.Numerator + "/" + row.Denominator)
		if !ok || carry.Sign() < 0 {
			return ErrAmount
		}
		amount, remainder := Accumulate(value, carry)
		if !amount.Valid() {
			return ErrAmount
		}
		result = roundingResult{AccountID: account, SourceKey: source, OperationKey: operation, ExactNumerator: value.Num().String(), ExactDenominator: value.Denom().String(), Amount: amount, RemainderNumerator: remainder.Num().String(), RemainderDenominator: remainder.Denom().String()}
		if e = t.Create(&result).Error; e != nil {
			return e
		}
		if row.Numerator != result.RemainderNumerator || row.Denominator != result.RemainderDenominator {
			if e = t.Exec(`INSERT INTO ledger_carries(account_id,source_key,numerator,denominator,version) VALUES(?,?,?,?,1)
 ON CONFLICT(account_id,source_key) DO UPDATE SET numerator=EXCLUDED.numerator,denominator=EXCLUDED.denominator,version=ledger_carries.version+1`,
				account, source, result.RemainderNumerator, result.RemainderDenominator).Error; e != nil {
				return e
			}
		}
		out = amount
		return nil
	})
	return out, e
}
