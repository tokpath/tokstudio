package incentives

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// AccrueGrantEvent must join the usage/expiry settlement transaction (or be
// replayed before any later incentive decision). Gift-only events are no-ops.
func (s *Service) AccrueGrantEvent(ctx context.Context, tx *gorm.DB, in GrantEventInput) error {
	if in.PaidMoney.IsZero() {
		return nil
	}
	if !validID(in.EventID) || !validID(in.OperatorID) || !validID(in.CustomerID) || !validID(in.OrderID) || (in.Kind != "usage" && in.Kind != "expiry") || in.PaidMoney.Sign() < 0 || !in.PaidMoney.Valid() || in.OccurredAt.IsZero() {
		return ErrInvalidInput
	}
	in.OccurredAt = in.OccurredAt.UTC().Truncate(time.Microsecond)
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.LockBusiness(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		var prior confirmationRow
		q := tx.Where("source_event_id=?", in.EventID).Limit(1).Find(&prior)
		if q.Error != nil {
			return q.Error
		}
		sourceCommand := in
		sourceCommand.BusinessSequence = 0
		fp := fingerprint(sourceCommand)
		if q.RowsAffected > 0 {
			if prior.Fingerprint != fp {
				sourceCommand.OperatorID = prior.OriginalOperatorID
				if prior.Fingerprint == fingerprint(sourceCommand) {
					return nil
				}
				return ErrConflict
			}
			return nil
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		occurred := in.OccurredAt
		sequence := instant.Sequence
		if in.BusinessSequence > 0 {
			sequence = in.BusinessSequence
		}
		if occurred.After(instant.At.Add(time.Second)) {
			return ErrInvalidInput
		}
		policy, err := s.policyForCustomerAt(tx, in.OperatorID, in.CustomerID, occurred)
		if err != nil {
			return err
		}
		var snapshot snapshotRow
		if err = tx.First(&snapshot, "order_id=?", in.OrderID).Error; err != nil {
			return err
		}
		if snapshot.OperatorID != in.OperatorID || snapshot.CustomerID != in.CustomerID || snapshot.Status != "fulfilled" {
			return ErrNotEligible
		}
		sourceCommand.OperatorID = policy.OperatorID
		fp = fingerprint(sourceCommand)
		e := confirmationRow{ID: uuid.NewString(), OriginalOperatorID: policy.OperatorID, SourceEventID: in.EventID, OperatorID: in.OperatorID, CustomerID: in.CustomerID, OrderID: in.OrderID, Kind: in.Kind, PaidMoney: in.PaidMoney, PolicyID: policy.ID, OccurredAt: occurred, SourceOccurredAt: in.OccurredAt, ConfirmAt: occurred.Add(time.Duration(policy.ConfirmationSeconds) * time.Second), Status: "pending", Fingerprint: fp, BusinessSequence: sequence}
		if err = tx.Create(&e).Error; err != nil {
			return err
		}
		if err = s.inheritCommissionOrderFlags(tx, e); err != nil {
			return err
		}
		if in.Kind == "usage" {
			for _, hop := range []struct {
				number   int
				customer *string
				rate     ledger.Decimal
			}{{1, snapshot.DirectCustomerID, snapshot.DirectRate.Sub(snapshot.ConcessionRate)}, {2, snapshot.IndirectCustomerID, snapshot.IndirectRate}} {
				if hop.customer == nil || hop.rate.IsZero() {
					continue
				}
				if hop.rate.Sign() < 0 {
					return ErrConflict
				}
				if _, err = s.eligibleKOL(ctx, tx, in.OperatorID, *hop.customer, occurred); err != nil {
					if errors.Is(err, ErrNotEligible) {
						continue
					}
					return err
				}
				amount, err := s.commissionAmount(tx, in.OperatorID, in.OrderID, *hop.customer, e.ID, hop.number, in.PaidMoney, hop.rate)
				if err != nil {
					return err
				}
				if amount.IsZero() {
					continue
				}
				entry := commissionRow{ID: uuid.NewString(), EventID: e.ID, OperatorID: in.OperatorID, OrderID: in.OrderID, BeneficiaryID: *hop.customer, Hop: hop.number, AmountMoney: amount, Status: "frozen", ConfirmAt: e.ConfirmAt, CreatedAt: in.OccurredAt}
				if err = tx.Create(&entry).Error; err != nil {
					return err
				}
				frozen, err := s.account(ctx, tx, in.OperatorID, *hop.customer, "commission:frozen")
				if err != nil {
					return err
				}
				control, err := s.account(ctx, tx, in.OperatorID, in.OperatorID, "control:commissions")
				if err != nil {
					return err
				}
				if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: in.OperatorID, BusinessKey: "commission-accrue:" + entry.ID, Kind: "commission_accrual", SourceID: e.ID, ActorID: "system:usage-settlement", Reason: "eligible original two-hop order commission", Lines: []ledger.Line{{AccountID: frozen.ID, Amount: amount}, {AccountID: control.ID, Amount: amount.Neg()}}}); err != nil {
					return err
				}
			}
		}
		if err = s.advance(ctx, tx, in.OperatorID, in.CustomerID, occurred); err != nil {
			return err
		}
		return s.event(tx, in.OperatorID, e.ID, "consumption.confirmation_recorded", ConfirmationEvent(e))
	})
}

