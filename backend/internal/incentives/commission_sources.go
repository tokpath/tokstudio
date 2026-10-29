package incentives

import (
	"context"
	"math/big"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type commissionSourceFlag struct {
	EventID, Reference, OperatorID   string
	NumeratorMoney, DenominatorMoney ledger.Decimal
	Active                           bool
	Evidence                         string
	CreatedAt, UpdatedAt             time.Time
}

func (commissionSourceFlag) TableName() string { return "incentives_commission_source_flags" }

type commissionEntryFlag struct {
	EntryID, Reference, EventID string
	BlockedMoney                ledger.Decimal
	Active                      bool
	CreatedAt                   time.Time
}

func (commissionEntryFlag) TableName() string { return "incentives_commission_entry_flags" }

type commissionDisputeHold struct {
	EntryID, Reference, OperatorID, HoldID string
	AmountMoney                            ledger.Decimal
	State                                  string
	CreatedAt                              time.Time
}

func (commissionDisputeHold) TableName() string { return "incentives_commission_dispute_holds" }

// FlagCommissionSource persists the source-specific execution gate as soon as
// a verified dispute is recorded, before an operator tries to apply its holds.
// Fractions use the still-unclaimed receipt basis, so repeated partial disputes
// consume only their own share of this source's remaining controllable money.
func (s *Service) FlagCommissionSource(ctx context.Context, t *gorm.DB, actor, op, sourceEvent, reference, evidence string, numerator, denominator ledger.Decimal) error {
	if t == nil || !validID(sourceEvent) || reference == "" || evidence == "" || numerator.Sign() <= 0 || denominator.Sign() <= 0 || numerator.Cmp(denominator) > 0 {
		return ErrInvalidInput
	}
	if e := s.org.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var event confirmationRow
	if e := t.First(&event, "operator_id=? AND source_event_id=?", op, sourceEvent).Error; e != nil {
		return e
	}
	var previous commissionSourceFlag
	q := t.Where("event_id=? AND reference=?", event.ID, reference).Limit(1).Find(&previous)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		if !previous.Active || previous.NumeratorMoney.Cmp(numerator) != 0 || previous.DenominatorMoney.Cmp(denominator) != 0 || previous.Evidence != evidence {
			return ErrConflict
		}
		return nil
	}
	flag := commissionSourceFlag{EventID: event.ID, Reference: reference, OperatorID: op, NumeratorMoney: numerator, DenominatorMoney: denominator, Active: true, Evidence: evidence}
	if e := t.Create(&flag).Error; e != nil {
		return e
	}
	var lots []commissionLot
	if e := t.Table("incentives_commission_funding_lots l").Select("l.*").Joins("JOIN incentives_commission_entries e ON e.id=l.entry_id").Where("e.event_id=?", event.ID).Find(&lots).Error; e != nil {
		return e
	}
	for _, lot := range lots {
		if e := s.applyFlagsToLot(t, lot); e != nil {
			return e
		}
	}
	return s.event(t, op, event.ID, "commission.source_restricted", map[string]any{"actor_id": actor, "evidence": evidence, "reference": reference, "source_event_id": sourceEvent, "numerator_money": numerator, "denominator_money": denominator})
}
func (s *Service) applyFlagsToLot(t *gorm.DB, lot commissionLot) error {
	var entry commissionRow
	if e := t.First(&entry, "id=?", lot.EntryID).Error; e != nil {
		return e
	}
	var flags []commissionSourceFlag
	if e := t.Where("event_id=? AND active", entry.EventID).Order("created_at,reference").Find(&flags).Error; e != nil {
		return e
	}
	for _, flag := range flags {
		var count int64
		if e := t.Model(&commissionEntryFlag{}).Where("entry_id=? AND reference=?", lot.EntryID, flag.Reference).Count(&count).Error; e != nil {
			return e
		}
		if count > 0 {
			continue
		}
		var already ledger.Decimal
		if e := t.Model(&commissionEntryFlag{}).Select("COALESCE(sum(blocked_money),0)::text").Where("entry_id=? AND active", lot.EntryID).Row().Scan(&already); e != nil {
			return e
		}
		remaining := lot.AmountMoney.Sub(lot.ConsumedMoney).Sub(already)
		if remaining.Sign() < 0 {
			remaining = ledger.Zero()
		}
		blocked, _ := ledger.Quantize(newRatio(remaining, flag.NumeratorMoney, flag.DenominatorMoney))
		if e := t.Create(&commissionEntryFlag{EntryID: lot.EntryID, Reference: flag.Reference, EventID: entry.EventID, BlockedMoney: blocked, Active: true}).Error; e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) expandRestoredFlags(t *gorm.DB, entry string, amount ledger.Decimal) error {
	var flags []commissionSourceFlag
	if e := t.Table("incentives_commission_source_flags f").Select("f.*").Joins("JOIN incentives_commission_entries e ON e.event_id=f.event_id").Where("e.id=? AND f.active", entry).Order("f.created_at,f.reference").Find(&flags).Error; e != nil {
		return e
	}
	remaining := amount
	for _, flag := range flags {
		blocked, _ := ledger.Quantize(newRatio(remaining, flag.NumeratorMoney, flag.DenominatorMoney))
		if blocked.Sign() > 0 {
			if e := t.Model(&commissionEntryFlag{}).Where("entry_id=? AND reference=? AND active", entry, flag.Reference).Update("blocked_money", gorm.Expr("blocked_money+?", blocked)).Error; e != nil {
				return e
			}
		}
		remaining = remaining.Sub(blocked)
	}
	return nil
}
func (s *Service) HoldCommissionSource(ctx context.Context, t *gorm.DB, actor, op, sourceEvent, reference, evidence string) error {
	if t == nil {
		return ErrInvalidInput
	}
	if e := s.org.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var event confirmationRow
	if e := t.First(&event, "operator_id=? AND source_event_id=?", op, sourceEvent).Error; e != nil {
		return e
	}
	var flags []commissionEntryFlag
	if e := t.Where("event_id=? AND reference=? AND active", event.ID, reference).Order("entry_id").Find(&flags).Error; e != nil {
		return e
	}
	for _, flag := range flags {
		var existing commissionDisputeHold
		q := t.Where("entry_id=? AND reference=?", flag.EntryID, reference).Limit(1).Find(&existing)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			continue
		}
		var lot commissionLot
		if e := t.First(&lot, "entry_id=?", flag.EntryID).Error; e != nil {
			return e
		}
		remaining := lot.AmountMoney.Sub(lot.ConsumedMoney)
		amount := minimum(flag.BlockedMoney, remaining)
		if amount.IsZero() {
			continue
		}
		if amount.Cmp(remaining.Sub(lot.ReservedMoney)) > 0 {
			return ErrBlocked
		}
		key := reference + ":" + lot.EntryID
		if _, e := s.reserveCommissionUse(ctx, t, op, lot.CustomerID, "dispute", key, amount, &lot.EntryID); e != nil {
			return e
		}
		account, e := s.account(ctx, t, op, lot.CustomerID, "commission:available")
		if e != nil {
			return e
		}
		at, e := s.BusinessInstant(ctx, t)
		if e != nil {
			return e
		}
		hold, e := s.ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: op, BusinessKey: "commission-source:" + key, Purpose: "commission_source_dispute", ExpiresAt: at.At.AddDate(10, 0, 0), Amounts: []ledger.Line{{AccountID: account.ID, Amount: amount}}})
		if e != nil {
			return e
		}
		if e = t.Create(&commissionDisputeHold{EntryID: lot.EntryID, Reference: reference, OperatorID: op, HoldID: hold.ID, AmountMoney: amount, State: "held"}).Error; e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) ReleaseCommissionSource(ctx context.Context, t *gorm.DB, actor, op, sourceEvent, reference, evidence string) error {
	if t == nil {
		return ErrInvalidInput
	}
	if e := s.org.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var event confirmationRow
	if e := t.First(&event, "operator_id=? AND source_event_id=?", op, sourceEvent).Error; e != nil {
		return e
	}
	var holds []commissionDisputeHold
	if e := t.Table("incentives_commission_dispute_holds h").Select("h.*").Joins("JOIN incentives_commission_entry_flags f ON f.entry_id=h.entry_id AND f.reference=h.reference").Where("f.event_id=? AND h.reference=? AND h.state='held'", event.ID, reference).Order("h.entry_id").Find(&holds).Error; e != nil {
		return e
	}
	for _, hold := range holds {
		var lot commissionLot
		if e := t.First(&lot, "entry_id=?", hold.EntryID).Error; e != nil {
			return e
		}
		if e := s.resolveCommissionUse(ctx, t, op, lot.CustomerID, "dispute", reference+":"+lot.EntryID, false); e != nil {
			return e
		}
		if _, e := s.ledger.Release(ctx, t, op, hold.HoldID, "commission-dispute-release:"+hold.EntryID+":"+reference); e != nil {
			return e
		}
		if e := t.Model(&commissionDisputeHold{}).Where("entry_id=? AND reference=?", hold.EntryID, reference).Update("state", "released").Error; e != nil {
			return e
		}
	}
	if e := t.Model(&commissionEntryFlag{}).Where("event_id=? AND reference=?", event.ID, reference).Update("active", false).Error; e != nil {
		return e
	}
	if e := t.Model(&commissionSourceFlag{}).Where("event_id=? AND reference=?", event.ID, reference).Updates(map[string]any{"active": false, "updated_at": gorm.Expr("clock_timestamp()")}).Error; e != nil {
		return e
	}
	return s.event(t, op, event.ID, "commission.source_released", map[string]any{"actor_id": actor, "evidence": evidence, "reference": reference})
}

func newRatio(amount, numerator, denominator ledger.Decimal) *big.Rat {
	return new(big.Rat).Mul(amount.Rat(), new(big.Rat).Quo(numerator.Rat(), denominator.Rat()))
}
