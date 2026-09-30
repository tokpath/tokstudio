package audit

import (
	"context"
	"strings"
	"time"
)

type ChannelEntry struct {
	ID           string    `json:"id"`
	ActorUserID  string    `json:"actor_user_id"`
	ActorEmail   string    `json:"actor_email"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// SearchChannels intentionally returns audit metadata only: global infrastructure
// configuration and secret-bearing before/after payloads do not belong to OEMs.
func (s *Service) SearchChannels(ctx context.Context, channelIDs []string, actors map[string]string, query string) ([]ChannelEntry, error) {
	items := make([]ChannelEntry, 0)
	if len(channelIDs) == 0 {
		return items, nil
	}
	actorIDs, matchingActors := make([]string, 0, len(actors)), make([]string, 0)
	for id, email := range actors {
		actorIDs = append(actorIDs, id)
		if query != "" && strings.Contains(strings.ToLower(email), strings.ToLower(query)) {
			matchingActors = append(matchingActors, id)
		}
	}
	q := s.db.WithContext(ctx).Table("audit_logs").
		Select("id, COALESCE(actor_user_id, '') AS actor_user_id, action, resource_type, COALESCE(resource_id, '') AS resource_id, created_at").
		Where(`actor_user_id IN ? OR (resource_type = 'channel' AND resource_id IN ?) OR
		(resource_type = 'user' AND resource_id IN ?)`, actorIDs, channelIDs, actorIDs)
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("action ILIKE ? OR resource_id ILIKE ? OR actor_user_id IN ?", like, like, matchingActors)
	}
	err := q.Order("created_at DESC, id").Limit(100).Scan(&items).Error
	for i := range items {
		items[i].ActorEmail = actors[items[i].ActorUserID]
	}
	return items, err
}
