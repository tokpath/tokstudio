package commerce

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MarkUndeliveredRefund stops delivery of a paid package that has not been
// granted. The original payment stays held until the refund is approved or the
// request is cancelled.
func (s *Service) MarkUndeliveredRefund(ctx context.Context, tx *gorm.DB, customerID, orderID string) (Order, error) {
	var out Order
	err := s.within(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.lockUndeliveredPackage(ctx, tx, customerID, orderID)
		if err != nil {
			return err
		}
		if o.ExceptionReason == "delivery_refund_requested" {
			out = o
			return nil
		}
		if o.Status != "paid" && o.Status != "exception" {
			return ErrOrderState
		}
		o.Status = "exception"
		o.ExceptionReason = "delivery_refund_requested"
		return s.saveOrder(tx, o, &out)
	})
	return out, err
}

// ClearUndeliveredRefund returns a not-yet-approved package to the delivery queue.
func (s *Service) ClearUndeliveredRefund(ctx context.Context, tx *gorm.DB, customerID, orderID string) error {
	return s.within(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.lockUndeliveredPackage(ctx, tx, customerID, orderID)
		if err != nil {
			return err
		}
		if o.ExceptionReason != "delivery_refund_requested" {
			return nil
		}
		o.ExceptionReason = ""
		o.Status = "paid"
		if o.CoverageStatus == "released" || o.InternalStatus == "released" {
			o.Status = "exception"
			o.ExceptionReason = "timely_payment_requires_commitment_restoration"
		}
		return s.saveOrder(tx, o, nil)
	})
}

// ReleaseUndeliveredPackage returns held wallet and commission funds and frees
// the reserved supply. External receipt funds stay on the order until the
// caller moves them into the refund payout.
func (s *Service) ReleaseUndeliveredPackage(ctx context.Context, tx *gorm.DB, customerID, orderID, actorID string) (Order, error) {
	var out Order
	err := s.within(ctx, tx, func(tx *gorm.DB) error {
		o, err := s.lockUndeliveredPackage(ctx, tx, customerID, orderID)
		if err != nil {
			return err
		}
		if o.ExceptionReason != "delivery_refund_requested" {
			return ErrOrderState
		}
		if o.InternalStatus == "held" && o.HoldID != nil {
			if o.CommissionMoney.Sign() > 0 {
				if s.commissionFunding == nil {
					return ErrNotReady
				}
				if err = s.commissionFunding.ReleaseCommissionFunding(ctx, tx, o.OperatorID, o.CustomerID, o.ID); err != nil {
					return err
				}
			}
			if _, err = s.ledger.Release(ctx, tx, o.OperatorID, *o.HoldID, "undelivered-refund:"+o.ID); err != nil {
				return err
			}
			o.InternalStatus = "released"
		}
		if o.CoverageStatus == "reserved" {
			if s.coverage == nil {
				return ErrNotReady
			}
			if err = s.coverage.Release(ctx, tx, o); err != nil {
				return err
			}
			o.CoverageStatus = "released"
		}
		if o.MarketingReference != "" {
			if s.marketing == nil {
				return ErrNotReady
			}
			if err = s.marketing.Release(ctx, tx, o); err != nil {
				return err
			}
		}
		now := s.now()
		o.Status = "closed"
		o.ExceptionReason = ""
		o.ClosedAt = &now
		_ = actorID
		return s.saveOrder(tx, o, &out)
	})
	return out, err
}

func (s *Service) lockUndeliveredPackage(ctx context.Context, tx *gorm.DB, customerID, orderID string) (Order, error) {
	var observed orderRow
	if err := tx.First(&observed, "id=? AND customer_id=?", orderID, customerID).Error; err != nil {
		return Order{}, err
	}
	if _, err := s.org.LockOwnership(ctx, tx, customerID, observed.OperatorID, observed.OwnershipVersion); err != nil {
		return Order{}, err
	}
	if err := s.lockBusinessScope(ctx, tx, observed.OperatorID); err != nil {
		return Order{}, err
	}
	var row orderRow
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=? AND customer_id=?", orderID, customerID).Error; err != nil {
		return Order{}, err
	}
	o := Order(row)
	if o.Kind != "package" && o.Kind != "renewal" {
		return Order{}, ErrOrderState
	}
	var deliveries int64
	if err := tx.Table("commerce_deliveries").Where("order_id=?", o.ID).Count(&deliveries).Error; err != nil {
		return Order{}, err
	}
	if deliveries > 0 || o.Status == "completed" {
		return Order{}, ErrOrderState
	}
	return o, nil
}

func (s *Service) saveOrder(tx *gorm.DB, o Order, out *Order) error {
	row := orderRow(o)
	if err := tx.Save(&row).Error; err != nil {
		return err
	}
	if out != nil {
		*out = Order(row)
	}
	return nil
}
