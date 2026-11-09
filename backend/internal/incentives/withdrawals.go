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
	"github.com/tokpath/tokstudio/backend/internal/paymentmethods"
	"golang.org/x/text/currency"
	"gorm.io/gorm"
)

const commissionWithdrawalTerms = "提现手续费由申请人承担；经营方单人审批后付款。"

func (s *Service) SetCommissionWithdrawalMinimum(ctx context.Context, tx *gorm.DB, actor, operator string, minimum ledger.Decimal) (WithdrawalPolicy, error) {
	return s.setWithdrawalMinimum(ctx, tx, actor, operator, "commission", "marketing.manage", minimum)
}

func (s *Service) SetBalanceWithdrawalMinimum(ctx context.Context, tx *gorm.DB, actor, operator string, minimum ledger.Decimal) (WithdrawalPolicy, error) {
	return s.setWithdrawalMinimum(ctx, tx, actor, operator, "balance", "withdrawals.manage", minimum)
}

func (s *Service) WithdrawalMinimum(ctx context.Context, tx *gorm.DB, operator, wallet string) ledger.Decimal {
	if wallet != "balance" {
		wallet = "commission"
	}
	var setting struct{ MinimumMoney ledger.Decimal }
	if err := s.dbFor(ctx, tx).Table("incentives_withdrawal_settings").Where("operator_id=? AND wallet=?", operator, wallet).Limit(1).Find(&setting).Error; err != nil || setting.MinimumMoney.Sign() <= 0 {
		return ledger.FromInt64(1)
	}
	return setting.MinimumMoney
}

func (s *Service) setWithdrawalMinimum(ctx context.Context, tx *gorm.DB, actor, operator, wallet, permission string, minimum ledger.Decimal) (out WithdrawalPolicy, err error) {
	if wallet != "commission" && wallet != "balance" || !minimum.Valid() || minimum.Sign() <= 0 {
		return out, ErrInvalidInput
	}
	// One settlement unit is the smallest amount an operator may publish.
	// Smaller values survive the form, then disappear when a quote is cut to
	// the currency's payable unit.
	if minimum.Cmp(ledger.FromInt64(1)) < 0 {
		if wallet == "balance" {
			return out, ErrBalanceWithdrawalMinimum
		}
		return out, ErrWithdrawalMinimum
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, t, actor, operator, permission); err != nil {
			return err
		}
		if err := s.LockBusiness(ctx, t, operator); err != nil {
			return err
		}
		if err := t.Exec("INSERT INTO incentives_withdrawal_settings(operator_id,wallet,minimum_money) VALUES(?,?,?) ON CONFLICT(operator_id,wallet) DO UPDATE SET minimum_money=excluded.minimum_money", operator, wallet, minimum).Error; err != nil {
			return err
		}
		out.MinimumMoney = minimum
		out.OperatorID = operator
		out.Wallet = wallet
		return s.event(t, operator, operator, "withdrawal.minimum_changed", map[string]any{"actor_id": actor, "wallet": wallet, "minimum_money": minimum})
	})
	return
}