type commissionCarry struct {
	OperatorID, OrderID, BeneficiaryID            string
	Hop                                           int
	BaseMoney, CreditedMoney, ReversedMoney, Rate ledger.Decimal
	RemainderRat                                  string
}

func (s *Service) commissionAmount(tx *gorm.DB, operator, order, customer, eventID string, hop int, base, rate ledger.Decimal) (ledger.Decimal, error) {
	var carry commissionCarry
	q := tx.Table("incentives_commission_carries").Where("order_id=? AND beneficiary_id=? AND hop=?", order, customer, hop).Limit(1).Find(&carry)
	if q.Error != nil {
		return ledger.Zero(), q.Error
	}
	if q.RowsAffected > 0 && carry.Rate.Cmp(rate) != 0 {
		return ledger.Zero(), ErrConflict
	}
	nextBase := carry.BaseMoney.Add(base)
	target, remainder := ledger.Quantize(new(big.Rat).Mul(nextBase.Rat(), rate.Rat()))
	amount := target.Sub(carry.CreditedMoney.Sub(carry.ReversedMoney))
	if amount.Sign() < 0 || !amount.Valid() || !nextBase.Valid() {
		return ledger.Zero(), ErrConflict
	}
	if err := tx.Exec(`INSERT INTO incentives_commission_bases(event_id,order_id,beneficiary_id,hop,base_money) VALUES(?,?,?,?,?)`, eventID, order, customer, hop, base).Error; err != nil {
		return ledger.Zero(), err
	}
	err := tx.Exec(`INSERT INTO incentives_commission_carries(operator_id,order_id,beneficiary_id,hop,base_money,credited_money,reversed_money,rate,remainder_rat) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(order_id,beneficiary_id,hop) DO UPDATE SET base_money=EXCLUDED.base_money,credited_money=EXCLUDED.credited_money,remainder_rat=EXCLUDED.remainder_rat`, operator, order, customer, hop, nextBase, carry.CreditedMoney.Add(amount), carry.ReversedMoney, rate, remainder.RatString()).Error
	return amount, err
}

func (s *Service) Advance(ctx context.Context, tx *gorm.DB, operator, customer string, asOf time.Time) (out Qualification, err error) {
	if !validID(operator) || !validID(customer) || asOf.IsZero() {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.LockBusiness(ctx, tx, operator); err != nil {
			return err
		}
		if err := s.advance(ctx, tx, operator, customer, asOf.UTC()); err != nil {
			return err
		}
		var err error
		out, err = s.ensureQualification(tx, operator, customer)
		return err
	})
	return
}
func (s *Service) GetQualification(ctx context.Context, tx *gorm.DB, operator, customer string) (Qualification, error) {
	instant, err := s.BusinessInstant(ctx, tx)
	if err != nil {
		return Qualification{}, err
	}
	return s.Advance(ctx, tx, operator, customer, instant.At)
}

