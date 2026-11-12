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
	OriginalFeeAmount      *ledger.Decimal `json:"original_fee_amount,omitempty"`
	OriginalNetAmount      *ledger.Decimal `json:"original_net_amount,omitempty"`
	SettlementFeeConfirmed bool            `json:"settlement_fee_confirmed"`
	ReceiptID              string          `json:"receipt_id"`
	NetMoney               ledger.Decimal  `json:"net_money"`
	FeeMoney               ledger.Decimal  `json:"fee_money"`
	SettledAt              *time.Time      `json:"settled_at"`
	Evidence               string          `json:"evidence"`
	VerifiedBy             string          `json:"verified_by"`
}

func (s *Service) RecordReceiptAccounting(ctx context.Context, tx *gorm.DB, in ReceiptAccounting) (out Receipt, err error) {
	if in.Evidence == "" {
		in.Evidence = "receipt-accounting:" + in.ReceiptID
	}
	if !validID(in.ReceiptID) || in.VerifiedBy == "" || in.NetMoney.Sign() < 0 || in.FeeMoney.Sign() < 0 || !in.NetMoney.Valid() || !in.FeeMoney.Valid() {
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
		base, e := ledger.GetMonetary(ctx, tx)
		if e != nil {
			return e
		}
		if r.Currency == base.Currency {
			if in.SettlementFeeConfirmed {
				if in.OriginalFeeAmount != nil && in.OriginalFeeAmount.Cmp(in.FeeMoney) != 0 {
					return ErrInvalidInput
				}
				fee, net := in.FeeMoney, in.NetMoney
				in.OriginalFeeAmount, in.OriginalNetAmount = &fee, &net
			} else if in.OriginalFeeAmount != nil {
				in.FeeMoney = *in.OriginalFeeAmount
				original, err := ledger.ParseDecimal(r.OriginalAmount)
				if err != nil {
					return err
				}
				in.NetMoney = original.Sub(in.FeeMoney)
				in.SettlementFeeConfirmed = true
			}
		}
		if in.OriginalFeeAmount != nil {
			original, e := ledger.ParseDecimal(r.OriginalAmount)
			if e != nil || !in.OriginalFeeAmount.Valid() || in.OriginalFeeAmount.Sign() < 0 || in.OriginalFeeAmount.Cmp(original) > 0 {
				return ErrInvalidInput
			}
			net := original.Sub(*in.OriginalFeeAmount)
			if in.OriginalNetAmount != nil && in.OriginalNetAmount.Cmp(net) != 0 {
				return ErrInvalidInput
			}
			if r.OriginalFeeAmount != nil && r.OriginalFeeAmount.Cmp(*in.OriginalFeeAmount) != 0 {
				return ErrConflict
			}
			r.OriginalFeeAmount = in.OriginalFeeAmount
			r.OriginalNetAmount = &net
			if e = tx.Model(&r).Updates(map[string]any{"original_fee_amount": r.OriginalFeeAmount, "original_net_amount": r.OriginalNetAmount}).Error; e != nil {
				return e
			}
		}
		if !in.SettlementFeeConfirmed {
			if r.OriginalNetAmount != nil {
				if err := s.RecordReceiptSettlement(ctx, tx, r.ID, r.Currency, *r.OriginalNetAmount, in.VerifiedBy); err != nil {
					return err
				}
				r.SettlementCurrency, r.SettlementNetAmount = r.Currency, r.OriginalNetAmount
			}
			out = Receipt(r)
			return s.event(tx, r.OperatorID, r.ID, "receipt.original_fee_confirmed", in)
		}
		original, parseErr := ledger.ParseDecimal(r.OriginalAmount)
		if parseErr != nil {
			return parseErr
		}
		if r.Currency == base.Currency && in.NetMoney.Add(in.FeeMoney).Cmp(original) != 0 {
			return ErrInvalidInput
		}
		if in.SettledAt != nil && r.PaidAt != nil && in.SettledAt.Before(*r.PaidAt) {
			return ErrInvalidInput
		}

		changed := false
		if r.FeeStatus == "confirmed" {
			if r.NetMoney == nil || r.FeeMoney == nil || r.NetMoney.Cmp(in.NetMoney) != 0 || r.FeeMoney.Cmp(in.FeeMoney) != 0 {
				return ErrConflict
			}
		} else {
			net, fee := in.NetMoney, in.FeeMoney
			r.NetMoney = &net
			r.FeeMoney = &fee
			r.FeeStatus = "confirmed"
			changed = true
		}
		if in.SettledAt != nil {
			if r.SettledAt != nil {
				if !r.SettledAt.Equal(*in.SettledAt) {
					return ErrConflict
				}
			} else {
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
		if r.OriginalNetAmount != nil {
			if err := s.RecordReceiptSettlement(ctx, tx, r.ID, r.Currency, *r.OriginalNetAmount, in.VerifiedBy); err != nil {
				return err
			}
			r.SettlementCurrency, r.SettlementNetAmount = r.Currency, r.OriginalNetAmount
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
