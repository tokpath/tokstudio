package payment

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// ConfirmRecorded only accepts manual pending receipts or a paid order's original fulfillment.
func (s *Service) ConfirmRecorded(ctx context.Context, orderID, actor string, occurred time.Time) (*OrderView, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.Status == StatusPending {
			if row.Adapter != AdapterManual {
				return ErrOrderNotPending
			}
			if occurred.IsZero() || occurred.After(time.Now().Add(5*time.Minute)) || actor == "" {
				return ErrInvalidAmount
			}
			occurred = occurred.UTC()
			row.ReceivedAt = &occurred
			row.RecordedBy = actor
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		} else if row.Status != StatusPaid {
			return ErrOrderNotPending
		}
		scoped := *s
		scoped.db = tx
		_, err := scoped.ConfirmManual(ctx, orderID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, orderID, "")
}
func (s *Service) RefundRecorded(ctx context.Context, orderID, actor string, occurred time.Time) (*OrderView, error) {
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row orderRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", orderID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if row.Status == StatusRefunded {
			return nil
		}
		if row.Adapter == AdapterManual && (occurred.IsZero() || occurred.After(time.Now().Add(5*time.Minute))) {
			return ErrInvalidAmount
		}
		scoped := *s
		scoped.db = tx
		if _, err := scoped.Refund(ctx, orderID); err != nil {
			return err
		}
		updates := map[string]any{"refund_recorded_by": actor}
		if row.Adapter == AdapterManual {
			updates["refunded_at"] = occurred.UTC()
		}
		return tx.Model(&orderRow{}).Where("id = ?", orderID).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, orderID, "")
}