func (s *Service) advance(ctx context.Context, tx *gorm.DB, operator, customer string, asOf time.Time) error {
	if err := s.LockBusiness(ctx, tx, operator); err != nil {
		return err
	}
	if _, err := s.ensureQualification(tx, operator, customer); err != nil {
		return err
	}
	var events []confirmationRow
	if err := tx.Where(`operator_id=? AND status='pending' AND confirm_at<=? AND (customer_id=? OR id IN (SELECT event_id FROM incentives_commission_entries WHERE operator_id=? AND beneficiary_id=? AND status='frozen'))`, operator, asOf, customer, operator, customer).Order("confirm_at,business_sequence").Find(&events).Error; err != nil {
		return err
	}
	for _, e := range events {
		q, err := s.ensureQualification(tx, operator, e.CustomerID)
		if err != nil {
			return err
		}
		next := q.ConfirmedMoney.Add(e.PaidMoney)
		if next.Sign() < 0 || !next.Valid() {
			return ErrConflict
		}
		if err = tx.Model(&qualificationRow{}).Where("operator_id=? AND customer_id=?", operator, e.CustomerID).Update("confirmed_money", next).Error; err != nil {
			return err
		}
		if err = tx.Model(&e).Update("status", "confirmed").Error; err != nil {
			return err
		}
		if err = s.maybeQualify(ctx, tx, operator, e.CustomerID, e.ConfirmAt); err != nil {
			return err
		}
		var commissions []commissionRow
		if err = tx.Where("event_id=? AND status='frozen'", e.ID).Order("beneficiary_id,hop").Find(&commissions).Error; err != nil {
			return err
		}
		for _, entry := range commissions {
			if err = s.releaseCommission(ctx, tx, CommissionEntry(entry), asOf); err != nil {
				return err
			}
		}
	}
	// A previously disputed release can mature after its source was confirmed.
	var entries []commissionRow
	if err := tx.Where(`operator_id=? AND beneficiary_id=? AND status='frozen' AND confirm_at<=? AND event_id IN (SELECT id FROM incentives_confirmation_events WHERE status='confirmed')`, operator, customer, asOf).Order("confirm_at,id").Find(&entries).Error; err != nil {
		return err
	}
	for _, e := range entries {
		if err := s.releaseCommission(ctx, tx, CommissionEntry(e), asOf); err != nil {
			return err
		}
	}
	if err := s.settleAvailableOffsets(ctx, tx, operator, customer); err != nil {
		return err
	}
	return s.maybeQualify(ctx, tx, operator, customer, asOf)
}
func (s *Service) maybeQualify(ctx context.Context, tx *gorm.DB, operator, customer string, at time.Time) error {
	q, err := s.ensureQualification(tx, operator, customer)
	if err != nil {
		return err
	}
	if q.AcquiredAt != nil {
		return nil
	}
	p, err := s.policyForCustomerAt(tx, operator, customer, at)
	if err != nil {
		return err
	}
	var knownPendingCorrections ledger.Decimal
	if err = tx.Model(&confirmationRow{}).Select("COALESCE(SUM(paid_money),0)").Where("operator_id=? AND customer_id=? AND kind='correction' AND status='pending' AND confirm_at<=? AND occurred_at<=?", operator, customer, at, at).Scan(&knownPendingCorrections).Error; err != nil {
		return err
	}
	var confirmedAtTime ledger.Decimal
	if err = tx.Model(&confirmationRow{}).Select("COALESCE(SUM(paid_money),0)").Where("operator_id=? AND customer_id=? AND status='confirmed' AND confirm_at<=? AND (kind<>'correction' OR occurred_at<=?)", operator, customer, at, at).Scan(&confirmedAtTime).Error; err != nil {
		return err
	}
	if confirmedAtTime.Add(knownPendingCorrections).Cmp(p.ThresholdMoney) < 0 {
		return nil
	}
	c, err := s.org.GetCustomer(ctx, tx, customer)
	if err != nil {
		return err
	}
	// Derive the logical qualification time from confirmed facts and the
	// applicable threshold version, rather than the worker's catch-up time.
	acquired := p.EffectiveAt
	if p.ThresholdMoney.Sign() > 0 {
		var events []confirmationRow
		if err := tx.Where("operator_id=? AND customer_id=? AND status='confirmed' AND confirm_at<=? AND (kind<>'correction' OR occurred_at<=?)", operator, customer, at, at).Order("confirm_at,business_sequence").Find(&events).Error; err != nil {
			return err
		}
		accumulated := ledger.Zero()
		found := false
		for _, event := range events {
			accumulated = accumulated.Add(event.PaidMoney)
			if accumulated.Cmp(p.ThresholdMoney) >= 0 {
				if event.ConfirmAt.After(acquired) {
					acquired = event.ConfirmAt
				}
				found = true
				break
			}
		}
		if !found {
			acquired = at
		}
	}
	if c.RegisteredAt.After(acquired) {
		acquired = c.RegisteredAt
	}
	if p.EffectiveAt.After(acquired) {
		acquired = p.EffectiveAt
	}
	if err = tx.Model(&qualificationRow{}).Where("operator_id=? AND customer_id=? AND acquired_at IS NULL", operator, customer).Updates(map[string]any{"acquired_at": acquired, "acquired_policy_id": p.ID}).Error; err != nil {
		return err
	}
	return s.event(tx, operator, customer, "qualification.acquired", map[string]any{"customer_id": customer, "policy_id": p.ID, "acquired_at": acquired})
}

