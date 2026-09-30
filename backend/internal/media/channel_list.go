package media

import (
	"context"
	"time"
)

// ChannelJob exposes operational status without prompts, supplier identifiers or
// signed asset URLs. Empty scopes always return an empty list.
type ChannelJob struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	ChannelOrgID string    `json:"channel_org_id"`
	Kind         string    `json:"kind"`
	TaskType     string    `json:"task_type"`
	Status       string    `json:"status"`
	Model        string    `json:"model"`
	Progress     int       `json:"progress"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Service) ListChannels(ctx context.Context, channelIDs []string, query string) ([]ChannelJob, error) {
	items := make([]ChannelJob, 0)
	if len(channelIDs) == 0 {
		return items, nil
	}
	q := s.db.WithContext(ctx).Table("media_jobs").Select("id, user_id, channel_org_id, job_kind AS kind, task_type, status, public_model_id AS model, progress, created_at").Where("channel_org_id IN ?", channelIDs)
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("id ILIKE ? OR public_model_id ILIKE ? OR status ILIKE ?", like, like, like)
	}
	err := q.Order("created_at DESC, id").Limit(100).Scan(&items).Error
	return items, err
}

func (s *Service) FailedChannelJobCounts(ctx context.Context, channelIDs []string) (map[string]int64, error) {
	counts := make(map[string]int64)
	if len(channelIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		ChannelOrgID string
		Count        int64
	}
	err := s.db.WithContext(ctx).Table("media_jobs").Select("channel_org_id, COUNT(*) AS count").Where("channel_org_id IN ? AND status = ? AND created_at >= ?", channelIDs, StatusFailed, time.Now().UTC().Add(-24*time.Hour)).Group("channel_org_id").Scan(&rows).Error
	for _, row := range rows {
		counts[row.ChannelOrgID] = row.Count
	}
	return counts, err
}
