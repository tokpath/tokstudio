package cooperation

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/incentives"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

func rewardOrder(p incentives.RegistrationPromise) commerce.Order {
	return commerce.Order{ID: p.IntentID, OperatorID: p.OperatorID, PackageGiftATG: p.InviteeATG.Add(p.InviterATG)}
}
func (s *Service) ReserveRegistration(ctx context.Context, tx *gorm.DB, p incentives.RegistrationPromise) (commerce.Commitment, error) {
	return s.Reserve(ctx, tx, rewardOrder(p))
}
func (s *Service) FulfillRegistration(ctx context.Context, tx *gorm.DB, p incentives.RegistrationPromise, actual ledger.Decimal) error {
	if actual.Sign() < 0 || actual.Cmp(p.InviteeATG.Add(p.InviterATG)) > 0 {
		return ErrConflict
	}
	return s.Fulfill(ctx, tx, rewardOrder(p))
}
func (s *Service) ReleaseRegistration(ctx context.Context, tx *gorm.DB, p incentives.RegistrationPromise) error {
	return s.Release(ctx, tx, rewardOrder(p))
}
