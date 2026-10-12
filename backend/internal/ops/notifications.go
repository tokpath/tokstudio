package ops

import (
	"context"
	"time"
)

func (s *Service) NotificationAlertOpen(ctx context.Context, alertID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&alertRow{}).Where("id=? AND status=?", alertID, StatusOpen).Count(&count).Error
	return count > 0, err
}

func (s *Service) RunAlerts(ctx context.Context, report func(error)) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if _, err := s.EvaluateAlerts(ctx); err != nil && report != nil {
			report(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
