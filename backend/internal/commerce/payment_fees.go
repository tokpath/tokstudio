package commerce

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func (s *Service) receiptAllocationLines(ctx context.Context, tx *gorm.DB, op, from, to string, principal, fee ledger.Decimal) ([]ledger.Line, error) {
	lines := []ledger.Line{{AccountID: from, Amount: principal.Add(fee).Neg()}, {AccountID: to, Amount: principal}}
	if fee.Sign() > 0 {
		account, e := s.account(ctx, tx, op, op, "control:payment_fees")
		if e != nil {
			return nil, e
		}
		lines = append(lines, ledger.Line{AccountID: account.ID, Amount: fee})
	}
	return lines, nil
}

// AssignManualPaymentFee classifies a previously unallocated manual receipt
// when its original order is selected. The caller validates the order snapshot.
func (s *Service) AssignManualPaymentFee(ctx context.Context, tx *gorm.DB, op, id string, fee ledger.Decimal) error {
	if tx == nil || !fee.Valid() || fee.Sign() < 0 {
		return ErrInvalidInput
	}
	result := tx.WithContext(ctx).Exec("UPDATE commerce_receipts SET payment_fee_money=? WHERE id=? AND operator_id=? AND provider='manual' AND allocated_money=0 AND payment_fee_money=0 AND gross_money>?", fee, id, op, fee)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}
