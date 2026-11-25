package usage

import (
	"context"

	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
)

func (s *Service) PendingRequestsPage(ctx context.Context, cursor string, limit int) ([]Request, error) {
	if limit < 1 || limit > 200 {
		return nil, ErrKey
	}
	q := s.DB.WithContext(ctx).Select("id", "customer_id", "async_image").Where("status IN ('accepted','executing','submitted','awaiting_usage') OR (status='completed' AND operation=? AND completed_at>? AND NOT EXISTS(SELECT 1 FROM usage_results result WHERE result.request_id=usage_requests.id AND result.state='ready'))",
		modelruntime.Video, s.wallNow().Add(-s.ResultRetention))
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
