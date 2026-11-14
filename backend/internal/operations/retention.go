package operations

import (
	"context"
	"errors"
	"time"
)

type RetentionResult struct {
	ExportsPurged      int64 `json:"exports_purged"`
	ModelResultsPurged int64 `json:"model_results_purged"`
}

func (s *Service) RunRetention(ctx context.Context, at time.Time, limit int) (RetentionResult, error) {
	if at.IsZero() || at.After(time.Now().Add(time.Minute)) {
		return RetentionResult{}, ErrInvalid
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	var out RetentionResult
	var expired []Export
	if err := s.DB.WithContext(ctx).Where("expires_at<=? AND state<>'expired'", at).Order("expires_at,id").Limit(limit).Find(&expired).Error; err != nil {
		return out, err
	}
	var cleanupErr error
	for _, row := range expired {
		if row.ObjectKey != "" {
			if s.ExportStore == nil {
				return out, ErrNotReady
			}
			if err := s.ExportStore.Delete(ctx, row.ObjectKey); err != nil {
				cleanupErr = errors.Join(cleanupErr, err)
				continue
			}
		}
		r := s.DB.WithContext(ctx).Model(&Export{}).Where("id=? AND state<>'expired'", row.ID).
			Updates(map[string]any{"object_key": "", "state": "expired", "lease_until": nil, "attempt_id": nil})
		if r.Error != nil {
			return out, r.Error
		}
		out.ExportsPurged += r.RowsAffected
	}
	if s.Results == nil {
		return out, errors.Join(cleanupErr, ErrNotReady)
	}
	var e error
	out.ModelResultsPurged, e = s.Results.PurgeExpiredResults(ctx, at, limit)
	return out, errors.Join(cleanupErr, e)
}
