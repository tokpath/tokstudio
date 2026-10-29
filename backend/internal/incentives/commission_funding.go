package incentives

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type commissionLot struct {
	EntryID, OperatorID, OriginalOperatorID, CustomerID string
	AmountMoney, ReservedMoney, ConsumedMoney           ledger.Decimal
	CreatedAt                                           time.Time
}

func (commissionLot) TableName() string { return "incentives_commission_funding_lots" }

type commissionFundingCommand struct {
	ID, OperatorID, CustomerID, Kind, Reference string
	AmountMoney                                 ledger.Decimal
	RestrictedEntryID                           *string
	Attempt                                     int
	State                                       string
	CreatedAt                                   time.Time
}

func (commissionFundingCommand) TableName() string { return "incentives_commission_funding_commands" }

type CommissionFundingAllocation struct {
	ID            string         `json:"id"`
	CommandID     string         `json:"command_id"`
	EntryID       string         `json:"entry_id"`
	Attempt       int            `json:"attempt"`
	AmountMoney   ledger.Decimal `json:"amount_money"`
	RestoredMoney ledger.Decimal `json:"restored_money"`
	State         string         `json:"state"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (CommissionFundingAllocation) TableName() string {
	return "incentives_commission_funding_allocations"
}

func (s *Service) fundingLot(ctx context.Context, t *gorm.DB, e CommissionEntry, amount ledger.Decimal) error {
	if amount.Sign() <= 0 {
		return nil
	}
	lot := commissionLot{EntryID: e.ID, OperatorID: e.OperatorID, OriginalOperatorID: e.OriginalOperatorID, CustomerID: e.BeneficiaryID, AmountMoney: amount}
	if lot.OriginalOperatorID == "" {
		lot.OriginalOperatorID = e.OperatorID
	}
	if err := t.Create(&lot).Error; err != nil {
		return err
	}
	return s.applyFlagsToLot(t, lot)
}
func (s *Service) lotSpendable(t *gorm.DB, lot commissionLot) (ledger.Decimal, error) {
	var blocked, held ledger.Decimal
	if e := t.Table("incentives_commission_entry_flags").Select("COALESCE(sum(blocked_money),0)::text").Where("entry_id=? AND active", lot.EntryID).Row().Scan(&blocked); e != nil {
		return ledger.Zero(), e
	}
	if e := t.Table("incentives_commission_funding_allocations a").Select("COALESCE(sum(a.amount_money),0)::text").Joins("JOIN incentives_commission_funding_commands c ON c.id=a.command_id").Where("a.entry_id=? AND a.state='reserved' AND c.kind='dispute'", lot.EntryID).Row().Scan(&held); e != nil {
		return ledger.Zero(), e
	}
	blocked = blocked.Sub(held)
	if blocked.Sign() < 0 {
		blocked = ledger.Zero()
	}
	available := lot.AmountMoney.Sub(lot.ReservedMoney).Sub(lot.ConsumedMoney).Sub(blocked)
	if available.Sign() < 0 {
		available = ledger.Zero()
	}
	return available, nil
}
func (s *Service) reserveCommissionUse(ctx context.Context, t *gorm.DB, op, customer, kind, reference string, amount ledger.Decimal, restricted *string) (commissionFundingCommand, error) {
	var command commissionFundingCommand
	if t == nil || !validID(op) || !validID(customer) || reference == "" || !amount.Valid() || amount.Sign() <= 0 {
		return command, ErrInvalidInput
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return command, e
	}
	if kind == "order" || kind == "withdrawal" {
		if e := s.requireCommissionUseAllowed(ctx, t, op, customer); e != nil {
			return command, e
		}
	}
	q := t.Where("operator_id=? AND customer_id=? AND kind=? AND reference=?", op, customer, kind, reference).Limit(1).Find(&command)
	if q.Error != nil {
		return command, q.Error
	}
	if q.RowsAffected > 0 {
		if command.AmountMoney.Cmp(amount) != 0 || !sameOptionalString(command.RestrictedEntryID, restricted) {
			return command, ErrConflict
		}
		if command.State != "released" {
			return command, nil
		}
		command.Attempt++
		command.State = "reserved"
		if e := t.Save(&command).Error; e != nil {
			return command, e
		}
	} else {
		command = commissionFundingCommand{ID: uuid.NewString(), OperatorID: op, CustomerID: customer, Kind: kind, Reference: reference, AmountMoney: amount, RestrictedEntryID: restricted, Attempt: 1, State: "reserved"}
		if e := t.Create(&command).Error; e != nil {
			return command, e
		}
	}
	query := t.Clauses(clause.Locking{Strength: "UPDATE"}).Where("operator_id=? AND customer_id=? AND reserved_money+consumed_money<amount_money", op, customer)
	if restricted != nil {
		query = query.Where("entry_id=?", *restricted)
	}
	var lots []commissionLot
	if e := query.Order("created_at,entry_id").Find(&lots).Error; e != nil {
		return command, e
	}
	remaining := amount
	for _, lot := range lots {
		available, e := s.lotSpendable(t, lot)
		if e != nil {
			return command, e
		}
		if kind == "dispute" {
			available = lot.AmountMoney.Sub(lot.ReservedMoney).Sub(lot.ConsumedMoney)
		}
		take := minimum(remaining, available)
		if take.Sign() <= 0 {
			continue
		}
		if e = t.Model(&commissionLot{}).Where("entry_id=?", lot.EntryID).Update("reserved_money", lot.ReservedMoney.Add(take)).Error; e != nil {
			return command, e
		}
		allocation := CommissionFundingAllocation{ID: uuid.NewString(), CommandID: command.ID, EntryID: lot.EntryID, Attempt: command.Attempt, AmountMoney: take, State: "reserved"}
		if e = t.Create(&allocation).Error; e != nil {
			return command, e
		}
		remaining = remaining.Sub(take)
		if remaining.IsZero() {
			break
		}
	}
	if !remaining.IsZero() {
		return command, ErrBlocked
	}
	return command, nil
}
func sameOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
func (s *Service) resolveCommissionUse(ctx context.Context, t *gorm.DB, op, customer, kind, reference string, consume bool) error {
	if t == nil {
		return ErrInvalidInput
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var command commissionFundingCommand
	if e := t.Where("operator_id=? AND customer_id=? AND kind=? AND reference=?", op, customer, kind, reference).First(&command).Error; e != nil {
		return e
	}
	state := "released"
	if consume {
		state = "consumed"
	}
	if command.State == state {
		return nil
	}
	if command.State != "reserved" {
		return ErrConflict
	}
	var allocations []CommissionFundingAllocation
	if e := t.Where("command_id=? AND attempt=? AND state='reserved'", command.ID, command.Attempt).Order("entry_id").Find(&allocations).Error; e != nil {
		return e
	}
	for _, allocation := range allocations {
		updates := map[string]any{"reserved_money": gorm.Expr("reserved_money-?", allocation.AmountMoney)}
		if consume {
			updates["consumed_money"] = gorm.Expr("consumed_money+?", allocation.AmountMoney)
		}
		if e := t.Model(&commissionLot{}).Where("entry_id=?", allocation.EntryID).Updates(updates).Error; e != nil {
			return e
		}
		if e := t.Model(&CommissionFundingAllocation{}).Where("id=?", allocation.ID).Update("state", state).Error; e != nil {
			return e
		}
	}
	return t.Model(&command).Update("state", state).Error
}
func (s *Service) consumeCommissionCash(ctx context.Context, t *gorm.DB, op, customer, reference string, amount ledger.Decimal) error {
	if amount.IsZero() {
		return nil
	}
	if _, e := s.reserveCommissionUse(ctx, t, op, customer, "recovery", reference, amount, nil); e != nil {
		return e
	}
	return s.resolveCommissionUse(ctx, t, op, customer, "recovery", reference, true)
}

func (s *Service) consumeCorrectedCommission(ctx context.Context, t *gorm.DB, entry CommissionEntry, reference string, amount ledger.Decimal) error {
	var lot commissionLot
	q := t.Where("entry_id=?", entry.ID).Limit(1).Find(&lot)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		available, e := s.lotSpendable(t, lot)
		if e != nil {
			return e
		}
		own := minimum(amount, available)
		if own.Sign() > 0 {
			if _, e = s.reserveCommissionUse(ctx, t, entry.OperatorID, entry.BeneficiaryID, "recovery", reference+":original", own, &entry.ID); e != nil {
				return e
			}
			if e = s.resolveCommissionUse(ctx, t, entry.OperatorID, entry.BeneficiaryID, "recovery", reference+":original", true); e != nil {
				return e
			}
			amount = amount.Sub(own)
		}
	}
	if amount.IsZero() {
		return nil
	}
	return s.consumeCommissionCash(ctx, t, entry.OperatorID, entry.BeneficiaryID, reference+":available", amount)
}

// The following methods track provenance only. The commerce/withdrawal caller
// reserves, captures or releases the matching ledger cash in the same tx.
func (s *Service) ReserveCommissionFunding(ctx context.Context, t *gorm.DB, op, customer, order string, amount ledger.Decimal) error {
	_, e := s.reserveCommissionUse(ctx, t, op, customer, "order", order, amount, nil)
	return e
}
func (s *Service) CompleteCommissionFunding(ctx context.Context, t *gorm.DB, op, customer, order string) error {
	return s.resolveCommissionUse(ctx, t, op, customer, "order", order, true)
}
func (s *Service) ReleaseCommissionFunding(ctx context.Context, t *gorm.DB, op, customer, order string) error {
	return s.resolveCommissionUse(ctx, t, op, customer, "order", order, false)
}
func (s *Service) RequireCommissionFundingExecution(ctx context.Context, t *gorm.DB, op, customer, kind, reference string) error {
	if t == nil {
		return ErrInvalidInput
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	if e := s.requireCommissionUseAllowed(ctx, t, op, customer); e != nil {
		return e
	}
	var allocations []CommissionFundingAllocation
	if e := t.Table("incentives_commission_funding_allocations a").Select("a.*").Joins("JOIN incentives_commission_funding_commands c ON c.id=a.command_id").Where("c.operator_id=? AND c.customer_id=? AND c.kind=? AND c.reference=? AND a.state='reserved'", op, customer, kind, reference).Find(&allocations).Error; e != nil {
		return e
	}
	if len(allocations) == 0 {
		return ErrConflict
	}
	for _, a := range allocations {
		var lot commissionLot
		if e := t.First(&lot, "entry_id=?", a.EntryID).Error; e != nil {
			return e
		}
		var blocked, disputeHeld ledger.Decimal
		if e := t.Table("incentives_commission_entry_flags").Select("COALESCE(sum(blocked_money),0)::text").Where("entry_id=? AND active", a.EntryID).Row().Scan(&blocked); e != nil {
			return e
		}
		if e := t.Table("incentives_commission_funding_allocations x").Select("COALESCE(sum(x.amount_money),0)::text").Joins("JOIN incentives_commission_funding_commands c ON c.id=x.command_id").Where("x.entry_id=? AND x.state='reserved' AND c.kind='dispute'", a.EntryID).Row().Scan(&disputeHeld); e != nil {
			return e
		}
		unreserved := lot.AmountMoney.Sub(lot.ConsumedMoney).Sub(lot.ReservedMoney).Add(disputeHeld)
		if blocked.Cmp(unreserved) > 0 {
			return ErrBlocked
		}
	}
	return nil
}

func (s *Service) RestoreCommissionFunding(ctx context.Context, t *gorm.DB, op, customer, order, reference string, amount ledger.Decimal) error {
	if t == nil || !validID(op) || !validID(customer) || !validID(order) || reference == "" || !amount.Valid() || amount.Sign() <= 0 {
		return ErrInvalidInput
	}
	if e := s.LockBusiness(ctx, t, op); e != nil {
		return e
	}
	var previous struct{ AmountMoney ledger.Decimal }
	q := t.Table("incentives_commission_funding_restorations").Where("order_id=? AND reference=?", order, reference).Limit(1).Find(&previous)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected > 0 {
		if previous.AmountMoney.Cmp(amount) != 0 {
			return ErrConflict
		}
		return nil
	}
	var command commissionFundingCommand
	if e := t.First(&command, "operator_id=? AND customer_id=? AND kind='order' AND reference=?", op, customer, order).Error; e != nil {
		return e
	}
	if command.State != "consumed" {
		return ErrConflict
	}
	var allocations []CommissionFundingAllocation
	if e := t.Where("command_id=? AND state='consumed'", command.ID).Order("entry_id,id").Find(&allocations).Error; e != nil {
		return e
	}
	weights := make([]ledger.Decimal, len(allocations))
	total := ledger.Zero()
	for i, a := range allocations {
		weights[i] = a.AmountMoney.Sub(a.RestoredMoney)
		total = total.Add(weights[i])
	}
	if amount.Cmp(total) > 0 {
		return ErrConflict
	}
	parts, e := ledger.Proportional(amount, weights)
	if e != nil {
		return e
	}
	for i, a := range allocations {
		if parts[i].IsZero() {
			continue
		}
		if e = t.Model(&commissionLot{}).Where("entry_id=?", a.EntryID).Update("consumed_money", gorm.Expr("consumed_money-?", parts[i])).Error; e != nil {
			return e
		}
		if e = s.expandRestoredFlags(t, a.EntryID, parts[i]); e != nil {
			return e
		}
		if e = t.Model(&CommissionFundingAllocation{}).Where("id=?", a.ID).Update("restored_money", a.RestoredMoney.Add(parts[i])).Error; e != nil {
			return e
		}
	}
	type restoredSource struct {
		AllocationID string         `json:"allocation_id"`
		EntryID      string         `json:"entry_id"`
		AmountMoney  ledger.Decimal `json:"amount_money"`
	}
	sources := []restoredSource{}
	for i, a := range allocations {
		if parts[i].Sign() > 0 {
			sources = append(sources, restoredSource{AllocationID: a.ID, EntryID: a.EntryID, AmountMoney: parts[i]})
		}
	}
	encoded, _ := json.Marshal(sources)
	return t.Exec("INSERT INTO incentives_commission_funding_restorations(id,operator_id,customer_id,order_id,reference,amount_money,allocations) VALUES(?,?,?,?,?,?,?::jsonb)", uuid.NewString(), op, customer, order, reference, amount, string(encoded)).Error
}

type CommissionFundingView struct {
	CommissionFundingAllocation `gorm:"embedded"`
	SourceOperatorID            string `json:"source_operator_id"`
	SourceEventID               string `json:"source_event_id"`
}

func (s *Service) OrderCommissionFunding(ctx context.Context, t *gorm.DB, op, customer, order string) ([]CommissionFundingView, error) {
	if !validID(op) || !validID(customer) || !validID(order) {
		return nil, ErrInvalidInput
	}
	rows := []CommissionFundingView{}
	e := s.dbFor(ctx, t).Table("incentives_commission_funding_allocations a").Select("a.*,l.original_operator_id AS source_operator_id,e.source_event_id").Joins("JOIN incentives_commission_funding_commands c ON c.id=a.command_id").Joins("JOIN incentives_commission_funding_lots l ON l.entry_id=a.entry_id").Joins("JOIN incentives_commission_entries ce ON ce.id=a.entry_id").Joins("JOIN incentives_confirmation_events e ON e.id=ce.event_id").Where("c.operator_id=? AND c.customer_id=? AND c.kind='order' AND c.reference=?", op, customer, order).Order("a.created_at,a.id").Scan(&rows).Error
	return rows, e
}
func (s *Service) CommissionSpendable(ctx context.Context, t *gorm.DB, op, customer string) (ledger.Decimal, error) {
	if e := s.requireCommissionUseAllowed(ctx, s.dbFor(ctx, t), op, customer); e != nil {
		if errors.Is(e, ErrBlocked) {
			return ledger.Zero(), nil
		}
		return ledger.Zero(), e
	}
	return s.commissionProvenanceSpendable(ctx, t, op, customer)
}
func (s *Service) commissionProvenanceSpendable(ctx context.Context, t *gorm.DB, op, customer string) (ledger.Decimal, error) {
	var lots []commissionLot
	if e := s.dbFor(ctx, t).Where("operator_id=? AND customer_id=?", op, customer).Find(&lots).Error; e != nil {
		return ledger.Zero(), e
	}
	amount := ledger.Zero()
	for _, lot := range lots {
		value, e := s.lotSpendable(s.dbFor(ctx, t), lot)
		if e != nil {
			return ledger.Zero(), e
		}
		amount = amount.Add(value)
	}
	return amount, nil
}

func (s *Service) requireCommissionUseAllowed(ctx context.Context, t *gorm.DB, op, customer string) error {
	instant, e := s.BusinessInstant(ctx, t)
	if e != nil {
		return e
	}
	state, e := s.stateAt(t, op, customer, instant.At)
	if e != nil {
		return e
	}
	if state.ReleaseBlocked {
		return ErrBlocked
	}
	return nil
}
