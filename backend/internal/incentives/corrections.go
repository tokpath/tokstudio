package incentives

import (
	"context"
	"math/big"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

// ReverseEvent corrects a proven source error, preserving the original facts.
// OriginalEventID is the ledger source event ID, not an internal confirmation ID.
// It also corrects sub-atom commission carry across this order's eligible bases.
func (s *Service) ReverseEvent(ctx context.Context, tx *gorm.DB, in CorrectionInput) error {
	if !validID(in.OperatorID) || !validID(in.OriginalEventID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.Evidence == "" || in.AmountMoney.Sign() <= 0 || !in.AmountMoney.Valid() {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, "refunds.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		var original confirmationRow
		if err := tx.First(&original, "operator_id=? AND source_event_id=? AND kind IN ('usage','expiry')", in.OperatorID, in.OriginalEventID).Error; err != nil {
			return err
		}
		var prior struct {
			OriginalEventID, Evidence string
			AmountMoney               ledger.Decimal
		}
		q := tx.Table("incentives_corrections").Where("operator_id=? AND original_event_id=? AND business_key=?", in.OperatorID, original.ID, in.BusinessKey).Limit(1).Find(&prior)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if prior.OriginalEventID != original.ID || prior.AmountMoney.Cmp(in.AmountMoney) != 0 || prior.Evidence != in.Evidence {
				return ErrConflict
			}
			return nil
		}
		var corrected ledger.Decimal
		if err := tx.Table("incentives_corrections").Select("COALESCE(SUM(amount_money),0)").Where("original_event_id=?", original.ID).Scan(&corrected).Error; err != nil {
			return err
		}
		if corrected.Add(in.AmountMoney).Cmp(original.PaidMoney) > 0 {
			return ErrConflict
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		correctionID := uuid.NewString()
		if err = tx.Exec(`INSERT INTO incentives_corrections(id,operator_id,original_event_id,business_key,amount_money,evidence,created_at) VALUES(?,?,?,?,?,?,?)`, correctionID, in.OperatorID, original.ID, in.BusinessKey, in.AmountMoney, in.Evidence, instant.At).Error; err != nil {
			return err
		}
		status := "pending"
		if original.Status == "disputed" {
			status = "disputed"
		}
		negative := confirmationRow{ID: uuid.NewString(), SourceEventID: correctionID, OperatorID: in.OperatorID, CustomerID: original.CustomerID, OrderID: original.OrderID, Kind: "correction", PaidMoney: in.AmountMoney.Neg(), PolicyID: original.PolicyID, OccurredAt: instant.At, SourceOccurredAt: instant.At, ConfirmAt: original.ConfirmAt, Status: status, Fingerprint: fingerprint(in), BusinessSequence: instant.Sequence, OriginalEventID: &original.ID}
		if err = tx.Create(&negative).Error; err != nil {
			return err
		}
		var bases []struct {
			BeneficiaryID string
			Hop           int
		}
		if err = tx.Table("incentives_commission_bases").Where("event_id=?", original.ID).Find(&bases).Error; err != nil {
			return err
		}
		for _, base := range bases {
			var carry commissionCarry
			if err = tx.Table("incentives_commission_carries").Where("order_id=? AND beneficiary_id=? AND hop=?", original.OrderID, base.BeneficiaryID, base.Hop).First(&carry).Error; err != nil {
				return err
			}
			nextBase := carry.BaseMoney.Sub(in.AmountMoney)
			if nextBase.Sign() < 0 {
				return ErrConflict
			}
			target, remainder := ledger.Quantize(new(big.Rat).Mul(nextBase.Rat(), carry.Rate.Rat()))
			toReverse := carry.CreditedMoney.Sub(carry.ReversedMoney).Sub(target)
			if toReverse.Sign() < 0 {
				return ErrConflict
			}
			remaining := toReverse
			var entries []commissionRow
			if err = tx.Where("order_id=? AND beneficiary_id=? AND hop=? AND reversed_money<amount_money", original.OrderID, base.BeneficiaryID, base.Hop).Order("created_at,id").Find(&entries).Error; err != nil {
				return err
			}
			for _, entry := range entries {
				take := minimum(entry.AmountMoney.Sub(entry.ReversedMoney), remaining)
				if take.Sign() > 0 {
					if err = s.reverseEntry(ctx, tx, CommissionEntry(entry), take, correctionID, in.ActorStaffID, in.Evidence); err != nil {
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
			if err = tx.Table("incentives_commission_carries").Where("order_id=? AND beneficiary_id=? AND hop=?", original.OrderID, base.BeneficiaryID, base.Hop).Updates(map[string]any{"base_money": nextBase, "reversed_money": carry.ReversedMoney.Add(toReverse), "remainder_rat": remainder.RatString()}).Error; err != nil {
				return err
			}
		}
		if err = s.advance(ctx, tx, in.OperatorID, original.CustomerID, instant.At); err != nil {
			return err
		}
		return s.event(tx, in.OperatorID, original.ID, "consumption.corrected", in)
	})
}
func (s *Service) reverseEntry(ctx context.Context, tx *gorm.DB, e CommissionEntry, amount ledger.Decimal, correctionID, actor, evidence string) error {
	if amount.Sign() <= 0 || amount.Cmp(e.AmountMoney.Sub(e.ReversedMoney)) > 0 {
		return ErrConflict
	}
	purpose := "commission:available"
	if e.Status == "frozen" {
		purpose = "commission:frozen"
	}
	account, err := s.account(ctx, tx, e.OperatorID, e.BeneficiaryID, purpose)
	if err != nil {
		return err
	}
	take := minimum(account.Available(), amount)
	if e.Status != "frozen" {
		spendable, e := s.commissionProvenanceSpendable(ctx, tx, e.OperatorID, e.BeneficiaryID)
		if e != nil {
			return e
		}
		take = minimum(take, spendable)
	}
	if e.Status == "frozen" && take.Cmp(amount) != 0 {
		return ErrConflict
	}
	pending := amount.Sub(take)
	if take.Sign() > 0 {
		if e.Status != "frozen" {
			if err = s.consumeCorrectedCommission(ctx, tx, e, "correction:"+correctionID+":"+e.ID, take); err != nil {
				return err
			}
		}
		control, err := s.account(ctx, tx, e.OperatorID, e.OperatorID, "control:commissions")
		if err != nil {
			return err
		}
		if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: e.OperatorID, BusinessKey: "commission-reversal:" + correctionID + ":" + e.ID, Kind: "commission_correction", SourceID: e.ID, ActorID: actor, Reason: evidence, Lines: []ledger.Line{{AccountID: account.ID, Amount: take.Neg()}, {AccountID: control.ID, Amount: take}}}); err != nil {
			return err
		}
	}
	if pending.Sign() > 0 {
		q, err := s.ensureQualification(tx, e.OperatorID, e.BeneficiaryID)
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO incentives_offsets(id,operator_id,customer_id,source_reference,amount_money,created_at) VALUES(?,?,?,?,?,clock_timestamp())`, uuid.NewString(), e.OperatorID, e.BeneficiaryID, correctionID+":"+e.ID, pending).Error; err != nil {
			return err
		}
		if err = tx.Model(&qualificationRow{}).Where("operator_id=? AND customer_id=?", e.OperatorID, e.BeneficiaryID).Update("pending_offset_money", q.PendingOffsetMoney.Add(pending)).Error; err != nil {
			return err
		}
	}
	status := e.Status
	reversed := e.ReversedMoney.Add(amount)
	if reversed.Cmp(e.AmountMoney) == 0 {
		status = "reversed"
	}
	return tx.Model(&commissionRow{}).Where("id=?", e.ID).Updates(map[string]any{"reversed_money": reversed, "status": status}).Error
}
