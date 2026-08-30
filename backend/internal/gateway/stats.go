package gateway

import (
	"context"
	"sort"
	"time"
)

type DailyStat struct {
	Day       string
	Requests  int64
	Successes int64
	Errors    int64
}

type TrafficStat struct {
	Dimension    string
	Key          string
	Requests     int64
	Successes    int64
	Errors       int64
	SuccessRate  float64
	LatencyP50MS int64
	LatencyP95MS int64
	LatencyP99MS int64
	Fallbacks    int64
	HTTP429      int64
	HTTP5xx      int64
	Timeouts     int64
}

func (s *Service) DimStats(ctx context.Context, dimension string) ([]TrafficStat, error) {
	type row struct {
		Key       string
		Requests  int64
		Successes int64
		Errors    int64
		Fallbacks int64
		HTTP429   int64 `gorm:"column:http429"`
		HTTP5xx   int64 `gorm:"column:http5xx"`
		Timeouts  int64 `gorm:"column:timeouts"`
	}
	var grouped []row
	switch dimension {
	case "provider":
		if err := s.db.WithContext(ctx).Raw(`
			SELECT provider_id AS key,
				COUNT(*) AS requests,
				COUNT(*) FILTER (WHERE status = 'succeeded') AS successes,
				COUNT(*) FILTER (WHERE status = 'failed') AS errors,
				COUNT(*) FILTER (WHERE attempt_no > 1) AS fallbacks,
				COUNT(*) FILTER (WHERE http_status = 429) AS http429,
				COUNT(*) FILTER (WHERE http_status >= 500) AS http5xx,
				COUNT(*) FILTER (WHERE error_code IN ('timeout', 'deadline_exceeded') OR http_status = 408) AS timeouts
			FROM gateway_attempts GROUP BY provider_id
		`).Scan(&grouped).Error; err != nil {
			return nil, err
		}
	case "model":
		if err := s.db.WithContext(ctx).Raw(`
			SELECT public_model_id AS key,
				COUNT(*) AS requests,
				COUNT(*) FILTER (WHERE status = 'succeeded') AS successes,
				COUNT(*) FILTER (WHERE status <> 'succeeded') AS errors,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND a.attempt_no > 1)) AS fallbacks,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND (a.error_code IN ('timeout', 'deadline_exceeded') OR a.http_status = 408))) AS timeouts
			FROM gateway_requests GROUP BY public_model_id
		`).Scan(&grouped).Error; err != nil {
			return nil, err
		}
	case "channel":
		if err := s.db.WithContext(ctx).Raw(`
			SELECT COALESCE(channel_org_id, '') AS key,
				COUNT(*) AS requests,
				COUNT(*) FILTER (WHERE status = 'succeeded') AS successes,
				COUNT(*) FILTER (WHERE status <> 'succeeded') AS errors,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND a.attempt_no > 1)) AS fallbacks,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND (a.error_code IN ('timeout', 'deadline_exceeded') OR a.http_status = 408))) AS timeouts
			FROM gateway_requests GROUP BY channel_org_id
		`).Scan(&grouped).Error; err != nil {
			return nil, err
		}
	case "user":
		if err := s.db.WithContext(ctx).Raw(`
			SELECT user_id AS key,
				COUNT(*) AS requests,
				COUNT(*) FILTER (WHERE status = 'succeeded') AS successes,
				COUNT(*) FILTER (WHERE status <> 'succeeded') AS errors,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND a.attempt_no > 1)) AS fallbacks,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND (a.error_code IN ('timeout', 'deadline_exceeded') OR a.http_status = 408))) AS timeouts
			FROM gateway_requests GROUP BY user_id
		`).Scan(&grouped).Error; err != nil {
			return nil, err
		}
	case "api_key":
		if err := s.db.WithContext(ctx).Raw(`
			SELECT COALESCE(api_key_id, '') AS key,
				COUNT(*) AS requests,
				COUNT(*) FILTER (WHERE status = 'succeeded') AS successes,
				COUNT(*) FILTER (WHERE status <> 'succeeded') AS errors,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND a.attempt_no > 1)) AS fallbacks,
				COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM gateway_attempts a WHERE a.request_pk = gateway_requests.id AND (a.error_code IN ('timeout', 'deadline_exceeded') OR a.http_status = 408))) AS timeouts
			FROM gateway_requests GROUP BY api_key_id
		`).Scan(&grouped).Error; err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}

	out := make([]TrafficStat, 0, len(grouped))
	for _, item := range grouped {
		stat := TrafficStat{
			Dimension: dimension, Key: item.Key, Requests: item.Requests, Successes: item.Successes, Errors: item.Errors,
			Fallbacks: item.Fallbacks, HTTP429: item.HTTP429, HTTP5xx: item.HTTP5xx, Timeouts: item.Timeouts,
		}
		if item.Requests > 0 {
			stat.SuccessRate = float64(item.Successes) / float64(item.Requests)
		}
		stat.LatencyP50MS, stat.LatencyP95MS, stat.LatencyP99MS = s.latencies(ctx, dimension, item.Key)
		out = append(out, stat)
	}
	return out, nil
}

