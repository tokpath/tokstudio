package remediation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/commerce"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
)

type ReceiptCapacity struct {
	AvailableMoney ledger.Decimal `json:"available_money"`
	ReservedMoney  ledger.Decimal `json:"reserved_money"`
	CompletedMoney ledger.Decimal `json:"completed_money"`
	ReviewRequired bool           `json:"review_required"`
	Evidence       string         `json:"evidence"`
}

// RequireReceiptAllocation blocks a new use of a disputed or invalid source.
// It never rejects recording the independently verified receipt itself.
func (s *Service) RequireReceiptAllocation(ctx context.Context, t *gorm.DB, op, receipt string) error {
	if t == nil {
		return ErrInvalid
	}
	if e := lock(t, op+":receipt:"+receipt); e != nil {
		return e
	}
	if _, e := s.Commerce.LockReceipt(ctx, t, op, receipt); e != nil {
		return e
	}
	var count int64
	if e := t.Table("remediation_chargebacks").Where("operator_id=? AND receipt_id=? AND state='disputed' AND restrictions_applied_at IS NULL", op, receipt).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return commerce.ErrReceiptRestricted
	}
	return nil
}

type capacity struct {
	OperatorID, ReceiptID, Reference string
	AmountMoney                      ledger.Decimal
	CompletedMoney                   ledger.Decimal
	State                            string
	CreatedAt                        time.Time
	ResolvedAt                       *time.Time
}

