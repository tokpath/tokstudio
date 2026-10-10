package plans

import "context"

func (s *Service) CustomerSubscriptionsInScope(ctx context.Context, userID string, channelIDs []string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&subRow{}).Where("user_id=? AND COALESCE(channel_org_id,'') NOT IN ?", userID, channelIDs).Count(&count).Error
	return count == 0, err
}
