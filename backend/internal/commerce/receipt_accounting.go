package commerce

import (
	"context"
	"time"

	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReceiptAccounting is a verified provider settlement/fee fact. The original
// gross customer payment and all previous allocations remain immutable.
type ReceiptAccounting struct {
	ReceiptID  string         `json:"receipt_id"`
	NetUSD     ledger.Decimal `json:"net_usd"`
	FeeUSD     ledger.Decimal `json:"fee_usd"`
	SettledAt  *time.Time     `json:"settled_at"`
	Evidence   string         `json:"evidence"`
	VerifiedBy string         `json:"verified_by"`
}

func (s *Service) RecordReceiptAccounting(ctx context.Context, tx *gorm.DB, in ReceiptAccounting) (out Receipt, err error) {
	if !validID(in.ReceiptID) || in.Evidence == "" || in.VerifiedBy == "" || in.NetUSD.Sign() < 0 || in.FeeUSD.Sign() < 0 || !in.NetUSD.Valid() || !in.FeeUSD.Valid() {
		return out, ErrInvalidInput
	}
	if in.SettledAt != nil {
		v := in.SettledAt.UTC()
		in.SettledAt = &v
		if v.IsZero() || v.After(s.now().Add(5*time.Minute)) {
			return out, ErrInvalidInput
		}
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		var r receiptRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", in.ReceiptID).Error; err != nil {
			return err
		}
		if in.NetUSD.Add(in.FeeUSD).Cmp(r.GrossUSD) != 0 {
			return ErrInvalidInput
		}
		if in.SettledAt != nil && r.PaidAt != nil && in.SettledAt.Before(*r.PaidAt) {
			return ErrInvalidInput
		}
		receivable, err := s.account(ctx, tx, r.OperatorID, r.OperatorID, "control:provider_receivable")
		if err != nil {
			return err
		}
		changed := false
		if r.FeeStatus == "confirmed" {
			if r.NetUSD == nil || r.FeeUSD == nil || r.NetUSD.Cmp(in.NetUSD) != 0 || r.FeeUSD.Cmp(in.FeeUSD) != 0 {
				return ErrConflict
			}
		} else {
			if in.FeeUSD.Sign() > 0 {
				fees, err := s.account(ctx, tx, r.OperatorID, r.OperatorID, "control:merchant_fees")
				if err != nil {
					return err
				}
				if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: r.OperatorID, BusinessKey: "receipt-fees:" + r.ID, Kind: "merchant_fee", SourceID: r.ID, ActorID: in.VerifiedBy, Reason: in.Evidence, Lines: []ledger.Line{{AccountID: receivable.ID, Amount: in.FeeUSD}, {AccountID: fees.ID, Amount: in.FeeUSD.Neg()}}}); err != nil {
					return err
				}
			}
			net, fee := in.NetUSD, in.FeeUSD
			r.NetUSD = &net
			r.FeeUSD = &fee
			r.FeeStatus = "confirmed"
			changed = true
		}
		if in.SettledAt != nil {
			if r.SettledAt != nil {
				if !r.SettledAt.Equal(*in.SettledAt) {
					return ErrConflict
				}
			} else {
				if in.NetUSD.Sign() > 0 {
					cash, err := s.account(ctx, tx, r.OperatorID, r.OperatorID, "control:external_cash")
					if err != nil {
						return err
					}
					if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: r.OperatorID, BusinessKey: "receipt-settlement:" + r.ID, Kind: "provider_settlement", SourceID: r.ID, ActorID: in.VerifiedBy, Reason: in.Evidence, Lines: []ledger.Line{{AccountID: receivable.ID, Amount: in.NetUSD}, {AccountID: cash.ID, Amount: in.NetUSD.Neg()}}}); err != nil {
						return err
					}
				}
				r.SettledAt = in.SettledAt
				changed = true
			}
		}
		if changed {
			if err = tx.Save(&r).Error; err != nil {
				return err
			}
			if err = s.event(tx, r.OperatorID, r.ID, "receipt.accounting_confirmed", in); err != nil {
				return err
			}
		}
		out = Receipt(r)
		return nil
	})
	return
}

// ConfirmReceiptPaymentTime adds real provider timing to an earlier success
// observation. Conflicting evidence never silently rewrites funded orders.
func (s *Service) ConfirmReceiptPaymentTime(ctx context.Context, tx *gorm.DB, receiptID string, paidAt time.Time, evidence, verifiedBy string) (out Receipt, err error) {
	if !validID(receiptID) || paidAt.IsZero() || evidence == "" || verifiedBy == "" || paidAt.After(s.now().Add(5*time.Minute)) {
		return out, ErrInvalidInput
	}
	paidAt = paidAt.UTC()
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		var r receiptRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", receiptID).Error; err != nil {
			return err
		}
		if r.PaidAt != nil {
			if !r.PaidAt.Equal(paidAt) {
				return ErrConflict
			}
			out = Receipt(r)
			return nil
		}
		var conflicting int64
		if err := tx.Table("commerce_receipt_allocations a").Joins("JOIN commerce_orders o ON o.id=a.order_id").Where("a.receipt_id=? AND o.expires_at<?", receiptID, paidAt).Count(&conflicting).Error; err != nil {
			return err
		}
		if conflicting > 0 {
			return ErrConflict
		}
		if err := tx.Table("commerce_receipt_funds_reservations").Where("receipt_id=? AND deadline<?", receiptID, paidAt).Count(&conflicting).Error; err != nil {
			return err
		}
		if conflicting > 0 {
			return ErrConflict
		}
		r.PaidAt = &paidAt
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		out = Receipt(r)
		return s.event(tx, r.OperatorID, r.ID, "receipt.payment_time_confirmed", map[string]any{"paid_at": paidAt, "evidence": evidence, "verified_by": verifiedBy})
	})
	return
}
