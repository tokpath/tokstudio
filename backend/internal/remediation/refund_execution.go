package remediation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/payments"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExecutionProof struct {
	BusinessKey        string                            `json:"business_key"`
	Reference          string                            `json:"reference"`
	Evidence           string                            `json:"evidence"`
	ActualMoney        ledger.Decimal                    `json:"actual_money"`
	FeeMoney           ledger.Decimal                    `json:"fee_money"`
	FeeStatus          string                            `json:"fee_status"`
	Currency           string                            `json:"currency"`
	Amount             string                            `json:"amount"`
	OccurredAt         time.Time                         `json:"occurred_at"`
	ExternalRefund     *payments.ExternalRefundExecution `json:"external_refund,omitempty"`
	ExecutionClosureID string                            `json:"execution_closure_id,omitempty"`
}

func (s *Service) lockLeg(ctx context.Context, t *gorm.DB, op, id string) (Case, RefundLeg, error) {
	var l RefundLeg
	if e := t.First(&l, "id=? AND operator_id=?", id, op).Error; e != nil {
		return Case{}, l, e
	}
	c, e := s.lockCase(ctx, t, op, l.CaseID)
	if e != nil {
		return c, l, e
	}
	e = t.Clauses(clause.Locking{Strength: "UPDATE"}).First(&l, "id=?", id).Error
	return c, l, e
}
func (s *Service) BeginManualRefund(ctx context.Context, actor, op, legID, key, evidence string) (RefundLeg, error) {
	var out RefundLeg
	if key == "" || len(key) > 150 || evidence == "" {
		return out, ErrInvalid
	}
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		c, l, e := s.lockLeg(ctx, t, op, legID)
		if e != nil {
			return e
		}
		out = l
		if l.ExecutionKey == key && l.ExecutionMode == "manual" {
			return nil
		}
		caseOpen := c.State == "approved" || c.State == "paying" || c.State == "exception"
		initial := l.State == "approved" && caseOpen
		supplemental := l.State == "exception" && caseOpen && (l.SettledMoney.Sign() > 0 || l.MisdirectedMoney.Sign() > 0) && l.SettledMoney.Cmp(l.AmountMoney) < 0
		if !initial && !supplemental {
			return ErrState
		}
		if required, e := s.externalDestinationRequired(ctx, t, l); e != nil {
			return e
		} else if required {
			if _, e = s.requireAuthorizedDestination(ctx, t, l); e != nil {
				return e
			}
		}
		if e = s.RequireReceiptRefundExecution(ctx, t, l.SourceOperatorID, l.ReceiptID, l.ID); e != nil {
			return e
		}
		l.ExecutionKey = key
		l.ExecutionMode = "manual"
		l.State = "executing"
		if e = t.Save(&l).Error; e != nil {
			return e
		}
		c.State = "paying"
		if e = t.Save(&c).Error; e != nil {
			return e
		}
		out = l
		return s.event(t, op, l.ID, "refund.manual_execution_started", actor, evidence, map[string]any{"case_id": c.ID, "execution_key": key, "supplemental": supplemental, "remaining_money": l.AmountMoney.Sub(l.SettledMoney)})
	})
	return out, e
}
func (s *Service) MarkRefundUnknown(ctx context.Context, actor, op, legID, evidence string) (RefundLeg, error) {
	var out RefundLeg
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		_, l, e := s.lockLeg(ctx, t, op, legID)
		if e != nil {
			return e
		}
		if evidence == "" || l.State != "executing" && l.State != "unknown" {
			return ErrState
		}
		l.State = "unknown"
		out = l
		if e = t.Save(&l).Error; e != nil {
			return e
		}
		return s.event(t, op, l.ID, "refund.outcome_unknown", actor, evidence, l)
	})
	return out, e
}

