package usage

import (
	"context"
	"github.com/google/uuid"
)

func (s *Service) PendingRequestsPage(ctx context.Context, cursor string, limit int) ([]Request, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrKey
	}
	q := s.DB.WithContext(ctx).Where("status IN ('accepted','executing','submitted','awaiting_usage') OR (status='completed' AND operation='video' AND completed_at>clock_timestamp()-interval '24 hours' AND NOT EXISTS(SELECT 1 FROM usage_results result WHERE result.request_id=usage_requests.id AND result.state='ready'))")
	if cursor != "" {
		if _, e := uuid.Parse(cursor); e != nil {
			return nil, e
		}
		q = q.Where("id>?", cursor)
	}
	rows := []Request{}
	e := q.Order("id").Limit(limit).Find(&rows).Error
	return rows, e
}
