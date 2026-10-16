package commerce

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReceiptFundsInput struct {
	OperatorID    string         `json:"operator_id"`
	ReceiptID     string         `json:"receipt_id"`
	DestinationID string         `json:"destination_id"`
	BusinessKey   string         `json:"business_key"`
	ActorID       string         `json:"actor_id"`
	AmountUSD     ledger.Decimal `json:"amount_usd"`
	Deadline      time.Time      `json:"deadline"`
}
type ReceiptFundsReservation struct {
	ID            string         `json:"id"`
	OperatorID    string         `json:"operator_id"`
	ReceiptID     string         `json:"receipt_id"`
	DestinationID string         `json:"destination_id"`
	BusinessKey   string         `json:"business_key"`
	AmountUSD     ledger.Decimal `json:"amount_usd"`
	Deadline      time.Time      `json:"deadline"`
	TimingProof   string         `json:"timing_proof"`
	CreatedAt     time.Time      `json:"created_at"`
}

func (ReceiptFundsReservation) TableName() string { return "commerce_receipt_funds_reservations" }

func (s *Service) GetReceipt(ctx context.Context, tx *gorm.DB, operatorID, receiptID string) (Receipt, error) {
	if !validID(operatorID) || !validID(receiptID) {
		return Receipt{}, ErrInvalidInput
	}
	var r receiptRow
	if err := s.dbFor(ctx, tx).First(&r, "id=? AND operator_id=?", receiptID, operatorID).Error; err != nil {
		return Receipt{}, classify(err)
	}
	return Receipt(r), nil
}

// ReserveReceiptFunds is exclusively for trusted cross-module order
// orchestrators (for example procurement). The caller validates its destination
// order under a lock and must use this same transaction for its own allocation.
// It is never an HTTP action and does not grant a customer transfer capability.
func (s *Service) ReserveReceiptFunds(ctx context.Context, tx *gorm.DB, in ReceiptFundsInput) (out ReceiptFundsReservation, err error) {
	if tx == nil || !validID(in.OperatorID) || !validID(in.ReceiptID) || !validID(in.DestinationID) || in.BusinessKey == "" || len(in.BusinessKey) > 150 || in.ActorID == "" || in.Deadline.IsZero() || in.AmountUSD.Sign() <= 0 || !in.AmountUSD.Valid() {
		return out, ErrInvalidInput
	}
	err = s.within(ctx, tx, func(tx *gorm.DB) error {
		if err := lockCommand(tx, in.OperatorID, "external-allocation:"+in.BusinessKey); err != nil {
			return err
		}
		var existing ReceiptFundsReservation
		q := tx.Where("operator_id=? AND business_key=?", in.OperatorID, in.BusinessKey).Limit(1).Find(&existing)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected > 0 {
			if existing.ReceiptID != in.ReceiptID || existing.DestinationID != in.DestinationID || existing.AmountUSD.Cmp(in.AmountUSD) != 0 || !existing.Deadline.Equal(in.Deadline) {
				return ErrConflict
			}
			out = existing
			return nil
		}
		if s.receiptGuard != nil {
			if err := s.receiptGuard.RequireReceiptAllocation(ctx, tx, in.OperatorID, in.ReceiptID); err != nil {
				return err
			}
		}
		var r receiptRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&r, "id=?", in.ReceiptID).Error; err != nil {
			return err
		}
		if r.OperatorID != in.OperatorID {
			return ErrScope
		}
		proof := "actual_provider_time"
		if r.PaidAt != nil {
			if r.PaidAt.After(in.Deadline) {
				return ErrLatePayment
			}
		} else {
			if r.ObservedAt.After(in.Deadline) {
				return ErrTimingUnverified
			}
			proof = "observed_before_expiry"
		}
		if r.GrossUSD.Sub(r.AllocatedUSD).Cmp(in.AmountUSD) < 0 {
			return ErrExcessAllocation
		}
		out = ReceiptFundsReservation{ID: uuid.NewString(), OperatorID: in.OperatorID, ReceiptID: in.ReceiptID, DestinationID: in.DestinationID, BusinessKey: in.BusinessKey, AmountUSD: in.AmountUSD, Deadline: in.Deadline.UTC(), TimingProof: proof, CreatedAt: s.now()}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		from, err := s.account(ctx, tx, in.OperatorID, in.ReceiptID, "receipt:unallocated")
		if err != nil {
			return err
		}
		to, err := s.account(ctx, tx, in.OperatorID, in.DestinationID, "order:funds")
		if err != nil {
			return err
		}
		if _, err = s.ledger.Post(ctx, tx, ledger.Posting{OperatorID: in.OperatorID, BusinessKey: "external-allocation:" + out.ID, Kind: "receipt_allocation", SourceID: out.ID, ActorID: in.ActorID, Reason: "verified receipt assigned to trusted external order", Lines: []ledger.Line{{AccountID: from.ID, Amount: in.AmountUSD.Neg()}, {AccountID: to.ID, Amount: in.AmountUSD}}}); err != nil {
			return err
		}
		return s.event(tx, in.OperatorID, in.DestinationID, "receipt.external_order_allocated", out)
	})
	return
}
