package commerce

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *Service) RecordManualReceipt(ctx context.Context, tx *gorm.DB, actorID string, fact VerifiedReceipt) (Receipt, error) {
	var out Receipt
	err := s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := s.org.RequirePermission(ctx, tx, actorID, fact.OperatorID, "payments.manage"); err != nil {
			return err
		}
		fact.Provider = "manual"
		fact.VerifiedBy = "staff:" + actorID
		var err error
		out, err = s.RecordReceipt(ctx, tx, fact)
		return err
	})
	return out, err
}

// RecordReceipt accepts only an already authenticated and independently
// verified provider fact. It records gross customer value and merchant fees
// once; fees never reduce the customer's agreed order entitlement.
func (s *Service) RecordReceipt(ctx context.Context, tx *gorm.DB, f VerifiedReceipt) (out Receipt, err error) {
	f.Currency = strings.ToUpper(strings.TrimSpace(f.Currency))
	if f.PaidAt != nil {
		value := f.PaidAt.UTC()
		f.PaidAt = &value
	}
	if f.FeeStatus == "" {
		f.FeeStatus = "confirmed"
	}
	if !f.ObservedAt.IsZero() {
		f.ObservedAt = f.ObservedAt.UTC()
	}
	if f.SettledAt != nil {
		t := f.SettledAt.UTC()
		f.SettledAt = &t
	}
	original, parseErr := ledger.ParseDecimal(f.OriginalAmount)
	if f.OriginalFeeAmount != nil {
		if !f.OriginalFeeAmount.Valid() || f.OriginalFeeAmount.Sign() < 0 || f.OriginalFeeAmount.Cmp(original) > 0 {
			return out, ErrInvalidInput
		}
		net := original.Sub(*f.OriginalFeeAmount)
		if f.OriginalNetAmount != nil && f.OriginalNetAmount.Cmp(net) != 0 {
			return out, ErrInvalidInput
		}
		f.OriginalNetAmount = &net
	} else if f.OriginalNetAmount != nil {
		return out, ErrInvalidInput
	}
	manual := (f.Provider == "manual" || strings.HasPrefix(f.Provider, "manual:")) && strings.HasPrefix(f.VerifiedBy, "staff:") && strings.TrimSpace(strings.TrimPrefix(f.VerifiedBy, "staff:")) != ""
	if !validID(f.OperatorID) || strings.TrimSpace(f.Provider) == "" || strings.TrimSpace(f.ProviderReceiptID) == "" || f.GrossMoney.Sign() <= 0 || !f.GrossMoney.Valid() || f.Currency == "" || parseErr != nil || original.Sign() <= 0 || (!manual && (f.RecipientReference == "" || f.PayerReference == "" || f.VerificationEvidence == "")) || f.ValuationEvidence == "" || f.VerifiedBy == "" || (f.PaidAt != nil && (f.PaidAt.IsZero() || f.PaidAt.After(s.now().Add(5*time.Minute)))) || (f.SettledAt != nil && f.PaidAt != nil && f.SettledAt.Before(*f.PaidAt)) || (!f.ObservedAt.IsZero() && f.ObservedAt.After(s.now().Add(5*time.Minute))) {
		return out, ErrInvalidInput
	}
	if f.FeeStatus == "pending" {
		if f.NetMoney != nil || f.FeeMoney != nil || f.SettledAt != nil {
			return out, ErrInvalidInput
		}
	} else if f.FeeStatus == "confirmed" {
		if f.NetMoney == nil || f.FeeMoney == nil || f.NetMoney.Sign() < 0 || f.FeeMoney.Sign() < 0 || !f.NetMoney.Valid() || !f.FeeMoney.Valid() || f.NetMoney.Add(*f.FeeMoney).Cmp(f.GrossMoney) != 0 {
			return out, ErrInvalidInput
		}
	} else {
		return out, ErrInvalidInput
	}
	f.OriginalAmount = original.String()
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		base, e := ledger.GetMonetary(ctx, tx)
		if e != nil {
			return e
		}
		if f.Currency == base.Currency {
			if original.Cmp(f.GrossMoney) != 0 {
				return ErrInvalidInput
			}
			if f.FeeMoney != nil {
				if f.OriginalFeeAmount != nil && f.OriginalFeeAmount.Cmp(*f.FeeMoney) != 0 {
					return ErrInvalidInput
				}
				fee, net := *f.FeeMoney, original.Sub(*f.FeeMoney)
				f.OriginalFeeAmount, f.OriginalNetAmount = &fee, &net
			}
		}
		if _, err := s.org.GetOperator(ctx, tx, f.OperatorID); err != nil {
			return err
		}
		if err := lockCommand(tx, f.OperatorID, "receipt:"+f.Provider+":"+f.ProviderReceiptID); err != nil {
			return err
		}
		var old receiptRow
		q := tx.Where("operator_id=? AND provider=? AND provider_receipt_id=?", f.OperatorID, f.Provider, f.ProviderReceiptID).Limit(1).Find(&old)
		if q.Error != nil {
			return q.Error
		}
		if f.ObservedAt.IsZero() {
			if q.RowsAffected > 0 {
				f.ObservedAt = old.ObservedAt.UTC()
			} else {
				f.ObservedAt = s.now()
			}
		}
		fp := fingerprint(f)
		if q.RowsAffected > 0 {
			if old.Fingerprint != fp {
				return ErrConflict
			}
			out = Receipt(old)
			return nil
		}
		r := receiptRow{OriginalFeeAmount: f.OriginalFeeAmount, OriginalNetAmount: f.OriginalNetAmount, ID: uuid.NewString(), OperatorID: f.OperatorID, Provider: f.Provider, ProviderReceiptID: f.ProviderReceiptID, Fingerprint: fp, GrossMoney: f.GrossMoney, NetMoney: f.NetMoney, FeeMoney: f.FeeMoney, FeeStatus: f.FeeStatus, Currency: f.Currency, OriginalAmount: f.OriginalAmount, Network: f.Network, RecipientReference: f.RecipientReference, PayerReference: f.PayerReference, ValuationEvidence: f.ValuationEvidence, VerificationEvidence: f.VerificationEvidence, VerifiedBy: f.VerifiedBy, PaidAt: f.PaidAt, ObservedAt: f.ObservedAt, SettledAt: f.SettledAt, CreatedAt: s.now()}
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		unallocated, err := s.account(ctx, tx, f.OperatorID, r.ID, "receipt:unallocated")
		if err != nil {
			return err
		}
		sourcePurpose := "control:provider_receivable"
		if f.SettledAt != nil {
			sourcePurpose = "control:external_cash"
		}
		cash, err := s.account(ctx, tx, f.OperatorID, f.OperatorID, sourcePurpose)
		if err != nil {
			return err
		}
		net := f.GrossMoney
		if f.NetMoney != nil {
			net = *f.NetMoney
		}
		lines := []ledger.Line{{AccountID: unallocated.ID, Amount: f.GrossMoney}, {AccountID: cash.ID, Amount: net.Neg()}}
		if f.FeeMoney != nil && f.FeeMoney.Sign() > 0 {
			fees, err := s.account(ctx, tx, f.OperatorID, f.OperatorID, "control:merchant_fees")
			if err != nil {
				return err
			}
			lines = append(lines, ledger.Line{AccountID: fees.ID, Amount: f.FeeMoney.Neg()})
		}
		reason := f.VerificationEvidence
		if strings.TrimSpace(reason) == "" {
			reason = "Staff confirmed offline receipt"
		}
		if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: f.OperatorID, BusinessKey: "receipt:" + r.ID, Kind: "external_receipt", SourceID: r.ID, ActorID: f.VerifiedBy, Reason: reason, Lines: lines}); err != nil {
			return err
		}
		out = Receipt(r)
		return s.event(tx, f.OperatorID, r.ID, "receipt.verified", map[string]any{"actor_id": f.VerifiedBy, "evidence": f.VerificationEvidence, "before": nil, "after": out})
	})
	return
}