func (s *Service) settleAvailableOffsets(ctx context.Context, tx *gorm.DB, operator, customer string) error {
	q, err := s.ensureQualification(tx, operator, customer)
	if err != nil {
		return err
	}
	if q.PendingOffsetMoney.IsZero() {
		return nil
	}
	account, err := s.account(ctx, tx, operator, customer, "commission:available")
	if err != nil {
		return err
	}
	amount := minimum(account.Available(), q.PendingOffsetMoney)
	spendable, err := s.commissionProvenanceSpendable(ctx, tx, operator, customer)
	if err != nil {
		return err
	}
	amount = minimum(amount, spendable)
	if amount.Sign() <= 0 {
		return nil
	}
	if err = s.consumeCommissionCash(ctx, tx, operator, customer, "offset:"+uuid.NewString(), amount); err != nil {
		return err
	}
	control, err := s.account(ctx, tx, operator, operator, "control:commission_recoveries")
	if err != nil {
		return err
	}
	instant, err := s.BusinessInstant(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: operator, BusinessKey: "commission-offset:" + customer + ":" + uuid.NewString(), Kind: "commission_recovery", SourceID: customer, ActorID: "system:recovery", Reason: "pending commission correction offset", Lines: []ledger.Line{{AccountID: account.ID, Amount: amount.Neg()}, {AccountID: control.ID, Amount: amount}}}); err != nil {
		return err
	}
	if err = s.consumeOffsets(tx, operator, customer, amount); err != nil {
		return err
	}
	if err = tx.Model(&qualificationRow{}).Where("operator_id=? AND customer_id=?", operator, customer).Update("pending_offset_money", q.PendingOffsetMoney.Sub(amount)).Error; err != nil {
		return err
	}
	return s.event(tx, operator, customer, "commission.pending_offset_recovered", map[string]any{"amount_money": amount, "at": instant.At})
}

