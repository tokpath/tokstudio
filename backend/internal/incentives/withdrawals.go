package incentives

import (
	"context"
	"math/big"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

func (s *Service) SetWithdrawalPolicy(ctx context.Context, tx *gorm.DB, actor, operator string, in WithdrawalPolicy, expectedVersion int64) (out WithdrawalPolicy, err error) {
	if !validID(operator) || expectedVersion < 0 || in.MinimumUSD.Sign() <= 0 || !in.MinimumUSD.Valid() || in.FlatFeeUSD.Sign() < 0 || !in.FlatFeeUSD.Valid() || !rateValid(in.FeeRate) || in.FeeRate.Cmp(ledger.FromInt64(1)) == 0 || in.MethodReference == "" {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, actor, operator, "withdrawals.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, operator); err != nil {
			return err
		}
		var old withdrawalPolicyRow
		q := tx.Where("operator_id=?", operator).Order("version DESC").Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if old.Version != expectedVersion {
			return ErrConflict
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		r := withdrawalPolicyRow{ID: uuid.NewString(), OperatorID: operator, Version: old.Version + 1, MinimumUSD: in.MinimumUSD, FlatFeeUSD: in.FlatFeeUSD, FeeRate: in.FeeRate, RequiresDualApproval: in.RequiresDualApproval, MethodReference: in.MethodReference, EffectiveAt: instant.At}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = WithdrawalPolicy(r)
		var before any
		if q.RowsAffected > 0 {
			before = WithdrawalPolicy(old)
		}
		return s.event(tx, operator, r.ID, "withdrawal.policy_changed", map[string]any{"actor_id": actor, "evidence": in.MethodReference, "before": before, "after": out})
	})
	return
}
func (s *Service) QuoteWithdrawal(ctx context.Context, tx *gorm.DB, operator string, amount ledger.Decimal) (WithdrawalQuote, error) {
	var policy withdrawalPolicyRow
	q := s.dbFor(ctx, tx).Where("operator_id=?", operator).Order("version DESC").Limit(1).Find(&policy)
	if q.Error != nil {
		return WithdrawalQuote{}, q.Error
	}
	if q.RowsAffected == 0 {
		return WithdrawalQuote{}, ErrNotReady
	}
	return quoteWithdrawal(WithdrawalPolicy(policy), amount)
}
func quoteWithdrawal(policy WithdrawalPolicy, amount ledger.Decimal) (WithdrawalQuote, error) {
	if amount.Sign() <= 0 || !amount.Valid() || amount.Cmp(policy.MinimumUSD) < 0 {
		return WithdrawalQuote{}, ErrInvalidInput
	}
	exact := new(big.Rat).Add(policy.FlatFeeUSD.Rat(), new(big.Rat).Mul(amount.Rat(), policy.FeeRate.Rat()))
	fee, remainder := ledger.Quantize(exact)
	net := amount.Sub(fee)
	if net.Sign() <= 0 {
		return WithdrawalQuote{}, ErrInvalidInput
	}
	return WithdrawalQuote{Policy: policy, AmountUSD: amount, FeeUSD: fee, NetUSD: net, FeeRemainder: remainder.RatString()}, nil
}
func (s *Service) RequestWithdrawal(ctx context.Context, tx *gorm.DB, in WithdrawalInput) (out Withdrawal, err error) {
	if !validID(in.OperatorID) || !validID(in.CustomerID) || !validID(in.PolicyID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.RecipientReference == "" {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		own, err := s.org.CurrentOwnership(ctx, tx, in.CustomerID)
		if err != nil {
			return err
		}
		if own.OperatorID != in.OperatorID {
			return ErrNotEligible
		}
		if _, err = s.org.LockOwnership(ctx, tx, own.CustomerID, own.OperatorID, own.Version); err != nil {
			return err
		}
		if err = s.org.RequireCustomerAction(ctx, tx, in.CustomerID, in.OperatorID, "withdrawal"); err != nil {
			return err
		}
		if err = s.org.RequireBusiness(ctx, tx, in.OperatorID, organization.Settlement); err != nil {
			return err
		}
		if err = s.LockBusiness(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		var old withdrawalRow
		q := tx.Where("operator_id=? AND customer_id=? AND business_key=?", in.OperatorID, in.CustomerID, in.BusinessKey).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		fp := fingerprint(in)
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			out = Withdrawal(old)
			return nil
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		if err = s.advance(ctx, tx, in.OperatorID, in.CustomerID, instant.At); err != nil {
			return err
		}
		state, err := s.stateAt(tx, in.OperatorID, in.CustomerID, instant.At)
		if err != nil {
			return err
		}
		if state.ReleaseBlocked {
			return ErrBlocked
		}
		quote, err := s.QuoteWithdrawal(ctx, tx, in.OperatorID, in.AmountUSD)
		if err != nil {
			return err
		}
		if quote.Policy.ID != in.PolicyID {
			return ErrConflict
		}
		account, err := s.account(ctx, tx, in.OperatorID, in.CustomerID, "commission:available")
		if err != nil {
			return err
		}
		id := uuid.NewString()
		if _, err = s.reserveCommissionUse(ctx, tx, in.OperatorID, in.CustomerID, "withdrawal", id, in.AmountUSD, nil); err != nil {
			return err
		}
		hold, err := s.ledger.Reserve(ctx, tx, ledger.Reservation{OperatorID: in.OperatorID, BusinessKey: "withdrawal:" + id, Purpose: "commission_withdrawal", ExpiresAt: instant.At.AddDate(10, 0, 0), Amounts: []ledger.Line{{AccountID: account.ID, Amount: in.AmountUSD}}})
		if err != nil {
			return err
		}
		r := withdrawalRow{ID: id, OperatorID: in.OperatorID, CustomerID: in.CustomerID, BusinessKey: in.BusinessKey, Fingerprint: fp, PolicyID: quote.Policy.ID, AmountUSD: quote.AmountUSD, FeeUSD: quote.FeeUSD, NetUSD: quote.NetUSD, FeeRemainder: quote.FeeRemainder, RecipientReference: in.RecipientReference, HoldID: hold.ID, Status: "requested", CreatedAt: instant.At}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = Withdrawal(r)
		return s.event(tx, in.OperatorID, r.ID, "withdrawal.requested", out)
	})
	return
}
func (s *Service) GetWithdrawal(ctx context.Context, tx *gorm.DB, customer, id string) (Withdrawal, error) {
	if !validID(customer) || !validID(id) {
		return Withdrawal{}, ErrInvalidInput
	}
	var r withdrawalRow
	err := s.dbFor(ctx, tx).First(&r, "id=? AND customer_id=?", id, customer).Error
	return Withdrawal(r), classify(err)
}
func (s *Service) withdrawalForStaff(ctx context.Context, tx *gorm.DB, actor, operator, id string) (withdrawalRow, error) {
	if err := s.org.RequirePermission(ctx, tx, actor, operator, "withdrawals.manage"); err != nil {
		return withdrawalRow{}, err
	}
	if err := s.LockBusiness(ctx, tx, operator); err != nil {
		return withdrawalRow{}, err
	}
	var r withdrawalRow
	err := tx.First(&r, "id=? AND operator_id=?", id, operator).Error
	return r, err
}
func (s *Service) ApproveWithdrawal(ctx context.Context, tx *gorm.DB, actor, operator, id string) (out Withdrawal, err error) {
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.Status == "approved" {
			out = Withdrawal(r)
			return nil
		}
		if r.Status != "requested" {
			return ErrWithdrawalState
		}
		r.Status = "approved"
		r.ApprovedBy = &actor
		if err = tx.Save(&r).Error; err != nil {
			return err
		}
		out = Withdrawal(r)
		return s.event(tx, operator, id, "withdrawal.approved", map[string]any{"actor_id": actor})
	})
	return
}
func (s *Service) BeginWithdrawalPayment(ctx context.Context, tx *gorm.DB, actor, operator, id string) (out Withdrawal, err error) {
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.Status == "paying" {
			out = Withdrawal(r)
			return nil
		}
		if r.Status != "approved" || r.ApprovedBy == nil {
			return ErrWithdrawalState
		}
		if err = s.RequireCommissionFundingExecution(ctx, tx, operator, r.CustomerID, "withdrawal", r.ID); err != nil {
			return err
		}
		var p withdrawalPolicyRow
		if err = tx.First(&p, "id=?", r.PolicyID).Error; err != nil {
			return err
		}
		if p.RequiresDualApproval && *r.ApprovedBy == actor {
			return organization.ErrForbidden
		}
		intent := uuid.NewString()
		r.Status = "paying"
		r.PaymentActorID = &actor
		r.PaymentIntentID = &intent
		if err = tx.Save(&r).Error; err != nil {
			return err
		}
		out = Withdrawal(r)
		return s.event(tx, operator, id, "withdrawal.payment_intent_recorded", map[string]any{"payment_intent_id": intent, "actor_id": actor})
	})
	return
}
func (s *Service) MarkWithdrawalUnknown(ctx context.Context, tx *gorm.DB, actor, operator, id, evidence string) error {
	if evidence == "" {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.Status != "paying" && r.Status != "unknown" {
			return ErrWithdrawalState
		}
		if r.Status == "unknown" && r.PaymentEvidence == evidence {
			return nil
		}
		before := Withdrawal(r)
		if e := tx.Model(&r).Updates(map[string]any{"status": "unknown", "payment_evidence": evidence}).Error; e != nil {
			return e
		}
		r.Status, r.PaymentEvidence = "unknown", evidence
		return s.event(tx, operator, id, "withdrawal.outcome_unknown", map[string]any{"actor_id": actor, "evidence": evidence, "before": before, "after": Withdrawal(r)})
	})
}
func (s *Service) VerifyWithdrawalPaid(ctx context.Context, tx *gorm.DB, actor, operator, id string, proof WithdrawalPaymentProof) (out Withdrawal, err error) {
	original, parseErr := ledger.ParseDecimal(proof.OriginalAmount)
	if proof.PaymentReference == "" || proof.Evidence == "" || proof.Currency == "" || proof.ValuationEvidence == "" || proof.PaidAt.IsZero() || parseErr != nil || original.Sign() <= 0 {
		return out, ErrInvalidInput
	}
	proof.PaidAt = proof.PaidAt.UTC()
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.Status == "paid" {
			if r.PaymentReference == nil || *r.PaymentReference != proof.PaymentReference || r.NetUSD.Cmp(proof.PaidUSD) != 0 {
				return ErrConflict
			}
			out = Withdrawal(r)
			return nil
		}
		if r.Status != "paying" && r.Status != "unknown" {
			return ErrWithdrawalState
		}
		if r.NetUSD.Cmp(proof.PaidUSD) != 0 {
			return ErrInvalidInput
		}
		before := Withdrawal(r)
		available, err := s.account(ctx, tx, operator, r.CustomerID, "commission:available")
		if err != nil {
			return err
		}
		payouts, err := s.account(ctx, tx, operator, operator, "control:commission_payouts")
		if err != nil {
			return err
		}
		lines := []ledger.Line{{AccountID: available.ID, Amount: r.AmountUSD.Neg()}, {AccountID: payouts.ID, Amount: r.NetUSD}}
		if r.FeeUSD.Sign() > 0 {
			fees, err := s.account(ctx, tx, operator, operator, "control:withdrawal_fees")
			if err != nil {
				return err
			}
			lines = append(lines, ledger.Line{AccountID: fees.ID, Amount: r.FeeUSD})
		}
		if _, err = s.ledger.Capture(ctx, tx, r.HoldID, ledger.Posting{OperatorID: operator, BusinessKey: "withdrawal-paid:" + r.ID, Kind: "commission_payout", SourceID: r.ID, ActorID: actor, Reason: proof.Evidence, Lines: lines}, nil); err != nil {
			return err
		}
		if err = s.resolveCommissionUse(ctx, tx, operator, r.CustomerID, "withdrawal", r.ID, true); err != nil {
			return err
		}
		r.Status = "paid"
		r.PaymentReference = &proof.PaymentReference
		r.PaymentEvidence = proof.Evidence
		r.PaidAt = &proof.PaidAt
		if err = tx.Save(&r).Error; err != nil {
			return err
		}
		out = Withdrawal(r)
		return s.event(tx, operator, id, "withdrawal.payment_verified", map[string]any{"actor_id": actor, "evidence": proof.Evidence, "before": before, "after": out, "proof": proof})
	})
	return
}
func (s *Service) CancelWithdrawal(ctx context.Context, tx *gorm.DB, customer, id string) error {
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		var r withdrawalRow
		if err := tx.First(&r, "id=? AND customer_id=?", id, customer).Error; err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, r.OperatorID); err != nil {
			return err
		}
		if err := tx.First(&r, "id=?", id).Error; err != nil {
			return err
		}
		if r.Status == "cancelled" {
			return nil
		}
		if r.Status != "requested" {
			return ErrWithdrawalState
		}
		if err := s.resolveCommissionUse(ctx, tx, r.OperatorID, r.CustomerID, "withdrawal", r.ID, false); err != nil {
			return err
		}
		if _, err := s.ledger.Release(ctx, tx, r.OperatorID, r.HoldID, "withdrawal-cancel:"+id); err != nil {
			return err
		}
		if err := tx.Model(&r).Update("status", "cancelled").Error; err != nil {
			return err
		}
		return s.event(tx, r.OperatorID, id, "withdrawal.cancelled", map[string]any{"customer_id": customer})
	})
}
func (s *Service) FailWithdrawal(ctx context.Context, tx *gorm.DB, actor, operator, id, evidence string, verifiedUnpaid bool) error {
	if !verifiedUnpaid || evidence == "" {
		return ErrWithdrawalState
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.Status == "failed" {
			return nil
		}
		if r.Status != "approved" && r.Status != "paying" && r.Status != "unknown" {
			return ErrWithdrawalState
		}
		if err = s.resolveCommissionUse(ctx, tx, operator, r.CustomerID, "withdrawal", r.ID, false); err != nil {
			return err
		}
		if _, err = s.ledger.Release(ctx, tx, operator, r.HoldID, "withdrawal-failed:"+id); err != nil {
			return err
		}
		if err = tx.Model(&r).Updates(map[string]any{"status": "failed", "payment_evidence": evidence}).Error; err != nil {
			return err
		}
		return s.event(tx, operator, id, "withdrawal.unpaid_verified", map[string]any{"evidence": evidence})
	})
}