// RecordManualRefund records a transfer which finance has actually verified.
// An application or an intention to transfer is never settlement evidence.
func (s *Service) RecordManualRefund(ctx context.Context, actor, op, legID string, proof ExecutionProof) (RefundLeg, error) {
	var out RefundLeg
	var mismatch bool
	e := s.within(ctx, nil, func(t *gorm.DB) error {
		if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
			return e
		}
		c, l, e := s.lockLeg(ctx, t, op, legID)
		if e != nil {
			return e
		}
		verifiedClosed := false
		if proof.ExecutionClosureID != "" {
			var closure RefundExecutionClosure
			if e = t.First(&closure, "id=? AND leg_id=? AND operator_id=? AND execution_mode='manual'", proof.ExecutionClosureID, l.ID, op).Error; e != nil {
				return e
			}
			var auth int64
			if e = t.Model(&RefundDestinationAuthorization{}).Where("leg_id=? AND route_version=?", l.ID, closure.RouteVersion).Count(&auth).Error; e != nil {
				return e
			}
			if auth != 0 {
				return ErrReview
			}
			verifiedClosed = true
		}
		if l.ExecutionMode != "manual" && !verifiedClosed {
			return ErrState
		}
		// Chain/third-party stablecoin facts must come through the dedicated
		// payment verifier. A staff JSON payload cannot assert RPC verification.
		if required, e := s.externalDestinationRequired(ctx, t, l); e != nil {
			return e
		} else if required && !verifiedClosed {
			return ErrReview
		}
		if proof.ExternalRefund != nil {
			return ErrInvalid
		}
		if strings.TrimSpace(proof.Reference) == "" {
			execution := l.ExecutionKey
			if proof.ExecutionClosureID != "" {
				execution = proof.ExecutionClosureID
			}
			proof.Reference = payments.ManualReference("refund", l.ID, execution)
		}
		mismatch, e = s.recordPayment(ctx, t, &c, &l, actor, proof, verifiedClosed)
		out = l
		return e
	})
	if e == nil && mismatch {
		return out, ErrPaymentMismatch
	}
	return out, e
}

