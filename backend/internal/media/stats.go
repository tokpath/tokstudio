package media

import (
	"context"
	"sort"
)

func (s *Service) CallbackLatencyP95MS(ctx context.Context) (int64, error) {
	var values []int64
	if err := s.db.WithContext(ctx).Raw(`
		SELECT GREATEST(0, FLOOR(EXTRACT(EPOCH FROM (e.processed_at - j.created_at)) * 1000))::bigint
		FROM media_callback_events e
		JOIN media_jobs j ON j.id = e.media_job_id
		WHERE e.signature_valid AND e.media_job_id IS NOT NULL AND e.media_job_id <> ''
		ORDER BY 1
		LIMIT 500
	`).Scan(&values).Error; err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	p95 := (len(values) * 95) / 100
	if p95 >= len(values) {
		p95 = len(values) - 1
	}
	return values[p95], nil
}