func (s *Service) releaseCommission(ctx context.Context, tx *gorm.DB, e CommissionEntry, asOf time.Time) error {
	instant, err := s.BusinessInstant(ctx, tx)
	if err != nil {
		return err
	}
	state, err := s.stateAt(tx, e.OperatorID, e.BeneficiaryID, instant.At)
	if err != nil {
		return err
	}
	if state.ReleaseBlocked {
		return nil
	}
	amount := e.AmountMoney.Sub(e.ReversedMoney)
	if amount.IsZero() {
		return tx.Model(&commissionRow{}).Where("id=?", e.ID).Update("status", "reversed").Error
	}
	q, err := s.ensureQualification(tx, e.OperatorID, e.BeneficiaryID)
	if err != nil {
		return err
	}
	if err = s.fundingLot(ctx, tx, e, amount); err != nil {
		return err
	}
	var releasedLot commissionLot
	if err = tx.First(&releasedLot, "entry_id=?", e.ID).Error; err != nil {
		return err
	}
	spendable, err := s.lotSpendable(tx, releasedLot)
	if err != nil {
		return err
	}
	offset := minimum(q.PendingOffsetMoney, spendable)
	available := amount.Sub(offset)
	frozen, err := s.account(ctx, tx, e.OperatorID, e.BeneficiaryID, "commission:frozen")
	if err != nil {
		return err
	}
	lines := []ledger.Line{{AccountID: frozen.ID, Amount: amount.Neg()}}
	if available.Sign() > 0 {
		a, err := s.account(ctx, tx, e.OperatorID, e.BeneficiaryID, "commission:available")
		if err != nil {
			return err
		}
		lines = append(lines, ledger.Line{AccountID: a.ID, Amount: available})
	}
	if offset.Sign() > 0 {
		reference := "release-offset:" + e.ID
		if _, err = s.reserveCommissionUse(ctx, tx, e.OperatorID, e.BeneficiaryID, "recovery", reference, offset, &e.ID); err != nil {
			return err
		}
		if err = s.resolveCommissionUse(ctx, tx, e.OperatorID, e.BeneficiaryID, "recovery", reference, true); err != nil {
			return err
		}
		recoveries, err := s.account(ctx, tx, e.OperatorID, e.OperatorID, "control:commission_recoveries")
		if err != nil {
			return err
		}
		lines = append(lines, ledger.Line{AccountID: recoveries.ID, Amount: offset})
		if err = s.consumeOffsets(tx, e.OperatorID, e.BeneficiaryID, offset); err != nil {
			return err
		}
		if err = tx.Model(&qualificationRow{}).Where("operator_id=? AND customer_id=?", e.OperatorID, e.BeneficiaryID).Update("pending_offset_money", q.PendingOffsetMoney.Sub(offset)).Error; err != nil {
			return err
		}
	}
	if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: e.OperatorID, BusinessKey: "commission-release:" + e.ID, Kind: "commission_release", SourceID: e.ID, ActorID: "system:confirmation", Reason: "confirmed commission; pending recovery settled first", Lines: lines}); err != nil {
		return err
	}
	return tx.Model(&commissionRow{}).Where("id=?", e.ID).Updates(map[string]any{"status": "available", "offset_money": offset}).Error
}
func (s *Service) consumeOffsets(tx *gorm.DB, operator, customer string, amount ledger.Decimal) error {
	var rows []struct {
		ID                          string
		AmountMoney, RecoveredMoney ledger.Decimal
	}
	if err := tx.Table("incentives_offsets").Where("operator_id=? AND customer_id=? AND recovered_money<amount_money", operator, customer).Order("created_at,id").Find(&rows).Error; err != nil {
		return err
	}
	remaining := amount
	for _, r := range rows {
		take := minimum(r.AmountMoney.Sub(r.RecoveredMoney), remaining)
		if take.Sign() > 0 {
			if err := tx.Table("incentives_offsets").Where("id=?", r.ID).Update("recovered_money", r.RecoveredMoney.Add(take)).Error; err != nil {
				return err
			}
			remaining = remaining.Sub(take)
		}
		if remaining.IsZero() {
			break
		}
	}
	if !remaining.IsZero() {
		return ErrConflict
	}
	return nil
}

func (s *Service) SetEventDispute(ctx context.Context, tx *gorm.DB, actor, operator, sourceEventID, evidence string, disputed bool) error {
	return s.setEventDisputeReference(ctx, tx, actor, operator, sourceEventID, "manual", evidence, disputed)
}
func (s *Service) CommissionEntries(ctx context.Context, tx *gorm.DB, operator, customer string) ([]CommissionEntry, error) {
	var rows []commissionRow
	if err := s.dbFor(ctx, tx).Where("operator_id=? AND beneficiary_id=?", operator, customer).Order("created_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]CommissionEntry, len(rows))
	for i, r := range rows {
		out[i] = CommissionEntry(r)
	}
	return out, nil
}
