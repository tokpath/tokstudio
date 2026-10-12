package plans

import (
	"context"
	"time"

	"gorm.io/gorm"
)

func subscriptionNotice(row subRow) map[string]any {
	return map[string]any{"user_id": row.UserID, "subscription_id": row.ID, "plan_id": row.PlanID,
		"period_end": row.CurrentPeriodEnd, "next_retry_at": row.NextRetryAt, "attempt": row.RetryCount}
}

func (s *Service) saveSubscriptionNotice(tx *gorm.DB, row *subRow, kind string) error {
	if err := tx.Save(row).Error; err != nil {
		return err
	}
	// One failed-period notice even when payment retries run several times.
	if row.CurrentPeriodEnd == nil {
		return nil
	}
	inserted := tx.Exec(`INSERT INTO plans_notification_marks(subscription_id, period_end, kind)
        VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, row.ID, *row.CurrentPeriodEnd, kind)
	if inserted.Error != nil {
		return inserted.Error
	}
	if inserted.RowsAffected == 0 {
		return nil
	}
	_, err := s.outbox.EnqueueTx(tx, kind, "subscription", row.ID, subscriptionNotice(*row))
	return err
}

// ProcessExpiryReminders emits one reminder at 3 days and one at 1 day per period.
// Downtime that skips the earlier window never causes two reminders on recovery.
func (s *Service) ProcessExpiryReminders(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []subRow
		if err := tx.Where("status IN ? AND current_period_end > ? AND current_period_end <= ?",
			[]string{SubActive, SubCancelAtPeriodEnd}, now, now.Add(72*time.Hour)).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			kind := "plans.subscription.expiring_3d"
			if !row.CurrentPeriodEnd.After(now.Add(24 * time.Hour)) {
				kind = "plans.subscription.expiring_1d"
			}
			res := tx.Exec(`INSERT INTO plans_notification_marks(subscription_id, period_end, kind)
                VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, row.ID, *row.CurrentPeriodEnd, kind)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				continue
			}
			if _, err := s.outbox.EnqueueTx(tx, kind, "subscription", row.ID, subscriptionNotice(row)); err != nil {
				return err
			}
		}
		return nil
	})
}
