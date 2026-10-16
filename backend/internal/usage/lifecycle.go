package usage

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// ExpireCustomerCredits shares the ownership fence with admission and takeover.
// A queued job for the previous operator becomes a no-op after ownership moves;
// the new operator's scan will process the same original grants and deadlines.
func (s *Service) ExpireCustomerCredits(ctx context.Context, operatorID, customerID string) error {
	if s.Organization == nil || s.Ledger == nil || s.GrantObserver == nil {
		return ErrNotReady
	}
	return s.within(ctx, nil, func(t *gorm.DB) error {
		own, err := s.Organization.CurrentOwnership(ctx, t, customerID)
		if err != nil {
			return err
		}
		if own.OperatorID != operatorID {
			return nil
		}
		if _, err = s.Organization.LockOwnership(ctx, t, customerID, operatorID, own.Version); err != nil {
			return err
		}
		if err = s.GrantObserver.LockBusiness(ctx, t, operatorID); err != nil {
			return err
		}
		at, err := s.businessTime(t)
		if err != nil {
			return err
		}
		events, err := s.Ledger.ExpireCredits(ctx, t, operatorID, customerID, at)
		if err != nil {
			return err
		}
		for _, event := range events {
			grant, err := s.Ledger.GetGrant(ctx, t, event.GrantID)
			if err != nil {
				return err
			}
			if err = s.GrantObserver.ObserveGrantEvent(ctx, t, grant, event); err != nil {
				return err
			}
		}
		return nil
	})
}

// DisableCustomerKeys is a trusted lifecycle hook. Existing accepted requests
// keep their price, holds and settlement; re-enabling an API key is explicit.
func (s *Service) DisableCustomerKeys(ctx context.Context, t *gorm.DB, customer, evidence string) error {
	if t == nil || evidence == "" {
		return ErrConflict
	}
	return t.WithContext(ctx).Model(&Key{}).Where("customer_id=? AND kind='api' AND status='active'", customer).Updates(map[string]any{"status": "disabled", "version": gorm.Expr("version+1"), "updated_at": gorm.Expr("clock_timestamp()")}).Error
}
func (s *Service) ClosureRequirements(ctx context.Context, t *gorm.DB, customer string) ([]organization.Requirement, error) {
	if t == nil {
		t = s.DB
	}
	var counts struct{ InFlight, Pending int64 }
	e := t.WithContext(ctx).Model(&Request{}).Select("count(*) FILTER(WHERE status IN ('accepted','executing','submitted')) AS in_flight,count(*) FILTER(WHERE status='awaiting_usage') AS pending").Where("customer_id=?", customer).Scan(&counts).Error
	return []organization.Requirement{{Code: "in_flight", Description: "accepted model requests must finish", Ready: counts.InFlight == 0}, {Code: "pending_usage", Description: "usage verification must settle or expire free", Ready: counts.Pending == 0}}, e
}
