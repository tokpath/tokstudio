package ledger

import (
	"context"
	"time"

	"gorm.io/gorm"
)

func (s *Service) GetGrant(ctx context.Context, tx *gorm.DB, id string) (Grant, error) {
	var row Grant
	e := s.within(ctx, tx, func(t *gorm.DB) error { return t.First(&row, "id=?", id).Error })
	return row, e
}

func (s *Service) AccountInTransaction(ctx context.Context, tx *gorm.DB, id string) (Account, error) {
	var out Account
	err := s.within(ctx, tx, func(t *gorm.DB) error { return t.First(&out, "id=?", id).Error })
	return out, err
}

// HoldInTransaction reads an operator-scoped hold without taking locks out of
// order. Mutating callers must still use Capture or Release to validate and
// lock the account balances before resolving the hold.
func (s *Service) HoldInTransaction(ctx context.Context, tx *gorm.DB, operator, id string) (Hold, error) {
	var out Hold
	if operator == "" || id == "" {
		return out, ErrScope
	}
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.First(&out, "id=? AND operator_id=?", id, operator).Error
	})
	return out, err
}
func (s *Service) ListAccounts(ctx context.Context, tx *gorm.DB, operator, owner string) ([]Account, error) {
	var rows []Account
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Where("operator_id=? AND owner_id=?", operator, owner).Order("purpose,id").Find(&rows).Error
	})
	return rows, err
}

// UnheldLiability excludes only model reservations already covered upstream.
// Refund, dispute and other holds still carry the original service obligation.
func (s *Service) UnheldLiability(ctx context.Context, tx *gorm.DB, operator string, at time.Time) (Decimal, error) {
	var out Decimal
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Raw("SELECT COALESCE(sum(a.balance-COALESCE((SELECT sum(ha.amount) FROM ledger_hold_allocations ha JOIN ledger_holds h ON h.id=ha.hold_id WHERE ha.account_id=a.id AND h.status='active' AND h.purpose='model'),0)),0)::text FROM ledger_grants g JOIN ledger_accounts a ON a.id=g.account_id WHERE g.operator_id=? AND g.effective_at<=? AND (g.expires_at IS NULL OR g.expires_at>?)", operator, at, at).Row().Scan(&out)
	})
	return out, err
}

func (s *Service) HoldsForAccount(ctx context.Context, tx *gorm.DB, operator, account string) ([]Hold, error) {
	var rows []Hold
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		return t.Table("ledger_holds h").Select("h.*").Joins("JOIN ledger_hold_allocations a ON a.hold_id=h.id").Where("h.operator_id=? AND a.account_id=? AND h.status='active'", operator, account).Find(&rows).Error
	})
	return rows, err
}

func (s *Service) HoldDetails(ctx context.Context, tx *gorm.DB, operator, id string) (Hold, []HoldAllocation, error) {
	var hold Hold
	var allocations []HoldAllocation
	err := s.within(ctx, tx, func(t *gorm.DB) error {
		if e := t.First(&hold, "id=? AND operator_id=?", id, operator).Error; e != nil {
			return e
		}
		return t.Where("hold_id=?", id).Order("account_id").Find(&allocations).Error
	})
	return hold, allocations, err
}