func (capacity) TableName() string { return "remediation_refund_capacity" }
func (s *Service) ReceiptRefundCapacity(ctx context.Context, tx *gorm.DB, op, receipt string) (ReceiptCapacity, error) {
	var out ReceiptCapacity
	e := s.within(ctx, tx, func(t *gorm.DB) error {
		r, e := s.Commerce.GetReceipt(ctx, t, op, receipt)
		if e != nil {
			return e
		}
		var v struct{ Reserved, Completed ledger.Decimal }
		if e = t.Model(&capacity{}).Select("COALESCE(sum(CASE WHEN state='reserved' THEN amount_money-completed_money ELSE 0 END),0)::text AS reserved,COALESCE(sum(completed_money),0)::text AS completed").Where("operator_id=? AND receipt_id=?", op, receipt).Scan(&v).Error; e != nil {
			return e
		}
		var c struct {
			Amount ledger.Decimal
			Review bool
		}
		if e = t.Table("remediation_chargebacks").Select("COALESCE(sum(CASE WHEN state='invalid_payment' THEN amount_money ELSE 0 END),0)::text AS amount,COALESCE(bool_or(state='disputed'),false) AS review").Where("operator_id=? AND receipt_id=?", op, receipt).Scan(&c).Error; e != nil {
			return e
		}
		out = ReceiptCapacity{AvailableMoney: r.GrossMoney.Sub(v.Reserved).Sub(v.Completed).Sub(c.Amount), ReservedMoney: v.Reserved, CompletedMoney: v.Completed, ReviewRequired: c.Review, Evidence: fmt.Sprintf("receipt:%s/reserved:%s/completed:%s/invalid:%s", receipt, v.Reserved.String(), v.Completed.String(), c.Amount.String())}
		if out.AvailableMoney.Sign() < 0 {
			out.AvailableMoney = ledger.Zero()
			out.ReviewRequired = true
		}
		return nil
	})
	return out, e
}
func (s *Service) ReserveReceiptRefund(ctx context.Context, t *gorm.DB, op, receipt, reference string, amount ledger.Decimal) error {
	if t == nil || reference == "" || !validAmount(amount) {
		return ErrInvalid
	}
	if e := lock(t, op+":receipt:"+receipt); e != nil {
		return e
	}
	var old capacity
	e := t.First(&old, "operator_id=? AND receipt_id=? AND reference=?", op, receipt, reference).Error
	if e == nil {
		if old.AmountMoney.Cmp(amount) != 0 || old.State == "released" {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	c, e := s.ReceiptRefundCapacity(ctx, t, op, receipt)
	if e != nil {
		return e
	}
	if c.ReviewRequired {
		return ErrReview
	}
	if c.AvailableMoney.Cmp(amount) < 0 {
		return ErrConflict
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	return t.Create(&capacity{OperatorID: op, ReceiptID: receipt, Reference: reference, AmountMoney: amount, State: "reserved", CreatedAt: at}).Error
}

// RequireReceiptRefundExecution gates a NEW external instruction. A verified
// transfer already executed must instead remain recordable through Complete.
// Callers take their ownership/business fences before this shared receipt lock.
func (s *Service) RequireReceiptRefundExecution(ctx context.Context, t *gorm.DB, op, receipt, reference string) error {
	if t == nil || !validID(op) || !validID(receipt) || reference == "" {
		return ErrInvalid
	}
	if e := lock(t, op+":receipt:"+receipt); e != nil {
		return e
	}
	if _, e := s.Commerce.LockReceipt(ctx, t, op, receipt); e != nil {
		return e
	}
	var reservation capacity
	if e := t.First(&reservation, "operator_id=? AND receipt_id=? AND reference=?", op, receipt, reference).Error; e != nil {
		return e
	}
	if reservation.State != "reserved" {
		return ErrState
	}
	current, e := s.ReceiptRefundCapacity(ctx, t, op, receipt)
	if e != nil {
		return e
	}
	if current.ReviewRequired {
		return ErrReview
	}
	return nil
}
func (s *Service) resolveReceiptCapacity(ctx context.Context, t *gorm.DB, op, receipt, reference, state string) error {
	if t == nil {
		return ErrInvalid
	}
	if e := lock(t, op+":receipt:"+receipt); e != nil {
		return e
	}
	var old capacity
	if e := t.First(&old, "operator_id=? AND receipt_id=? AND reference=?", op, receipt, reference).Error; e != nil {
		return e
	}
	if old.State == state {
		return nil
	}
	if old.State != "reserved" {
		return ErrState
	}
	at, e := instant(t)
	if e != nil {
		return e
	}
	changes := map[string]any{"state": state, "resolved_at": at}
	if state == "completed" {
		changes["completed_money"] = old.AmountMoney
	}
	return t.Model(&capacity{}).Where("operator_id=? AND receipt_id=? AND reference=?", op, receipt, reference).Updates(changes).Error
}

type ReceiptRefundObservation struct {
	OperatorID     string         `json:"operator_id"`
	ReceiptID      string         `json:"receipt_id"`
	Reference      string         `json:"reference"`
	FactID         string         `json:"fact_id"`
	ActualMoney    ledger.Decimal `json:"actual_money"`
	SettledMoney   ledger.Decimal `json:"settled_money"`
	ExcessMoney    ledger.Decimal `json:"excess_money"`
	RemainingMoney ledger.Decimal `json:"remaining_money"`
	CreatedAt      time.Time      `json:"created_at"`
	Replayed       bool           `json:"replayed" gorm:"-"`
}

func (ReceiptRefundObservation) TableName() string { return "remediation_receipt_refund_observations" }

// ObserveReceiptRefund consumes only actual verified repayment of this one
// reserved obligation. An overpayment remains evidence but cannot consume a
// different customer's capacity. The caller records the full cash movement.
func (s *Service) ObserveReceiptRefund(ctx context.Context, t *gorm.DB, op, receipt, reference, factID string, actualMoney ledger.Decimal) (ReceiptRefundObservation, error) {
	var out ReceiptRefundObservation
	if t == nil || !validID(op) || !validID(receipt) || reference == "" || factID == "" || len(factID) > 500 || !validAmount(actualMoney) {
		return out, ErrInvalid
	}
	if e := lock(t, op+":receipt:"+receipt); e != nil {
		return out, e
	}
	if _, e := s.Commerce.LockReceipt(ctx, t, op, receipt); e != nil {
		return out, e
	}
	if e := lock(t, op+":refund-fact:"+factID); e != nil {
		return out, e
	}
	e := t.First(&out, "operator_id=? AND fact_id=?", op, factID).Error
	if e == nil {
		if out.ReceiptID != receipt || out.Reference != reference || out.ActualMoney.Cmp(actualMoney) != 0 {
			return out, ErrConflict
		}
		out.Replayed = true
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	var c capacity
	if e = t.First(&c, "operator_id=? AND receipt_id=? AND reference=?", op, receipt, reference).Error; e != nil {
		return out, e
	}
	if c.State != "reserved" && c.State != "completed" {
		return out, ErrState
	}
	remaining := c.AmountMoney.Sub(c.CompletedMoney)
	settled := min(actualMoney, remaining)
	at, e := instant(t)
	if e != nil {
		return out, e
	}
	out = ReceiptRefundObservation{OperatorID: op, ReceiptID: receipt, Reference: reference, FactID: factID, ActualMoney: actualMoney, SettledMoney: settled, ExcessMoney: actualMoney.Sub(settled), RemainingMoney: remaining.Sub(settled), CreatedAt: at}
	if e = t.Create(&out).Error; e != nil {
		return out, e
	}
	changes := map[string]any{"completed_money": c.CompletedMoney.Add(settled)}
	if out.RemainingMoney.IsZero() {
		changes["state"] = "completed"
		changes["resolved_at"] = at
	}
	if e = t.Model(&capacity{}).Where("operator_id=? AND receipt_id=? AND reference=?", op, receipt, reference).Updates(changes).Error; e != nil {
		return out, e
	}
	return out, nil
}
func (s *Service) CompleteReceiptRefund(ctx context.Context, t *gorm.DB, op, receipt, reference string) error {
	return s.resolveReceiptCapacity(ctx, t, op, receipt, reference, "completed")
}
func (s *Service) ReleaseReceiptRefund(ctx context.Context, t *gorm.DB, op, receipt, reference string) error {
	return s.resolveReceiptCapacity(ctx, t, op, receipt, reference, "released")
}
