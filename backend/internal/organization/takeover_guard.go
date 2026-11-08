package organization

import (
	"context"
	"gorm.io/gorm"
)

// The operator row is the common fence used by registration, new purchases and
// takeover preparation. Settlement of already accepted work remains available.
func (s *Service) RequireNoTakeover(ctx context.Context, tx *gorm.DB, operator string) error {
	var n int64
	if e := s.dbFor(ctx, tx).Table("organization_takeover_locks").Where("operator_id=?", operator).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return ErrBusinessClosed
	}
	return nil
}
func (s *Service) LockChannelForTakeover(ctx context.Context, tx *gorm.DB, channel, id string) error {
	if tx == nil {
		return ErrInvalidInput
	}
	return tx.Exec("INSERT INTO organization_takeover_locks(operator_id,takeover_id) VALUES(?,?)", channel, id).Error
}
func (s *Service) RequireTakeoverRead(ctx context.Context, tx *gorm.DB, actor, operator string) error {
	if e := s.RequirePermission(ctx, tx, actor, operator, "cooperation.manage"); e == nil {
		return nil
	}
	return s.RequirePermission(ctx, tx, actor, operator, "ledger.read")
}
