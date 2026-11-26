package cooperation

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func (s *Service) IncreaseProcurementReservation(ctx context.Context, tx *gorm.DB, buyer, request, holdID string, target ledger.Decimal, expiry time.Time) error {
	if err := s.Lock(ctx, tx, buyer); err != nil {
		return err
	}
	position, err := s.Position(ctx, tx, buyer)
	if err != nil {
		return err
	}
	return s.Ledger.IncreaseProcurementHold(ctx, tx, buyer, holdID, "procurement:"+request, position.AccountID, target, expiry)
}
