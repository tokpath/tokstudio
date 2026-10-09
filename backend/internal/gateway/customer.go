package gateway

import (
	"context"
	"time"
)

// Public request facts only: no prompts, provider identifiers, credentials or
// upstream attempts are exposed in customer support projections.
type CustomerRequest struct {
	RequestID     string    `json:"request_id"`
	PublicModelID string    `json:"public_model_id"`
	Status        string    `json:"status"`
	StartedAt     time.Time `json:"started_at"`
}
type CustomerActivity struct {
	Count int64             `json:"count"`
	Items []CustomerRequest `json:"items"`
}

func (s *Service) CustomerActivity(ctx context.Context, userID string, channelIDs []string) (*CustomerActivity, error) {
	out := &CustomerActivity{Items: []CustomerRequest{}}
	q := s.db.WithContext(ctx).Model(&requestRow{}).Where("user_id=? AND COALESCE(channel_org_id, '') IN ?", userID, channelIDs)
	if err := q.Count(&out.Count).Error; err != nil {
		return nil, err
	}
	err := q.Select("request_id,public_model_id,status,started_at").Order("started_at DESC,id DESC").Limit(10).Scan(&out.Items).Error
	return out, err
}
