package ledger

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// CancelClosureGifts is invoked by the authenticated lifecycle coordinator in
// the same transaction that formally closes the customer. Purchased rights
// must already be consumed, expired or explicitly waived. This does not grant
// permission to waive a customer's paid rights, cash or commission balances.
func (s *Service) CancelClosureGifts(ctx context.Context, t *gorm.DB, operator, customer, key, actor, evidence string, at time.Time) error {
	if t == nil || operator == "" || customer == "" || key == "" || actor == "" || evidence == "" || at.IsZero() {
		return ErrScope
	}
	if e := lockCustomer(t, operator, customer); e != nil {
		return e
	}
	var paid int64
	if e := t.WithContext(ctx).Table("ledger_grant_sources s").Joins("JOIN ledger_grants g ON g.id=s.grant_id").Where("g.operator_id=? AND g.customer_id=? AND s.kind='paid' AND s.quantity-s.consumed-s.expired-s.cancelled>0", operator, customer).Count(&paid).Error; e != nil {
		return e
	}
	if paid > 0 {
		return ErrScope
	}
	// All remaining sources are gifts. The existing cancellation path keeps
	// source allocation, balanced journals, idempotency and hold protection.
	return s.WaiveCustomerCredits(ctx, t, operator, customer, "closure-gifts:"+key, actor, evidence, at)
}

// WaiveCustomerCredits is called only by an authenticated closure coordinator
// with the customer's recorded explicit waiver. It never touches cash or
// commission and refuses to discard credits held by an unfinished operation.
func (s *Service) WaiveCustomerCredits(ctx context.Context, t *gorm.DB, operator, customer, key, actor, evidence string, at time.Time) error {
	if t == nil || key == "" || actor == "" || evidence == "" || at.IsZero() {
		return ErrScope
	}
	if e := lockCustomer(t, operator, customer); e != nil {
		return e
	}
	grants, e := s.CustomerGrants(ctx, t, operator, customer)
	if e != nil {
		return e
	}
	for _, g := range grants {
		a, e := s.AccountInTransaction(ctx, t, g.AccountID)
		if e != nil {
			return e
		}
		if a.Held.Sign() > 0 {
			return ErrHoldState
		}
		if a.Balance.IsZero() {
			continue
		}
		h, e := s.Reserve(ctx, t, Reservation{OperatorID: operator, BusinessKey: "waiver:" + key + ":" + g.ID, Purpose: "waiver", ExpiresAt: at.AddDate(10, 0, 0), Amounts: []Line{{g.AccountID, a.Balance}}})
		if e != nil {
			return e
		}
		if _, e = s.CancelReservedGrant(ctx, t, operator, customer, g.ID, h.ID, "waiver:"+key+":"+g.ID, actor, evidence, at); e != nil {
			return e
		}
	}
	return nil
}