// SetWithdrawalPolicy is the domain entry used by setup and integrations. A
// method now has one rule for both wallets; wallet only selects the minimum.
func (s *Service) SetWithdrawalPolicy(ctx context.Context, tx *gorm.DB, actor, operator string, in WithdrawalPolicy, expectedVersion int64) (out WithdrawalPolicy, err error) {
	profile, ok := FindPayoutProfile(in.PayoutKind)
	if !ok || !validID(operator) || in.MinimumMoney.Sign() <= 0 {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(t *gorm.DB) error {
		if e := s.org.RequirePermission(ctx, t, actor, operator, "withdrawals.manage"); e != nil {
			return e
		}
		if e := s.LockBusiness(ctx, t, operator); e != nil {
			return e
		}
		if in.MethodID == "" {
			rows, e := paymentmethods.List(ctx, t, operator)
			if e != nil {
				return e
			}
			for _, r := range rows {
				if r.PayoutKind != "" {
					in.MethodID = r.MethodID
					break
				}
			}
			if in.MethodID == "" {
				in.MethodID = uuid.NewString()
			}
			in.Enabled = true
		}
		if in.MethodName == "" {
			in.MethodName = profile.Label
		}
		old, e := paymentmethods.Current(ctx, t, operator, in.MethodID)
		if e != nil && e != gorm.ErrRecordNotFound {
			return e
		}
		r, e := paymentmethods.Save(ctx, t, paymentmethods.Rule{Kind: old.Kind, Paused: old.Paused, CreatedBy: actor, MethodID: in.MethodID, OperatorID: operator, Name: in.MethodName, Collection: old.Collection, PayoutEnabled: in.Enabled, PayoutKind: in.PayoutKind, FlatFeeMoney: in.FlatFeeMoney, FeeRate: in.FeeRate}, expectedVersion)
		if e == paymentmethods.ErrChanged {
			return ErrConflict
		}
		if e != nil {
			return ErrInvalidInput
		}
		if in.Wallet != "balance" {
			in.Wallet = "commission"
		}
		for _, wallet := range []string{"balance", "commission"} {
			if e = t.Exec("INSERT INTO incentives_withdrawal_settings(operator_id,wallet,minimum_money) VALUES(?,?,?) ON CONFLICT(operator_id,wallet) DO NOTHING", operator, wallet, in.MinimumMoney).Error; e != nil {
				return e
			}
		}
		out = withdrawalRule(r, in.Wallet, in.MinimumMoney, "")
		return s.event(t, operator, r.ID, "withdrawal.method_changed", map[string]any{"actor_id": actor, "after": out})
	})
	return
}
func withdrawalRule(r paymentmethods.Rule, wallet string, minimum ledger.Decimal, currency string) WithdrawalPolicy {
	return WithdrawalPolicy{ID: r.ID, MethodID: r.MethodID, OperatorID: r.OperatorID, Version: r.Version, Wallet: wallet, MethodName: r.Name, PayoutKind: r.PayoutKind, Enabled: r.PayoutEnabled && !r.Paused, FlatFeeMoney: r.FlatFeeMoney, FeeRate: r.FeeRate, MinimumMoney: minimum, Currency: currency, MethodReference: commissionWithdrawalTerms, EffectiveAt: r.CreatedAt}
}
func (s *Service) WithdrawalMethods(ctx context.Context, tx *gorm.DB, operator string, enabledOnly bool, wallet string) ([]WithdrawalPolicy, error) {
	if wallet != "balance" {
		wallet = "commission"
	}
	monetary, e := ledger.GetMonetary(ctx, s.dbFor(ctx, tx))
	if e != nil {
		return nil, e
	}
	rows, e := paymentmethods.List(ctx, s.dbFor(ctx, tx), operator)
	if e != nil {
		return nil, e
	}
	minimum := s.WithdrawalMinimum(ctx, tx, operator, wallet)
	out := []WithdrawalPolicy{}
	for _, r := range rows {
		if r.PayoutKind == "" || enabledOnly && (!r.PayoutEnabled || r.Paused) {
			continue
		}
		out = append(out, withdrawalRule(r, wallet, minimum, monetary.Currency))
	}
	return out, nil
}
func (s *Service) QuoteWithdrawal(ctx context.Context, tx *gorm.DB, operator string, amount ledger.Decimal, selection ...string) (WithdrawalQuote, error) {
	wallet := "commission"
	if len(selection) > 1 && selection[1] == "balance" {
		wallet = "balance"
	}
	methods, e := s.WithdrawalMethods(ctx, tx, operator, true, wallet)
	if e != nil {
		return WithdrawalQuote{}, e
	}
	if len(selection) > 0 && selection[0] != "" {
		for _, m := range methods {
			if m.MethodID == selection[0] {
				return quoteWithdrawal(m, amount)
			}
		}
		return WithdrawalQuote{}, ErrNotReady
	}
	if len(methods) == 0 {
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
	if in.Wallet != "" && in.Wallet != "balance" && in.Wallet != "commission" {
		return out, ErrInvalidInput
	}
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
		selected, err := paymentmethods.Version(ctx, tx, in.OperatorID, in.PolicyID)
		if err != nil {
			return err
		}
		balance := in.Wallet == "balance"
		action := "withdrawal"
		if balance {
			action = "balance_withdrawal"
		}
		if err = s.org.RequireCustomerAction(ctx, tx, in.CustomerID, in.OperatorID, action); err != nil {
			return err
		}
		if !balance {
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
		}
		quote, err := s.QuoteWithdrawal(ctx, tx, in.OperatorID, in.AmountMoney, selected.MethodID, in.Wallet)
		if err != nil {
			return err
		}
		if quote.Policy.ID != in.PolicyID {
			return ErrConflict
		}
		purpose := "commission:available"
		holdPurpose := "commission_withdrawal"
		if balance {
			purpose = "customer:money"
			holdPurpose = "balance_withdrawal"
		}
		account, err := s.account(ctx, tx, in.OperatorID, in.CustomerID, purpose)
		if err != nil {
			return err
		}
		id := uuid.NewString()
		if !balance {
			if _, err = s.reserveCommissionUse(ctx, tx, in.OperatorID, in.CustomerID, "withdrawal", id, quote.AmountMoney, nil); err != nil {
				return err
			}
		}
		hold, err := s.ledger.Reserve(ctx, tx, ledger.Reservation{OperatorID: in.OperatorID, BusinessKey: "withdrawal:" + id, Purpose: holdPurpose, ExpiresAt: instant.At.AddDate(10, 0, 0), Amounts: []ledger.Line{{AccountID: account.ID, Amount: quote.AmountMoney}}})
		if err != nil {
			return err
		}
		wallet := "commission"
		if balance {
			wallet = "balance"
		}
		r := withdrawalRow{Wallet: wallet, MethodID: quote.Policy.MethodID, MethodName: quote.Policy.MethodName, ID: id, OperatorID: in.OperatorID, CustomerID: in.CustomerID, BusinessKey: in.BusinessKey, Fingerprint: fp, PolicyID: quote.Policy.ID, AmountMoney: quote.AmountMoney, FeeMoney: quote.FeeMoney, NetMoney: quote.NetMoney, FeeRemainder: quote.FeeRemainder, RecipientReference: in.RecipientReference, HoldID: hold.ID, Status: "requested", CreatedAt: instant.At}
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
func (s *Service) requireWithdrawalAction(ctx context.Context, tx *gorm.DB, r withdrawalRow) error {
	action := "withdrawal"
	if r.Wallet == "balance" {
		action = "balance_withdrawal"
	}
	return s.org.RequireCustomerAction(ctx, tx, r.CustomerID, r.OperatorID, action)
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
		if err = s.requireWithdrawalAction(ctx, tx, r); err != nil {
			return err
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
func completeWithdrawalProof(proof *WithdrawalPaymentProof, net ledger.Decimal, settlement string) error {
	proof.Currency = strings.ToUpper(strings.TrimSpace(proof.Currency))
	if proof.Currency == "" || proof.PaidAt.IsZero() || !net.Valid() || net.Sign() <= 0 {
		return ErrInvalidInput
	}
	// The settlement amount was fixed when the customer applied. Recording
	// payment does not choose a second amount.
	proof.PaidMoney = net
	if proof.Currency == settlement {
		if strings.TrimSpace(proof.OriginalAmount) != "" {
			amount, err := ledger.ParseDecimal(strings.TrimSpace(proof.OriginalAmount))
			if err != nil || amount.Cmp(net) != 0 {
				return ErrWithdrawalSameCurrencyAmount
			}
		}
		proof.OriginalAmount = net.String()
		if strings.TrimSpace(proof.ValuationEvidence) == "" {
			proof.ValuationEvidence = "Verified payment in the settlement currency"
		}
		return nil
	}
	amount, err := ledger.ParseDecimal(strings.TrimSpace(proof.OriginalAmount))
	if err != nil || amount.Sign() <= 0 || strings.TrimSpace(proof.ValuationEvidence) == "" {
		return ErrWithdrawalForeignAmount
	}
	proof.OriginalAmount = amount.String()
	return nil
}

func (s *Service) VerifyWithdrawalPaid(ctx context.Context, tx *gorm.DB, actor, operator, id string, proof WithdrawalPaymentProof) (out Withdrawal, err error) {
	proof.PaidAt = proof.PaidAt.UTC()
	if proof.PaidAt.After(time.Now().UTC().Add(5 * time.Minute)) {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		monetary, e := ledger.GetMonetary(ctx, tx)
		if e != nil {
			return e
		}
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.PayoutCurrency != "" {
			if proof.Currency != "" && !strings.EqualFold(proof.Currency, r.PayoutCurrency) {
				return ErrInvalidInput
			}
			if proof.OriginalAmount != "" {
				v, e := ledger.ParseDecimal(proof.OriginalAmount)
				if e != nil || v.Cmp(r.PayoutAmount) != 0 {
					return ErrInvalidInput
				}
			}
			proof.Currency = r.PayoutCurrency
			proof.OriginalAmount = r.PayoutAmount.String()
			if proof.ValuationEvidence == "" {
				proof.ValuationEvidence = "withdrawal-accepted-quote:" + r.PolicyID
			}
		}
		if err = completeWithdrawalProof(&proof, r.NetMoney, monetary.Currency); err != nil {
			return err
		}
		if strings.TrimSpace(proof.PaymentReference) == "" {
			proof.PaymentReference = "manual:withdrawal:" + r.ID
		}
		if strings.TrimSpace(proof.Evidence) == "" {
			proof.Evidence = "withdrawal-payment:" + r.ID
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
		if r.Status != "approved" {
			return ErrWithdrawalState
		}
		if err = s.requireWithdrawalAction(ctx, tx, r); err != nil {
			return err
		}
		balance := r.Wallet == "balance"
		if !balance {
			if err = s.RequireCommissionFundingExecution(ctx, tx, operator, r.CustomerID, "withdrawal", r.ID); err != nil {
				return err
			}
		}
		before := Withdrawal(r)
		source := "commission:available"
		payoutPurpose := "control:commission_payouts"
		kind := "commission_payout"
		if balance {
			source = "customer:money"
			// Cash actually leaves the operator. The fee stays as withdrawal income.
			payoutPurpose = "control:external_cash"
			kind = "balance_payout"
		}
		available, err := s.account(ctx, tx, operator, r.CustomerID, source)
		if err != nil {
			return err
		}
		payouts, err := s.account(ctx, tx, operator, operator, payoutPurpose)
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
		if _, err = s.ledger.Capture(ctx, tx, r.HoldID, ledger.Posting{OperatorID: operator, BusinessKey: "withdrawal-paid:" + r.ID, Kind: kind, SourceID: r.ID, ActorID: actor, Reason: proof.Evidence, Lines: lines}, nil); err != nil {
			return err
		}
		if !balance {
			if err = s.resolveCommissionUse(ctx, tx, operator, r.CustomerID, "withdrawal", r.ID, true); err != nil {
				return err
			}
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
		if r.Wallet != "balance" {
			if err := s.resolveCommissionUse(ctx, tx, r.OperatorID, r.CustomerID, "withdrawal", r.ID, false); err != nil {
				return err
			}
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
func (s *Service) FailWithdrawal(ctx context.Context, tx *gorm.DB, actor, operator, id, evidence string) error {
	evidence = strings.TrimSpace(evidence)
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		r, err := s.withdrawalForStaff(ctx, tx, actor, operator, id)
		if err != nil {
			return err
		}
		if r.Status == "failed" {
			return nil
		}
		if r.Status != "approved" && r.Status != "requested" {
			return ErrWithdrawalState
		}
		if r.Wallet != "balance" {
			if err = s.resolveCommissionUse(ctx, tx, operator, r.CustomerID, "withdrawal", r.ID, false); err != nil {
				return err
			}
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
