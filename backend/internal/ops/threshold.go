package ops

import (
	"context"
	"time"
)

const ThresholdID = "thr_default"

type thresholdRow struct {
	ID             string    `gorm:"column:id;primaryKey"`
	SuccessRateMin float64   `gorm:"column:success_rate_min"`
	MinRequests    int64     `gorm:"column:min_requests"`
	PendingCount   int64     `gorm:"column:pending_count"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (thresholdRow) TableName() string { return "ops_alert_thresholds" }

func DefaultThresholds() Thresholds {
	return Thresholds{SuccessRateMin: 0.5, MinRequests: 5, PendingCount: 1}
}

func ClampThresholds(in Thresholds) Thresholds {
	out := DefaultThresholds()
	if in.SuccessRateMin > 0 && in.SuccessRateMin <= 1 {
		out.SuccessRateMin = in.SuccessRateMin
	}
	if in.MinRequests >= 1 {
		out.MinRequests = in.MinRequests
	}
	if in.MinRequests > 10000 {
		out.MinRequests = 10000
	}
	if in.PendingCount >= 1 {
		out.PendingCount = in.PendingCount
	}
	return out
}

func (s *Service) Thresholds(ctx context.Context) (*Thresholds, error) {
	row := thresholdRow{ID: ThresholdID}
	_ = s.db.WithContext(ctx).Where("id = ?", ThresholdID).FirstOrCreate(&row, defaultThresholdRow()).Error
	if row.ID == "" {
		def := DefaultThresholds()
		return &def, nil
	}
	view := ClampThresholds(Thresholds{
		SuccessRateMin: row.SuccessRateMin, MinRequests: row.MinRequests, PendingCount: row.PendingCount,
	})
	return &view, nil
}

func (s *Service) SetThresholds(ctx context.Context, in Thresholds) (*Thresholds, error) {
	view := ClampThresholds(in)
	row := defaultThresholdRow()
	row.SuccessRateMin = view.SuccessRateMin
	row.MinRequests = view.MinRequests
	row.PendingCount = view.PendingCount
	row.UpdatedAt = time.Now().UTC()
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	return &view, nil
}

func defaultThresholdRow() thresholdRow {
	def := DefaultThresholds()
	return thresholdRow{
		ID: ThresholdID, SuccessRateMin: def.SuccessRateMin, MinRequests: def.MinRequests,
		PendingCount: def.PendingCount, UpdatedAt: time.Now().UTC(),
	}
}