func (s *Service) ErrorBreakdown(ctx context.Context) (map[string]int64, error) {
	type row struct {
		Code  string
		Count int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(NULLIF(error_code, ''), 'unknown') AS code, COUNT(*) AS count
		FROM gateway_attempts
		WHERE status = 'failed'
		GROUP BY 1
	`).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, item := range rows {
		out[item.Code] += item.Count
	}
	return out, nil
}

func (s *Service) DailySeries(ctx context.Context, since time.Time) ([]DailyStat, error) {
	type row struct {
		Day       string
		Requests  int64
		Successes int64
		Errors    int64
	}
	var rows []row
	if err := s.db.WithContext(ctx).Raw(`
		SELECT to_char((started_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD') AS day,
			COUNT(*) AS requests,
			COUNT(*) FILTER (WHERE status = 'succeeded') AS successes,
			COUNT(*) FILTER (WHERE status <> 'succeeded') AS errors
		FROM gateway_requests
		WHERE started_at >= ?
		GROUP BY 1
		ORDER BY 1
	`, since.UTC()).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DailyStat, 0, len(rows))
	for _, item := range rows {
		out = append(out, DailyStat{Day: item.Day, Requests: item.Requests, Successes: item.Successes, Errors: item.Errors})
	}
	return out, nil
}

func (s *Service) latencies(ctx context.Context, dimension, key string) (int64, int64, int64) {
	q := s.db.WithContext(ctx).Model(&attemptRow{}).Select("latency_ms")
	switch dimension {
	case "provider":
		q = q.Where("provider_id = ?", key)
	case "model":
		q = q.Where("request_pk IN (SELECT id FROM gateway_requests WHERE public_model_id = ?)", key)
	case "channel":
		q = q.Where("request_pk IN (SELECT id FROM gateway_requests WHERE channel_org_id = ?)", key)
	case "user":
		q = q.Where("request_pk IN (SELECT id FROM gateway_requests WHERE user_id = ?)", key)
	case "api_key":
		q = q.Where("request_pk IN (SELECT id FROM gateway_requests WHERE api_key_id = ?)", key)
	default:
		return 0, 0, 0
	}
	var values []int
	if err := q.Limit(500).Scan(&values).Error; err != nil || len(values) == 0 {
		return 0, 0, 0
	}
	sort.Ints(values)
	p95 := (len(values) * 95) / 100
	if p95 >= len(values) {
		p95 = len(values) - 1
	}
	p99 := (len(values) * 99) / 100
	if p99 >= len(values) {
		p99 = len(values) - 1
	}
	return int64(values[len(values)/2]), int64(values[p95]), int64(values[p99])
}
