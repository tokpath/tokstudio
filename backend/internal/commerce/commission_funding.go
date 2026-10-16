package commerce

import (
	"context"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func (s *Service) RestoreCommissionFunding(ctx context.Context, t *gorm.DB, op, customer, order, reference string, amount ledger.Decimal) error {
	if t == nil {
		return ErrInvalidInput
	}
	if s.commissionFunding == nil {
		return ErrNotReady
	}
	return s.commissionFunding.RestoreCommissionFunding(ctx, t, op, customer, order, reference, amount)
}
