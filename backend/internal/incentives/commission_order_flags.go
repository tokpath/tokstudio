package incentives

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type commissionOrderFlag struct {
	OrderID, Reference, OperatorID   string
	NumeratorMoney, DenominatorMoney ledger.Decimal
	Active, FutureEvents             bool
	Evidence                         string
	Fingerprint                      string
	CreatedAt                        time.Time
}

func (commissionOrderFlag) TableName() string { return "incentives_commission_order_flags" }

func (s *Service) FlagCommissionOrder(ctx context.Context, t *gorm.DB, actor, op, order, reference, evidence string, claim, basis ledger.Decimal) error {
	if t == nil || !validID(order) || reference == "" || evidence == "" || claim.Sign() <= 0 || basis.Sign() <= 0 {
		return ErrInvalidInput
	}
	if e := s.org.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var snapshot snapshotRow
	if e := t.First(&snapshot, "order_id=? AND operator_id=?", order, op).Error; e != nil {
		return e
	}
	fp := fingerprint(map[string]any{"operator_id": op, "order_id": order, "reference": reference, "evidence": evidence, "claim": claim, "basis": basis})
	var old commissionOrderFlag
	q := t.Where("order_id=? AND reference=?", order, reference).Limit(1).Find(&old)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		if old.Fingerprint != fp {
			return ErrConflict
		}
		return nil
	}
	var previous ledger.Decimal
	if e := t.Model(&commissionOrderFlag{}).Select("COALESCE(sum(numerator_money),0)::text").Where("order_id=? AND active", order).Row().Scan(&previous); e != nil {
		return e
	}
	remaining := basis.Sub(previous)
	if remaining.Sign() <= 0 {
		return nil
	}
	claim = minimum(claim, remaining)
	flag := commissionOrderFlag{OrderID: order, Reference: reference, OperatorID: op, NumeratorMoney: claim, DenominatorMoney: remaining, Active: true, FutureEvents: true, Evidence: evidence, Fingerprint: fp}
	if e := t.Create(&flag).Error; e != nil {
		return e
	}
	var events []confirmationRow
	if e := t.Where("order_id=? AND operator_id=? AND kind IN ('usage','expiry')", order, op).Find(&events).Error; e != nil {
		return e
	}
	for _, event := range events {
		if e := s.FlagCommissionSource(ctx, t, actor, op, event.SourceEventID, reference, evidence, claim, remaining); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) inheritCommissionOrderFlags(t *gorm.DB, event confirmationRow) error {
	var flags []commissionOrderFlag
	if e := t.Where("order_id=? AND operator_id=? AND active AND future_events", event.OrderID, event.OperatorID).Order("created_at,reference").Find(&flags).Error; e != nil {
		return e
	}
	for _, flag := range flags {
		source := commissionSourceFlag{EventID: event.ID, Reference: flag.Reference, OperatorID: event.OperatorID, NumeratorMoney: flag.NumeratorMoney, DenominatorMoney: flag.DenominatorMoney, Active: true, Evidence: flag.Evidence}
		if e := t.Create(&source).Error; e != nil {
			return e
		}
	}
	return nil
}

// Seal stops future inheritance only once the original receipt's remaining
// rights are actually held. New consumption then comes from the valid remainder.
func (s *Service) SealCommissionOrderRestrictions(ctx context.Context, t *gorm.DB, op, reference string) error {
	if t == nil {
		return ErrInvalidInput
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	return t.Model(&commissionOrderFlag{}).Where("operator_id=? AND reference=?", op, reference).Update("future_events", false).Error
}
func (s *Service) ReleaseCommissionRestrictions(ctx context.Context, t *gorm.DB, actor, op, reference, evidence string) error {
	if t == nil {
		return ErrInvalidInput
	}
	if e := s.org.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var flags []commissionSourceFlag
	if e := t.Where("operator_id=? AND reference=? AND active", op, reference).Order("event_id").Find(&flags).Error; e != nil {
		return e
	}
	for _, flag := range flags {
		var event confirmationRow
		if e := t.First(&event, "id=?", flag.EventID).Error; e != nil {
			return e
		}
		if e := s.ReleaseCommissionSource(ctx, t, actor, op, event.SourceEventID, reference, evidence); e != nil {
			return e
		}
	}
	return t.Model(&commissionOrderFlag{}).Where("operator_id=? AND reference=?", op, reference).Updates(map[string]any{"active": false, "future_events": false}).Error
}
