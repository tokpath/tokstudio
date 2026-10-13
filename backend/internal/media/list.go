package media

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/platform/page"
	"gorm.io/gorm"
)

type JobQuery struct {
	UserID, Kind, Status, Query string
}

func (s *Service) jobsQuery(ctx context.Context, in JobQuery) *gorm.DB {
	q := s.db.WithContext(ctx).Model(&jobRow{})
	if in.UserID != "" {
		q = q.Where("user_id = ?", in.UserID)
	}
	if in.Kind != "" {
		q = q.Where("job_kind = ?", in.Kind)
	}
	if in.Status != "" {
		q = q.Where("status = ?", in.Status)
	}
	if in.Query != "" {
		like := "%" + in.Query + "%"
		q = q.Where("(id ILIKE ? OR public_model_id ILIKE ? OR status ILIKE ? OR job_kind ILIKE ?)", like, like, like, like)
	}
	return q
}

func (s *Service) ListPage(ctx context.Context, in JobQuery, limit int, cursor string) ([]JobView, string, error) {
	limit = page.Size(limit)
	q, err := page.BeforeCreated(s.jobsQuery(ctx, in), cursor)
	if err != nil {
		return nil, "", err
	}
	var rows []jobRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = page.Encode(last.CreatedAt, last.ID)
	}
	items := make([]JobView, 0, len(rows))
	for _, row := range rows {
		items = append(items, *s.view(ctx, row))
	}
	return items, next, nil
}

// Export visits the entire filtered result without materializing it in memory
// or loading prompts, upstream identifiers and asset data into export records.
func (s *Service) Export(ctx context.Context, in JobQuery, visit func(JobView) error) error {
	rows, err := s.jobsQuery(ctx, in).Select("id, job_kind, task_type, status, public_model_id").Order("created_at DESC, id DESC").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item JobView
		if err := rows.Scan(&item.ID, &item.Kind, &item.TaskType, &item.Status, &item.Model); err != nil {
			return err
		}
		if err := visit(item); err != nil {
			return err
		}
	}
	return rows.Err()
}
