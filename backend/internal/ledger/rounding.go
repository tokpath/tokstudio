package ledger

import (
	"context"
	"errors"
	"math/big"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		if e = t.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return e
		}
		if e = t.First(&row, "account_id=? AND source_key=?", account, source).Error; e != nil {
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
		if e = t.Model(&carryRow{}).Where("account_id=? AND source_key=?", account, source).Updates(map[string]any{"numerator": result.RemainderNumerator, "denominator": result.RemainderDenominator, "version": gorm.Expr("version+1")}).Error; e != nil {
			return e
		}
		out = amount
		return nil
	})
	return out, e
}
