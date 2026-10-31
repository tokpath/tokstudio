package incentives

import (
	"context"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
	"time"
)

// CloseCustomerPromotion records the customer's confirmed future commission
// cutoff, preserving acquisition, chain, historical earnings and any existing
// release dispute. It is a trusted lifecycle hook, never a raw HTTP command.
func (s *Service) CloseCustomerPromotion(ctx context.Context, tx *gorm.DB, operator, customer, evidence string, at time.Time) error {
	if tx == nil || !validID(operator) || !validID(customer) || evidence == "" || at.IsZero() {
		return ErrInvalidInput
	}
	own, e := s.org.CurrentOwnership(ctx, tx, customer)
	if e != nil {
		return e
	}
	if own.OperatorID != operator {
		return ErrNotEligible
	}
	if e = s.LockBusiness(ctx, tx, operator); e != nil {
		return e
	}
	state, e := s.stateAt(tx, operator, customer, at)
	if e != nil {
		return e
	}
	if state.Status == "terminated" {
		return nil
	}
	if e = s.advance(ctx, tx, operator, customer, at); e != nil {
		return e
	}
	instant, e := s.BusinessInstant(ctx, tx)
	if e != nil {
		return e
	}
	r := functionRow{ID: uuid.NewString(), OperatorID: operator, CustomerID: customer, Status: "terminated", ReleaseBlocked: state.ReleaseBlocked, EvidenceReference: evidence, ActorID: customer, EffectiveAt: at.UTC(), BusinessSequence: instant.Sequence}
	if e = tx.Create(&r).Error; e != nil {
		return e
	}
	return s.event(tx, operator, customer, "commission.function_changed", FunctionState(r))
}

// ClosureRequirements reports actual outstanding commission obligations. It
// deliberately avoids advancing/altering incentive state during a close check.
func (s *Service) ClosureRequirements(ctx context.Context, tx *gorm.DB, operator, customer string) ([]organization.Requirement, error) {
	own, e := s.org.CurrentOwnership(ctx, tx, customer)
	if e != nil {
		return nil, e
	}
	if own.OperatorID != operator {
		return nil, ErrNotEligible
	}
	frozen, e := s.account(ctx, tx, operator, customer, "commission:frozen")
	if e != nil {
		return nil, e
	}
	available, e := s.account(ctx, tx, operator, customer, "commission:available")
	if e != nil {
		return nil, e
	}
	var q qualificationRow
	if e = s.dbFor(ctx, tx).Where("operator_id=? AND customer_id=?", operator, customer).Limit(1).Find(&q).Error; e != nil {
		return nil, e
	}
	var payouts int64
	if e = s.dbFor(ctx, tx).Model(&withdrawalRow{}).Where("operator_id=? AND customer_id=? AND status IN ('requested','approved')", operator, customer).Count(&payouts).Error; e != nil {
		return nil, e
	}
	return []organization.Requirement{{Code: "commission_frozen", Ready: frozen.Balance.IsZero(), Description: "frozen commission 结算币种: " + frozen.Balance.String()}, {Code: "commission_available", Ready: available.Balance.IsZero() && available.Held.IsZero(), Description: "available/held commission 结算币种: " + available.Balance.String() + " / " + available.Held.String()}, {Code: "pending_offsets", Ready: q.PendingOffsetMoney.IsZero(), Description: "pending commission recovery 结算币种: " + q.PendingOffsetMoney.String()}, {Code: "payouts", Ready: payouts == 0, Description: "all requested and approved withdrawals must be resolved"}}, nil
}
