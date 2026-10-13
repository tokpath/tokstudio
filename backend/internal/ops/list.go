package ops

import (
	"context"

	"github.com/tokpath/tokstudio/backend/internal/platform/page"
	"gorm.io/gorm"
)

func (s *Service) alertsQuery(ctx context.Context, status, query string) *gorm.DB {
	q := s.db.WithContext(ctx).Model(&alertRow{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("(id ILIKE ? OR kind ILIKE ? OR message ILIKE ?)", like, like, like)
	}
	return q
}

func (s *Service) ListAlertsPage(ctx context.Context, status, query string, limit int, cursor string) ([]AlertView, string, error) {
	limit = page.Size(limit)
	q, err := page.BeforeCreated(s.alertsQuery(ctx, status, query), cursor)
	if err != nil {
		return nil, "", err
	}
	var rows []alertRow
	if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		next = page.Encode(last.CreatedAt, last.ID)
	}
	items := make([]AlertView, 0, len(rows))
	for _, row := range rows {
		items = append(items, AlertView{ID: row.ID, Kind: row.Kind, Severity: row.Severity, Status: row.Status, Message: row.Message, CreatedAt: row.CreatedAt})
	}
	return items, next, nil
}

// Dashboard and evaluation responses are previews; history uses ListAlertsPage.
func (s *Service) RecentAlerts(ctx context.Context, status string) ([]AlertView, error) {
	items, _, err := s.ListAlertsPage(ctx, status, "", 50, "")
	return items, err
}

func (s *Service) ExportAlerts(ctx context.Context, status, query string, visit func(AlertView) error) error {
	rows, err := s.alertsQuery(ctx, status, query).Select("id, kind, severity, status, message, created_at").Order("created_at DESC, id DESC").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item AlertView
		if err := rows.Scan(&item.ID, &item.Kind, &item.Severity, &item.Status, &item.Message, &item.CreatedAt); err != nil {
			return err
		}
		if err := visit(item); err != nil {
			return err
		}
	}
	return rows.Err()
}
