package incentives

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/organization"
	"golang.org/x/text/currency"
	"gorm.io/gorm"
)

const commissionWithdrawalTerms = "提现手续费由申请人承担；经营方单人审批后付款。"

func (s *Service) SetCommissionWithdrawalMinimum(ctx context.Context, tx *gorm.DB, actor, operator string, minimum ledger.Decimal) (out WithdrawalPolicy, err error) {
	if !minimum.Valid() || minimum.Sign() <= 0 {
		return out, ErrInvalidInput
	}
	// One settlement unit is the smallest amount an operator may publish.
	// Smaller values survive the form, then disappear when a quote is cut to
	// the currency's payable unit.
	if minimum.Cmp(ledger.FromInt64(1)) < 0 {
		return out, ErrWithdrawalMinimum
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, t, actor, operator, "marketing.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, t, operator); err != nil {
			return err
		}
		if err := t.Exec("INSERT INTO incentives_withdrawal_settings(operator_id,minimum_money) VALUES(?,?) ON CONFLICT(operator_id) DO UPDATE SET minimum_money=excluded.minimum_money", operator, minimum).Error; err != nil {
			return err
		}
		out.MinimumMoney = minimum
		out.OperatorID = operator
		return s.event(t, operator, operator, "withdrawal.minimum_changed", map[string]any{"actor_id": actor, "minimum_money": minimum})
	})
	return
}

func (s *Service) SetWithdrawalPolicy(ctx context.Context, tx *gorm.DB, actor, operator string, in WithdrawalPolicy, expectedVersion int64) (out WithdrawalPolicy, err error) {
	profile, knownKind := FindPayoutProfile(in.PayoutKind)
	if !validID(operator) || expectedVersion < 0 || !knownKind || !in.MinimumMoney.Valid() || in.MinimumMoney.Sign() <= 0 || !in.FlatFeeMoney.Valid() || in.FlatFeeMoney.Sign() < 0 || !rateValid(in.FeeRate) || in.FeeRate.Cmp(ledger.FromInt64(1)) == 0 || in.FlatFeeMoney.Sign() > 0 && in.FeeRate.Sign() > 0 {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, t, actor, operator, "withdrawals.manage"); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, t, operator); err != nil {
			return err
		}
		var latest withdrawalPolicyRow
		if err := t.Where("operator_id=?", operator).Order("version DESC").Limit(1).Find(&latest).Error; err != nil {
			return err
		}
		// An omitted method selects the operator's initial method, convenient
		// for bootstrap. The management UI always edits an explicit method.
		if in.MethodID == "" {
			in.Enabled = true
			in.MethodID = latest.MethodID
			if in.MethodID == "" {
				in.MethodID = uuid.NewString()
			}
		}
		if !validID(in.MethodID) {
			return ErrInvalidInput
		}
		var previous withdrawalPolicyRow
		if err := t.Where("operator_id=? AND method_id=?", operator, in.MethodID).Order("version DESC").Limit(1).Find(&previous).Error; err != nil {
			return err
		}
		if previous.Version != expectedVersion {
			return ErrConflict
		}
		in.RequiresDualApproval = false
		if in.MethodName == "" {
			in.MethodName = profile.Label
		}
		if len(in.MethodName) > 100 {
			return ErrInvalidInput
		}
		if in.MethodReference == "" {
			in.MethodReference = commissionWithdrawalTerms
		}
		instant, err := s.BusinessInstant(ctx, t)
		if err != nil {
			return err
		}
		r := withdrawalPolicyRow(in)
		r.ID = uuid.NewString()
		r.OperatorID = operator
		r.Version = latest.Version + 1
		r.EffectiveAt = instant.At
		if err = t.Create(&r).Error; err != nil {
			return err
		}
		if err = t.Exec("INSERT INTO incentives_withdrawal_settings(operator_id,minimum_money) VALUES(?,?) ON CONFLICT(operator_id) DO NOTHING", operator, in.MinimumMoney).Error; err != nil {
			return err
		}
		out = WithdrawalPolicy(r)
		return s.event(t, operator, r.ID, "withdrawal.method_changed", map[string]any{"actor_id": actor, "before": previous, "after": out})
	})
	return
}