// AllocateReceipt can be called by verified matching workflows (empty actor)
// or finance staff (nonempty actor, permission checked here). HTTP handlers
// must never expose the empty-actor path directly to customers.
func (s *Service) AllocateReceipt(ctx context.Context, tx *gorm.DB, in AllocateReceiptInput) (out Allocation, err error) {
	if !validID(in.OperatorID) || !validID(in.ReceiptID) || !validID(in.OrderID) || strings.TrimSpace(in.BusinessKey) == "" || len(in.BusinessKey) > 150 || in.AmountMoney.Sign() <= 0 || !in.AmountMoney.Valid() {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if in.ActorStaffID != "" {
			if err := s.org.RequirePermission(ctx, tx, in.ActorStaffID, in.OperatorID, "payments.manage"); err != nil {
				return err
			}
		}
		if err := lockCommand(tx, in.OperatorID, "allocate:"+in.BusinessKey); err != nil {
			return err
		}
		var previous allocationRow
		q := tx.Where("operator_id=? AND business_key=?", in.OperatorID, in.BusinessKey).Limit(1).Find(&previous)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if previous.ReceiptID != in.ReceiptID || previous.OrderID != in.OrderID || previous.AmountMoney.Cmp(in.AmountMoney) != 0 {
				return ErrConflict
			}
			out = Allocation(previous)
			return nil
		}
		// Match fulfilment and financial remediation: ownership/business fences
		// precede receipt and order rows. In production the coverage adapter's
		// business lock serializes competing receipt->order / order->receipt paths.
		var observed orderRow
		if err := tx.First(&observed, "id=?", in.OrderID).Error; err != nil {
			return err
		}
		if observed.OperatorID != in.OperatorID {
			return ErrScope
		}
		if _, err := s.org.LockOwnership(ctx, tx, observed.CustomerID, in.OperatorID, observed.OwnershipVersion); err != nil {
			return err
		}
		if err := s.lockBusinessScope(ctx, tx, in.OperatorID); err != nil {
			return err
		}
		if s.receiptGuard != nil {
			if err := s.receiptGuard.RequireReceiptAllocation(ctx, tx, in.OperatorID, in.ReceiptID); err != nil {
				return err
			}
		}
		var receipt receiptRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&receipt, "id=?", in.ReceiptID).Error; err != nil {
			return err
		}
		var row orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", in.OrderID).Error; err != nil {
			return err
		}
		o := Order(row)
		before := o
		if receipt.OperatorID != in.OperatorID || o.OperatorID != in.OperatorID {
			return ErrScope
		}
		if receipt.PaidAt != nil && receipt.PaidAt.After(o.ExpiresAt) {
			return ErrLatePayment
		}
		if receipt.PaidAt == nil && receipt.ObservedAt.After(o.ExpiresAt) {
			return ErrTimingUnverified
		}
		if o.Status == "completed" {
			return ErrOrderState
		}
		if receipt.GrossMoney.Sub(receipt.AllocatedMoney).Cmp(in.AmountMoney) < 0 || externalDue(o).Sub(o.ExternalAllocatedMoney).Cmp(in.AmountMoney) < 0 {
			return ErrExcessAllocation
		}
		if !receipt.AllocatedMoney.IsZero() || !o.ExternalAllocatedMoney.IsZero() || in.AmountMoney.Cmp(externalDue(o)) != 0 {
			return ErrWholePayment
		}
		timingProof := "actual_provider_time"
		if receipt.PaidAt == nil {
			timingProof = "observed_before_expiry"
		}
		a := allocationRow{ID: uuid.NewString(), OperatorID: in.OperatorID, BusinessKey: in.BusinessKey, ReceiptID: receipt.ID, OrderID: o.ID, AmountMoney: in.AmountMoney, TimingProof: timingProof, CreatedAt: s.now()}
		if err := tx.Create(&a).Error; err != nil {
			return err
		}
		from, err := s.account(ctx, tx, o.OperatorID, receipt.ID, "receipt:unallocated")
		if err != nil {
			return err
		}
		to, err := s.account(ctx, tx, o.OperatorID, o.ID, "order:funds")
		if err != nil {
			return err
		}
		actor := in.ActorStaffID
		if actor == "" {
			actor = "system:verified-receipt-matching"
		}
		if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: o.OperatorID, BusinessKey: "allocation:" + a.ID, Kind: "receipt_allocation", SourceID: a.ID, ActorID: actor, Reason: "verified receipt assigned to original order", Lines: []ledger.Line{{AccountID: from.ID, Amount: in.AmountMoney.Neg()}, {AccountID: to.ID, Amount: in.AmountMoney}}}); err != nil {
			return err
		}
		o.ExternalAllocatedMoney = o.ExternalAllocatedMoney.Add(in.AmountMoney)
		if o.Status == "closed" || o.CoverageStatus == "released" || o.InternalStatus == "released" {
			o.Status = "exception"
			o.ExceptionReason = "timely_payment_requires_commitment_restoration"
		} else {
			o.Status = "paid"
		}
		if err = tx.Model(&row).Updates(map[string]any{"status": o.Status, "exception_reason": o.ExceptionReason}).Error; err != nil {
			return err
		}
		out = Allocation(a)
		return s.event(tx, o.OperatorID, o.ID, "order.receipt_allocated", map[string]any{"actor_id": actor, "evidence": receipt.VerificationEvidence, "before": before, "after": o, "allocation": out, "status": o.Status})
	})
	return
}

func (s *Service) UnallocatedReceipts(ctx context.Context, tx *gorm.DB, actorID, operatorID string) ([]Receipt, error) {
	if err := s.org.RequirePermission(ctx, tx, actorID, operatorID, "payments.manage"); err != nil {
		return nil, err
	}
	var rows []receiptRow
	if err := s.dbFor(ctx, tx).Where("operator_id=? AND allocated_money<gross_money", operatorID).Order("paid_at,id").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Receipt, len(rows))
	for i, r := range rows {
		out[i] = Receipt(r)
	}
	return out, nil
}
