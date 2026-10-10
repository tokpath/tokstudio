package gateway

import (
	"context"
	"time"
)

type SuccessfulRequestEvidence struct {
	UserID        string    `json:"-"`
	RequestID     string    `json:"request_id"`
	PublicModelID string    `json:"public_model_id"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// SuccessfulRequestsForChannel returns observed successful calls, without prompts,
// provider details or secrets. The assembly layer checks the matching billing fact.
func (s *Service) SuccessfulRequestsForChannel(ctx context.Context, channelID string, since time.Time) ([]SuccessfulRequestEvidence, error) {
	var rows []SuccessfulRequestEvidence
	err := s.db.WithContext(ctx).Table("gateway_requests r").Select("r.user_id,r.request_id,r.public_model_id,r.started_at AS occurred_at").Joins("JOIN gateway_attempts a ON a.id=r.final_attempt_id AND a.status='succeeded'").Where("r.channel_org_id=? AND r.status='succeeded' AND r.started_at>=?", channelID, since).Order("r.started_at DESC").Scan(&rows).Error
	return rows, err
}
