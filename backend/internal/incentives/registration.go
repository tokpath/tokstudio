package incentives

import (
	"context"
	"strings"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"gorm.io/gorm"
)

// PrepareRegistrationPromise receives the intent returned by organization,
// reserves the maximum advertised reward before emailing/advertising it, and
// binds the promise to that exact final attribution. It is not a public DTO API.
func (s *Service) PrepareRegistrationPromise(ctx context.Context, tx *gorm.DB, intent organization.RegistrationIntent) (out RegistrationPromise, err error) {
	if !validID(intent.ID) || !validID(intent.OperatorID) || intent.Purpose != "registration" || (intent.Status != "pending" && intent.Status != "verified") {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.LockBusiness(ctx, tx, intent.OperatorID); err != nil {
			return err
		}
		var old promiseRow
		q := tx.Where("intent_id=?", intent.ID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if old.OperatorID != intent.OperatorID || !sameID(old.InviterID, intent.InviterID) {
				return ErrConflict
			}
			out = RegistrationPromise(old)
			return nil
		}
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		if !intent.ExpiresAt.After(instant.At) {
			return ErrRewardPromise
		}
		p, err := s.policyAt(tx, intent.OperatorID, instant.At)
		if err != nil {
			return err
		}
		inviterReward := p.InviterATG
		if intent.InviterID == nil {
			inviterReward = ledger.Zero()
		}
		r := promiseRow{IntentID: intent.ID, OperatorID: intent.OperatorID, InviterID: intent.InviterID, PolicyID: p.ID, InviteeATG: p.InviteeATG, InviterATG: inviterReward, RewardValiditySeconds: p.RewardValiditySeconds, ExpiresAt: intent.ExpiresAt, Status: "reserved", CreatedAt: instant.At}
		if r.InviteeATG.Add(r.InviterATG).Sign() > 0 {
			if s.rewardCoverage == nil {
				return ErrNotReady
			}
			commitment, err := s.rewardCoverage.ReserveRegistration(ctx, tx, RegistrationPromise(r))
			if err != nil {
				return err
			}
			if commitment.Reference == "" || commitment.Version < 1 {
				return ErrNotReady
			}
			r.CoverageReference = commitment.Reference
			r.CoverageVersion = commitment.Version
		}
		if err = tx.Create(&r).Error; err != nil {
			return err
		}
		out = RegistrationPromise(r)
		return s.event(tx, r.OperatorID, r.IntentID, "registration.reward_promised", out)
	})
	return
}
func sameID(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// OnRegistered implements organization.RegistrationHook. A delivery failure is
// isolated by a savepoint: the fulfilled registration and its funded, durable
// pending reward obligation both survive and can be retried by a worker.
func (s *Service) OnRegistered(ctx context.Context, tx *gorm.DB, result organization.RegistrationResult) error {
	if tx == nil {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		operator, intentID := result.Ownership.OperatorID, result.Ownership.EvidenceReference
		if err := s.LockBusiness(ctx, tx, operator); err != nil {
			return err
		}
		var r promiseRow
		q := tx.Where("intent_id=?", intentID).Limit(1).Find(&r)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			policy, err := s.policyAt(tx, operator, result.Customer.RegisteredAt)
			if err != nil {
				return err
			}
			if policy.InviteeATG.Sign() > 0 || (result.InviterID != nil && policy.InviterATG.Sign() > 0) {
				return ErrRewardPromise
			}
			return nil
		}
		if r.OperatorID != operator || !sameID(r.InviterID, result.InviterID) {
			return ErrConflict
		}
		if r.Status == "delivered" {
			return nil
		}
		if r.Status != "reserved" {
			return ErrRewardPromise
		}
		actual := r.InviterATG
		if r.InviterID != nil {
			if err := s.advance(ctx, tx, operator, *r.InviterID, result.Customer.RegisteredAt); err != nil {
				return err
			}
			qualification, err := s.ensureQualification(tx, operator, *r.InviterID)
			if err != nil {
				return err
			}
			if qualification.AcquiredAt != nil && !qualification.AcquiredAt.After(result.Customer.RegisteredAt) {
				actual = ledger.Zero()
			}
		}
		r.CustomerID = &result.Customer.ID
		r.RegisteredAt = &result.Customer.RegisteredAt
		r.ActualInviterATG = actual
		r.Status = "pending_delivery"
		if err := tx.Model(&promiseRow{}).Where("intent_id=?", intentID).Updates(map[string]any{"customer_id": r.CustomerID, "registered_at": r.RegisteredAt, "actual_inviter_atg": actual, "status": "pending_delivery"}).Error; err != nil {
			return err
		}
		point := "incentives_reward_" + strings.ReplaceAll(intentID, "-", "")
		if err := tx.SavePoint(point).Error; err != nil {
			return err
		}
		if err := s.deliverRegistration(ctx, tx, RegistrationPromise(r)); err != nil {
			if rollback := tx.RollbackTo(point).Error; rollback != nil {
				return rollback
			}
			message := err.Error()
			if len(message) > 2000 {
				message = message[:2000]
			}
			if update := tx.Model(&promiseRow{}).Where("intent_id=?", intentID).Update("last_error", message).Error; update != nil {
				return update
			}
			return s.event(tx, operator, intentID, "registration.reward_delivery_pending", map[string]any{"customer_id": result.Customer.ID})
		}
		return nil
	})
}
func (s *Service) DeliverRegistrationRewards(ctx context.Context, tx *gorm.DB, intentID string) error {
	if !validID(intentID) {
		return ErrInvalidInput
	}
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		var r promiseRow
		if err := tx.First(&r, "intent_id=?", intentID).Error; err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, tx, r.OperatorID); err != nil {
			return err
		}
		if err := tx.First(&r, "intent_id=?", intentID).Error; err != nil {
			return err
		}
		if r.Status == "delivered" {
			return nil
		}
		if r.Status != "pending_delivery" {
			return ErrRewardPromise
		}
		return s.deliverRegistration(ctx, tx, RegistrationPromise(r))
	})
}
func (s *Service) deliverRegistration(ctx context.Context, tx *gorm.DB, p RegistrationPromise) error {
	if p.CustomerID == nil || p.RegisteredAt == nil {
		return ErrRewardPromise
	}
	end := p.RegisteredAt.Add(time.Duration(p.RewardValiditySeconds) * time.Second)
	for _, reward := range []struct {
		customer *string
		amount   ledger.Decimal
	}{{p.CustomerID, p.InviteeATG}, {p.InviterID, p.ActualInviterATG}} {
		if reward.customer == nil || reward.amount.IsZero() {
			continue
		}
		ownership, err := s.org.CurrentOwnership(ctx, tx, *reward.customer)
		if err != nil {
			return err
		}
		if ownership.OperatorID != p.OperatorID {
			return organization.ErrStaleOwnership
		}
		grant, err := s.ledger.IssueGrant(ctx, tx, ledger.GrantInput{OperatorID: p.OperatorID, CustomerID: *reward.customer, OrderID: "", BusinessKey: "registration:" + p.IntentID + ":" + *reward.customer, ActorID: "system:registration", Reason: "funded formal registration reward", PaidMoney: ledger.Zero(), Sources: []ledger.GrantSource{{Kind: "registration_gift", Amount: reward.amount}}, EffectiveAt: *p.RegisteredAt, ExpiresAt: &end})
		if err != nil {
			return err
		}
		if err = tx.Exec(`INSERT INTO incentives_registration_deliveries(intent_id,customer_id,grant_id) VALUES(?,?,?) ON CONFLICT DO NOTHING`, p.IntentID, *reward.customer, grant.ID).Error; err != nil {
			return err
		}
	}
	if p.CoverageReference != "" {
		if s.rewardCoverage == nil {
			return ErrNotReady
		}
		if err := s.rewardCoverage.FulfillRegistration(ctx, tx, p, p.InviteeATG.Add(p.ActualInviterATG)); err != nil {
			return err
		}
	}
	if err := tx.Model(&promiseRow{}).Where("intent_id=?", p.IntentID).Updates(map[string]any{"status": "delivered", "last_error": ""}).Error; err != nil {
		return err
	}
	return s.event(tx, p.OperatorID, p.IntentID, "registration.rewards_delivered", map[string]any{"customer_id": p.CustomerID, "invitee_atg": p.InviteeATG, "inviter_atg": p.ActualInviterATG, "expires_at": end})
}
func (s *Service) ReleaseExpiredRegistrationPromises(ctx context.Context, tx *gorm.DB, limit int) (count int, err error) {
	if limit < 1 || limit > 1000 {
		return 0, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		instant, err := s.BusinessInstant(ctx, tx)
		if err != nil {
			return err
		}
		var rows []promiseRow
		if err = tx.Where("status='reserved' AND expires_at<=?", instant.At).Order("expires_at,intent_id").Limit(limit).Find(&rows).Error; err != nil {
			return err
		}
		for _, p := range rows {
			if err = s.LockBusiness(ctx, tx, p.OperatorID); err != nil {
				return err
			}
			var current promiseRow
			if err = tx.First(&current, "intent_id=?", p.IntentID).Error; err != nil {
				return err
			}
			if current.Status != "reserved" {
				continue
			}
			if p.CoverageReference != "" {
				if s.rewardCoverage == nil {
					return ErrNotReady
				}
				if err = s.rewardCoverage.ReleaseRegistration(ctx, tx, RegistrationPromise(p)); err != nil {
					return err
				}
			}
			if err = tx.Model(&promiseRow{}).Where("intent_id=?", p.IntentID).Update("status", "released").Error; err != nil {
				return err
			}
			count++
		}
		return nil
	})
	if err != nil {
		count = 0
	}
	return
}