func (s *Service) WithdrawalMethods(ctx context.Context, tx *gorm.DB, operator string, enabledOnly bool) ([]WithdrawalPolicy, error) {
	monetary, err := ledger.GetMonetary(ctx, s.dbFor(ctx, tx))
	if err != nil {
		return nil, err
	}
	var rows []withdrawalPolicyRow
	if err := s.dbFor(ctx, tx).Raw("SELECT * FROM (SELECT DISTINCT ON(method_id) * FROM incentives_withdrawal_policies WHERE operator_id=? ORDER BY method_id,version DESC) methods ORDER BY method_name,method_id", operator).Scan(&rows).Error; err != nil {
		return nil, err
	}
	var minimum struct{ MinimumMoney ledger.Decimal }
	if err := s.dbFor(ctx, tx).Table("incentives_withdrawal_settings").Where("operator_id=?", operator).Limit(1).Find(&minimum).Error; err != nil {
		return nil, err
	}
	out := []WithdrawalPolicy{}
	for _, r := range rows {
		if enabledOnly && !r.Enabled {
			continue
		}
		p := WithdrawalPolicy(r)
		p.Currency = monetary.Currency
		if minimum.MinimumMoney.Sign() > 0 {
			p.MinimumMoney = minimum.MinimumMoney
		}
		out = append(out, p)
	}
	return out, nil
}
func (s *Service) QuoteWithdrawal(ctx context.Context, tx *gorm.DB, operator string, amount ledger.Decimal, methodID ...string) (WithdrawalQuote, error) {
	methods, err := s.WithdrawalMethods(ctx, tx, operator, true)
	if err != nil {
		return WithdrawalQuote{}, err
	}
	if len(methods) == 0 {
		return WithdrawalQuote{}, ErrNotReady
	}
	if len(methodID) > 0 && methodID[0] != "" {
		for _, m := range methods {
			if m.MethodID == methodID[0] {
				return quoteWithdrawal(m, amount)
			}
		}
		return WithdrawalQuote{}, ErrNotReady
	}
	if len(methods) != 1 {
		return WithdrawalQuote{}, ErrInvalidInput
	}
	return quoteWithdrawal(methods[0], amount)
}
func quoteWithdrawal(policy WithdrawalPolicy, amount ledger.Decimal) (WithdrawalQuote, error) {
	if !amount.Valid() || amount.Sign() <= 0 {
		return WithdrawalQuote{}, ErrInvalidInput
	}
	var scale *big.Int
	if policy.Currency != "" {
		unit, err := currency.ParseISO(policy.Currency)
		if err != nil {
			return WithdrawalQuote{}, ErrInvalidInput
		}
		digits, _ := currency.Standard.Rounding(unit)
		scale = new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(digits)), nil)
		// Only the payable part is requested; sub-unit wallet balances remain
		// available to the customer. Preview precedes the wallet reservation.
		raw := new(big.Rat).Mul(amount.Rat(), new(big.Rat).SetInt(scale))
		units := new(big.Int).Quo(raw.Num(), raw.Denom())
		amount, _ = ledger.Quantize(new(big.Rat).SetFrac(units, scale))
	}
	if amount.Sign() <= 0 || !amount.Valid() || amount.Cmp(policy.MinimumMoney) < 0 {
		return WithdrawalQuote{}, ErrInvalidInput
	}
	exact := new(big.Rat).Add(policy.FlatFeeMoney.Rat(), new(big.Rat).Mul(amount.Rat(), policy.FeeRate.Rat()))
	fee, remainder := ledger.Quantize(exact)
	if scale != nil {
		raw := new(big.Rat).Mul(exact, new(big.Rat).SetInt(scale))
		twice := new(big.Int).Mul(raw.Num(), big.NewInt(2))
		twice.Add(twice, raw.Denom())
		units := new(big.Int).Quo(twice, new(big.Int).Mul(raw.Denom(), big.NewInt(2)))
		fee, _ = ledger.Quantize(new(big.Rat).SetFrac(units, scale))
		remainder = new(big.Rat).Sub(exact, fee.Rat())
	}
	net := amount.Sub(fee)
	if net.Sign() <= 0 {
		return WithdrawalQuote{}, ErrInvalidInput
	}
	return WithdrawalQuote{Policy: policy, AmountMoney: amount, FeeMoney: fee, NetMoney: net, FeeRemainder: remainder.RatString()}, nil
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
		var selected withdrawalPolicyRow
		if err := tx.First(&selected, "id=? AND operator_id=?", in.PolicyID, in.OperatorID).Error; err != nil {
			return err
		}
		quote, err := s.QuoteWithdrawal(ctx, tx, in.OperatorID, in.AmountMoney, selected.MethodID)
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
		if _, err = s.reserveCommissionUse(ctx, tx, in.OperatorID, in.CustomerID, "withdrawal", id, quote.AmountMoney, nil); err != nil {
			return err
		}
		hold, err := s.ledger.Reserve(ctx, tx, ledger.Reservation{OperatorID: in.OperatorID, BusinessKey: "withdrawal:" + id, Purpose: "commission_withdrawal", ExpiresAt: instant.At.AddDate(10, 0, 0), Amounts: []ledger.Line{{AccountID: account.ID, Amount: quote.AmountMoney}}})
		if err != nil {
			return err
		}
		r := withdrawalRow{MethodID: quote.Policy.MethodID, MethodName: quote.Policy.MethodName, ID: id, OperatorID: in.OperatorID, CustomerID: in.CustomerID, BusinessKey: in.BusinessKey, Fingerprint: fp, PolicyID: quote.Policy.ID, AmountMoney: quote.AmountMoney, FeeMoney: quote.FeeMoney, NetMoney: quote.NetMoney, FeeRemainder: quote.FeeRemainder, RecipientReference: in.RecipientReference, HoldID: hold.ID, Status: "requested", CreatedAt: instant.At}
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
	if proof.Evidence == "" || proof.Currency == "" || proof.Currency != strings.ToUpper(proof.Currency) || proof.ValuationEvidence == "" || proof.PaidAt.IsZero() || parseErr != nil || original.Sign() <= 0 || !proof.PaidMoney.Valid() || proof.PaidMoney.Sign() <= 0 {
		return out, ErrInvalidInput
	}
	proof.PaidAt = proof.PaidAt.UTC()
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		monetary, e := ledger.GetMonetary(ctx, tx)
		if e != nil {
			return e
		}
		if proof.Currency == monetary.Currency && original.Cmp(proof.PaidMoney) != 0 {
			return ErrInvalidInput
		}
		if proof.PaidAt.After(time.Now().UTC().Add(5 * time.Minute)) {
			return ErrInvalidInput
		}
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if strings.TrimSpace(proof.PaymentReference) == "" {
			proof.PaymentReference = "manual:withdrawal:" + r.ID
		}
		if r.Status == "paid" {
			if r.PaymentReference == nil || *r.PaymentReference != proof.PaymentReference || r.NetMoney.Cmp(proof.PaidMoney) != 0 {
				return ErrConflict
			}
			var previous struct{ Payload json.RawMessage }
			if e := tx.Table("incentives_events").Select("payload").Where("operator_id=? AND aggregate_id=? AND kind='withdrawal.payment_verified'", operator, id).First(&previous).Error; e != nil {
				return e
			}
			var saved struct {
				Proof WithdrawalPaymentProof `json:"proof"`
			}
			if json.Unmarshal(previous.Payload, &saved) != nil || fingerprint(saved.Proof) != fingerprint(proof) {
				return ErrConflict
			}
			out = Withdrawal(r)
			return nil
		}
		if r.Status != "paying" && r.Status != "unknown" {
			return ErrWithdrawalState
		}
		if r.NetMoney.Cmp(proof.PaidMoney) != 0 {
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
		lines := []ledger.Line{{AccountID: available.ID, Amount: r.AmountMoney.Neg()}, {AccountID: payouts.ID, Amount: r.NetMoney}}
		if r.FeeMoney.Sign() > 0 {
			fees, err := s.account(ctx, tx, operator, operator, "control:withdrawal_fees")
			if err != nil {
				return err
			}
			lines = append(lines, ledger.Line{AccountID: fees.ID, Amount: r.FeeMoney})
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