// RequireApprovedRefund implements the payment adapter's business authorization
// gate. It is called before its durable external intent is committed.
func (s *Service) RequireApprovedRefund(ctx context.Context, t *gorm.DB, actor string, r payments.Refund) error {
	if t == nil || s.Payments == nil {
		return ErrNotReady
	}
	op := r.Order.OperatorID
	if e := s.Organization.RequirePermission(ctx, t, actor, op, "refunds.manage"); e != nil {
		return e
	}
	c, l, e := s.lockLeg(ctx, t, op, r.ID)
	if e != nil {
		return e
	}
	if l.SourceOperatorID != op || l.RouteVersion > 1 || l.State != "approved" || c.State != "approved" && c.State != "paying" || r.MoneyAmount.Cmp(l.AmountMoney) != 0 {
		return ErrState
	}
	if e = s.RequireReceiptRefundExecution(ctx, t, l.SourceOperatorID, l.ReceiptID, l.ID); e != nil {
		return e
	}
	adapter := *s.Payments
	adapter.DB = t
	source, e := adapter.PaymentSourceForReceipt(ctx, op, l.ReceiptID)
	if e != nil {
		return e
	}
	if source.MerchantID != r.Order.MerchantID || source.OriginalOrder.ID != r.Order.ID {
		return ErrScope
	}
	l.ExecutionKey = r.ID
	l.ExecutionMode = "provider"
	l.PaymentRefundID = r.ID
	l.State = "executing"
	if e = t.Save(&l).Error; e != nil {
		return e
	}
	c.State = "paying"
	if e = t.Save(&c).Error; e != nil {
		return e
	}
	return s.event(t, op, l.ID, "refund.provider_execution_started", actor, "approved original-receipt refund", map[string]string{"case_id": c.ID, "merchant_id": source.MerchantID, "order_id": r.Order.ID})
}
func (s *Service) RecordVerifiedRefund(ctx context.Context, t *gorm.DB, m payments.Merchant, o payments.Order, r payments.Refund, f payments.Fact, value payments.MoneyValue) error {
	if t == nil || !f.Verified() || f.Kind != "refund" || m.OperatorID != o.OperatorID || f.OperatorID != m.OperatorID || f.OrderID != o.ID || f.MerchantID != m.ID || r.Order.ID != o.ID {
		return ErrScope
	}
	c, l, e := s.lockLeg(ctx, t, m.OperatorID, r.ID)
	if e != nil {
		return e
	}
	if r.MoneyAmount.Cmp(l.AmountMoney) != 0 {
		return ErrState
	}
	if f.ExternalRefund != nil {
		if f.RefundSource != nil {
			if f.RefundSource.SourceOperatorID != l.SourceOperatorID || f.RefundSource.ReceiptID != l.ReceiptID || f.RefundSource.ResponsibilityReference != l.ID || o.ID != l.ID || o.OperatorID != l.OperatorID || o.ContextKind != "refund_execution" {
				return ErrScope
			}
			if _, e = s.responsibleRefundBasis(ctx, t, l, f.ExternalRefund.Destination); e != nil {
				return e
			}
		} else {
			if e = s.matchExternalRefundFact(ctx, t, l, r, f.ExternalRefund.Destination); e != nil {
				return e
			}
		}
	} else if l.ExecutionMode != "provider" || l.PaymentRefundID != r.ID {
		wasProvider, e := s.historicalProviderInstruction(t, l, r.ID)
		if e != nil {
			return e
		}
		if !wasProvider {
			return ErrState
		}
	}
	at := f.ObservedAt
	if f.OccurredAt != nil {
		at = *f.OccurredAt
	}
	proof := ExecutionProof{BusinessKey: "provider:" + f.ID, Reference: f.ProviderID, Evidence: string(f.Evidence), ActualMoney: value.Amount, FeeStatus: "pending", Currency: f.Money.Currency, Amount: f.Money.Amount.String(), OccurredAt: at}
	proof.ExternalRefund = f.ExternalRefund
	// Preserve the verified fact and unknown liability on a conversion mismatch.
	// Returning an error here would roll back that evidence and invite re-payment.
	mismatch, e := s.recordPayment(ctx, t, &c, &l, "verified-payment-adapter", proof, true)
	if e != nil {
		return e
	}
	if mismatch {
		return fmt.Errorf("actual refund USD differs from the original obligation: %w", payments.ErrSettlementReview)
	}
	return nil
}
func (s *Service) recordPayment(ctx context.Context, t *gorm.DB, c *Case, l *RefundLeg, actor string, p ExecutionProof, verifiedFact bool) (bool, error) {
	// Omitted zero conveys no evidence that the merchant was charged no fee.
	// An explicit nonzero verified fee remains a known cost for manual inputs.
	if p.FeeStatus == "" {
		p.FeeStatus = "pending"
		if p.FeeMoney.Sign() > 0 {
			p.FeeStatus = "confirmed"
		}
	}
	amount, e := ledger.ParseDecimal(p.Amount)
	if e != nil || amount.Sign() <= 0 || p.BusinessKey == "" || len(p.BusinessKey) > 150 || p.Reference == "" || p.Evidence == "" || !validAmount(p.ActualMoney) || !p.FeeMoney.Valid() || p.FeeMoney.Sign() < 0 || (p.FeeStatus != "pending" && p.FeeStatus != "confirmed") || (p.FeeStatus == "pending" && !p.FeeMoney.IsZero()) || p.Currency == "" || p.Currency != strings.ToUpper(p.Currency) || p.OccurredAt.IsZero() {
		return false, ErrInvalid
	}
	var old struct{ Fingerprint string }
	fp := fingerprint(p)
	e = t.Table("remediation_payment_observations").Where("leg_id=? AND business_key=?", l.ID, p.BusinessKey).Take(&old).Error
	if e == nil {
		if old.Fingerprint != fp {
			return false, ErrConflict
		}
		return l.State == "exception" || l.ExcessMoney.Sign() > 0 || l.MisdirectedMoney.Sign() > 0, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return false, e
	}
	if l.State != "executing" && l.State != "unknown" && !(l.State == "exception" && p.ExternalRefund != nil) && !(verifiedFact && (l.State == "exception" || l.State == "completed" || (l.RouteVersion > 1 && l.State == "approved"))) {
		return false, ErrState
	}
	at, e := instant(t)
	if e != nil {
		return false, e
	}
	if p.OccurredAt.After(at.Add(5 * time.Minute)) {
		return false, ErrInvalid
	}
	if p.ExternalRefund != nil {
		if e = s.validateExternalRefundProof(ctx, t, *l, *p.ExternalRefund); e != nil {
			return false, e
		}
	}
	// All provenance fences precede ledger rows, including the independent fee
	// posting and a verified transfer whose amount requires reconciliation.
	if c.Kind == "wallet" && c.CustomerID != nil {
		if e = s.Commerce.LockWalletFunding(ctx, t, c.OperatorID, *c.CustomerID); e != nil {
			return false, e
		}
	}
	if e = lock(t, l.SourceOperatorID+":receipt:"+l.ReceiptID); e != nil {
		return false, e
	}
	if _, e = s.Commerce.LockReceipt(ctx, t, l.SourceOperatorID, l.ReceiptID); e != nil {
		return false, e
	}
	factID := "manual:" + fingerprint([]string{p.Currency, p.Reference})
	if p.ExternalRefund != nil || strings.HasPrefix(p.BusinessKey, "provider:") {
		factID = p.BusinessKey
	}
	if e = lock(t, c.OperatorID+":refund-fact:"+factID); e != nil {
		return false, e
	}
	var bound struct{ LegID, Fingerprint string }
	e = t.Table("remediation_payment_fact_bindings").Where("operator_id=? AND fact_id=?", c.OperatorID, factID).Take(&bound).Error
	if e == nil {
		return false, ErrConflict
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return false, e
	}
	observationID := uuid.NewString()
	var knownFee any
	if p.FeeStatus == "confirmed" {
		knownFee = p.FeeMoney
	}
	if e = t.Exec("INSERT INTO remediation_payment_observations(id,leg_id,business_key,fingerprint,actual_money,currency,amount,fee_status,fee_money,evidence,occurred_at,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", observationID, l.ID, p.BusinessKey, fp, p.ActualMoney, p.Currency, p.Amount, p.FeeStatus, knownFee, p.Reference+" | "+p.Evidence, p.OccurredAt, at).Error; e != nil {
		return false, e
	}
	if e = t.Table("remediation_payment_fact_bindings").Create(map[string]any{"operator_id": c.OperatorID, "fact_id": factID, "leg_id": l.ID, "observation_id": observationID, "fingerprint": fp}).Error; e != nil {
		return false, e
	}
	if p.ExternalRefund != nil {
		x := p.ExternalRefund
		if e = t.Exec("INSERT INTO remediation_external_refund_observations(observation_id,authorization_id,network,asset,token_contract,address,transaction_hash,log_index,block_hash,sender_address,recipient_address,confirmations,verification_method,verified_by,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", observationID, x.Destination.AuthorizationReference, x.Destination.Network, x.Destination.Asset, x.Destination.TokenContract, x.Destination.Address, x.TransactionHash, x.LogIndex, x.BlockHash, x.SenderAddress, x.RecipientAddress, x.Confirmations, x.VerificationMethod, x.VerifiedBy, at).Error; e != nil {
			return false, e
		}
	}
	if p.FeeStatus == "confirmed" {
		if _, e = s.recordRefundFee(ctx, t, *l, observationID, RefundFeeInput{OperatorID: c.OperatorID, LegID: l.ID, PaymentBusinessKey: p.BusinessKey, BusinessKey: "execution:" + p.BusinessKey, ActorID: actor, Reference: p.Reference, Evidence: p.Evidence, FeeMoney: p.FeeMoney, OccurredAt: p.OccurredAt}); e != nil {
			return false, e
		}
	}
	if p.ExternalRefund != nil && externalRefundMisdirected(*p.ExternalRefund) {
		cash, e := s.account(ctx, t, c.OperatorID, c.OperatorID, "control:external_cash")
		if e != nil {
			return false, e
		}
		loss, e := s.account(ctx, t, c.OperatorID, c.OperatorID, "control:refund_misdirected")
		if e != nil {
			return false, e
		}
		if _, e = s.Ledger.Post(ctx, t, ledger.Posting{OperatorID: c.OperatorID, BusinessKey: "refund-misdirected:" + observationID, Kind: "external_refund_misdirected", SourceID: l.ID, ActorID: actor, Reason: p.Reference + " | " + p.Evidence, Lines: []ledger.Line{{AccountID: cash.ID, Amount: p.ActualMoney}, {AccountID: loss.ID, Amount: p.ActualMoney.Neg()}}}); e != nil {
			return false, e
		}
		l.MisdirectedMoney = l.MisdirectedMoney.Add(p.ActualMoney)
		l.State = "exception"
		c.State = "exception"
		if e = t.Save(l).Error; e != nil {
			return false, e
		}
		if e = t.Save(c).Error; e != nil {
			return false, e
		}
		return true, s.event(t, c.OperatorID, l.ID, "refund.execution_recipient_mismatch", actor, p.Evidence, map[string]any{"case_id": c.ID, "payment": p})
	}
	if l.HoldID == nil || l.SourceAccountID == nil {
		return false, ErrConflict
	}
	capacityObservation, e := s.ObserveReceiptRefund(ctx, t, l.SourceOperatorID, l.ReceiptID, l.ID, factID, p.ActualMoney)
	if e != nil {
		return false, e
	}
	if c.Kind == "wallet" {
		var links []struct{ AllocationID string }
		if e = t.Table("remediation_wallet_links").Where("leg_id=?", l.ID).Find(&links).Error; e != nil {
			return false, e
		}
		if len(links) == 0 {
			return false, ErrConflict
		}
		allocations, e := s.Commerce.WalletRefundAllocations(ctx, t, c.OperatorID, *c.CustomerID, c.ID)
		if e != nil {
			return false, e
		}
		byID := map[string]bool{}
		for _, link := range links {
			byID[link.AllocationID] = true
		}
		left := capacityObservation.SettledMoney
		for _, a := range allocations {
			if !byID[a.ID] || left.IsZero() {
				continue
			}
			take := min(left, a.AmountMoney.Sub(a.RefundedMoney))
			if take.Sign() <= 0 {
				continue
			}
			if e = s.Commerce.ObserveWalletRefundAllocation(ctx, t, c.OperatorID, *c.CustomerID, c.ID, a.ID, observationID, take); e != nil {
				return false, e
			}
			left = left.Sub(take)
		}
		if !left.IsZero() {
			return false, ErrConflict
		}
	}
	// Provenance is fenced before the receipt, and the receipt before cash rows;
	// verified executions remain recordable even if a later dispute arrived.
	if e = lock(t, l.SourceOperatorID+":receipt:"+l.ReceiptID); e != nil {
		return false, e
	}
	if _, e = s.Commerce.LockReceipt(ctx, t, l.SourceOperatorID, l.ReceiptID); e != nil {
		return false, e
	}
	cash, e := s.account(ctx, t, c.OperatorID, c.OperatorID, "control:external_cash")
	if e != nil {
		return false, e
	}
	hold, e := s.Ledger.HoldInTransaction(ctx, t, c.OperatorID, *l.HoldID)
	if e != nil {
		return false, e
	}
	lines := []ledger.Line{{AccountID: cash.ID, Amount: p.ActualMoney}}
	if capacityObservation.SettledMoney.Sign() > 0 {
		lines = append(lines, ledger.Line{AccountID: *l.SourceAccountID, Amount: capacityObservation.SettledMoney.Neg()})
	}
	if capacityObservation.ExcessMoney.Sign() > 0 {
		loss, e := s.account(ctx, t, c.OperatorID, c.OperatorID, "control:refund_overpayment")
		if e != nil {
			return false, e
		}
		lines = append(lines, ledger.Line{AccountID: loss.ID, Amount: capacityObservation.ExcessMoney.Neg()})
	}
	posting := ledger.Posting{OperatorID: c.OperatorID, BusinessKey: "refund-paid:" + observationID, Kind: "external_refund", SourceID: l.ID, ActorID: actor, Reason: p.Reference + " | " + p.Evidence, Lines: lines}
	if capacityObservation.SettledMoney.Sign() > 0 {
		_, e = s.Ledger.Capture(ctx, t, *l.HoldID, posting, nil)
	} else {
		_, e = s.Ledger.Post(ctx, t, posting)
	}
	if e != nil {
		return false, e
	}
	l.SettledMoney = l.SettledMoney.Add(capacityObservation.SettledMoney)
	l.ExcessMoney = l.ExcessMoney.Add(capacityObservation.ExcessMoney)
	if capacityObservation.RemainingMoney.Sign() > 0 {
		next, e := s.Ledger.Reserve(ctx, t, ledger.Reservation{OperatorID: c.OperatorID, BusinessKey: "refund-remainder:" + observationID, Purpose: "refund", ExpiresAt: hold.ExpiresAt, Amounts: []ledger.Line{{AccountID: *l.SourceAccountID, Amount: capacityObservation.RemainingMoney}}})
		if e != nil {
			return false, e
		}
		l.HoldID = &next.ID
		l.State = "exception"
		c.State = "exception"
		if e = t.Save(l).Error; e != nil {
			return false, e
		}
		if e = t.Save(c).Error; e != nil {
			return false, e
		}
		return true, s.event(t, c.OperatorID, l.ID, "refund.execution_amount_mismatch", actor, p.Evidence, map[string]any{"case_id": c.ID, "payment": p, "settled_money": capacityObservation.SettledMoney, "remaining_money": capacityObservation.RemainingMoney})
	}
	l.State = "completed"
	l.CompletedAt = &at
	l.ExecutionReference = p.Reference
	if e = t.Save(l).Error; e != nil {
		return false, e
	}
	if e = s.completeCase(ctx, t, c); e != nil {
		return false, e
	}
	return capacityObservation.ExcessMoney.Sign() > 0, s.event(t, c.OperatorID, l.ID, "refund.completed", actor, p.Evidence, map[string]any{"case_id": c.ID, "payment": p, "excess_money": capacityObservation.ExcessMoney})
}
