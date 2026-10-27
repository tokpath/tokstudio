package operations

import (
	"context"
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
	r := s.DB.WithContext(ctx).Model(&Export{}).Where("expires_at<=? AND state<>'expired'", at).Updates(map[string]any{"content_ciphertext": "", "state": "expired", "lease_until": nil})
	out.ExportsPurged = r.RowsAffected
	if r.Error != nil {
		return out, r.Error
	}
	if s.Results == nil {
		return out, ErrNotReady
	}
	var e error
	out.ModelResultsPurged, e = s.Results.PurgeExpiredResults(ctx, at, limit)
	return out, e
}
